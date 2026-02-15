package ctl

import (
	"encoding/json"
	"fmt"
	"net/http"
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
func Status(baseURL string) error {
	baseURL = strings.TrimRight(baseURL, "/")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL + "/api/status")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var s StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return err
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
