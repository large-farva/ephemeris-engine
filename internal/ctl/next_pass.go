package ctl

import (
	"fmt"
	"strings"
	"time"
)

// NextPassOptions configures the next-pass command.
type NextPassOptions struct {
	Satellite string
	JSON      bool
}

// NextPass shows the next upcoming satellite pass.
func NextPass(baseURL string, opts NextPassOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	path := "/api/next-pass"
	if opts.Satellite != "" {
		path += "?satellite=" + opts.Satellite
	}

	var resp struct {
		Pass *struct {
			Satellite   string  `json:"satellite"`
			NoradID     int     `json:"norad_id"`
			FreqHz      int     `json:"freq_hz"`
			AOS         string  `json:"aos"`
			LOS         string  `json:"los"`
			MaxElev     float64 `json:"max_elev"`
			MaxElevTime string  `json:"max_elev_time"`
			DurationS   int     `json:"duration_s"`
		} `json:"pass"`
		CountdownS int `json:"countdown_s"`
		Station    struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
			Alt float64 `json:"alt"`
		} `json:"station"`
	}
	if err := getJSON(baseURL, path, &resp); err != nil {
		return err
	}

	if opts.JSON {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  NEXT PASS"))
	fmt.Println("  " + strings.Repeat("─", 42))

	if resp.Pass == nil {
		fmt.Println("  No upcoming passes found.")
		fmt.Println()
		return nil
	}

	p := resp.Pass
	countdown := time.Duration(resp.CountdownS) * time.Second

	fmt.Printf("  Satellite:  %s (NORAD %d)\n", p.Satellite, p.NoradID)
	fmt.Printf("  Frequency:  %.3f MHz\n", float64(p.FreqHz)/1e6)
	fmt.Printf("  AOS:        %s\n", p.AOS)
	fmt.Printf("  LOS:        %s\n", p.LOS)
	fmt.Printf("  Max elev:   %.1f°\n", p.MaxElev)
	fmt.Printf("  Duration:   %s\n", formatDuration(time.Duration(p.DurationS)*time.Second))

	if countdown > 0 {
		fmt.Printf("  Countdown:  %s\n", formatDuration(countdown))
	} else {
		fmt.Printf("  Status:     %s\n", colorize(green, "NOW"))
	}

	fmt.Println()
	return nil
}
