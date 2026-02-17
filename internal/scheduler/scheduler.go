// Package scheduler orchestrates the predict-wait-capture loop that drives
// the Ephemeris Engine daemon. It continuously computes upcoming passes, waits for
// each AOS, records the pass, and cycles back to idle.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/capture"
	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/predict"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// PassInfo mirrors the app.PassInfo struct for callback typing.
type PassInfo struct {
	Satellite string  `json:"satellite"`
	NoradID   int     `json:"norad_id"`
	FreqHz    int     `json:"freq_hz"`
	AOS       string  `json:"aos"`
	LOS       string  `json:"los"`
	MaxElev   float64 `json:"max_elev"`
	Stage     string  `json:"stage"`
}

// Command represents an external command sent to the scheduler via its
// Commands channel. The Reply channel receives exactly one result.
type Command struct {
	Type    string
	Payload json.RawMessage
	Reply   chan<- CommandResult
}

// CommandResult is the response sent back through a Command's Reply channel.
type CommandResult struct {
	OK                bool   `json:"ok"`
	Message           string `json:"message,omitempty"`
	Error             string `json:"error,omitempty"`
	SatellitesUpdated int    `json:"satellites_updated,omitempty"`
}

// Runner owns the main scheduling loop, coordinating the predictor and
// capture runner through each satellite pass.
type Runner struct {
	Hub *ws.Hub
	Cfg config.Config
	Log *log.Logger

	// Commands receives external commands from HTTP handlers.
	// The scheduler checks this channel during wait periods.
	Commands chan Command

	predictor *predict.Predictor
	capturer  *capture.Runner

	// Pause state.
	paused atomic.Bool

	// Cancel support: when a capture is active, captureCancel can abort it.
	captureMu     sync.Mutex
	captureCancel context.CancelFunc

	// Callbacks into the app layer.
	passCallback    func(*PassInfo)
	captureCallback func(satellite string, bytesWritten int64)
}

// New creates a scheduler with its own predictor and capture runner.
func New(hub *ws.Hub, cfg config.Config, logger *log.Logger) *Runner {
	return &Runner{
		Hub:       hub,
		Cfg:       cfg,
		Log:       logger,
		Commands:  make(chan Command, 4),
		predictor: predict.NewPredictor(hub, cfg, logger),
		capturer:  capture.New(hub, cfg, logger, false),
	}
}

// SetPassCallback registers a function called when the current pass changes.
func (r *Runner) SetPassCallback(fn func(*PassInfo)) {
	r.passCallback = fn
}

// SetCaptureCallback registers a function called when a capture completes.
func (r *Runner) SetCaptureCallback(fn func(string, int64)) {
	r.captureCallback = fn
}

// IsPaused reports whether the scheduler is paused.
func (r *Runner) IsPaused() bool {
	return r.paused.Load()
}

