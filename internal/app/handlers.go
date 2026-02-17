package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/large-farva/ephemeris-engine/internal/capture"
	"github.com/large-farva/ephemeris-engine/internal/config"
	"github.com/large-farva/ephemeris-engine/internal/predict"
	"github.com/large-farva/ephemeris-engine/internal/scheduler"
)

// ---------------------------------------------------------------------------
// Core handlers
// ---------------------------------------------------------------------------

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// If the client asks for JSON, return component-level health checks.
	if r.Header.Get("Accept") == "application/json" {
		a.handleHealthDetailed(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (a *App) handleStatus(w http.ResponseWriter, _ *http.Request) {
	cfg := a.getConfig()

	resp := map[string]any{
		"name":           "ephemeris-engine",
		"state":          a.state.Load().(string),
		"uptime_seconds": int64(time.Since(a.startedAt).Seconds()),
		"data_root":      cfg.Data.Root,
		"archive_dir":    cfg.Data.Archive,
		"demo_enabled":   cfg.Demo.Enabled,
	}

	if cfg.Demo.Enabled {
		resp["mode"] = "demo"
	} else {
		resp["mode"] = "live"
	}

	// Include current pass info if available.
	if pi, ok := a.currentPass.Load().(*scheduler.PassInfo); ok && pi != nil {
		resp["current_pass"] = pi
	}

	// Disk usage for data root.
	if du := diskUsage(cfg.Data.Root); du != nil {
		resp["disk"] = du
	}

	// Scheduler paused state.
	if a.scheduler != nil {
		resp["paused"] = a.scheduler.IsPaused()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

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
	_ = json.NewEncoder(w).Encode(a.getConfig())
}

func (a *App) handlePasses(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfig()
	predictor := predict.NewPredictor(a.wsHub, cfg, a.log)
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

	result := passesToJSON(passes)

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
		jsonError(w, "not available in demo mode", http.StatusConflict)
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
		jsonError(w, "unknown satellite", http.StatusBadRequest)
		return
	}

	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 600
	}

	payload, _ := json.Marshal(map[string]any{
		"norad_id":         sat.NoradID,
		"duration_seconds": req.DurationSeconds,
	})

	result := a.sendSchedulerCommand("trigger", payload)
	writeCommandResult(w, result)
}

func (a *App) handleTLERefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if a.scheduler == nil {
		jsonError(w, "not available in demo mode", http.StatusConflict)
		return
	}

	result := a.sendSchedulerCommand("tle_refresh", nil)
	writeCommandResult(w, result)
}

// ---------------------------------------------------------------------------
// Phase 2: Captures + Config Profiles
// ---------------------------------------------------------------------------

func (a *App) handleCaptures(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfig()

	if r.Method == http.MethodDelete {
		name := r.URL.Query().Get("name")
		if name == "" {
			jsonError(w, "name parameter required", http.StatusBadRequest)
			return
		}
		// Prevent path traversal.
		if strings.Contains(name, "/") || strings.Contains(name, "..") {
			jsonError(w, "invalid filename", http.StatusBadRequest)
			return
		}
		path := filepath.Join(cfg.Data.Root, name)
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				jsonError(w, "file not found", http.StatusNotFound)
			} else {
				jsonError(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "deleted " + name})
		return
	}

	// GET: list captures.
	matches, _ := filepath.Glob(filepath.Join(cfg.Data.Root, "*.wav"))

	type captureInfo struct {
		Filename  string `json:"filename"`
		Satellite string `json:"satellite"`
		Timestamp string `json:"timestamp"`
		Size      int64  `json:"size"`
	}

	captures := make([]captureInfo, 0, len(matches))
	for _, m := range matches {
		base := filepath.Base(m)
		info, err := os.Stat(m)
		if err != nil {
			continue
		}

		// Parse satellite name and timestamp from "NOAA-19_20260215T143022Z.wav".
		sat, ts := parseCaptureName(base)
		captures = append(captures, captureInfo{
			Filename:  base,
			Satellite: sat,
			Timestamp: ts,
			Size:      info.Size(),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"captures": captures})
}

func (a *App) handleConfigProfiles(w http.ResponseWriter, _ *http.Request) {
	profiles, err := config.ListProfiles(config.DefaultConfigDir())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if profiles == nil {
		profiles = []config.ProfileInfo{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"config_dir": config.DefaultConfigDir(),
		"profiles":   profiles,
	})
}

