package ctl

import (
	"fmt"
	"strings"
	"time"
)

// Stats shows aggregate capture statistics from the daemon.
func Stats(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var resp struct {
		TotalCaptures int            `json:"total_captures"`
		TotalBytes    int64          `json:"total_bytes"`
		CapturesBySat map[string]int `json:"captures_by_satellite"`
		LastCaptureAt string         `json:"last_capture_at"`
		UptimeSeconds int64          `json:"uptime_seconds"`
	}
	if err := getJSON(baseURL, "/api/stats", &resp); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  CAPTURE STATISTICS"))
	fmt.Println("  " + strings.Repeat("─", 42))
	fmt.Printf("  Uptime:          %s\n", formatDuration(time.Duration(resp.UptimeSeconds)*time.Second))
	fmt.Printf("  Total captures:  %d\n", resp.TotalCaptures)
	fmt.Printf("  Total data:      %s\n", formatBytes(resp.TotalBytes))

	if resp.LastCaptureAt != "" {
		fmt.Printf("  Last capture:    %s\n", resp.LastCaptureAt)
	} else {
		fmt.Printf("  Last capture:    none\n")
	}

	if len(resp.CapturesBySat) > 0 {
		fmt.Println()
		fmt.Println(header("  BY SATELLITE"))
		t := newTable("  ", "Satellite", "Captures")
		t.alignRight(1)
		for sat, count := range resp.CapturesBySat {
			t.row(sat, fmt.Sprintf("%d", count))
		}
		t.flush()
	}

	fmt.Println()
	return nil
}
