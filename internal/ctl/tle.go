package ctl

import (
	"fmt"
	"strings"
)

// TLERefresh sends a TLE refresh request to the daemon.
func TLERefresh(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var resp struct {
		OK                bool   `json:"ok"`
		Message           string `json:"message"`
		Error             string `json:"error"`
		SatellitesUpdated int    `json:"satellites_updated"`
	}
	if err := postJSON(baseURL, "/api/tle-refresh", nil, &resp); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(resp)
	}

	fmt.Println()
	if resp.OK {
		fmt.Printf("  %s  %s\n", colorize(green, "REFRESHED"), resp.Message)
	} else {
		fmt.Printf("  %s  %s\n", colorize(red, "FAILED"), resp.Error)
	}
	fmt.Println()

	return nil
}
