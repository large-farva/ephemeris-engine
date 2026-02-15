package ctl

import (
	"fmt"
	"strings"
)

// Build-time variables set via -ldflags.
var (
	Version   = "dev"
	GoVersion = "unknown"
)

// VersionInfo fetches daemon version via GET /api/version and displays both
// the CLI and daemon version information.
func VersionInfo(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var daemon struct {
		Version   string `json:"version"`
		GoVersion string `json:"go_version"`
		BuiltAt   string `json:"built_at"`
	}
	daemonErr := getJSON(baseURL, "/api/version", &daemon)

	if jsonOutput {
		resp := map[string]any{
			"cli": map[string]any{
				"version":    Version,
				"go_version": GoVersion,
			},
		}
		if daemonErr == nil {
			resp["daemon"] = daemon
		} else {
			resp["daemon_error"] = daemonErr.Error()
		}
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  EPHEMERIS VERSION"))
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 38)))
	fmt.Printf("  %-12s %s\n", colorize(dim, "CLI:"), Version+" ("+GoVersion+")")
	if daemonErr != nil {
		fmt.Printf("  %-12s %s\n", colorize(dim, "Daemon:"), colorize(red, "unreachable: "+daemonErr.Error()))
	} else {
		fmt.Printf("  %-12s %s\n", colorize(dim, "Daemon:"), daemon.Version+" ("+daemon.GoVersion+")")
		fmt.Printf("  %-12s %s\n", colorize(dim, "Built:"), daemon.BuiltAt)
	}
	fmt.Println()

	return nil
}
