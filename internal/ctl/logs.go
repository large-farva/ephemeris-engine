package ctl

import (
	"fmt"
	"strings"
	"time"
)

// LogsOptions configures the logs command.
type LogsOptions struct {
	Level string
	Limit int
	Tail  bool
	JSON  bool
}

// Logs shows recent daemon log messages, or streams them live with --tail.
func Logs(baseURL string, opts LogsOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	// --tail mode: use WebSocket watch with log filter.
	if opts.Tail {
		return Watch(baseURL, WatchOptions{
			Filter: []string{"log"},
			JSON:   opts.JSON,
		})
	}

	// Query the log buffer.
	path := "/api/logs"
	var params []string
	if opts.Level != "" {
		params = append(params, "level="+opts.Level)
	}
	if opts.Limit > 0 {
		params = append(params, fmt.Sprintf("limit=%d", opts.Limit))
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	var resp struct {
		Logs []struct {
			TS        string `json:"ts"`
			Level     string `json:"level"`
			Message   string `json:"message"`
			Component string `json:"component"`
		} `json:"logs"`
	}
	if err := getJSON(baseURL, path, &resp); err != nil {
		return err
	}

	if opts.JSON {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  DAEMON LOGS"))
	fmt.Println("  " + strings.Repeat("─", 70))

	if len(resp.Logs) == 0 {
		fmt.Println("  No log entries found.")
	} else {
		for _, entry := range resp.Logs {
			ts := entry.TS
			if t, err := time.Parse(time.RFC3339Nano, entry.TS); err == nil {
				ts = t.Local().Format("15:04:05")
			}

			levelColor := dim
			switch entry.Level {
			case "info":
				levelColor = green
			case "error":
				levelColor = red
			case "warn":
				levelColor = yellow
			}

			fmt.Printf("  %s %s  [%s] %s\n",
				ts,
				colorize(levelColor, padRight(entry.Level, 5)),
				entry.Component,
				entry.Message,
			)
		}
	}

	fmt.Println()
	return nil
}
