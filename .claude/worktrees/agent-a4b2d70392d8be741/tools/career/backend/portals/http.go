// http.go is the plain HTTP mirror of this package's own MCP tools
// (portals.go) — a human's own browser session and the AI's own MCP
// tool-calling loop are two different callers needing two different
// transports to reach the same underlying data. Every handler here
// calls the exact same functions portals.go already built — never
// independently reimplemented. Deliberately not MCP-exposed
// themselves; scope enforcement happens at the host's own reverse
// proxy via manifest.json's routes[], same as every other tool's own
// HTTP routes.
//
// Originally these handlers shared one flat http.go with every other
// domain's own; each now lives beside the business logic it calls, in
// that domain's own package.
package portals

import (
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/shared"
)

// writePortalAwareError maps errUnknownPortal to a 400 (a caller
// mistake) and everything else to a 500.
func writePortalAwareError(w http.ResponseWriter, action string, err error) {
	shared.WriteKnownError(w, err, errUnknownPortal, "unknown portal id", action)
}

// WritePortalLinkAwareError additionally maps errDuplicatePortalLink
// (the same url added twice to one portal) and errInvalidCrawlInstructions
// (step 19's own shape check) to a 400 — the latter's own error text
// already carries the specific, actionable reason (e.g. "pagination.
// maxPages is required"), so it's passed straight through rather than
// replaced with a generic message. Too many distinct sentinels to go
// through shared.WriteKnownError, which handles the single-sentinel
// case every other domain has.
//
// Exported because the crawl package's own "Crawl now" / "Crawl job
// details now" handlers report the exact same set of failures —
// BuildCrawlRequest/BuildJobDetailCrawlRequest are this package's own
// functions, so they return this package's own sentinels.
func WritePortalLinkAwareError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, errUnknownPortal):
		http.Error(w, "unknown portal id", http.StatusBadRequest)
	case errors.Is(err, ErrUnknownPortalLink):
		http.Error(w, "unknown portal link id", http.StatusBadRequest)
	case errors.Is(err, errDuplicatePortalLink):
		http.Error(w, "this url is already a link on this portal", http.StatusBadRequest)
	case errors.Is(err, errInvalidCrawlInstructions):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, errNoCrawlInstructions):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, errNoJobDetailCrawlInstructions):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

// --- /portals ---

type portalRequest struct {
	Name string `json:"name"`
}

type portalUpdateRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func PortalsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "failed to list portals: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodPost:
		var body portalRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		id, err := createPortal(body.Name)
		if err != nil {
			http.Error(w, "failed to create portal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal created but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || body.Name == "" {
			http.Error(w, "id and name are both required", http.StatusBadRequest)
			return
		}
		if err := updatePortal(body.ID, body.Name); err != nil {
			writePortalAwareError(w, "update portal", err)
			return
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := deletePortal(id); err != nil {
			http.Error(w, "failed to delete portal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- /portal-links ---

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
		id, err := addPortalLink(body.PortalID, body.URL, body.Title)
		if err != nil {
			WritePortalLinkAwareError(w, "add portal link", err)
			return
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal link added but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"id": id, "portals": portals})

	case http.MethodPut:
		var body portalLinkUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if body.URL == nil && body.Title == nil && body.CrawlInstructions == nil && body.JobDetailCrawlInstructions == nil &&
			body.InstructionsAIError == nil && body.InstructionsAIConversationID == nil &&
			body.JobDetailInstructionsAIError == nil && body.JobDetailInstructionsAIConversationID == nil {
			http.Error(w, "url, title, crawlInstructions, jobDetailCrawlInstructions, instructionsAiError, instructionsAiConversationId, jobDetailInstructionsAiError, or jobDetailInstructionsAiConversationId is required", http.StatusBadRequest)
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
			if err := updatePortalLink(body.ID, body.URL, body.Title); err != nil {
				WritePortalLinkAwareError(w, "update portal link", err)
				return
			}
		}
		if body.CrawlInstructions != nil {
			if err := updatePortalLinkCrawlInstructions(body.ID, *body.CrawlInstructions); err != nil {
				WritePortalLinkAwareError(w, "update portal link crawl instructions", err)
				return
			}
		}
		if body.JobDetailCrawlInstructions != nil {
			if err := updatePortalLinkJobDetailCrawlInstructions(body.ID, *body.JobDetailCrawlInstructions); err != nil {
				WritePortalLinkAwareError(w, "update portal link job detail crawl instructions", err)
				return
			}
		}
		if body.InstructionsAIError != nil {
			if err := updatePortalLinkInstructionsAIStatus(body.ID, body.InstructionsAIError); err != nil {
				WritePortalLinkAwareError(w, "update portal link instructions AI status", err)
				return
			}
		}
		if body.InstructionsAIConversationID != nil {
			if err := updatePortalLinkInstructionsAIConversationID(body.ID, body.InstructionsAIConversationID); err != nil {
				WritePortalLinkAwareError(w, "update portal link instructions AI conversation id", err)
				return
			}
		}
		if body.JobDetailInstructionsAIError != nil {
			if err := updatePortalLinkJobDetailInstructionsAIStatus(body.ID, body.JobDetailInstructionsAIError); err != nil {
				WritePortalLinkAwareError(w, "update portal link job detail instructions AI status", err)
				return
			}
		}
		if body.JobDetailInstructionsAIConversationID != nil {
			if err := updatePortalLinkJobDetailInstructionsAIConversationID(body.ID, body.JobDetailInstructionsAIConversationID); err != nil {
				WritePortalLinkAwareError(w, "update portal link job detail instructions AI conversation id", err)
				return
			}
		}
		portals, err := listPortals()
		if err != nil {
			http.Error(w, "portal link updated but failed to reload: "+err.Error(), http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(w, map[string]any{"portals": portals})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := removePortalLink(id); err != nil {
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
// Registered as its own route rather than a sub-path of /portal-links
// — this tool's own router (main.go's plain http.HandleFunc calls) has
// no path-parameter matching, only exact registered paths, the same
// reason every other by-id operation here takes its id via ?id= or a
// JSON body field instead of a URL segment. See
// plan/ai/tools/career/step-27-ai-free-manual-crawl.md.

// CrawlRequestHandler handles GET /portal-links/crawl-request?id=... —
// the frontend's first step in the deterministic "Crawl now" flow:
// turn the link's own stored crawl_instructions into the JSON shape
// browser's own POST /crawl-paginated (called directly by the
// frontend, via that tool's own proxy route) expects.
//
// The flow's LAST step (POST /portal-links/ingest-crawl-results) lives
// in the crawl package instead — ingesting results means saving jobs,
// which is the jobs package's own concern, and only crawl imports both.
func CrawlRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "id query parameter is required", http.StatusBadRequest)
		return
	}
	req, err := BuildCrawlRequest(id)
	if err != nil {
		WritePortalLinkAwareError(w, "build crawl request", err)
		return
	}
	shared.WriteJSON(w, req)
}
