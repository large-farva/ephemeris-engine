package predict

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/akhenakh/sgp4"
	"github.com/large-farva/ephemeris-engine/internal/capture"
)

//go:embed noaa_tle.txt
var embeddedTLE string

const tleCacheFile = "weather_tle.txt"

// TLEStore fetches and caches Two-Line Element sets for the NOAA satellites.
// It uses a tiered fallback strategy: fresh disk cache, network fetch,
// stale disk cache, and finally embedded data baked into the binary.
type TLEStore struct {
	url      string
	dataRoot string
	maxAge   time.Duration
}

// NewTLEStore returns a store that fetches TLEs from the given URL and
// caches them under dataRoot.
func NewTLEStore(tleURL, dataRoot string, refreshHours int) *TLEStore {
	return &TLEStore{
		url:      tleURL,
		dataRoot: dataRoot,
		maxAge:   time.Duration(refreshHours) * time.Hour,
	}
}

// Fetch returns TLEs for the hardcoded NOAA satellites, keyed by NORAD ID.
// It tries the disk cache first, then the network, then stale cache, and
// finally falls back to embedded TLE data compiled into the binary.
func (s *TLEStore) Fetch() (map[int]*sgp4.TLE, error) {
	cachePath := filepath.Join(s.dataRoot, tleCacheFile)

	raw, err := s.loadOrFetch(cachePath)
	if err != nil {
		return nil, err
	}

	return s.parseForNOAA(raw)
}

// loadOrFetch walks the four-tier fallback chain to get raw TLE text:
// fresh cache -> network -> stale cache -> embedded data.
func (s *TLEStore) loadOrFetch(cachePath string) (string, error) {
	// Tier 1: fresh disk cache
	info, err := os.Stat(cachePath)
	if err == nil && time.Since(info.ModTime()) < s.maxAge {
		if b, readErr := os.ReadFile(cachePath); readErr == nil && len(b) > 0 {
			return string(b), nil
		}
	}

	// Tier 2: network fetch
	body, fetchErr := s.fetchFromNetwork()
	if fetchErr == nil {
		// Cache write failure is non-fatal; we already have the data in memory.
		_ = s.writeCache(cachePath, body)
		return body, nil
	}

	// Tier 3: stale disk cache
	if b, readErr := os.ReadFile(cachePath); readErr == nil && len(b) > 0 {
		return string(b), nil
	}

	// Tier 4: embedded fallback baked into the binary
	if embeddedTLE != "" {
		return embeddedTLE, nil
	}

	return "", fmt.Errorf("all TLE sources exhausted: %w", fetchErr)
}

// fetchFromNetwork downloads the TLE data set from CelesTrak (or whatever
// URL is configured). Times out after 30 seconds.
func (s *TLEStore) fetchFromNetwork() (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(s.url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("TLE fetch returned HTTP %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// writeCache atomically writes data to cachePath via a temp file and rename
// so readers never see a half-written file.
func (s *TLEStore) writeCache(cachePath, data string) error {
	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, "tle-*.tmp")
	if err != nil {
		return err
	}

	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}

	return os.Rename(tmp.Name(), cachePath)
}

// parseForNOAA extracts TLEs for the hardcoded NOAA satellites from a bulk
// TLE text dump. Input is expected in standard 3-line format (name, line 1,
// line 2) as served by CelesTrak.
func (s *TLEStore) parseForNOAA(raw string) (map[int]*sgp4.TLE, error) {
	wanted := make(map[int]bool, len(capture.Satellites))
	for _, sat := range capture.Satellites {
		wanted[sat.NoradID] = true
	}

	result := make(map[int]*sgp4.TLE)
	lines := strings.Split(strings.TrimSpace(raw), "\n")

	for i := 0; i+2 < len(lines); i += 3 {
		group := strings.TrimSpace(lines[i]) + "\n" +
			strings.TrimSpace(lines[i+1]) + "\n" +
			strings.TrimSpace(lines[i+2])

		tle, err := sgp4.ParseTLE(group)
		if err != nil {
			continue
		}

		if wanted[tle.SatelliteNumber] {
			result[tle.SatelliteNumber] = tle
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no matching NOAA TLEs found in %d lines of input", len(lines))
	}

	return result, nil
}
