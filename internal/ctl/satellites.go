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

	t := newTable("  ", "Name", "NORAD ID", "Frequency")
	for _, s := range resp.Satellites {
		t.row(s.Name, fmt.Sprintf("%d", s.NoradID), fmt.Sprintf("%.3f MHz", float64(s.FreqHz)/1e6))
	}
	t.flush()
	fmt.Println()

	return nil
}