// ---------------------------------------------------------------------------
// Phase 3: TLE Info + Next Pass + System Info
// ---------------------------------------------------------------------------

func (a *App) handleTLEInfo(w http.ResponseWriter, _ *http.Request) {
	cfg := a.getConfig()
	store := predict.NewTLEStore(cfg.Predict.TLEURL, cfg.Data.Root, cfg.Predict.TLERefreshHours)
	info := store.CacheInfo()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

func (a *App) handleNextPass(w http.ResponseWriter, r *http.Request) {
	cfg := a.getConfig()
	predictor := predict.NewPredictor(a.wsHub, cfg, a.log)
	passes, err := predictor.ComputePasses()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	// Find the first future pass.
	now := time.Now().UTC()
	var next *predict.Pass
	for i := range passes {
		if passes[i].AOS.After(now) {
			next = &passes[i]
			break
		}
	}

	resp := map[string]any{"pass": nil}
	if next != nil {
		pj := passesToJSON([]predict.Pass{*next})
		resp["pass"] = pj[0]
		resp["countdown_s"] = int(time.Until(next.AOS).Seconds())
	}

	loc, _ := predictor.ResolveLocation()
	resp["station"] = map[string]any{
		"lat": loc.Lat,
		"lon": loc.Lon,
		"alt": loc.Alt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleSystem(w http.ResponseWriter, _ *http.Request) {
	cfg := a.getConfig()

	resp := map[string]any{
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"data_root":  cfg.Data.Root,
		"config_dir": config.DefaultConfigDir(),
	}

	// Check for rtl_fm.
	if _, err := exec.LookPath("rtl_fm"); err == nil {
		resp["sdr_available"] = true
	} else {
		resp["sdr_available"] = false
	}

	// Disk usage.
	if du := diskUsage(cfg.Data.Root); du != nil {
		resp["disk"] = du
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// ---------------------------------------------------------------------------
// Phase 4: Logs + Stats + Enhanced Health
// ---------------------------------------------------------------------------

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	a.logBufMu.Lock()
	entries := make([]logEntry, len(a.logBuf))
	copy(entries, a.logBuf)
	a.logBufMu.Unlock()

	// Apply filters.
	levelFilter := r.URL.Query().Get("level")
	if levelFilter != "" {
		var filtered []logEntry
		for _, e := range entries {
			if e.Level == levelFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	limitStr := r.URL.Query().Get("limit")
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n < len(entries) {
			entries = entries[len(entries)-n:]
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"logs": entries})
}

func (a *App) handleStats(w http.ResponseWriter, _ *http.Request) {
	a.captureStats.mu.Lock()
	resp := map[string]any{
		"total_captures":        a.captureStats.TotalCaptures,
		"total_bytes":           a.captureStats.TotalBytes,
		"captures_by_satellite": a.captureStats.CapturesBySat,
		"last_capture_at":       a.captureStats.LastCaptureAt,
		"uptime_seconds":        int64(time.Since(a.startedAt).Seconds()),
	}
	a.captureStats.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *App) handleHealthDetailed(w http.ResponseWriter, _ *http.Request) {
	cfg := a.getConfig()

	checks := map[string]any{}
	allOK := true

	// Check data directory.
	tmpPath := filepath.Join(cfg.Data.Root, ".healthcheck")
	if err := os.WriteFile(tmpPath, []byte("ok"), 0o644); err != nil {
		checks["data_dir"] = map[string]any{"ok": false, "error": err.Error()}
		allOK = false
	} else {
		os.Remove(tmpPath)
		checks["data_dir"] = map[string]any{"ok": true, "path": cfg.Data.Root}
	}

	// Check TLE cache.
	tlePath := filepath.Join(cfg.Data.Root, "weather_tle.txt")
	if info, err := os.Stat(tlePath); err != nil {
		checks["tle_cache"] = map[string]any{"ok": false, "error": "cache file not found"}
		allOK = false
	} else {
		age := time.Since(info.ModTime())
		maxAge := time.Duration(cfg.Predict.TLERefreshHours) * time.Hour
		fresh := age < maxAge
		if !fresh {
			allOK = false
		}
		checks["tle_cache"] = map[string]any{
			"ok":    fresh,
			"age_s": int(age.Seconds()),
			"fresh": fresh,
		}
	}

	// Check SDR (only in live mode).
	if !cfg.Demo.Enabled {
		if _, err := exec.LookPath("rtl_fm"); err != nil {
			checks["sdr"] = map[string]any{"ok": false, "error": "rtl_fm not found in PATH"}
			allOK = false
		} else {
			checks["sdr"] = map[string]any{"ok": true}
		}
	}

	// Config file readable.
	if a.configPath != "" {
		if _, err := os.Stat(a.configPath); err != nil {
			checks["config_file"] = map[string]any{"ok": false, "error": err.Error()}
			allOK = false
		} else {
			checks["config_file"] = map[string]any{"ok": true, "path": a.configPath}
		}
	}

	status := http.StatusOK
	if !allOK {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"healthy": allOK,
		"checks":  checks,
	})
}

// ---------------------------------------------------------------------------
// Phase 5: Scheduler Controls + Reload
// ---------------------------------------------------------------------------

func (a *App) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.scheduler == nil {
		jsonError(w, "not available in demo mode", http.StatusConflict)
		return
	}
	result := a.sendSchedulerCommand("pause", nil)
	writeCommandResult(w, result)
}

func (a *App) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.scheduler == nil {
		jsonError(w, "not available in demo mode", http.StatusConflict)
		return
	}
	result := a.sendSchedulerCommand("resume", nil)
	writeCommandResult(w, result)
}

func (a *App) handleSkip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.scheduler == nil {
		jsonError(w, "not available in demo mode", http.StatusConflict)
		return
	}
	result := a.sendSchedulerCommand("skip", nil)
	writeCommandResult(w, result)
}

func (a *App) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.scheduler == nil {
		jsonError(w, "not available in demo mode", http.StatusConflict)
		return
	}
	result := a.sendSchedulerCommand("cancel", nil)
	writeCommandResult(w, result)
}

