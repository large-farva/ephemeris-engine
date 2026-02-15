// Package config handles loading, defaulting, and validation of the Ephemeris Engine
// TOML configuration file. Every section maps to a typed struct so the rest
// of the codebase gets strong typing without manual key lookups.
package config

import (
	"errors"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level configuration, mirroring the TOML sections.
type Config struct {
	Data    DataConfig    `toml:"data"`
	Logging LoggingConfig `toml:"logging"`
	Server  ServerConfig  `toml:"server"`
	Demo    DemoConfig    `toml:"demo"`
	Station StationConfig `toml:"station"`
	SDR     SDRConfig     `toml:"sdr"`
	Predict PredictConfig `toml:"predict"`
}

type DataConfig struct {
	Root    string `toml:"root"`
	Archive string `toml:"archive"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
}

type ServerConfig struct {
	Bind string `toml:"bind"`
}

type DemoConfig struct {
	Enabled         bool `toml:"enabled"`
	IntervalSeconds int  `toml:"interval_seconds"`
}

type StationConfig struct {
	Latitude     float64 `toml:"latitude"`
	Longitude    float64 `toml:"longitude"`
	Altitude     float64 `toml:"altitude"`
	MinElevation float64 `toml:"min_elevation"`
	UseGPSD      bool    `toml:"use_gpsd"`
	GPSDHost     string  `toml:"gpsd_host"`
}

type SDRConfig struct {
	DeviceIndex   int     `toml:"device_index"`
	Gain          float64 `toml:"gain"`
	PPMCorrection int     `toml:"ppm_correction"`
	SampleRate    int     `toml:"sample_rate"`
}

type PredictConfig struct {
	TLEURL          string `toml:"tle_url"`
	TLERefreshHours int    `toml:"tle_refresh_hours"`
	LookaheadHours  int    `toml:"lookahead_hours"`
}

// Default returns a Config populated with sane defaults. Values here are
// used whenever the TOML file omits a field.
func Default() Config {
	return Config{
		Data: DataConfig{
			Root:    "/var/lib/ephemeris",
			Archive: "/var/lib/ephemeris/archive",
		},
		Logging: LoggingConfig{
			Level: "info",
		},
		Server: ServerConfig{
			Bind: "0.0.0.0:8080",
		},
		Demo: DemoConfig{
			Enabled:         true,
			IntervalSeconds: 1,
		},
		Station: StationConfig{
			Latitude:     0.0,
			Longitude:    0.0,
			Altitude:     0.0,
			MinElevation: 10,
			UseGPSD:      false,
			GPSDHost:     "localhost:2947",
		},
		SDR: SDRConfig{
			DeviceIndex:   0,
			Gain:          40.0,
			PPMCorrection: 0,
			SampleRate:    48000,
		},
		Predict: PredictConfig{
			TLEURL:          "https://celestrak.org/NORAD/elements/gp.php?GROUP=weather&FORMAT=tle",
			TLERefreshHours: 24,
			LookaheadHours:  24,
		},
	}
}

// Load reads the TOML file at path, layers it on top of the defaults, and
// validates the result. An error is returned if the file can't be read,
// parsed, or if any constraint is violated.
func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := toml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}

	if err := validate(cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func validate(cfg Config) error {
	if cfg.Data.Root == "" {
		return errors.New("data.root must not be empty")
	}
	if cfg.Data.Archive == "" {
		return errors.New("data.archive must not be empty")
	}
	if cfg.Demo.IntervalSeconds < 0 {
		return errors.New("demo.interval_seconds must be >= 0")
	}
	if cfg.SDR.SampleRate <= 0 {
		return errors.New("sdr.sample_rate must be > 0")
	}
	if cfg.Station.MinElevation < 0 || cfg.Station.MinElevation > 90 {
		return errors.New("station.min_elevation must be between 0 and 90")
	}
	if cfg.Predict.TLERefreshHours < 1 {
		return errors.New("predict.tle_refresh_hours must be >= 1")
	}
	if cfg.Predict.LookaheadHours < 1 {
		return errors.New("predict.lookahead_hours must be >= 1")
	}
	return nil
}
