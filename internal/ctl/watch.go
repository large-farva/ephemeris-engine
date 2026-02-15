package ctl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// WatchOptions controls the watch command behavior.
type WatchOptions struct {
	Filter []string // event types to show (empty = all)
	JSON   bool     // output raw JSON per event
}

// Watch connects to the daemon's WebSocket endpoint and streams events to
// the terminal in a human-readable format until interrupted.
func Watch(baseURL string, opts WatchOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	u, err := url.Parse(baseURL)
	if err != nil {
		return err
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return fmt.Errorf("unsupported scheme: %s", u.Scheme)
	}
	u.Path = "/ws"
	u.RawQuery = ""

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	if !opts.JSON {
		fmt.Println()
		fmt.Printf("  %s %s\n", colorize(green, "connected"), colorize(dim, u.String()))
		if len(opts.Filter) > 0 {
			fmt.Printf("  %s %s\n", colorize(dim, "filter:"), colorize(dim, strings.Join(opts.Filter, ", ")))
		}
		fmt.Println(colorize(dim, "  "+strings.Repeat("─", 50)))
		fmt.Println()
	}

	// Build a filter set for O(1) lookup.
	filterSet := make(map[string]bool, len(opts.Filter))
	for _, f := range opts.Filter {
		filterSet[f] = true
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			// Apply event type filter.
			if len(filterSet) > 0 {
				var ev map[string]any
				if err := json.Unmarshal(msg, &ev); err == nil {
					evType, _ := ev["type"].(string)
					if !filterSet[evType] {
						continue
					}
				}
			}

			if opts.JSON {
				fmt.Println(string(msg))
			} else {
				renderEvent(msg)
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sig:
		if !opts.JSON {
			fmt.Println()
			fmt.Println(colorize(dim, "  disconnecting..."))
		}
		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"),
			time.Now().Add(1*time.Second),
		)
		return nil
	case <-done:
		return nil
	}
}

// renderEvent parses a JSON event and prints it in a human-friendly format.
// Falls back to raw JSON for unrecognized event types.
func renderEvent(raw []byte) {
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		fmt.Printf("  %s\n", string(raw))
		return
	}

	evType, _ := ev["type"].(string)
	ts := formatEventTime(ev)

	switch evType {
	case "heartbeat":
		// Heartbeats are noisy — show them dimmed on a single line.
		state, _ := ev["state"].(string)
		uptime, _ := ev["uptime_seconds"].(float64)
		uptimeStr := formatDuration(time.Duration(uptime) * time.Second)
		fmt.Printf("  %s %s  %s  up %s\n",
			colorize(dim, ts),
			colorize(dim, "heartbeat"),
			colorize(stateColor(state), state),
			colorize(dim, uptimeStr),
		)

	case "state":
		from, _ := ev["from"].(string)
		to, _ := ev["to"].(string)
		fmt.Printf("  %s %s  %s %s %s\n",
			colorize(dim, ts),
			colorize(bold, "STATE"),
			colorize(stateColor(from), from),
			colorize(dim, "->"),
			colorize(stateColor(to), to),
		)

	case "log":
		level, _ := ev["level"].(string)
		message, _ := ev["message"].(string)
		component, _ := ev["component"].(string)
		levelStr := formatLogLevel(level)
		src := ""
		if component != "" {
			src = colorize(dim, "["+component+"] ")
		}
		fmt.Printf("  %s %s  %s%s\n", colorize(dim, ts), levelStr, src, message)

	case "progress":
		stage, _ := ev["stage"].(string)
		pct, _ := ev["percent"].(float64)
		detail, _ := ev["detail"].(string)
		bar := progressBar(int(pct), 20)
		fmt.Printf("  %s %s  [%s] %3.0f%%  %s\n",
			colorize(dim, ts),
			colorize(cyan, padRight(stage, 10)),
			bar,
			pct,
			colorize(dim, detail),
		)

	case "pass_scheduled":
		sat, _ := ev["satellite"].(string)
		aos, _ := ev["aos"].(string)
		los, _ := ev["los"].(string)
		maxElev, _ := ev["max_elev"].(float64)
		freqHz, _ := ev["freq_hz"].(float64)
		durSec, _ := ev["duration_s"].(float64)

		freqMHz := freqHz / 1e6
		durStr := formatDuration(time.Duration(durSec) * time.Second)

		fmt.Println()
		fmt.Printf("  %s %s\n", colorize(dim, ts), header("PASS SCHEDULED"))
		fmt.Printf("    %-14s %s\n", colorize(dim, "Satellite:"), colorize(bold, sat))
		fmt.Printf("    %-14s %.3f MHz\n", colorize(dim, "Frequency:"), freqMHz)
		fmt.Printf("    %-14s %s\n", colorize(dim, "AOS:"), aos)
		fmt.Printf("    %-14s %s\n", colorize(dim, "LOS:"), los)
		fmt.Printf("    %-14s %.1f°\n", colorize(dim, "Max elev:"), maxElev)
		fmt.Printf("    %-14s %s\n", colorize(dim, "Duration:"), durStr)
		fmt.Println()

	default:
		// Unknown event type — dump as indented JSON so nothing is lost.
		pretty, err := json.MarshalIndent(ev, "  ", "  ")
		if err != nil {
			fmt.Printf("  %s\n", string(raw))
			return
		}
		fmt.Printf("  %s\n", string(pretty))
	}
}

// formatEventTime extracts and shortens the timestamp from an event.
func formatEventTime(ev map[string]any) string {
	tsRaw, ok := ev["ts"].(string)
	if !ok {
		return "          "
	}
	t, err := time.Parse(time.RFC3339Nano, tsRaw)
	if err != nil {
		return tsRaw[:10]
	}
	return t.Local().Format("15:04:05")
}

// formatLogLevel returns a colored, fixed-width log level label.
func formatLogLevel(level string) string {
	switch level {
	case "info":
		return colorize(green, "INFO ")
	case "warn":
		return colorize(yellow, "WARN ")
	case "error":
		return colorize(red, "ERROR")
	default:
		return padRight(level, 5)
	}
}
