// Package scheduler orchestrates the predict-wait-capture loop that drives
// the Ephemeris Engine daemon. It continuously computes upcoming passes, waits for
// each AOS, records the pass, and cycles back to idle.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/capture"
	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/predict"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// Runner owns the main scheduling loop, coordinating the predictor and
// capture runner through each satellite pass.
type Runner struct {
	Hub *ws.Hub
	Cfg config.Config
	Log *log.Logger

	predictor *predict.Predictor
	capturer  *capture.Runner
}

// New creates a scheduler with its own predictor and capture runner.
func New(hub *ws.Hub, cfg config.Config, logger *log.Logger) *Runner {
	return &Runner{
		Hub:       hub,
		Cfg:       cfg,
		Log:       logger,
		predictor: predict.NewPredictor(hub, cfg, logger),
		capturer:  capture.New(hub, cfg, logger, false),
	}
}

// Run is the main scheduler loop. It follows the same pattern as
// demo.Runner.Run: accepts a context for cancellation and a setState
// callback for state transitions.
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

		passes, err := r.predictor.ComputePasses()
		if err != nil {
			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "error",
				"message": "prediction failed: " + err.Error(),
			})
			if !sleepOrCancel(ctx, 5*time.Minute) {
				return
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
			if !sleepOrCancel(ctx, refreshDur) {
				return
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

			setState("WAITING_FOR_PASS")

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

			if !r.waitForAOS(ctx, pass) {
				return
			}

			req := capture.CaptureRequest{
				Satellite: pass.Satellite,
				AOS:       pass.AOS,
				LOS:       pass.LOS,
				MaxElev:   pass.MaxElev,
			}

			if _, err := r.capturer.Capture(ctx, req, setState); err != nil {
				r.broadcast(map[string]any{
					"type":    "log",
					"level":   "error",
					"message": "capture failed: " + err.Error(),
				})
			}

			// TODO: APT decoding — this is it'll process the WAV into an image.
			setState("DECODING")
			r.broadcast(map[string]any{
				"type":    "log",
				"level":   "info",
				"message": fmt.Sprintf("decoding placeholder for %s (not yet implemented)", pass.Satellite.Name),
			})
			if !sleepOrCancel(ctx, 2*time.Second) {
				return
			}

			setState("IDLE")
		}
	}
}

// waitForAOS sleeps until AOS, broadcasting countdown progress every 30s.
// Returns false if the context was cancelled.
func (r *Runner) waitForAOS(ctx context.Context, pass predict.Pass) bool {
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
		if !sleepOrCancel(ctx, sleepDur) {
			return false
		}
	}
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
