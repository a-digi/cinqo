// linkcrawl_rules.go (plan 185, step 4) — GET/PUT /portal-links/link-crawl/rules:
// read and save a link's crawl method and its link-crawler rule.
//
// Saving enforces here, with the person's own session, what the screen only
// hides: a rule can be stored, and the method "links" chosen, only while the
// Browser tool's engine is Katana (requireKatanaSelected; 409 otherwise). The
// data rules (own host, no login link, validation, size) are the portal
// package's. Going back to "instructions" and clearing a rule never need
// Katana, so a person is never stuck with a method they cannot leave.
package crawl

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"career-tool-backend/portal"
	"career-tool-backend/portal/linkrule"
)

// Codes of the 409 answers (same vocabulary as the availability route).
const (
	codeKatanaNotSelected = linkCrawlReasonNotKatana
	codeEngineUnknown     = linkCrawlReasonEngineRead
)

type linkRulesBody struct {
	PortalLinkID string `json:"portalLinkId"`
	CrawlMethod  string `json:"crawlMethod"`
	// Rules: absent = keep the stored rule, null = clear it, an object = store it.
	Rules json.RawMessage `json:"rules"`
}

func writeLinkRulesJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// LinkCrawlRulesHandler handles GET and PUT /portal-links/link-crawl/rules.
func LinkCrawlRulesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		id := strings.TrimSpace(r.URL.Query().Get("portalLinkId"))
		if id == "" {
			http.Error(w, "portalLinkId is required", http.StatusBadRequest)
			return
		}
		settings, err := portal.GetLinkCrawlSettings(id)
		if err != nil {
			portal.WritePortalLinkAwareError(w, "read the link crawler settings", err)
			return
		}
		writeLinkRulesJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		putLinkRules(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func putLinkRules(w http.ResponseWriter, r *http.Request) {
	var body linkRulesBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*portal.MaxLinkRulesJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.PortalLinkID) == "" {
		http.Error(w, "portalLinkId and crawlMethod are required (and only portalLinkId, crawlMethod and rules are accepted)", http.StatusBadRequest)
		return
	}
	if body.CrawlMethod != portal.CrawlMethodInstructions && body.CrawlMethod != portal.CrawlMethodLinks {
		http.Error(w, portal.ErrInvalidCrawlMethod.Error(), http.StatusBadRequest)
		return
	}
	var rules *linkrule.Rule
	clearRules := false
	if len(body.Rules) > 0 {
		if string(bytes.TrimSpace(body.Rules)) == "null" {
			clearRules = true
		} else {
			var rule linkrule.Rule
			rd := json.NewDecoder(bytes.NewReader(body.Rules))
			rd.DisallowUnknownFields()
			if err := rd.Decode(&rule); err != nil {
				http.Error(w, "rules must be {host, include, exclude}", http.StatusBadRequest)
				return
			}
			rules = &rule
		}
	}

	// The engine gate: only for what needs Katana (storing a rule, choosing the method "links").
	if rules != nil || body.CrawlMethod == portal.CrawlMethodLinks {
		accessCookie, err := r.Cookie("access_token")
		if err != nil {
			http.Error(w, "no active session cookie found", http.StatusUnauthorized)
			return
		}
		coreURL, err := coreAPIURL()
		if err != nil {
			http.Error(w, "core API address is not configured", http.StatusInternalServerError)
			return
		}
		if err := requireKatanaSelected(r.Context(), coreURL, accessCookie.Value); err != nil {
			code := codeEngineUnknown
			if errors.Is(err, errKatanaNotSelected) {
				code = codeKatanaNotSelected
			}
			writeLinkRulesJSON(w, http.StatusConflict, map[string]string{"code": code, "message": err.Error()})
			return
		}
	}

	if err := portal.SaveLinkCrawlSettings(body.PortalLinkID, body.CrawlMethod, rules, clearRules); err != nil {
		portal.WritePortalLinkAwareError(w, "save the link crawler settings", err)
		return
	}
	settings, err := portal.GetLinkCrawlSettings(body.PortalLinkID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read the link crawler settings", err)
		return
	}
	writeLinkRulesJSON(w, http.StatusOK, settings)
}
