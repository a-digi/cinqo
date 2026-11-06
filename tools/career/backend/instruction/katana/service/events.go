package service

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/domainevent"
)

// The Katana engine's own event (plan 192, step 7). Saving a rule publishes it; its handler raises the generic "crawl this link" request that
// an open Platforms page consumes (the crawl is engine-aware since plan 190); the crawl's existing "jobs crawled" event then carries the chain on
// to the job details. The topic is Katana's own: Rod's "listing_instructions_ready" is never reused, so either engine can be removed alone.

// TopicRuleSaved is published when the AI has saved a job list rule for a link.
const TopicRuleSaved = "career.portal_link.katana_rule_saved"

// RuleSavedPath is the path suffix manifest.json lists for the handler (event_listeners).
const RuleSavedPath = "events/katana-rule-saved"

// publish is a var so tests can see what was published.
var publish = domainevent.Publish

type ruleSavedPayload struct {
	PortalLinkID string `json:"portal_link_id"`
}

func publishRuleSaved(linkID string) {
	publish(TopicRuleSaved, map[string]string{"portal_link_id": linkID})
}

// RuleSavedEventHandler handles POST /events/katana-rule-saved: it only raises the durable request flag (a backend event handler has no
// live session to crawl with); the open Platforms page crawls under the person's own session. A link that needs a login is never crawled
// on its own (manual_only.go), so no flag is raised for it.
func RuleSavedEventHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var evt db.EventEnvelope
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&evt); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	var payload ruleSavedPayload
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.PortalLinkID == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	if _, err := db.JobsDB.Exec(
		`UPDATE portal_links SET listing_crawl_requested = 1, updated_at = datetime('now') WHERE id = ? AND requires_login = 0`,
		payload.PortalLinkID,
	); err != nil {
		http.Error(w, "failed to record request", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}
