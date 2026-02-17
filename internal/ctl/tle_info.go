package ctl

import (
	"fmt"
	"strings"
	"time"
)

// TLEInfo shows TLE cache status and freshness.
func TLEInfo(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var resp struct {
		Path      string `json:"path"`
		Exists    bool   `json:"exists"`
		Fresh     bool   `json:"fresh"`
		ModTime   string `json:"mod_time"`
		AgeS      int    `json:"age_s"`
		Size      int64  `json:"size"`
		SourceURL string `json:"source_url"`
		MaxAgeH   int    `json:"max_age_hours"`
	}
	if err := getJSON(baseURL, "/api/tle-info", &resp); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  TLE CACHE INFO"))
	fmt.Println("  " + strings.Repeat("─", 50))
	fmt.Printf("  Cache file: %s\n", resp.Path)

	if !resp.Exists {
		fmt.Printf("  Status:     %s\n", colorize(red, "NOT FOUND"))
		fmt.Printf("  Source:     %s\n", resp.SourceURL)
		fmt.Println()
		return nil
	}

	if resp.Fresh {
		fmt.Printf("  Status:     %s\n", colorize(green, "FRESH"))
	} else {
		fmt.Printf("  Status:     %s\n", colorize(yellow, "STALE"))
	}

	age := time.Duration(resp.AgeS) * time.Second
	fmt.Printf("  Age:        %s\n", formatDuration(age))
	fmt.Printf("  Last fetch: %s\n", resp.ModTime)
	fmt.Printf("  Max age:    %dh\n", resp.MaxAgeH)
	fmt.Printf("  Size:       %s\n", formatBytes(resp.Size))
	fmt.Printf("  Source:     %s\n", resp.SourceURL)
	fmt.Println()
	return nil
}
