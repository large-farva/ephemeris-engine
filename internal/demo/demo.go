// Package demo simulates the full satellite pass lifecycle so the daemon,
// CLI, and web dashboard can be tested end-to-end without SDR hardware.
// The simulated passes cycle through real NOAA satellite names, frequencies,
// and plausible orbital parameters so the event stream looks realistic.
package demo

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/capture"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// Runner broadcasts simulated pass events on a configurable interval.
type Runner struct {
	Hub      *ws.Hub
	Interval time.Duration // time between simulated passes

	passIndex int // cycles through the satellite catalog
}

// New creates a demo runner with a sensible default interval.
func New(hub *ws.Hub) *Runner {
	return &Runner{
		Hub:      hub,
		Interval: 30 * time.Second,
	}
}

// Run kicks off the demo loop. It fires one simulated pass immediately,
// then repeats on the configured interval until ctx is cancelled.
func (r *Runner) Run(ctx context.Context, setState func(string)) {
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": "demo mode active — simulating satellite passes",
	})

	if !sleepOrCancel(ctx, 2*time.Second) {
		return
	}
	r.runPass(ctx, setState)

	t := time.NewTicker(r.Interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.runPass(ctx, setState)
		}
	}
}

// runPass simulates one full pass lifecycle: schedule announcement,
// countdown to AOS, recording progress, decoding progress, then idle.
func (r *Runner) runPass(ctx context.Context, setState func(string)) {
	sat := r.nextSatellite()
	now := time.Now().UTC()

	// Plausible orbital parameters for the simulated pass.
	maxElev := 20.0 + rand.Float64()*60.0 // 20°–80°
	passDur := 8*time.Minute + time.Duration(rand.IntN(7))*time.Minute // 8–14 min
	aos := now.Add(5 * time.Second) // AOS is 5 seconds from now
	los := aos.Add(passDur)

	// Announce the scheduled pass, matching the real scheduler's event shape.
	setState("WAITING_FOR_PASS")
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("next pass: %s at %s (max elev %.1f°, duration %s)", sat.Name, aos.Format(time.RFC3339), maxElev, passDur.Truncate(time.Second)),
	})
	r.broadcast(map[string]any{
		"type":       "pass_scheduled",
		"satellite":  sat.Name,
		"norad_id":   sat.NoradID,
		"freq_hz":    sat.Freq,
		"aos":        aos.Format(time.RFC3339),
		"los":        los.Format(time.RFC3339),
		"max_elev":   maxElev,
		"duration_s": int(passDur.Seconds()),
	})

	// Countdown to AOS.
	for i := 5; i > 0; i-- {
		r.broadcast(map[string]any{
			"type":    "progress",
			"stage":   "waiting",
			"percent": 0,
			"detail":  fmt.Sprintf("AOS in %ds for %s", i, sat.Name),
		})
		if !sleepOrCancel(ctx, 1*time.Second) {
			return
		}
	}

	// Simulate recording.
	setState("RECORDING")
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("starting simulated capture for %s at %d Hz", sat.Name, sat.Freq),
	})

	bytesWritten := int64(0)
	for p := 0; p <= 100; p += 5 {
		bytesWritten += int64(48000 * 2 / 5) // ~48 kHz 16-bit, scaled
		r.broadcast(map[string]any{
			"type":    "progress",
			"stage":   "recording",
			"percent": p,
			"detail":  fmt.Sprintf("%s simulated capture: %d bytes", sat.Name, bytesWritten),
		})
		if !sleepOrCancel(ctx, 200*time.Millisecond) {
			return
		}
	}

	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("finished %s, %d bytes written", sat.Name, bytesWritten),
	})

	// Simulate decoding.
	setState("DECODING")
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("decoding APT image from %s pass", sat.Name),
	})

	for p := 0; p <= 100; p += 10 {
		r.broadcast(map[string]any{
			"type":    "progress",
			"stage":   "decoding",
			"percent": p,
			"detail":  fmt.Sprintf("%s APT decode", sat.Name),
		})
		if !sleepOrCancel(ctx, 250*time.Millisecond) {
			return
		}
	}

	// Back to idle.
	setState("IDLE")
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("pass complete for %s — next pass in %s", sat.Name, r.Interval.Truncate(time.Second)),
	})
}

// nextSatellite cycles through the NOAA catalog so each simulated pass
// features a different bird.
func (r *Runner) nextSatellite() capture.Satellite {
	sat := capture.Satellites[r.passIndex%len(capture.Satellites)]
	r.passIndex++
	return sat
}

func (r *Runner) broadcast(v map[string]any) {
	v["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	v["component"] = "demo"
	r.Hub.BroadcastJSON(v)
}

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
