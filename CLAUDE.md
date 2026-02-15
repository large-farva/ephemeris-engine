# CLAUDE.md — Repo Memory (Ephemeris Engine)

## Prime directive
- Make small, safe diffs. Do not refactor unrelated code.
- Prefer correctness and debuggability over cleverness.
- If you must assume something, state assumptions before coding.

## What this repo is
Ephemeris Engine is a Go backend daemon + CLI for the VenSat satellite receiver platform.
It exposes an HTTP+JSON API plus WebSocket events for realtime status/telemetry.
No gRPC/protobuf.

- Go module: github.com/large-farva/ephemeris-engine
- Go version: 1.26
- CLI: ephctl
- Hardware focus: NOAA APT via RTL-SDR (e.g., NOAA-15/18/19)

## Architecture notes
- API endpoints include: /healthz, /api/status, /api/version, /api/satellites, /api/config, /api/passes, /api/trigger, /api/tle-refresh, and /ws.
- Daemon lifecycle/state machine: BOOTING → IDLE → WAITING_FOR_PASS → RECORDING → DECODING → IDLE.
- Active control uses a scheduler command channel (do not block the scheduler loop).

## Repo layout conventions
- cmd/            — binaries (ephctl, daemon entrypoints)
- internal/       — private packages (app, scheduler, predict, capture, ws, telemetry, etc.)
- configs/        — config TOML files (example + user-defined configs)

## Configuration philosophy (important)
- Example config file is: configs/example.toml
- Users should be able to drop additional TOML files into configs/.
- Long-term goal: register configs by TOML name for selection in CLI and eventually web UI (not fully implemented yet).

## CLI conventions (important)
- Use pflag.
- IMPORTANT: Keep `pflag.CommandLine.SetInterspersed(false)` before parsing globals,
  otherwise subcommand flags like `trigger --norad-id ...` break.

Command naming preference:
- Use hyphenated subcommands (e.g., `tle-refresh`, not `tle refresh`).

## Build / test commands
Backend / CLI:
- gofmt: gofmt -w .
- build: go build ./...
- test: go test ./...
- vet:  go vet ./...

## Known issues / landmines
- /api/passes has previously returned 500; client now prints server error body.
  Likely causes: missing data root directory or invalid station coordinates (0,0,0).
  When debugging: ensure data dirs exist and station coordinates are set.

## When implementing changes
1) Provide a short plan + list files to touch.
2) Implement with minimal diffs.
3) Provide exact run/test commands.
4) Put follow-up improvements in a separate section (do not sneak them into the diff).
