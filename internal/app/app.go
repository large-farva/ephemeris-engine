// Package app wires together the HTTP server, WebSocket hub, and either the
// live satellite scheduler or the demo runner. It owns the daemon's lifecycle
// and is the single source of truth for the current operating state.
package app

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/demo"
	"github.com/large-farva/ephemeris-engine/internal/scheduler"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// Options holds everything the App needs from the caller.
type Options struct {
	Logger *log.Logger
	Cfg    config.Config
	Bind   string
}

// App is the top-level daemon process. It manages the HTTP server, the
// WebSocket event hub, and the active runner (scheduler or demo).
type App struct {
	log    *log.Logger
	cfg    config.Config
	bind   string
	server *http.Server

	startedAt time.Time
	state     atomic.Value // current state string (BOOTING, IDLE, etc.)

	wsHub *ws.Hub
}

// New creates an App in the BOOTING state. Call Run to start serving.
func New(opts Options) *App {
	a := &App{
		log:       opts.Logger,
		cfg:       opts.Cfg,
		bind:      opts.Bind,
		startedAt: time.Now(),
		wsHub:     ws.NewHub(),
	}
	a.state.Store("BOOTING")
	return a
}

// Run starts the HTTP server, WebSocket hub, heartbeat ticker, and either the
// live scheduler or demo runner. It blocks until the context is cancelled or
// the server returns an error.
func (a *App) Run(ctx context.Context) error {
	bind := a.bind
	if bind == "" && a.cfg.Server.Bind != "" {
		bind = a.cfg.Server.Bind
	}
	if bind == "" {
		bind = "0.0.0.0:8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.Handle("/ws", a.wsHub.Handler())

	a.server = &http.Server{
		Addr:              bind,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return err
	}

	a.log.Printf("listening on http://%s", bind)

	go a.wsHub.Run(ctx)
	a.transition("IDLE")
	go a.heartbeatLoop(ctx)

	if a.cfg.Demo.Enabled {
		r := demo.New(a.wsHub)
		if a.cfg.Demo.IntervalSeconds > 0 {
			r.Interval = time.Duration(a.cfg.Demo.IntervalSeconds) * time.Second
		}
		go r.Run(ctx, a.setStateFromDemo)
	} else {
		s := scheduler.New(a.wsHub, a.cfg, a.log)
		go s.Run(ctx, a.setStateFromScheduler)
	}

	go func() {
		<-ctx.Done()
		a.log.Printf("shutdown requested")
		_ = a.server.Shutdown(context.Background())
	}()

	return a.server.Serve(ln)
}

// transition atomically updates the daemon state and broadcasts the change
// to all connected WebSocket clients.
func (a *App) transition(newState string) {
	old := a.state.Load().(string)
	if old == newState {
		return
	}
	a.state.Store(newState)

	ev := map[string]any{
		"type":      "state",
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"from":      old,
		"to":        newState,
		"component": "ephemerisd",
	}
	a.wsHub.BroadcastJSON(ev)
}

// heartbeatLoop sends a periodic heartbeat event so clients can detect
// connectivity and track uptime without polling.
func (a *App) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ev := map[string]any{
				"type": "heartbeat",
				"ts":   time.Now().UTC().Format(time.RFC3339Nano),
				"uptime_seconds": int64(time.Since(a.startedAt).Seconds()),
				"state":          a.state.Load().(string),
			}
			a.wsHub.BroadcastJSON(ev)
		}
	}
}

func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) handleStatus(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"name":           "ephemeris-engine",
		"state":          a.state.Load().(string),
		"uptime_seconds": int64(time.Since(a.startedAt).Seconds()),
		"data_root":      a.cfg.Data.Root,
		"archive_dir":    a.cfg.Data.Archive,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) setStateFromDemo(newState string) {
	a.transition(newState)
}

func (a *App) setStateFromScheduler(newState string) {
	a.transition(newState)
}

// emit stamps a payload with a timestamp and component name, then pushes it
// to every connected WebSocket client.
func (a *App) emit(component string, payload map[string]any) {
	payload["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	payload["component"] = component
	a.wsHub.BroadcastJSON(payload)
}