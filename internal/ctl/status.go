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
	Mode          string `json:"mode"`
	DemoEnabled   bool   `json:"demo_enabled"`
	Paused        bool   `json:"paused"`
	CurrentPass   *struct {
		Satellite string  `json:"satellite"`
		NoradID   int     `json:"norad_id"`
		FreqHz    int     `json:"freq_hz"`
		AOS       string  `json:"aos"`
		LOS       string  `json:"los"`
		MaxElev   float64 `json:"max_elev"`
		Stage     string  `json:"stage"`
	} `json:"current_pass"`
	Disk *struct {
		TotalBytes     uint64 `json:"total_bytes"`
		UsedBytes      uint64 `json:"used_bytes"`
		AvailableBytes uint64 `json:"available_bytes"`
	} `json:"disk"`
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
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 42)))
	fmt.Printf("  %-12s %s\n", colorize(dim, "Daemon:"), s.Name)
	fmt.Printf("  %-12s %s\n", colorize(dim, "State:"), stateStr)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Mode:"), s.Mode)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Uptime:"), uptime)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Data:"), s.DataRoot)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Archive:"), s.ArchiveDir)
	fmt.Printf("  %-12s %s\n", colorize(dim, "Host:"), baseURL)

	if s.Paused {
		fmt.Printf("  %-12s %s\n", colorize(dim, "Scheduler:"), colorize(yellow, "PAUSED"))
	}

	// Current/next pass details.
	if s.CurrentPass != nil {
		cp := s.CurrentPass
		fmt.Println()
		fmt.Println(header("  CURRENT PASS"))
		fmt.Println(colorize(dim, "  "+strings.Repeat("─", 42)))
		fmt.Printf("  %-12s %s (NORAD %d)\n", colorize(dim, "Satellite:"), cp.Satellite, cp.NoradID)
		fmt.Printf("  %-12s %.3f MHz\n", colorize(dim, "Frequency:"), float64(cp.FreqHz)/1e6)
		fmt.Printf("  %-12s %s\n", colorize(dim, "AOS:"), cp.AOS)
		fmt.Printf("  %-12s %s\n", colorize(dim, "LOS:"), cp.LOS)
		fmt.Printf("  %-12s %.1f°\n", colorize(dim, "Max elev:"), cp.MaxElev)
		fmt.Printf("  %-12s %s\n", colorize(dim, "Stage:"), colorize(stateColor(strings.ToUpper(cp.Stage)), cp.Stage))
	}

	// Disk usage.
	if s.Disk != nil {
		fmt.Println()
		fmt.Println(header("  DISK USAGE"))
		fmt.Println(colorize(dim, "  "+strings.Repeat("─", 42)))
		fmt.Printf("  %-12s %s\n", colorize(dim, "Total:"), formatBytes(int64(s.Disk.TotalBytes)))
		fmt.Printf("  %-12s %s\n", colorize(dim, "Used:"), formatBytes(int64(s.Disk.UsedBytes)))
		fmt.Printf("  %-12s %s\n", colorize(dim, "Available:"), formatBytes(int64(s.Disk.AvailableBytes)))
	}

	fmt.Println()
	return nil
}