func (a *App) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Accept optional profile name in body: {"profile": "palmdale"}
	var body struct {
		Profile string `json:"profile"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	loadPath := a.configPath
	if body.Profile != "" {
		// Resolve profile name to a file in the config directory.
		candidate := filepath.Join(config.DefaultConfigDir(), body.Profile+".toml")
		if _, err := os.Stat(candidate); err != nil {
			jsonError(w, fmt.Sprintf("profile %q not found at %s", body.Profile, candidate), http.StatusNotFound)
			return
		}
		loadPath = candidate
	}

	if loadPath == "" {
		jsonError(w, "no config file path set", http.StatusInternalServerError)
		return
	}

	newCfg, err := config.Load(loadPath)
	if err != nil {
		jsonError(w, "config reload failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	a.cfgMu.Lock()
	a.cfg = newCfg
	a.configPath = loadPath
	a.cfgMu.Unlock()

	a.emit("ephemerisd", map[string]any{
		"type":    "log",
		"level":   "info",
		"message": fmt.Sprintf("config reloaded from %s", loadPath),
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "configuration reloaded from " + loadPath,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sendSchedulerCommand sends a command to the scheduler and waits for the reply.
func (a *App) sendSchedulerCommand(cmdType string, payload json.RawMessage) scheduler.CommandResult {
	reply := make(chan scheduler.CommandResult, 1)
	a.scheduler.Commands <- scheduler.Command{
		Type:    cmdType,
		Payload: payload,
		Reply:   reply,
	}
	return <-reply
}

// jsonError writes a JSON error response.
func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": msg,
	})
}

// writeCommandResult writes a scheduler.CommandResult as JSON.
func writeCommandResult(w http.ResponseWriter, result scheduler.CommandResult) {
	w.Header().Set("Content-Type", "application/json")
	if !result.OK {
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(result)
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

func passesToJSON(passes []predict.Pass) []passJSON {
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
	return result
}

// parseCaptureName extracts satellite and timestamp from "NOAA-19_20260215T143022Z.wav".
func parseCaptureName(filename string) (satellite, timestamp string) {
	name := strings.TrimSuffix(filename, ".wav")
	idx := strings.LastIndex(name, "_")
	if idx < 0 {
		return name, ""
	}
	return name[:idx], name[idx+1:]
}
