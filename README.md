# Ephemeris Engine

The backend for the VenSat satellite receiver platform. Ephemeris Engine handles automated NOAA APT satellite pass scheduling, SDR capture, and real-time event streaming over HTTP and WebSocket.

Part of the VenSat suite:

- **Ephemeris Engine** (this repo) — Go backend daemon and CLI
- **Observatory** — Svelte web dashboard frontend
- **Apsis** — Custom OCI Fedora CoreOS deployment image

## Features

- Automated NOAA satellite pass prediction via SGP4
- SDR capture through rtl_fm with WAV recording
- Real-time WebSocket event streaming
- REST API for status and control
- Demo mode for hardware-free testing
- TLE caching with four-tier fallback (disk, network, stale cache, embedded)
- Optional GPSD integration for dynamic ground station location

## Building

```sh
go build ./cmd/ephemerisd   # daemon
go build ./cmd/ephctl       # control CLI
```

## Running

```sh
# Start the daemon
ephemerisd -c /etc/ephemeris/ephemeris.toml

# Query status
ephctl status

# Stream live events
ephctl --host http://192.168.8.1:8080 watch
```

## Configuration

See [configs/example.toml](configs/example.toml) for all available options.

## License

Apache License 2.0
