package ctl

import (
	"fmt"
	"strings"
)

// Pause pauses automatic pass scheduling on the daemon.
func Pause(baseURL string, jsonOutput bool) error {
	return schedulerControl(baseURL, "/api/pause", "PAUSED", jsonOutput)
}

// Resume resumes automatic pass scheduling on the daemon.
func Resume(baseURL string, jsonOutput bool) error {
	return schedulerControl(baseURL, "/api/resume", "RESUMED", jsonOutput)
}

// Skip skips the current or next scheduled pass.
func Skip(baseURL string, jsonOutput bool) error {
	return schedulerControl(baseURL, "/api/skip", "SKIPPED", jsonOutput)
}

// Cancel aborts an in-progress capture.
func Cancel(baseURL string, jsonOutput bool) error {
	return schedulerControl(baseURL, "/api/cancel", "CANCELLED", jsonOutput)
}

func schedulerControl(baseURL, path, label string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := postJSON(baseURL, path, nil, &result); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(result)
	}

	if result.OK {
		fmt.Printf("\n  %s  %s\n\n", colorize(green, label), result.Message)
	} else {
		fmt.Printf("\n  %s  %s\n\n", colorize(red, "ERROR"), result.Error)
	}
	return nil
}
