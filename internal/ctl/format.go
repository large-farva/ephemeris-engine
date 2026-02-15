// Package ctl implements the client-side commands for ephctl.
// It talks to a running ephemerisd over HTTP and WebSocket and renders the results to the terminal.
package ctl

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// ANSI escape codes for terminal formatting.
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	white  = "\033[37m"
)

// colorEnabled reports whether stdout is a terminal. When output is piped
// or redirected, ANSI escape codes are suppressed.
func colorEnabled() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// stateColor returns the ANSI color code appropriate for a daemon state.
func stateColor(state string) string {
	if !colorEnabled() {
		return ""
	}
	switch state {
	case "IDLE":
		return green
	case "WAITING_FOR_PASS":
		return yellow
	case "RECORDING":
		return blue
	case "DECODING":
		return cyan
	case "BOOTING":
		return dim
	default:
		return white
	}
}

// colorize wraps text with an ANSI color sequence.
// Returns the text unchanged when color output is disabled.
func colorize(color, text string) string {
	if !colorEnabled() {
		return text
	}
	return color + text + reset
}

// header returns a bold section header, or plain text when color is off.
func header(title string) string {
	if colorEnabled() {
		return bold + title + reset
	}
	return title
}

// padRight pads s with spaces to reach the given width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// formatDuration renders a time.
// Duration as a compact human string like "2h 14m 8s" or "45s".
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatBytes renders a byte count as a human-readable string.
func formatBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// progressBar builds a simple ASCII bar of the given width.
// The filled portion is colored green when color output is enabled.
func progressBar(pct, width int) string {
	filled := (pct * width) / 100
	if filled > width {
		filled = width
	}
	empty := width - filled
	if colorEnabled() {
		return green + strings.Repeat("=", filled) + reset + strings.Repeat(" ", empty)
	}
	return strings.Repeat("=", filled) + strings.Repeat(" ", empty)
}
