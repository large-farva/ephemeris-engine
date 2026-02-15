package capture

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// CaptureRequest holds the parameters for a single satellite recording session.
type CaptureRequest struct {
	Satellite Satellite
	AOS       time.Time // acquisition of signal
	LOS       time.Time // loss of signal
	MaxElev   float64   // peak elevation in degrees
}

// Runner records satellite passes to WAV files. When Simulate is true it
// generates a synthetic tone instead of spawning rtl_fm, allowing the full
// pipeline to be tested without SDR hardware.
type Runner struct {
	Hub      *ws.Hub
	Cfg      config.Config
	Log      *log.Logger
	Simulate bool
}

// New creates a capture runner. Set simulate to true when no SDR hardware
// is available; the runner will generate a synthetic WAV file instead.
func New(hub *ws.Hub, cfg config.Config, logger *log.Logger, simulate bool) *Runner {
	return &Runner{
		Hub:      hub,
		Cfg:      cfg,
		Log:      logger,
		Simulate: simulate,
	}
}

// Capture runs a single recording session. It creates a timestamped WAV file
// under the configured data root and either records from rtl_fm or generates
// a synthetic tone, depending on the Simulate flag. The method blocks until
// LOS or context cancellation.
func (r *Runner) Capture(ctx context.Context, req CaptureRequest, setState func(string)) (string, error) {
	setState("RECORDING")

	ts := req.AOS.UTC().Format("20060102T150405Z")
	filename := fmt.Sprintf("%s_%s.wav", req.Satellite.Name, ts)
	outPath := filepath.Join(r.Cfg.Data.Root, filename)

	mode := "live"
	if r.Simulate {
		mode = "simulated"
	}
	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("starting %s capture for %s at %d Hz -> %s", mode, req.Satellite.Name, req.Satellite.Freq, outPath),
	})

	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create wav: %w", err)
	}
	defer f.Close()

	if err := writeWAVHeader(f, uint32(r.Cfg.SDR.SampleRate), 0); err != nil {
		return "", fmt.Errorf("write wav header: %w", err)
	}

	var bytesWritten int64
	if r.Simulate {
		bytesWritten = r.simulateCapture(ctx, f, req)
	} else {
		var captureErr error
		bytesWritten, captureErr = r.rtlCapture(ctx, f, req)
		if captureErr != nil {
			return "", captureErr
		}
	}

	if bytesWritten > 0 {
		if err := fixWAVHeader(f); err != nil {
			r.Log.Printf("capture: failed to finalize WAV header: %v", err)
		}
	}

	r.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("finished %s, %d bytes written to %s", req.Satellite.Name, bytesWritten, filename),
	})

	return outPath, nil
}

// rtlCapture records a pass by running rtl_fm as a subprocess. The process
// is killed automatically when the LOS deadline arrives or the context is
// cancelled.
func (r *Runner) rtlCapture(ctx context.Context, f *os.File, req CaptureRequest) (int64, error) {
	losCtx, losCancel := context.WithDeadline(ctx, req.LOS)
	defer losCancel()

	args := buildRtlFmArgs(r.Cfg.SDR, req.Satellite.Freq)
	cmd := exec.CommandContext(losCtx, "rtl_fm", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start rtl_fm: %w", err)
	}

	totalDuration := req.LOS.Sub(req.AOS)
	bytesWritten := r.streamWithProgress(losCtx, f, stdout, req, totalDuration)

	// CommandContext sends SIGKILL on cancel; explicit Kill is a safety net.
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()

	return bytesWritten, nil
}

