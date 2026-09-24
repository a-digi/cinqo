package db

import "encoding/json"

// EventEnvelope mirrors domainevent.Event's own JSON shape
// (api/src/domainevent/event.go) — this backend is a separate Go
// module from cinqo's own core API and cannot import that package
// directly, so this is a small, deliberate local duplicate of just the
// shape a Domain Events listener handler needs to decode a pushed
// event. Shared here (rather than declared once per listener package)
// because portal and jobs are two packages within this SAME module —
// unlike the cross-module boundary that justifies this type existing
// at all, there's no reason for each of THEM to also duplicate it a
// second time from each other. See
// plan/ai/tools/career/step-83-job-created-company-linking.md and
// plan/ai/tools/career/step-65-career-event-listeners.md (this type's
// own original, portal-local introduction).
type EventEnvelope struct {
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	SourceKind string          `json:"source_kind"`
	SourceID   string          `json:"source_id"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt string          `json:"occurred_at"`
}
