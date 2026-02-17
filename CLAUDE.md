# CLAUDE.md — Repo Memory (Ephemeris Engine)

## Prime Directive
- Make small, safe diffs. Do not refactor unrelated code.
- Prefer correctness and debuggability over cleverness.
- State assumptions before coding if requirements are ambiguous.
- Do not introduce breaking API changes without explicit instruction.

## What This Project Is
Ephemeris Engine is a Go backend daemon + CLI for the VenSat satellite receiver platform.

It provides:

- Full headless operation (CLI-first; no web UI required)
- HTTP + JSON API
- WebSocket event stream for realtime telemetry
- NOAA APT satellite support via RTL-SDR

No gRPC. No protobuf.

Module: `github.com/large-farva/ephemeris-engine`
Go version: 1.26
CLI: `ephctl`
Daemon: `ephemerisd`

## Design Philosophy
This system must be fully usable without a web UI.

`ephctl` is feature-complete and must remain equivalent to all API capabilities.

The CLI is not a thin wrapper — it is a primary interface.

## Architecture Overview

### API Style
- REST + JSON
- WebSocket at `/ws`
- No RPC frameworks

### Scheduler Model
- Single goroutine event loop
- Command channel for active control
- Must never block scheduler loop
- Uses `Command` / `CommandResult` types
- Pause state via `atomic.Bool`
- Capture cancellation via `context.WithCancel`

### State Machine
BOOTING -> IDLE -> WAITING_FOR_PASS -> RECORDING -> DECODING -> IDLE

## XDG Directory Model (Important)
Follow the XDG Base Directory spec.

### Config Location
Auto-discovery order:
1. `$EPHEMERIS_CONFIG` (must exit)
2. `$XDG_CONFIG_HOME/ephemeris/config.toml`
3. `~/.config/ephemeris/config.toml` (legacy fallback)
4. `configs/example.toml` (fallback for dev/demo)

If nothing is found, daemon usess `config.Default()` and logs instructions.

Users are expected to place configs in:
`~/.config/ephemeris/*.toml`

### Data Location
- `$XDG_DATA_HOME/ephemeris`
- `~/.local/share/ephemeris`

### Directory Handling Standard
- Directories are created silently and automatically (standard daemon behavior).
- No interactie prompting.
- No `ephctl init`.

### TLE Source (Critical Operational Detail)

Default TLE source is **CelesTrak GROUP=noaa**.

Reason: CelesTrak `GROUP=weather` no longer contains NOAA-15/18/19 APT satellites, which breaks `passes` / `next-pass` with "no matching NOAA TLEs found...".

NOAA APT NORAD IDs:
- NOAA-15: 25338
- NOAA-18: 27667
- NOAA-19: 36674

## CLI conventions (Critical)
- Use `pflag`.
- Always keep:
  - `pflag.CommandLine.SetInterspersed(false)`
  Otherwise subcommand flags break (e.g., `trigger --norad-id ...`)

### Command Naming
- Hyphenated commands and subcommands only
- Example: `tle-refresh`, not `tle refresh`
- Example: `tle-info`, not `tle_info`

## Current CLI Surface (Feature Complete)

Query:
- status
- health
- version
- satellites
- config
- config-list
- passes
- next-pass
- captures
- tle-info
- stats
- logs
- system-info

Control:
- trigger
- tle-refresh
- pause
- resume
- skip
- cancel
- reload

Live:
- watch

If adding new API capabilities, they must be accessible via CLI.

## Output Formatting Standard (Tables)

Avoid fixed-width `printf("%-Ns")` tables. ANSI coloring breaks alignment.

Use the dynamic table helper in `internal/ctl/format.go`:
- buffers rows
- computes max column widths
- supports right-aligned columns
- renders headers/separators cleanly

Commands known to use tables:
- satellites
- passes
- captures
- config-list
- stats (by-satellite section)


## Config Profiles / Switching

Profiles live in:
- `~/.config/ephemeris/<name>.toml`

`config-list` displays available profiles.

Switching profiles is done via reload:
- `ephctl reload --profile <name>`

Implementation:
- `/api/reload` accepts optional JSON body: `{"profile":"palmdale"}`
- Server resolves to config dir + `<profile>.toml`, validates existence, then reloads and updates `configPath`.

## Repo Layout

```
cmd/            — CLI + daemon entrypoints
internal/app/   — daemon core + HTTP handlers
internal/scheduler/
internal/predict/
internal/capture/
internal/config/
internal/ctl/   — CLI commands (and formatting helpers)
configs/        — example TOML only
```

## Build & Verification

Always run before claiming completion:
- `gofmt -w .`
- `go vet ./...`
- `go build ./...`
- `go test ./...`

## Known Landmines

- `pflag.SetInterspersed(false)` is mandatory.
- Scheduler must not block on control commands.
- `PassInfo` lives in scheduler package — do not duplicate it.
- Ensure data directories exist before prediction runs (auto-created via EnsureDirectories/Load).

## When Implementing Changes

1. Provide a short plan (bullets).
2. List files that will change.
3. Implement minimal diff.
4. Provide exact run/test commands.
5. List optional improvements separately.