// Run is the main scheduler loop.
//
// Lifecycle:
//  1. Compute passes (IDLE state)
//  2. If none, sleep for tle_refresh_hours then recompute
//  3. Pick next pass, transition to WAITING_FOR_PASS
//  4. Sleep until AOS
//  5. Transition to RECORDING, run capture
//  6. Transition to DECODING (placeholder for future APT decoding)
//  7. Transition to IDLE, loop back to step 1
func (r *Runner) Run(ctx context.Context, setState func(string)) {
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": "scheduler started",
	})

	for {
		if ctx.Err() != nil {
			return
		}

		// If paused, wait until resumed or a command arrives.
		if r.paused.Load() {
			setState("IDLE")
			r.notifyPass(nil)
			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "info",
				"message": "scheduler paused, waiting for resume",
			})
			// Sleep for a very long time; a resume command will interrupt.
			if r.sleepOrCommand(ctx, 24*365*time.Hour, setState) == sleepCancelled {
				return
			}
			continue
		}

		passes, err := r.predictor.ComputePasses()
		if err != nil {
			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "error",
				"message": "prediction failed: " + err.Error(),
			})
			if r.sleepOrCommand(ctx, 5*time.Minute, setState) != sleepCompleted {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			continue
		}

		// Drop any passes whose AOS is already in the past.
		now := time.Now().UTC()
		var upcoming []predict.Pass
		for _, p := range passes {
			if p.AOS.After(now) {
				upcoming = append(upcoming, p)
			}
		}

		if len(upcoming) == 0 {
			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "info",
				"message": "no upcoming passes, will recompute later",
			})
			refreshDur := time.Duration(r.Cfg.Predict.TLERefreshHours) * time.Hour
			if r.sleepOrCommand(ctx, refreshDur, setState) != sleepCompleted {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			continue
		}

		for _, pass := range upcoming {
			if ctx.Err() != nil {
				return
			}

			// A long capture may push us past the next pass's AOS; skip it.
			if time.Now().UTC().After(pass.AOS) {
				continue
			}

			// If paused while iterating, break to recompute.
			if r.paused.Load() {
				break
			}

			setState("WAITING_FOR_PASS")

			r.notifyPass(&PassInfo{
				Satellite: pass.Satellite.Name,
				NoradID:   pass.Satellite.NoradID,
				FreqHz:    pass.Satellite.Freq,
				AOS:       pass.AOS.Format(time.RFC3339),
				LOS:       pass.LOS.Format(time.RFC3339),
				MaxElev:   pass.MaxElev,
				Stage:     "waiting",
			})

			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "info",
				"message": fmt.Sprintf("next pass: %s at %s (max elev %.1f°, duration %s)", pass.Satellite.Name, pass.AOS.Format(time.RFC3339), pass.MaxElev, pass.Duration.Truncate(time.Second)),
			})

			r.broadcast(map[string]any{
				"type":       "pass_scheduled",
				"satellite":  pass.Satellite.Name,
				"norad_id":   pass.Satellite.NoradID,
				"freq_hz":    pass.Satellite.Freq,
				"aos":        pass.AOS.Format(time.RFC3339),
				"los":        pass.LOS.Format(time.RFC3339),
				"max_elev":   pass.MaxElev,
				"duration_s": int(pass.Duration.Seconds()),
			})

			if !r.waitForAOS(ctx, pass, setState) {
				if ctx.Err() != nil {
					return
				}
				// A command interrupted the wait; break to recompute passes.
				break
			}

			// Update pass stage to recording.
			r.notifyPass(&PassInfo{
				Satellite: pass.Satellite.Name,
				NoradID:   pass.Satellite.NoradID,
				FreqHz:    pass.Satellite.Freq,
				AOS:       pass.AOS.Format(time.RFC3339),
				LOS:       pass.LOS.Format(time.RFC3339),
				MaxElev:   pass.MaxElev,
				Stage:     "recording",
			})

			req := capture.CaptureRequest{
				Satellite: pass.Satellite,
				AOS:       pass.AOS,
				LOS:       pass.LOS,
				MaxElev:   pass.MaxElev,
			}

			// Create a cancellable child context for this capture.
			captureCtx, captureCancel := context.WithCancel(ctx)
			r.captureMu.Lock()
			r.captureCancel = captureCancel
			r.captureMu.Unlock()

			outPath, err := r.capturer.Capture(captureCtx, req, setState)
			captureCancel()

			r.captureMu.Lock()
			r.captureCancel = nil
			r.captureMu.Unlock()

			if err != nil {
				r.broadcast(map[string]any{
					"type":    "log",
					"level":   "error",
					"message": "capture failed: " + err.Error(),
				})
			} else if outPath != "" {
				// Notify stats callback.
				if r.captureCallback != nil {
					if info, statErr := captureFileSize(outPath); statErr == nil {
						r.captureCallback(pass.Satellite.Name, info)
					}
				}
			}

			// TODO: APT decoding — this is where it'll process the WAV into an image.
			setState("DECODING")
			r.notifyPass(&PassInfo{
				Satellite: pass.Satellite.Name,
				NoradID:   pass.Satellite.NoradID,
				FreqHz:    pass.Satellite.Freq,
				AOS:       pass.AOS.Format(time.RFC3339),
				LOS:       pass.LOS.Format(time.RFC3339),
				MaxElev:   pass.MaxElev,
				Stage:     "decoding",
			})
			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "info",
				"message": fmt.Sprintf("decoding placeholder for %s (not yet implemented)", pass.Satellite.Name),
			})
			if !sleepOrCancel(ctx, 2*time.Second) {
				return
			}

			r.notifyPass(nil)
			setState("IDLE")
		}
	}
}

// notifyPass calls the pass callback if set.
func (r *Runner) notifyPass(info *PassInfo) {
	if r.passCallback != nil {
		r.passCallback(info)
	}
}

// sleepResult indicates what ended a sleep period.
type sleepResult int

const (
	sleepCompleted   sleepResult = iota // timer expired normally
	sleepCancelled                      // context was cancelled
	sleepInterrupted                    // a command was received and handled
)

// sleepOrCommand blocks for duration d, until ctx is cancelled, or until a
// command arrives on r.Commands. Commands are handled inline. Returns what
// ended the sleep.
func (r *Runner) sleepOrCommand(ctx context.Context, d time.Duration, setState func(string)) sleepResult {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return sleepCancelled
	case <-t.C:
		return sleepCompleted
	case cmd := <-r.Commands:
		r.handleCommand(ctx, cmd, setState)
		return sleepInterrupted
	}
}

// waitForAOS sleeps until AOS, broadcasting countdown progress every 30s.
// Returns true if AOS was reached, false if interrupted (by context cancel
// or a command).
func (r *Runner) waitForAOS(ctx context.Context, pass predict.Pass, setState func(string)) bool {
	for {
		remaining := time.Until(pass.AOS)
		if remaining <= 0 {
			return true
		}

		r.broadcast(map[string]any{
			"type":    "progress",
			"stage":   "waiting",
			"percent": 0,
			"detail":  fmt.Sprintf("AOS in %s for %s", remaining.Truncate(time.Second), pass.Satellite.Name),
		})

		sleepDur := 30 * time.Second
		if remaining < sleepDur {
			sleepDur = remaining
		}
		result := r.sleepOrCommand(ctx, sleepDur, setState)
		if result == sleepCancelled || result == sleepInterrupted {
			return false
		}
	}
}

