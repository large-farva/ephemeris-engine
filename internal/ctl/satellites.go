package ctl

import (
	"fmt"
	"strings"
)

// Satellites lists the NOAA satellite catalog from the daemon.
func Satellites(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	var resp struct {
		Satellites []struct {
			Name    string `json:"name"`
			NoradID int    `json:"norad_id"`
			FreqHz  int    `json:"freq_hz"`
		} `json:"satellites"`
	}
	if err := getJSON(baseURL, "/api/satellites", &resp); err != nil {
		return err
	}

	if jsonOutput {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  SATELLITE CATALOG"))
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 46)))
	fmt.Printf("  %-12s %-12s %s\n",
		colorize(dim, "Name"),
		colorize(dim, "NORAD ID"),
		colorize(dim, "Frequency"),
	)
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 46)))
	for _, s := range resp.Satellites {
		freqMHz := float64(s.FreqHz) / 1e6
		fmt.Printf("  %-12s %-12d %.3f MHz\n", s.Name, s.NoradID, freqMHz)
	}
	fmt.Println()

	return nil
}
