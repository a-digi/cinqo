// events.go holds this tool's own Domain Events listener handlers —
// the receiving side of the event-driven crawl-instructions pipeline
// (plan/ai/tools/career/step-64-event-driven-crawl-instructions.md).
// Both handlers are reached only via core's own loopback-bound
// dispatcher (plan/ai/domain-events/step-04-core-to-tool-delivery.md),
// declared under manifest.json's own event_listeners[]. Neither
// handler performs the privileged action its own topic ultimately
// causes (crawling, starting an AI conversation) — a backend event
// handler has no live end-user session to act as, so both only ever
// set a durable request flag the frontend consumes on its own next
// load, under a real user's own live session. See
// plan/ai/tools/career/step-65-career-event-listeners.md.
package portal

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
)

// eventEnvelope mirrors domainevent.Event's own JSON shape
// (api/src/domainevent/event.go) — this backend is a separate Go
// module from cinqo's own core API and cannot import that package
// directly, so this is a small, deliberate local duplicate of just the
// shape these handlers need, matching the established "package main
// cannot be imported" convention already used elsewhere in this
// backend for cross-module duplication.
type eventEnvelope struct {
	ID         string          `json:"id"`
	Topic      string          `json:"topic"`
	SourceKind string          `json:"source_kind"`
	SourceID   string          `json:"source_id"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt string          `json:"occurred_at"`
}

// portalLinkEventPayload is the payload shape both
// career.portal_link.* topics use — see step-66's own publishers.
type portalLinkEventPayload struct {
	PortalLinkID string `json:"portal_link_id"`
}

// decodePortalLinkEvent reads the request body as an eventEnvelope and
// extracts its own portal_link_id, shared by both handlers below.
func decodePortalLinkEvent(r *http.Request) (portalLinkID string, ok bool) {
	var evt eventEnvelope
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		return "", false
	}
	var payload portalLinkEventPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.PortalLinkID == "" {
		return "", false
	}
	return payload.PortalLinkID, true
}

// ListingInstructionsReadyEventHandler handles
// "career.portal_link.listing_instructions_ready" — always sets
// listing_crawl_requested, unconditionally (a regenerated instructions
// document should also re-trigger a fresh crawl, the same way a human
// manually re-clicking "Generate with AI" then "Crawl now" today
// would).
func ListingInstructionsReadyEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	portalLinkID, ok := decodePortalLinkEvent(r)
	if !ok {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	if _, err := db.JobsDB.Exec(
		`UPDATE portal_links SET listing_crawl_requested = 1, updated_at = datetime('now') WHERE id = ?`,
		portalLinkID,
	); err != nil {
		http.Error(w, "failed to record request", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// JobsCrawledEventHandler handles "career.portal_link.jobs_crawled" —
// the literal "check if there are instructions for this portal to
// crawl job description; if there are none, ..." from the request:
// only flags job_detail_instructions_ai_requested when this link
// doesn't already have job_detail_crawl_instructions set. It also
// unconditionally flags job_detail_crawl_requested, independently of
// that check — whether it's actually safe to start that crawl yet
// (job-detail instructions might not exist) is deliberately not this
// handler's own concern; the frontend's own consuming effect is what
// waits for readiness. See
// plan/ai/tools/career/step-76-job-detail-crawl-requested-backend.md.
func JobsCrawledEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	portalLinkID, ok := decodePortalLinkEvent(r)
	if !ok {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}

	var jobDetailInstructions sql.NullString
	err := db.JobsDB.QueryRow(
		`SELECT job_detail_crawl_instructions FROM portal_links WHERE id = ?`,
		portalLinkID,
	).Scan(&jobDetailInstructions)
	if err != nil {
		if err == sql.ErrNoRows {
			// The link no longer exists — nothing to flag.
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Error(w, "failed to check job detail instructions", http.StatusInternalServerError)
		return
	}

	if !jobDetailInstructions.Valid || jobDetailInstructions.String == "" {
		if _, err := db.JobsDB.Exec(
			`UPDATE portal_links SET job_detail_instructions_ai_requested = 1, updated_at = datetime('now') WHERE id = ?`,
			portalLinkID,
		); err != nil {
			http.Error(w, "failed to record request", http.StatusInternalServerError)
			return
		}
	}

	if _, err := db.JobsDB.Exec(
		`UPDATE portal_links SET job_detail_crawl_requested = 1, updated_at = datetime('now') WHERE id = ?`,
		portalLinkID,
	); err != nil {
		http.Error(w, "failed to record job detail crawl request", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
