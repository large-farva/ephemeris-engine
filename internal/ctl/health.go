package ctl

import (
	"fmt"
	"strings"
)

// Health checks daemon liveness via GET /healthz.
func Health(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	status, _, err := getRaw(baseURL, "/healthz")
	if err != nil {
		if jsonOutput {
			return printJSON(map[string]any{"healthy": false, "url": baseURL, "error": err.Error()})
		}
		return err
	}

	healthy := status == 200

	if jsonOutput {
		return printJSON(map[string]any{"healthy": healthy, "url": baseURL})
	}

	fmt.Println()
	if healthy {
		fmt.Printf("  %s  ephemerisd is reachable at %s\n", colorize(green, "HEALTHY"), colorize(dim, baseURL))
	} else {
		fmt.Printf("  %s  ephemerisd returned HTTP %d at %s\n", colorize(red, "UNHEALTHY"), status, colorize(dim, baseURL))
	}
	fmt.Println()

	return nil
}
