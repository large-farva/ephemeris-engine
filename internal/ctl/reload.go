package ctl

import (
	"fmt"
	"strings"
)

// ReloadOptions configures the reload command.
type ReloadOptions struct {
	Profile string
	JSON    bool
}

// Reload tells the daemon to re-read its config file from disk.
// If Profile is set, the daemon switches to that named profile.
func Reload(baseURL string, opts ReloadOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var body any
	if opts.Profile != "" {
		body = map[string]string{"profile": opts.Profile}
	}

	var result struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := postJSON(baseURL, "/api/reload", body, &result); err != nil {
		return err
	}

	if opts.JSON {
		return printJSON(result)
	}

	if result.OK {
		fmt.Printf("\n  %s  %s\n\n", colorize(green, "RELOADED"), result.Message)
	} else {
		fmt.Printf("\n  %s  %s\n\n", colorize(red, "ERROR"), result.Error)
	}
	return nil
}
