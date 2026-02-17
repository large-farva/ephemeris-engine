// Package config handles loading, defaulting, and validation of the Ephemeris Engine
// TOML configuration file. Every section maps to a typed struct so the rest
// of the codebase gets strong typing without manual key lookups.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config is the top-level configuration, mirroring the TOML sections.
type Config struct {
	Data    DataConfig    `toml:"data"    json:"data"`
	Logging LoggingConfig `toml:"logging" json:"logging"`
	Server  ServerConfig  `toml:"server"  json:"server"`
	Demo    DemoConfig    `toml:"demo"    json:"demo"`
	Station StationConfig `toml:"station" json:"station"`
	SDR     SDRConfig     `toml:"sdr"     json:"sdr"`
	Predict PredictConfig `toml:"predict" json:"predict"`
}

type DataConfig struct {
	Root    string `toml:"root"    json:"root"`
	Archive string `toml:"archive" json:"archive"`
}

type LoggingConfig struct {
	Level string `toml:"level" json:"level"`
}

type ServerConfig struct {
	Bind string `toml:"bind" json:"bind"`
}

type DemoConfig struct {
	Enabled         bool `toml:"enabled"          json:"enabled"`
	IntervalSeconds int  `toml:"interval_seconds" json:"interval_seconds"`
}

type StationConfig struct {
	Latitude     float64 `toml:"latitude"      json:"latitude"`
	Longitude    float64 `toml:"longitude"     json:"longitude"`
	Altitude     float64 `toml:"altitude"      json:"altitude"`
	MinElevation float64 `toml:"min_elevation" json:"min_elevation"`
	UseGPSD      bool    `toml:"use_gpsd"      json:"use_gpsd"`
	GPSDHost     string  `toml:"gpsd_host"     json:"gpsd_host"`
}

type SDRConfig struct {
	DeviceIndex   int     `toml:"device_index"   json:"device_index"`
	Gain          float64 `toml:"gain"           json:"gain"`
	PPMCorrection int     `toml:"ppm_correction" json:"ppm_correction"`
	SampleRate    int     `toml:"sample_rate"    json:"sample_rate"`
}

type PredictConfig struct {
	TLEURL          string `toml:"tle_url"           json:"tle_url"`
	TLERefreshHours int    `toml:"tle_refresh_hours" json:"tle_refresh_hours"`
	LookaheadHours  int    `toml:"lookahead_hours"   json:"lookahead_hours"`
}

// DefaultConfigDir returns the XDG-compliant config directory for Ephemeris.
// It respects $XDG_CONFIG_HOME and falls back to ~/.config/ephemeris.
func DefaultConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ephemeris")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "ephemeris")
}

// DefaultDataDir returns the XDG-compliant data directory for Ephemeris.
// It respects $XDG_DATA_HOME and falls back to ~/.local/share/ephemeris.
func DefaultDataDir() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ephemeris")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "ephemeris")
}

// FindConfigFile searches for a config file in standard locations:
//  1. $EPHEMERIS_CONFIG environment variable
//  2. $XDG_CONFIG_HOME/ephemeris/config.toml
//  3. ~/.config/ephemeris/config.toml
//  4. configs/example.toml (bundled fallback)
//
// Returns the path to the first file found, or empty string if none exist.
// An empty return means the caller should use Default() directly.
func FindConfigFile() string {
	if env := os.Getenv("EPHEMERIS_CONFIG"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}

	xdgPath := filepath.Join(DefaultConfigDir(), "config.toml")
	if _, err := os.Stat(xdgPath); err == nil {
		return xdgPath
	}

	legacyPath := "/etc/ephemeris/ephemeris.toml"
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}

	if _, err := os.Stat("configs/example.toml"); err == nil {
		return "configs/example.toml"
	}

	return ""
}

// ProfileInfo describes a config profile discovered in the config directory.
type ProfileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
}

// ListProfiles scans a directory for .toml files and returns them as profiles.
func ListProfiles(configDir string) ([]ProfileInfo, error) {
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var profiles []ProfileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".toml")
		profiles = append(profiles, ProfileInfo{
			Name:    name,
			Path:    filepath.Join(configDir, e.Name()),
			ModTime: info.ModTime(),
		})
	}
	return profiles, nil
}

// Default returns a Config populated with sane defaults. Values here are
// used whenever the TOML file omits a field.
func Default() Config {
	dataDir := DefaultDataDir()
	return Config{
		Data: DataConfig{
			Root:    dataDir,
			Archive: filepath.Join(dataDir, "archive"),
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
			TLEURL:          "https://celestrak.org/NORAD/elements/gp.php?GROUP=noaa&FORMAT=tle",
			TLERefreshHours: 24,
			LookaheadHours:  24,
		},
	}
}

// Load reads the TOML file at path, layers it on top of the defaults, and
// validates the result. Data directories are created automatically if they
// don't exist.
func Load(path string) (Config, error) {
	cfg := Default()

	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := toml.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}

	// Expand ~ in path fields so users can write "~/.local/share/..." in TOML.
	cfg.Data.Root = expandHome(cfg.Data.Root)
	cfg.Data.Archive = expandHome(cfg.Data.Archive)

	if err := validate(cfg); err != nil {
		return cfg, err
	}

	return cfg, ensureDirs(cfg)
}

// EnsureDirectories creates the XDG config dir and data directories.
// Called by the daemon on startup regardless of whether a config file was found.
func EnsureDirectories(cfg Config) error {
	if err := os.MkdirAll(DefaultConfigDir(), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return ensureDirs(cfg)
}

func ensureDirs(cfg Config) error {
	if err := os.MkdirAll(cfg.Data.Root, 0o755); err != nil {
		return fmt.Errorf("create data root: %w", err)
	}
	if err := os.MkdirAll(cfg.Data.Archive, 0o755); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	return nil
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
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
