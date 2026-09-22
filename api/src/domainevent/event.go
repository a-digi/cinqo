// Package domainevent is cinqo's own Domain Events engine — a single,
// in-process broker native core code publishes to and subscribes from.
// This step (plan/ai/domain-events/step-01-core-event-bus.md) is
// deliberately the smallest correct primitive: synchronous dispatch,
// no tool-process integration at all. Later steps
// (plan/ai/domain-events/domain-events.md) make dispatch async and
// panic-safe, then extend the same Event envelope and matching rules
// to tool-process listeners/publishers, so a native Go listener and a
// tool's own HTTP listener are ultimately handled the same way.
package domainevent

import (
	"encoding/json"
	"time"
)

// Event is the one envelope shape every listener receives, whether it's
// a native Go closure (this step) or, from a later step on, a tool's
// own HTTP handler — deliberately identical for both, since that
// symmetry is the whole point of the Domain Events engine.
type Event struct {
	// ID is minted fresh by Publish for every event — never
	// caller-supplied.
	ID string `json:"id"`
	// Topic is a dot-namespaced string. "cinqo." is reserved for
	// core-native events (e.g. this step's own "cinqo.tool.installed");
	// a later step reserves "<tool-slug>." for that tool's own
	// published events.
	Topic string `json:"topic"`
	// SourceKind is "core" for every event in this step — "tool"
	// becomes possible only once a later step lets a tool publish.
	SourceKind string `json:"source_kind"`
	// SourceID is empty for a core-native event; a later step sets it
	// to the publishing tool's own slug.
	SourceID string `json:"source_id"`
	// Payload is the caller-supplied value from Publish, already
	// JSON-encoded. json.RawMessage (not []byte) deliberately, so a
	// later step that JSON-encodes the whole Event to deliver it over
	// HTTP embeds this as a real JSON value, not a base64 string.
	Payload json.RawMessage `json:"payload"`
	// OccurredAt is set by Publish, always UTC.
	OccurredAt time.Time `json:"occurred_at"`
}
