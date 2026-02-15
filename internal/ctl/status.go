package ctl

import (
	"fmt"
	"strings"
	"time"
)

// StatusResponse mirrors the JSON returned by GET /api/status.
type StatusResponse struct {
	Name          string `json:"name"`
	State         string `json:"state"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	DataRoot      string `json:"data_root"`
	ArchiveDir    string `json:"archive_dir"`
}

// Status fetches the daemon status and prints a formatted summary.
func Status(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var s StatusResponse
	if err := getJSON(baseURL, "/api/status", &s); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(s)
	}

	uptime := formatDuration(time.Duration(s.UptimeSeconds) * time.Second)
	stateStr := colorize(stateColor(s.State), s.State)

	fmt.Println()
	fmt.Println(header("  EPHEMERIS ENGINE STATUS"))
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 38)))
	fmt.Printf("  %-12s %s\n", colorize(dim, "Daemon:"), s.Name)
	fmt.Printf("  %-12s %s\n", colorize(dim, "State:"), stateStr)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Uptime:"), uptime)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Data:"), s.DataRoot)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Archive:"), s.ArchiveDir)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Host:"), baseURL)
	fmt.Println()

	return nil
}
