// Package predict computes upcoming NOAA satellite passes for a ground
// station using SGP4 orbital propagation. It handles TLE fetching, station
// location resolution (static config or GPSD), and pass filtering by
// minimum elevation.
package predict

import (
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/capture"
	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/ws"
)

// Pass describes a single predicted overhead pass, from acquisition of
// signal (AOS) through loss of signal (LOS).
type Pass struct {
	Satellite   capture.Satellite
	AOS         time.Time
	LOS         time.Time
	MaxElev     float64
	MaxElevTime time.Time
	AOSAzimuth  float64
	LOSAzimuth  float64
	Duration    time.Duration
}

// Predictor resolves the ground station location, fetches current TLE data,
// and runs SGP4 propagation to find upcoming passes.
type Predictor struct {
	hub      *ws.Hub
	cfg      config.Config
	log      *log.Logger
	tleStore *TLEStore
}

// NewPredictor creates a predictor backed by a TLE store rooted in the
// configured data directory.
func NewPredictor(hub *ws.Hub, cfg config.Config, logger *log.Logger) *Predictor {
	return &Predictor{
		hub: hub,
		cfg: cfg,
		log: logger,
		tleStore: NewTLEStore(
			cfg.Predict.TLEURL,
			cfg.Data.Root,
			cfg.Predict.TLERefreshHours,
		),
	}
}

// ResolveLocation determines the ground station position. If use_gpsd is
// true, it tries gpsd first and falls back to the TOML config values.
func (p *Predictor) ResolveLocation() (Location, error) {
	if p.cfg.Station.UseGPSD {
		loc, err := LocationFromGPSD(p.cfg.Station.GPSDHost, 10*time.Second)
		if err != nil {
			p.log.Printf("predict: gpsd failed (%v), falling back to config", err)
		} else {
			p.broadcast(map[string]any{
				"type":    "log",
				"level":   "info",
				"message": fmt.Sprintf("location from gpsd: %.4f, %.4f, %.0fm", loc.Lat, loc.Lon, loc.Alt),
			})
			return loc, nil
		}
	}

	return Location{
		Lat: p.cfg.Station.Latitude,
		Lon: p.cfg.Station.Longitude,
		Alt: p.cfg.Station.Altitude,
	}, nil
}

// ComputePasses fetches TLEs, resolves the station location, and computes
// all upcoming passes within the lookahead window. Passes below min_elevation
// are filtered out. Results are sorted by AOS ascending.
func (p *Predictor) ComputePasses() ([]Pass, error) {
	loc, err := p.ResolveLocation()
	if err != nil {
		return nil, fmt.Errorf("resolve location: %w", err)
	}

	p.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("station: %.4f, %.4f, %.0fm", loc.Lat, loc.Lon, loc.Alt),
	})

	tles, err := p.tleStore.Fetch()
	if err != nil {
		return nil, fmt.Errorf("fetch TLEs: %w", err)
	}

	now := time.Now().UTC()
	end := now.Add(time.Duration(p.cfg.Predict.LookaheadHours) * time.Hour)

	var allPasses []Pass

	for _, sat := range capture.Satellites {
		tle, ok := tles[sat.NoradID]
		if !ok {
			p.log.Printf("predict: no TLE for %s (NORAD %d)", sat.Name, sat.NoradID)
			continue
		}

		rawPasses, err := tle.GeneratePasses(
			loc.Lat, loc.Lon, loc.Alt,
			now, end,
			1, // 1-second step for precision
		)
		if err != nil {
			p.log.Printf("predict: error computing passes for %s: %v", sat.Name, err)
			continue
		}

		for _, rp := range rawPasses {
			if rp.MaxElevation < p.cfg.Station.MinElevation {
				continue
			}
			allPasses = append(allPasses, Pass{
				Satellite:   sat,
				AOS:         rp.AOS,
				LOS:         rp.LOS,
				MaxElev:     rp.MaxElevation,
				MaxElevTime: rp.MaxElevationTime,
				AOSAzimuth:  rp.AOSAzimuth,
				LOSAzimuth:  rp.LOSAzimuth,
				Duration:    rp.Duration,
			})
		}
	}

	sort.Slice(allPasses, func(i, j int) bool {
		return allPasses[i].AOS.Before(allPasses[j].AOS)
	})

	p.broadcast(map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("found %d passes in next %dh", len(allPasses), p.cfg.Predict.LookaheadHours),
	})

	return allPasses, nil
}

// ForceRefreshTLEs fetches TLEs from the network regardless of cache age
// and returns the number of satellites updated.
func (p *Predictor) ForceRefreshTLEs() (int, error) {
	tles, err := p.tleStore.ForceRefresh()
	if err != nil {
		return 0, err
	}
	return len(tles), nil
}

func (p *Predictor) broadcast(v map[string]any) {
	v["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	v["component"] = "predict"
	p.hub.BroadcastJSON(v)
}
