// Package telemetry defines the typed event structs that flow over the
// WebSocket connection between ephemerisd and its clients. These types serve as
// documentation for the event schema; most internal code still broadcasts
// events as map[string]any for flexibility during early development.
package telemetry

import "time"

// EventType identifies the kind of WebSocket event.
type EventType string

const (
	EventHeartbeat EventType = "heartbeat"
	EventState     EventType = "state"
	EventProgress  EventType = "progress"
	EventLog       EventType = "log"
)

// Event is the base envelope shared by every event type.
type Event struct {
	Type EventType `json:"type"`
	TS   string    `json:"ts"`
}

// NowTS returns the current UTC time as an RFC 3339 nano string, matching the
// timestamp format used across all events.
func NowTS() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// Heartbeat is sent periodically so clients can detect connectivity and
// monitor daemon uptime.
type Heartbeat struct {
	Event
	State         string `json:"state"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// StateTransition is emitted whenever the daemon moves between operating
// states (e.g. IDLE -> WAITING_FOR_PASS).
type StateTransition struct {
	Event
	From string `json:"from"`
	To   string `json:"to"`
}

// Progress reports incremental completion of a long-running phase like
// recording or decoding.
type Progress struct {
	Event
	Stage   string  `json:"stage"`
	Percent float64 `json:"percent"`
	Detail  string  `json:"detail"`
}

// LogLine carries a human-readable log message at a severity level.
type LogLine struct {
	Event
	Level   string `json:"level"`
	Message string `json:"message"`
}