// simulateCapture writes a synthetic 2400 Hz sine wave (the APT subcarrier
// frequency) in buffered chunks. Duration is scaled by demo.interval_seconds
// so a real 12-minute pass can simulate in a few seconds.
func (r *Runner) simulateCapture(ctx context.Context, f io.Writer, req CaptureRequest) int64 {
	sampleRate := r.Cfg.SDR.SampleRate

	// Scale capture duration for simulation. Use interval_seconds as the
	// simulated pass length; default to 15 seconds if unset.
	simDuration := 15 * time.Second
	if r.Cfg.Demo.IntervalSeconds > 0 {
		simDuration = time.Duration(r.Cfg.Demo.IntervalSeconds) * time.Second
	}

	totalSamples := int(simDuration.Seconds()) * sampleRate
	freq := 2400.0 // APT uses a 2400 Hz AM subcarrier

	const chunkSamples = 4096
	buf := make([]byte, chunkSamples*2) // 16-bit = 2 bytes per sample

	var written int64
	lastReport := time.Now()
	samplesWritten := 0

	for samplesWritten < totalSamples {
		select {
		case <-ctx.Done():
			return written
		default:
		}

		n := chunkSamples
		if samplesWritten+n > totalSamples {
			n = totalSamples - samplesWritten
		}

		for i := 0; i < n; i++ {
			t := float64(samplesWritten+i) / float64(sampleRate)
			sample := int16(16000.0 * math.Sin(2.0*math.Pi*freq*t))
			binary.LittleEndian.PutUint16(buf[i*2:], uint16(sample))
		}

		nw, err := f.Write(buf[:n*2])
		written += int64(nw)
		samplesWritten += n
		if err != nil {
			r.Log.Printf("capture: simulated write error: %v", err)
			return written
		}

		// Throttle to roughly 10x real-time so progress events fire.
		if samplesWritten%(sampleRate/10) < chunkSamples {
			time.Sleep(100 * time.Millisecond)
		}

		if time.Since(lastReport) >= 2*time.Second {
			pct := (float64(samplesWritten) / float64(totalSamples)) * 100
			r.broadcast(map[string]any{
				"type":    "progress",
				"stage":   "recording",
				"percent": int(pct),
				"detail":  fmt.Sprintf("%s simulated capture: %d bytes", req.Satellite.Name, written),
			})
			lastReport = time.Now()
		}
	}

	return written
}

// streamWithProgress copies PCM data from a reader (typically rtl_fm stdout)
// to the WAV file, broadcasting progress events every 2 seconds.
func (r *Runner) streamWithProgress(ctx context.Context, dst io.Writer, src io.Reader, req CaptureRequest, totalDuration time.Duration) int64 {
	buf := make([]byte, 8192)
	var written int64
	lastReport := time.Now()
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			return written
		default:
		}

		n, readErr := src.Read(buf)
		if n > 0 {
			nw, writeErr := dst.Write(buf[:n])
			written += int64(nw)
			if writeErr != nil {
				r.Log.Printf("capture: write error: %v", writeErr)
				return written
			}
		}

		if time.Since(lastReport) >= 2*time.Second {
			elapsed := time.Since(startTime)
			pct := (elapsed.Seconds() / totalDuration.Seconds()) * 100
			if pct > 100 {
				pct = 100
			}
			r.broadcast(map[string]any{
				"type":    "progress",
				"stage":   "recording",
				"percent": int(pct),
				"detail":  fmt.Sprintf("%s capture: %d bytes", req.Satellite.Name, written),
			})
			lastReport = time.Now()
		}

		if readErr == io.EOF {
			return written
		}
		if readErr != nil {
			r.Log.Printf("capture: read error: %v", readErr)
			return written
		}
	}
}

// buildRtlFmArgs assembles the command-line flags for rtl_fm. Output goes
// to stdout ("-") so we can pipe it directly into the WAV writer.
func buildRtlFmArgs(sdr config.SDRConfig, freq int) []string {
	return []string{
		"-f", fmt.Sprintf("%d", freq),
		"-s", fmt.Sprintf("%d", sdr.SampleRate),
		"-g", fmt.Sprintf("%.1f", sdr.Gain),
		"-p", fmt.Sprintf("%d", sdr.PPMCorrection),
		"-d", fmt.Sprintf("%d", sdr.DeviceIndex),
		"-E", "dc",
		"-M", "fm",
		"-",
	}
}

func (r *Runner) broadcast(v map[string]any) {
	v["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	v["component"] = "capture"
	r.Hub.BroadcastJSON(v)
}
