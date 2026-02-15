package ctl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Config fetches and displays the daemon's running configuration.
func Config(baseURL string, jsonOutput bool) error {
	baseURL = strings.TrimRight(baseURL, "/")

	// Decode into a generic map to preserve all fields for both display modes.
	var raw json.RawMessage
	if err := getJSON(baseURL, "/api/config", &raw); err != nil {
		return err
	}

	if jsonOutput {
		var v any
		_ = json.Unmarshal(raw, &v)
		return printJSON(v)
	}

	// Decode into ordered sections for human-readable output.
	var cfg struct {
		Data struct {
			Root    string `json:"root"`
			Archive string `json:"archive"`
		} `json:"data"`
		Logging struct {
			Level string `json:"level"`
		} `json:"logging"`
		Server struct {
			Bind string `json:"bind"`
		} `json:"server"`
		Demo struct {
			Enabled         bool `json:"enabled"`
			IntervalSeconds int  `json:"interval_seconds"`
		} `json:"demo"`
		Station struct {
			Latitude     float64 `json:"latitude"`
			Longitude    float64 `json:"longitude"`
			Altitude     float64 `json:"altitude"`
			MinElevation float64 `json:"min_elevation"`
			UseGPSD      bool    `json:"use_gpsd"`
			GPSDHost     string  `json:"gpsd_host"`
		} `json:"station"`
		SDR struct {
			DeviceIndex   int     `json:"device_index"`
			Gain          float64 `json:"gain"`
			PPMCorrection int     `json:"ppm_correction"`
			SampleRate    int     `json:"sample_rate"`
		} `json:"sdr"`
		Predict struct {
			TLEURL          string `json:"tle_url"`
			TLERefreshHours int    `json:"tle_refresh_hours"`
			LookaheadHours  int    `json:"lookahead_hours"`
		} `json:"predict"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println(header("  DAEMON CONFIGURATION"))
	fmt.Println(colorize(dim, "  "+strings.Repeat("─", 50)))

	section := func(name string) {
		fmt.Printf("\n  %s\n", colorize(bold, "["+name+"]"))
	}
	field := func(key string, val any) {
		fmt.Printf("    %-20s %v\n", colorize(dim, key+":"), val)
	}

	section("data")
	field("root", cfg.Data.Root)
	field("archive", cfg.Data.Archive)

	section("logging")
	field("level", cfg.Logging.Level)

	section("server")
	field("bind", cfg.Server.Bind)

	section("demo")
	field("enabled", cfg.Demo.Enabled)
	field("interval_seconds", cfg.Demo.IntervalSeconds)

	section("station")
	field("latitude", cfg.Station.Latitude)
	field("longitude", cfg.Station.Longitude)
	field("altitude", cfg.Station.Altitude)
	field("min_elevation", cfg.Station.MinElevation)
	field("use_gpsd", cfg.Station.UseGPSD)
	field("gpsd_host", cfg.Station.GPSDHost)

	section("sdr")
	field("device_index", cfg.SDR.DeviceIndex)
	field("gain", cfg.SDR.Gain)
	field("ppm_correction", cfg.SDR.PPMCorrection)
	field("sample_rate", cfg.SDR.SampleRate)

	section("predict")
	field("tle_url", cfg.Predict.TLEURL)
	field("tle_refresh_hours", cfg.Predict.TLERefreshHours)
	field("lookahead_hours", cfg.Predict.LookaheadHours)

	fmt.Println()

	return nil
}
