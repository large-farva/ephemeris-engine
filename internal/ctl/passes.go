package ctl

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PassesOptions controls the passes command output.
type PassesOptions struct {
	Count     int
	Satellite string
	JSON      bool
}

// Passes lists upcoming satellite passes from the daemon.
func Passes(baseURL string, opts PassesOptions) error {
	baseURL = strings.TrimRight(baseURL, "/")

	// Build query string.
	params := url.Values{}
	if opts.Count > 0 {
		params.Set("count", strconv.Itoa(opts.Count))
	}
	if opts.Satellite != "" {
		params.Set("satellite", opts.Satellite)
	}
	path := "/api/passes"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp struct {
		Passes []struct {
			Satellite   string  `json:"satellite"`
			NoradID     int     `json:"norad_id"`
			FreqHz      int     `json:"freq_hz"`
			AOS         string  `json:"aos"`
			LOS         string  `json:"los"`
			MaxElev     float64 `json:"max_elev"`
			MaxElevTime string  `json:"max_elev_time"`
			AOSAzimuth  float64 `json:"aos_azimuth"`
			LOSAzimuth  float64 `json:"los_azimuth"`
			DurationS   int     `json:"duration_s"`
		} `json:"passes"`
		Station struct {
			Lat float64 `json:"lat"`
			Lon float64 `json:"lon"`
			Alt float64 `json:"alt"`
		} `json:"station"`
	}

	// Passes computation may involve TLE network fetches and SGP4 propagation,
	// so use a longer timeout than the default 5s client.
	passClient := &http.Client{Timeout: 60 * time.Second}
	fullURL := baseURL + path
	httpResp, err := passClient.Get(fullURL)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(httpResp.Body)
		msg := strings.TrimSpace(string(b))
		if msg != "" {
			return fmt.Errorf("HTTP %s: %s", httpResp.Status, msg)
		}
		return fmt.Errorf("HTTP %s from %s", httpResp.Status, path)
	}
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return err
	}

	if opts.JSON {
		return printJSON(resp)
	}

	fmt.Println()
	fmt.Println(header("  UPCOMING PASSES"))
	fmt.Printf("  %s %.4f, %.4f, %.0fm\n",
		colorize(dim, "Station:"),
		resp.Station.Lat, resp.Station.Lon, resp.Station.Alt,
	)
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 76)))

	if len(resp.Passes) == 0 {
		fmt.Println(colorize(dim, "  No upcoming passes found."))
		fmt.Println()
		return nil
	}

	fmt.Printf("  %-4s %-12s %-22s %-22s %6s  %s\n",
		colorize(dim, "#"),
		colorize(dim, "Satellite"),
		colorize(dim, "AOS"),
		colorize(dim, "LOS"),
		colorize(dim, "Elev"),
		colorize(dim, "Duration"),
	)
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 76)))

	for i, p := range resp.Passes {
		aosTime := formatPassTime(p.AOS)
		losTime := formatPassTime(p.LOS)
		dur := formatDuration(time.Duration(p.DurationS) * time.Second)

		fmt.Printf("  %-4d %-12s %-22s %-22s %5.1f°  %s\n",
			i+1,
			colorize(bold, p.Satellite),
			aosTime,
			losTime,
			p.MaxElev,
			dur,
		)
	}
	fmt.Println()

	return nil
}

// formatPassTime parses an RFC3339 timestamp and returns a local time string.
func formatPassTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04 MST")
}
