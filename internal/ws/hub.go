// Package ws provides a lightweight WebSocket pub/sub hub.
// Components broadcast JSON events through the hub, and every connected client
// receives them in real time. The hub also handles ping/pong keepalives
// so stale connections get cleaned up automatically.
package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Hub manages WebSocket client connections and fans out broadcast messages
// to all of them. It is safe for concurrent use; register, unregister, and
// broadcast all go through channels.
type Hub struct {
	clients    map[*websocket.Conn]struct{}
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	broadcast  chan []byte
	upgrader   websocket.Upgrader
}

// NewHub allocates a hub with buffered channels.
// Call Run in a goroutine to start the event loop.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*websocket.Conn]struct{}),
		register:   make(chan *websocket.Conn, 16),
		unregister: make(chan *websocket.Conn, 16),
		broadcast:  make(chan []byte, 256),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

// Run processes registrations, unregistrations, broadcasts, and keepalive
// pings in a single select loop. It closes all clients when ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			for c := range h.clients {
				_ = c.Close()
			}
			return

		case c := <-h.register:
			h.clients[c] = struct{}{}

		case c := <-h.unregister:
			delete(h.clients, c)
			_ = c.Close()

		case msg := <-h.broadcast:
			for c := range h.clients {
				_ = c.SetWriteDeadline(time.Now().Add(3 * time.Second))
				if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
					delete(h.clients, c)
					_ = c.Close()
				}
			}

		case <-ping.C:
			for c := range h.clients {
				_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if err := c.WriteMessage(websocket.PingMessage, nil); err != nil {
					delete(h.clients, c)
					_ = c.Close()
				}
			}
		}
	}
}

// Handler returns an http.Handler that upgrades incoming requests to
// WebSocket connections and registers them with the hub.
func (h *Hub) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
			return
		}
		h.register <- conn

		go func() {
			defer func() { h.unregister <- conn }()
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			conn.SetPongHandler(func(string) error {
				_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
				return nil
			})

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
	})
}

// BroadcastJSON marshals v to JSON and queues it for delivery to all
// connected clients. If the broadcast channel is full the message is
// silently dropped to avoid blocking the caller.
func (h *Hub) BroadcastJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	select {
	case h.broadcast <- b:
	default:
	}
}
