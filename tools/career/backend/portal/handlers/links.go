package portalhandlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

type portalLinkRequest struct {
	PortalID string `json:"portalId"`
	URL      string `json:"url"`
	Title    string `json:"title"`
}

type portalLinkUpdateRequest struct {
	ID                string  `json:"id"`
	URL               *string `json:"url,omitempty"`
	Title             *string `json:"title,omitempty"`
	CrawlInstructions *string `json:"crawlInstructions,omitempty"`
	// JobDetailCrawlInstructions (step XX) — same presence-gated shape
	// as CrawlInstructions above, for the separate job-detail-page
	// instruction document. See
	// plan/ai/tools/career/step-XX-job-detail-crawl-instructions.md.
	JobDetailCrawlInstructions *string `json:"jobDetailCrawlInstructions,omitempty"`
	// InstructionsAIError (step 33) — nil: don't touch. Present with
	// "": clear any previously recorded failure (a fresh attempt
	// succeeded). Present non-empty: record this as the most recent
	// failure. Same presence-gated shape CrawlInstructions above uses.
	InstructionsAIError *string `json:"instructionsAiError,omitempty"`
	// InstructionsAIConversationID (step 60) — nil: don't touch.
	// Present with "": clear (the run just finished, one way or
	// another). Present non-empty: record the hidden conversation that
	// just started generating/updating this link's own crawl
	// instructions. Same presence-gated shape InstructionsAIError above
	// uses.
	InstructionsAIConversationID *string `json:"instructionsAiConversationId,omitempty"`
	// JobDetailInstructionsAIError/JobDetailInstructionsAIConversationID
	// (step XX) — same presence-gated shape as InstructionsAIError/
	// InstructionsAIConversationID above, for the SEPARATE job-detail
	// instructions document's own AI generation tracking instead.
	JobDetailInstructionsAIError          *string `json:"jobDetailInstructionsAiError,omitempty"`
	JobDetailInstructionsAIConversationID *string `json:"jobDetailInstructionsAiConversationId,omitempty"`
	// ListingCrawlRequested/JobDetailInstructionsAIRequested (step 67)
	// — nil: don't touch. Present: set to that exact value. The
	// frontend only ever sends these as false, to clear a flag a
	// Domain Events listener (events.go) set to true — see
	// plan/ai/tools/career/step-67-frontend-flag-consumption.md.
	ListingCrawlRequested            *bool `json:"listingCrawlRequested,omitempty"`
	JobDetailInstructionsAIRequested *bool `json:"jobDetailInstructionsAiRequested,omitempty"`
	// InstructionsAIErrorDismissedAt/JobDetailInstructionsAIErrorDismissedAt
	// (step 71) — nil: don't touch. Present with "": clear. Present
	// non-empty: record when that document's own error banner was
	// dismissed — the frontend always sends the dismissed error's own
	// errorAt value here, never "now". See
	// plan/ai/tools/career/step-71-dismiss-tracking-data-model.md.
	InstructionsAIErrorDismissedAt          *string `json:"instructionsAiErrorDismissedAt,omitempty"`
	JobDetailInstructionsAIErrorDismissedAt *string `json:"jobDetailInstructionsAiErrorDismissedAt,omitempty"`
	// JobDetailCrawlRequested (step 76) — nil: don't touch. Present: set
	// to that exact value. Same shape as ListingCrawlRequested/
	// JobDetailInstructionsAIRequested above — the frontend only ever
	// sends this as false, to clear a flag events.go's own
	// JobsCrawledEventHandler set to true. See
	// plan/ai/tools/career/step-76-job-detail-crawl-requested-backend.md.
	JobDetailCrawlRequested *bool `json:"jobDetailCrawlRequested,omitempty"`
	// AutoDiscoveryEnabled (step 84 follow-up) — nil: don't touch.
	// Present: set to that exact value, gating whether this specific
	// link participates in the auto-discovery manager's own sweeps.
	// See portal.portalLink's own AutoDiscoveryEnabled doc comment.
	AutoDiscoveryEnabled *bool `json:"autoDiscoveryEnabled,omitempty"`
	// AutoDiscoveryIntervalMinutes/ClearAutoDiscoveryInterval (step 85)
	// set or clear this link's own auto-discovery schedule override.
	// nil is genuinely ambiguous for this one field — unlike every
	// other optional field above, whose own "don't touch" value is
	// simply omitting a string/bool that already has an unambiguous
	// zero value, `null` here is a real, meaningful result ("use the
	// global interval") a bare *int can't tell apart from "the field
	// was omitted" once decoded (both come out nil) — so clearing it
	// needs its own explicit signal instead. AutoDiscoveryIntervalMinutes
	// present (non-nil) sets that value; ClearAutoDiscoveryInterval=true
	// clears it back to nil. If a caller somehow sends both, the SET
	// takes precedence — see the handler switch below.
	AutoDiscoveryIntervalMinutes *int `json:"autoDiscoveryIntervalMinutes,omitempty"`
	ClearAutoDiscoveryInterval   bool `json:"clearAutoDiscoveryInterval,omitempty"`
	// EmptyCrawlAcknowledged (step 90) — true marks this link's current
	// run of zero-job crawls as expected ("this page has no jobs"),
	// false clears it; nil leaves it alone. See
	// plan/ai/tools/career/step-90-zero-jobs-crawl-flag.md.
	EmptyCrawlAcknowledged *bool `json:"emptyCrawlAcknowledged,omitempty"`
	// JobsQuestionAnswer (step 91) — nil: don't touch. "yes" or "no"
	// answers the pending "does this page list jobs?" question the
	// generation agent asked; both clear it, "no" also acknowledges the
	// zero-job streak. Any other value is a 400; answering with nothing
	// pending is a 400.
	JobsQuestionAnswer *string `json:"jobsQuestionAnswer,omitempty"`
}

func PortalLinksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body portalLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		if body.PortalID == "" {
			http.Error(w, "portalId is required", http.StatusBadRequest)
			return
		}
		if body.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		id, err := portal.AddPortalLink(body.PortalID, body.URL, body.Title)
		if err != nil {
			portal.WritePortalLinkAwareError(w, "add portal link", err)
			return
		}
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "portal link added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalLinkUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if body.URL == nil && body.Title == nil && body.CrawlInstructions == nil && body.JobDetailCrawlInstructions == nil &&
			body.InstructionsAIError == nil && body.InstructionsAIConversationID == nil &&
			body.JobDetailInstructionsAIError == nil && body.JobDetailInstructionsAIConversationID == nil &&
			body.ListingCrawlRequested == nil && body.JobDetailInstructionsAIRequested == nil &&
			body.InstructionsAIErrorDismissedAt == nil && body.JobDetailInstructionsAIErrorDismissedAt == nil &&
			body.JobDetailCrawlRequested == nil && body.AutoDiscoveryEnabled == nil &&
			body.AutoDiscoveryIntervalMinutes == nil && !body.ClearAutoDiscoveryInterval && body.EmptyCrawlAcknowledged == nil && body.JobsQuestionAnswer == nil {
			http.Error(w, "url, title, crawlInstructions, jobDetailCrawlInstructions, instructionsAiError, instructionsAiConversationId, jobDetailInstructionsAiError, jobDetailInstructionsAiConversationId, listingCrawlRequested, jobDetailInstructionsAiRequested, instructionsAiErrorDismissedAt, jobDetailInstructionsAiErrorDismissedAt, jobDetailCrawlRequested, autoDiscoveryEnabled, autoDiscoveryIntervalMinutes, clearAutoDiscoveryInterval, emptyCrawlAcknowledged, or jobsQuestionAnswer is required", http.StatusBadRequest)
			return
		}
		if body.URL != nil && *body.URL == "" {
			http.Error(w, "url cannot be empty", http.StatusBadRequest)
			return
		}
		if body.Title != nil && *body.Title == "" {
			http.Error(w, "title cannot be empty", http.StatusBadRequest)
			return
		}
		if body.URL != nil || body.Title != nil {
			if err := portal.UpdatePortalLink(body.ID, body.URL, body.Title); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link", err)
				return
			}
		}
		if body.CrawlInstructions != nil {
			if err := portal.UpdatePortalLinkCrawlInstructions(body.ID, *body.CrawlInstructions); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link crawl instructions", err)
				return
			}
		}
		if body.JobDetailCrawlInstructions != nil {
			if err := portal.UpdatePortalLinkJobDetailCrawlInstructions(body.ID, *body.JobDetailCrawlInstructions); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail crawl instructions", err)
				return
			}
		}
		if body.InstructionsAIError != nil {
			if err := portal.UpdatePortalLinkInstructionsAIStatus(body.ID, body.InstructionsAIError); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link instructions AI status", err)
				return
			}
		}
		if body.InstructionsAIConversationID != nil {
			if err := portal.UpdatePortalLinkInstructionsAIConversationID(body.ID, body.InstructionsAIConversationID); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link instructions AI conversation id", err)
				return
			}
		}
		if body.JobDetailInstructionsAIError != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIStatus(body.ID, body.JobDetailInstructionsAIError); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI status", err)
				return
			}
		}
		if body.JobDetailInstructionsAIConversationID != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIConversationID(body.ID, body.JobDetailInstructionsAIConversationID); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI conversation id", err)
				return
			}
		}
		if body.ListingCrawlRequested != nil {
			if err := portal.UpdatePortalLinkListingCrawlRequested(body.ID, *body.ListingCrawlRequested); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link listing crawl requested", err)
				return
			}
		}
		if body.JobDetailInstructionsAIRequested != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIRequested(body.ID, *body.JobDetailInstructionsAIRequested); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI requested", err)
				return
			}
		}
		if body.InstructionsAIErrorDismissedAt != nil {
			if err := portal.UpdatePortalLinkInstructionsAIErrorDismissedAt(body.ID, body.InstructionsAIErrorDismissedAt); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link instructions AI error dismissed at", err)
				return
			}
		}
		if body.JobDetailInstructionsAIErrorDismissedAt != nil {
			if err := portal.UpdatePortalLinkJobDetailInstructionsAIErrorDismissedAt(body.ID, body.JobDetailInstructionsAIErrorDismissedAt); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail instructions AI error dismissed at", err)
				return
			}
		}
		if body.JobDetailCrawlRequested != nil {
			if err := portal.UpdatePortalLinkJobDetailCrawlRequested(body.ID, *body.JobDetailCrawlRequested); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link job detail crawl requested", err)
				return
			}
		}
		if body.AutoDiscoveryEnabled != nil {
			if err := portal.UpdatePortalLinkAutoDiscoveryEnabled(body.ID, *body.AutoDiscoveryEnabled); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link auto-discovery enabled", err)
				return
			}
		}
		// AutoDiscoveryIntervalMinutes (a SET) takes precedence over
		// ClearAutoDiscoveryInterval if a caller somehow sends both —
		// see portalLinkUpdateRequest's own doc comment on these two
		// fields for why clearing this one needs an explicit signal
		// separate from "field omitted."
		if body.AutoDiscoveryIntervalMinutes != nil {
			if err := portal.UpdatePortalLinkAutoDiscoveryInterval(body.ID, body.AutoDiscoveryIntervalMinutes); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link auto-discovery interval", err)
				return
			}
		} else if body.ClearAutoDiscoveryInterval {
			if err := portal.UpdatePortalLinkAutoDiscoveryInterval(body.ID, nil); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link auto-discovery interval", err)
				return
			}
		}
		if body.EmptyCrawlAcknowledged != nil {
			if err := portal.SetEmptyCrawlAcknowledged(body.ID, *body.EmptyCrawlAcknowledged); err != nil {
				portal.WritePortalLinkAwareError(w, "update portal link empty-crawl acknowledgement", err)
				return
			}
		}
		if body.JobsQuestionAnswer != nil {
			if err := portal.AnswerJobsQuestion(body.ID, *body.JobsQuestionAnswer); err != nil {
				if errors.Is(err, portal.ErrInvalidJobsQuestionAnswer) || errors.Is(err, portal.ErrNoPendingJobsQuestion) {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				portal.WritePortalLinkAwareError(w, "answer portal link jobs question", err)
				return
			}
		}
		portals, err := portal.ListPortals()
		if err != nil {
			http.Error(w, "portal link updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := portal.RemovePortalLink(id); err != nil {
			http.Error(w, "failed to remove portal link: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- deterministic ("Crawl now") crawl support (step 27) ---
//
// Two new, separately registered routes rather than sub-paths of
// /portal-links — this tool's own router (main.go's plain
// http.HandleFunc calls) has no path-parameter matching, only exact
// registered paths, the same reason every other by-id operation in
// this file takes its id via ?id= or a JSON body field instead of a
// URL segment. See plan/ai/tools/career/step-27-ai-free-manual-crawl.md.