// handleCommand dispatches an incoming command to the appropriate handler.
func (r *Runner) handleCommand(ctx context.Context, cmd Command, setState func(string)) {
	switch cmd.Type {
	case "trigger":
		r.handleTriggerCommand(ctx, cmd, setState)
	case "tle_refresh":
		r.handleTLERefreshCommand(cmd)
	case "pause":
		r.handlePauseCommand(cmd)
	case "resume":
		r.handleResumeCommand(cmd)
	case "skip":
		r.handleSkipCommand(cmd)
	case "cancel":
		r.handleCancelCommand(cmd)
	default:
		cmd.Reply <- CommandResult{OK: false, Error: "unknown command: " + cmd.Type}
	}
}

// handleTriggerCommand starts an immediate capture for the requested satellite.
func (r *Runner) handleTriggerCommand(ctx context.Context, cmd Command, setState func(string)) {
	var payload struct {
		NoradID         int `json:"norad_id"`
		DurationSeconds int `json:"duration_seconds"`
	}
	if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
		cmd.Reply <- CommandResult{OK: false, Error: "invalid payload: " + err.Error()}
		return
	}

	sat := capture.SatelliteByNoradID(payload.NoradID)
	if sat == nil {
		cmd.Reply <- CommandResult{OK: false, Error: fmt.Sprintf("unknown NORAD ID: %d", payload.NoradID)}
		return
	}

	dur := time.Duration(payload.DurationSeconds) * time.Second
	now := time.Now().UTC()

	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("manual trigger: capturing %s for %s", sat.Name, dur.Truncate(time.Second)),
	})

	// Reply immediately so the HTTP handler is not blocked during capture.
	cmd.Reply <- CommandResult{
		OK:      true,
		Message: fmt.Sprintf("capture triggered for %s (%s)", sat.Name, dur.Truncate(time.Second)),
	}

	req := capture.CaptureRequest{
		Satellite: *sat,
		AOS:       now,
		LOS:       now.Add(dur),
		MaxElev:   90,
	}

	captureCtx, captureCancel := context.WithCancel(ctx)
	r.captureMu.Lock()
	r.captureCancel = captureCancel
	r.captureMu.Unlock()

	outPath, err := r.capturer.Capture(captureCtx, req, setState)
	captureCancel()

	r.captureMu.Lock()
	r.captureCancel = nil
	r.captureMu.Unlock()

	if err != nil {
		r.broadcast(map[string]any{
			"type":    "log",
			"level":   "error",
			"message": "triggered capture failed: " + err.Error(),
		})
	} else if outPath != "" && r.captureCallback != nil {
		if size, statErr := captureFileSize(outPath); statErr == nil {
			r.captureCallback(sat.Name, size)
		}
	}

	setState("IDLE")
}

// handleTLERefreshCommand forces an immediate TLE data refresh.
func (r *Runner) handleTLERefreshCommand(cmd Command) {
	n, err := r.predictor.ForceRefreshTLEs()
	if err != nil {
		cmd.Reply <- CommandResult{OK: false, Error: "TLE refresh failed: " + err.Error()}
		return
	}

	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("TLE data refreshed, %d satellites updated", n),
	})

	cmd.Reply <- CommandResult{
		OK:                true,
		Message:           fmt.Sprintf("TLE data refreshed, %d satellites updated", n),
		SatellitesUpdated: n,
	}
}

func (r *Runner) handlePauseCommand(cmd Command) {
	if r.paused.Load() {
		cmd.Reply <- CommandResult{OK: true, Message: "scheduler already paused"}
		return
	}
	r.paused.Store(true)
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": "scheduler paused by user",
	})
	cmd.Reply <- CommandResult{OK: true, Message: "scheduler paused"}
}

func (r *Runner) handleResumeCommand(cmd Command) {
	if !r.paused.Load() {
		cmd.Reply <- CommandResult{OK: true, Message: "scheduler already running"}
		return
	}
	r.paused.Store(false)
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": "scheduler resumed by user",
	})
	cmd.Reply <- CommandResult{OK: true, Message: "scheduler resumed"}
}

func (r *Runner) handleSkipCommand(cmd Command) {
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": "skipping current pass by user request",
	})
	r.notifyPass(nil)
	cmd.Reply <- CommandResult{OK: true, Message: "pass skipped, recomputing schedule"}
}

func (r *Runner) handleCancelCommand(cmd Command) {
	r.captureMu.Lock()
	cancel := r.captureCancel
	r.captureMu.Unlock()

	if cancel == nil {
		cmd.Reply <- CommandResult{OK: false, Error: "no capture in progress"}
		return
	}

	cancel()
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": "capture cancelled by user",
	})
	cmd.Reply <- CommandResult{OK: true, Message: "capture cancelled"}
}

func (r *Runner) broadcast(v map[string]any) {
	v["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	v["component"] = "scheduler"
	r.Hub.BroadcastJSON(v)
}

// sleepOrCancel blocks for duration d or until the context is cancelled.
// Returns true if the sleep completed, false if interrupted.
func sleepOrCancel(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// captureFileSize returns the size of a capture file.
func captureFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
