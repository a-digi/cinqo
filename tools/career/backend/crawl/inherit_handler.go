package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"career-tool-backend/db"
	"career-tool-backend/domainevent"
	"career-tool-backend/portal"
)

// inheritInFlight holds the links a trial is running for, so a second request for the
// same link (a double click, the automatic call after "Add link" meeting the one at
// "Generate") is refused instead of crawling the same page twice.
var (
	inheritMu       sync.Mutex
	inheritInFlight = map[string]bool{}
)

func startInherit(linkID string) bool {
	inheritMu.Lock()
	defer inheritMu.Unlock()
	if inheritInFlight[linkID] {
		return false
	}
	inheritInFlight[linkID] = true
	return true
}

func endInherit(linkID string) {
	inheritMu.Lock()
	defer inheritMu.Unlock()
	delete(inheritInFlight, linkID)
}

type inheritFrom struct {
	LinkID string `json:"linkId"`
	Title  string `json:"title"`
}

type inheritResponse struct {
	Inherited bool               `json:"inherited"`
	Listing   bool               `json:"listing"`
	JobDetail bool               `json:"jobDetail"`
	From      *inheritFrom       `json:"from,omitempty"`
	Tried     []CandidateOutcome `json:"tried"`
	Portals   any                `json:"portals"`
}

// InheritInstructionsHandler handles POST /portal-links/inherit-instructions (step 103):
// try the portal's other links' instructions on this (new, still uninstructed) link and, if
// they work, save them on it. The usual crawl-instructions pipeline then carries on by
// itself, because a saved listing publishes the same event an AI save does. Nothing is
// saved when no candidate works. The browser is called with the caller's own session
// token, as "Crawl now" does. See
// plan/ai/tools/career/step-103-inherit-portal-instructions-for-new-links.md.
func InheritInstructionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		http.Error(w, `body must be {"id": "…"}`, http.StatusBadRequest)
		return
	}
	accessCookie, err := r.Cookie("access_token")
	if err != nil || accessCookie.Value == "" {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}

	// Only a link with no listing instructions yet may inherit; nothing is overwritten.
	existing, err := portal.GetPortalLinkCrawlInstructions(body.ID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link", err)
		return
	}
	if existing != "" {
		http.Error(w, "this link already has crawl instructions", http.StatusBadRequest)
		return
	}

	if !startInherit(body.ID) {
		http.Error(w, "a trial is already running for this link", http.StatusConflict)
		return
	}
	defer endInherit(body.ID)

	result, err := TrialInherit(r.Context(), body.ID, accessCookie.Value)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "trying the portal's instructions took too long", http.StatusGatewayTimeout)
		return
	case errors.Is(err, context.Canceled):
		return // the caller went away; nothing was saved
	case err != nil:
		portal.WritePortalLinkAwareError(w, "try the portal's instructions", err)
		return
	}

	resp := inheritResponse{Tried: result.Tried}
	if resp.Tried == nil {
		resp.Tried = []CandidateOutcome{}
	}
	if result.Winner != nil && result.CopyListing {
		detail := ""
		if result.CopyJobDetail {
			detail = result.Winner.JobDetail
		}
		listingSaved, detailSaved, err := portal.SaveInheritedInstructions(body.ID, result.Winner.Listing, detail)
		if err != nil {
			portal.WritePortalLinkAwareError(w, "save the instructions", err)
			return
		}
		if !listingSaved {
			http.Error(w, "this link got crawl instructions while the trial ran, so nothing was changed", http.StatusConflict)
			return
		}
		resp.Inherited, resp.Listing, resp.JobDetail = true, true, detailSaved
		resp.From = &inheritFrom{LinkID: result.Winner.LinkID, Title: result.Winner.Title}
		// The same event an AI save publishes: the listing crawl, and then the job-detail
		// instructions or crawl, follow from it exactly as they do for an AI-written link.
		domainevent.Publish("career.portal_link.listing_instructions_ready", map[string]string{"portal_link_id": body.ID})
	}

	portals, err := portal.ListPortals()
	if err != nil {
		http.Error(w, "done, but the list could not be reloaded: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp.Portals = portals
	db.WriteJSON(w, resp)
}
