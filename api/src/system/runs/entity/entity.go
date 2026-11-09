// Package entity holds the records of the run recorder (plan 223, step 3): one row per run of the app, and one row per unexpected stop found.
package entity

// Heartbeat is what the app writes about itself every few seconds while it lives: the last thing it knew before it stopped, if it stops without
// saying goodbye.
type Heartbeat struct {
	At      string          `json:"at"` // RFC 3339, UTC
	Memory  HeartbeatMemory `json:"memory"`
	AppByte uint64          `json:"appBytes"` // the resident memory of the Cinqo program itself
	LocalAi HeartbeatAgent  `json:"localAi"`
}

type HeartbeatMemory struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	SwapUsedBytes  uint64 `json:"swapUsedBytes"`
	Pressure       string `json:"pressure"` // normal, warn, critical or unknown
}

// HeartbeatAgent is the local model program: its state ("" or "none" when it did not run), the model and the memory its process held.
type HeartbeatAgent struct {
	State string `json:"state"`
	Name  string `json:"name"`
	Bytes uint64 `json:"bytes"`
}

// Run is one run of the app. EndedAt is nil while it runs, or after it stopped without a trace until the next start finds it.
type Run struct {
	ID            string
	StartedAt     string
	LastSeenAt    string
	EndedAt       *string
	Clean         bool
	PID           int
	ProcessStart  string
	LastHeartbeat string // the last Heartbeat as JSON; "" before the first
}

// EvidenceItem is one fact beside a verdict.
type EvidenceItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Event is something that went wrong, with the app's own guess at why (Verdict) and the facts it is based on (Evidence).
type Event struct {
	ID             string
	At             string
	Kind           string
	Verdict        string
	Evidence       []EvidenceItem
	AcknowledgedAt *string
}

// The kinds of event.
const (
	KindAppUncleanExit = "app_unclean_exit" // a run that never got to say it was stopping
	KindLocalAiStopped = "local_ai_stopped" // the model program stopped on its own while the app lived
)
