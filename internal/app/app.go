// Package app wires together the HTTP server, WebSocket hub, and either the
// live satellite scheduler or the demo runner. It owns the daemon's lifecycle
// and is the single source of truth for the current operating state.
package app

import (
	"context"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/demo"
	"github.com/large-farva/ephemeris-engine/internal/scheduler"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// Options holds everything the App needs from the caller.
type Options struct {
	Logger     *log.Logger
	Cfg        config.Config
	Bind       string
	ConfigPath string
}

// logEntry is a single log message stored in the ring buffer.
type logEntry struct {
	TS        string `json:"ts"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Component string `json:"component"`
}

// stats tracks aggregate capture statistics.
type stats struct {
	mu            sync.Mutex
	TotalCaptures int            `json:"total_captures"`
	TotalBytes    int64          `json:"total_bytes"`
	CapturesBySat map[string]int `json:"captures_by_satellite"`
	LastCaptureAt string         `json:"last_capture_at,omitempty"`
}

// App is the top-level daemon process. It manages the HTTP server, the
// WebSocket event hub, and the active runner (scheduler or demo).
type App struct {
	log        *log.Logger
	cfg        config.Config
	cfgMu      sync.RWMutex // protects cfg for hot-reload
	configPath string
	bind       string
	server     *http.Server

	startedAt time.Time
	state     atomic.Value // current state string (BOOTING, IDLE, etc.)

	wsHub       *ws.Hub
	scheduler   *scheduler.Runner // nil in demo mode
	currentPass atomic.Value      // *scheduler.PassInfo or nil

	// Log ring buffer.
	logBuf    []logEntry
	logBufMu  sync.Mutex
	logBufCap int

	captureStats stats
}

// New creates an App in the BOOTING state. Call Run to start serving.
func New(opts Options) *App {
	a := &App{
		log:        opts.Logger,
		cfg:        opts.Cfg,
		configPath: opts.ConfigPath,
		bind:       opts.Bind,
		startedAt:  time.Now(),
		wsHub:      ws.NewHub(),
		logBufCap:  500,
		captureStats: stats{
			CapturesBySat: make(map[string]int),
		},
	}
	a.logBuf = make([]logEntry, 0, a.logBufCap)
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

	// Core endpoints.
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/version", a.handleVersion)
	mux.HandleFunc("/api/satellites", a.handleSatellites)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/passes", a.handlePasses)
	mux.HandleFunc("/api/trigger", a.handleTrigger)
	mux.HandleFunc("/api/tle-refresh", a.handleTLERefresh)
	mux.Handle("/ws", a.wsHub.Handler())

	// Data management.
	mux.HandleFunc("/api/captures", a.handleCaptures)
	mux.HandleFunc("/api/config/profiles", a.handleConfigProfiles)

	// Informational.
	mux.HandleFunc("/api/tle-info", a.handleTLEInfo)
	mux.HandleFunc("/api/next-pass", a.handleNextPass)
	mux.HandleFunc("/api/system", a.handleSystem)
	mux.HandleFunc("/api/logs", a.handleLogs)
	mux.HandleFunc("/api/stats", a.handleStats)

	// Scheduler controls + reload.
	mux.HandleFunc("/api/pause", a.handlePause)
	mux.HandleFunc("/api/resume", a.handleResume)
	mux.HandleFunc("/api/skip", a.handleSkip)
	mux.HandleFunc("/api/cancel", a.handleCancel)
	mux.HandleFunc("/api/reload", a.handleReload)

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
		a.scheduler = scheduler.New(a.wsHub, a.cfg, a.log)
		a.scheduler.SetPassCallback(a.onPassUpdate)
		a.scheduler.SetCaptureCallback(a.onCaptureComplete)
		go a.scheduler.Run(ctx, a.setStateFromScheduler)
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

	// Clear current pass when returning to IDLE.
	if newState == "IDLE" {
		a.currentPass.Store((*scheduler.PassInfo)(nil))
	}
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
				"type":           "heartbeat",
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"uptime_seconds": int64(time.Since(a.startedAt).Seconds()),
				"state":          a.state.Load().(string),
			}
			a.wsHub.BroadcastJSON(ev)
		}
	}
}

func (a *App) setStateFromDemo(newState string) {
	a.transition(newState)
}

func (a *App) setStateFromScheduler(newState string) {
	a.transition(newState)
}

// onPassUpdate is called by the scheduler when tracking a pass.
func (a *App) onPassUpdate(info *scheduler.PassInfo) {
	a.currentPass.Store(info)
}

// onCaptureComplete is called when a capture finishes, to update stats.
func (a *App) onCaptureComplete(satellite string, bytesWritten int64) {
	a.captureStats.mu.Lock()
	defer a.captureStats.mu.Unlock()
	a.captureStats.TotalCaptures++
	a.captureStats.TotalBytes += bytesWritten
	a.captureStats.CapturesBySat[satellite]++
	a.captureStats.LastCaptureAt = time.Now().UTC().Format(time.RFC3339)
}

// appendLog adds a log entry to the ring buffer.
func (a *App) appendLog(entry logEntry) {
	a.logBufMu.Lock()
	defer a.logBufMu.Unlock()
	if len(a.logBuf) >= a.logBufCap {
		a.logBuf = a.logBuf[1:]
	}
	a.logBuf = append(a.logBuf, entry)
}

// getConfig returns the current config (thread-safe for reload).
func (a *App) getConfig() config.Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg
}

// emit stamps a payload with a timestamp and component name, then pushes it
// to every connected WebSocket client. Log events are also buffered.
func (a *App) emit(component string, payload map[string]any) {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	payload["ts"] = ts
	payload["component"] = component
	a.wsHub.BroadcastJSON(payload)

	// Buffer log-type events for the /api/logs endpoint.
	if t, ok := payload["type"].(string); ok && t == "log" {
		level, _ := payload["level"].(string)
		msg, _ := payload["message"].(string)
		a.appendLog(logEntry{
			TS:        ts,
			Level:     level,
			Message:   msg,
			Component: component,
		})
	}
}
