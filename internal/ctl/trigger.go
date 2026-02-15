package ctl

import (
	"fmt"
	"strings"
)

// TriggerOptions controls the trigger command.
type TriggerOptions struct {
	Satellite       string
	NoradID         int
	DurationSeconds int
	JSON            bool
}

// Trigger sends a capture trigger request to the daemon.
func Trigger(baseURL string, opts TriggerOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	body := map[string]any{}
	if opts.NoradID != 0 {
		body["norad_id"] = opts.NoradID
	} else if opts.Satellite != "" {
		body["satellite"] = opts.Satellite
	} else {
		return fmt.Errorf("satellite name or --norad-id required")
	}
	if opts.DurationSeconds > 0 {
		body["duration_seconds"] = opts.DurationSeconds
	}

	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := postJSON(baseURL, "/api/trigger", body, &resp); err != nil {
		return err
	}

	if opts.JSON {
		return printJSON(resp)
	}

	fmt.Println()
	if resp.OK {
		fmt.Printf("  %s  %s\n", colorize(green, "TRIGGERED"), resp.Message)
	} else {
		fmt.Printf("  %s  %s\n", colorize(red, "FAILED"), resp.Error)
	}
	fmt.Println()

	return nil
}
