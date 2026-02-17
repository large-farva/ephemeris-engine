package ctl

import (
	"fmt"
	"net/http"
	"strings"
)

// Health checks daemon liveness and optionally component health via GET /healthz.
// When --json is used, it requests detailed component-level health checks.
func Health(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	if jsonOutput {
		return healthDetailed(baseURL)
	}

	status, _, err := getRaw(baseURL, "/healthz")
	if err != nil {
		return err
	}

	fmt.Println()
	if status == 200 {
		fmt.Printf("  %s  ephemerisd is reachable at %s\n", colorize(green, "HEALTHY"), colorize(dim, baseURL))
	} else {
		fmt.Printf("  %s  ephemerisd returned HTTP %d at %s\n", colorize(red, "UNHEALTHY"), status, colorize(dim, baseURL))
	}
	fmt.Println()
	return nil
}

// healthDetailed fetches component-level health checks via JSON Accept header.
func healthDetailed(baseURL string) error {
	url := strings.TrimRight(baseURL, "/") + "/healthz"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return printJSON(map[string]any{"healthy": false, "url": baseURL, "error": err.Error()})
	}
	defer resp.Body.Close()

	var result struct {
		Healthy bool                      `json:"healthy"`
		Checks  map[string]map[string]any `json:"checks"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return err
	}

	return printJSON(result)
}
