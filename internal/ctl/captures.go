package ctl

import (
	"fmt"
	"net/http"
	"strings"
)

// CapturesOptions configures the captures command.
type CapturesOptions struct {
	Delete string
	JSON   bool
}

// Captures lists or deletes capture files on the daemon.
func Captures(baseURL string, opts CapturesOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	// Handle deletion.
	if opts.Delete != "" {
		url := baseURL + "/api/captures?name=" + opts.Delete
		req, err := http.NewRequest(http.MethodDelete, url, nil)
		if err != nil {
			return err
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var result struct {
			OK      bool   `json:"ok"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		if err := decodeJSON(resp, &result); err != nil {
			return err
		}
		if opts.JSON {
			return printJSON(result)
		}
		if result.OK {
			fmt.Printf("\n  %s  %s\n\n", colorize(green, "DELETED"), result.Message)
		} else {
			fmt.Printf("\n  %s  %s\n\n", colorize(red, "ERROR"), result.Error)
		}
		return nil
	}

	// List captures.
	var resp struct {
		Captures []struct {
			Filename  string `json:"filename"`
			Satellite string `json:"satellite"`
			Timestamp string `json:"timestamp"`
			Size      int64  `json:"size"`
		} `json:"captures"`
	}
	if err := getJSON(baseURL, "/api/captures", &resp); err != nil {
		return err
	}

	if opts.JSON {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  CAPTURES"))

	if len(resp.Captures) == 0 {
		fmt.Println(colorize(dim, "  ────────────────────────"))
		fmt.Println("  No capture files found.")
	} else {
		t := newTable("  ", "Satellite", "Timestamp", "Size", "Filename")
		t.alignRight(2)
		for _, c := range resp.Captures {
			t.row(c.Satellite, c.Timestamp, formatBytes(c.Size), c.Filename)
		}
		t.flush()
	}
	fmt.Println()
	return nil
}
