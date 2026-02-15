package app

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/large-farva/ephemeris-engine/internal/capture"
	"github.com/large-farva/ephemeris-engine/internal/predict"
	"github.com/large-farva/ephemeris-engine/internal/scheduler"
)

func (a *App) handleVersion(w http.ResponseWriter, _ *http.Request) {
	resp := map[string]any{
		"version":    Version,
		"go_version": GoVersion,
		"built_at":   BuiltAt,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleSatellites(w http.ResponseWriter, _ *http.Request) {
	type satJSON struct {
		Name    string `json:"name"`
		NoradID int    `json:"norad_id"`
		FreqHz  int    `json:"freq_hz"`
	}
	sats := make([]satJSON, len(capture.Satellites))
	for i, s := range capture.Satellites {
		sats[i] = satJSON{Name: s.Name, NoradID: s.NoradID, FreqHz: s.Freq}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"satellites": sats})
}

func (a *App) handleConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.cfg)
}

func (a *App) handlePasses(w http.ResponseWriter, r *http.Request) {
	predictor := predict.NewPredictor(a.wsHub, a.cfg, a.log)
	passes, err := predictor.ComputePasses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Apply query param filters.
	satFilter := r.URL.Query().Get("satellite")
	if satFilter != "" {
		upper := strings.ToUpper(satFilter)
		var filtered []predict.Pass
		for _, p := range passes {
			if strings.ToUpper(p.Satellite.Name) == upper {
				filtered = append(filtered, p)
			}
		}
		passes = filtered
	}

	countStr := r.URL.Query().Get("count")
	if countStr != "" {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 && n < len(passes) {
			passes = passes[:n]
		}
	}

	type passJSON struct {
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
	}

	result := make([]passJSON, len(passes))
	for i, p := range passes {
		result[i] = passJSON{
			Satellite:   p.Satellite.Name,
			NoradID:     p.Satellite.NoradID,
			FreqHz:      p.Satellite.Freq,
			AOS:         p.AOS.Format("2006-01-02T15:04:05Z07:00"),
			LOS:         p.LOS.Format("2006-01-02T15:04:05Z07:00"),
			MaxElev:     p.MaxElev,
			MaxElevTime: p.MaxElevTime.Format("2006-01-02T15:04:05Z07:00"),
			AOSAzimuth:  p.AOSAzimuth,
			LOSAzimuth:  p.LOSAzimuth,
			DurationS:   int(p.Duration.Seconds()),
		}
	}

	loc, _ := predictor.ResolveLocation()
	resp := map[string]any{
		"passes": result,
		"station": map[string]any{
			"lat": loc.Lat,
			"lon": loc.Lon,
			"alt": loc.Alt,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.scheduler == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "not available in demo mode",
		})
		return
	}

	var req struct {
		Satellite       string `json:"satellite"`
		NoradID         int    `json:"norad_id"`
		DurationSeconds int    `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Resolve the satellite.
	var sat *capture.Satellite
	if req.NoradID != 0 {
		sat = capture.SatelliteByNoradID(req.NoradID)
	} else if req.Satellite != "" {
		sat = capture.SatelliteByName(req.Satellite)
	}
	if sat == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "unknown satellite",
		})
		return
	}

	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 600
	}

	payload, _ := json.Marshal(map[string]any{
		"norad_id":         sat.NoradID,
		"duration_seconds": req.DurationSeconds,
	})

	reply := make(chan scheduler.CommandResult, 1)
	a.scheduler.Commands <- scheduler.Command{
		Type:    "trigger",
		Payload: payload,
		Reply:   reply,
	}

	result := <-reply
	w.Header().Set("Content-Type", "application/json")
	if !result.OK {
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (a *App) handleTLERefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.scheduler == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":    false,
			"error": "not available in demo mode",
		})
		return
	}

	reply := make(chan scheduler.CommandResult, 1)
	a.scheduler.Commands <- scheduler.Command{
		Type:  "tle_refresh",
		Reply: reply,
	}

	result := <-reply
	w.Header().Set("Content-Type", "application/json")
	if !result.OK {
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(result)
}
