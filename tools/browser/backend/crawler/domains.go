// domains.go (step 96) is the Domains page's backend: every site the tool knows
// asked for a Cloudflare check, whether a person's clearance for it is kept, and
// whether a window is waiting for someone right now; plus "Clear now" (open the
// window on purpose, before a crawl needs it) and "Forget". Nothing here solves
// a check: Clear now opens the same visible window the human wait uses and a
// person does it. Never returns a cookie value. See
// plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.
package crawler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"browser-tool-backend/shared"
)

type domainClearanceView struct {
	// State is "valid" (kept and unexpired), "expired" (kept, past its expiry)
	// or "none".
	State     string `json:"state"`
	ExpiresAt string `json:"expiresAt,omitempty"`
	Source    string `json:"source,omitempty"`
}

type domainRequestView struct {
	// State is "waiting" (a window is open for a person), "cleared", "gave_up"
	// (the wait ended without a clearance) or "failed".
	State      string `json:"state"`
	RequestID  string `json:"requestId,omitempty"` // only while waiting: what /crawl-cancel takes
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

type domainView struct {
	Domain          string              `json:"domain"`
	Reason          string              `json:"reason,omitempty"`
	FirstDetectedAt string              `json:"firstDetectedAt,omitempty"`
	Clearance       domainClearanceView `json:"clearance"`
	Request         *domainRequestView  `json:"request,omitempty"`
}

// normalizeDomain accepts a bare host or a pasted URL and returns the lower-case
// hostname, or "" when there is none.
func normalizeDomain(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	if !strings.Contains(input, "://") {
		input = "https://" + input
	}
	host, err := hostnameOf(input)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(host, "www.")
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// listDomains merges the three sources: the domains a challenge was ever seen
// on, the domains with a kept clearance, and the domains a person was asked
// about in this process.
func listDomains(now time.Time) ([]domainView, error) {
	known, err := listCloudflareDomains()
	if err != nil {
		return nil, err
	}
	kept, err := shared.ClearanceSummaries()
	if err != nil {
		return nil, err
	}
	byDomain := map[string]*domainView{}
	order := []string{}
	get := func(d string) *domainView {
		if v, ok := byDomain[d]; ok {
			return v
		}
		v := &domainView{Domain: d, Clearance: domainClearanceView{State: "none"}}
		byDomain[d] = v
		order = append(order, d)
		return v
	}
	for _, k := range known {
		v := get(k.Domain)
		v.Reason, v.FirstDetectedAt = k.Reason, k.FirstDetectedAt
	}
	for _, c := range kept {
		v := get(c.Domain)
		state := "valid"
		if !c.ExpiresAt.After(now) {
			state = "expired"
		}
		v.Clearance = domainClearanceView{State: state, ExpiresAt: rfc3339(c.ExpiresAt), Source: c.Source}
	}
	humanAssistMu.Lock()
	for d, r := range humanRequests {
		v := get(d)
		rv := &domainRequestView{State: string(r.state), StartedAt: rfc3339(r.startedAt), FinishedAt: rfc3339(r.finishedAt), Detail: r.detail}
		if r.state == humanWaiting {
			rv.RequestID = r.requestID
		}
		v.Request = rv
	}
	humanAssistMu.Unlock()

	// Waiting first, then the rest by name: what needs a person comes to the top.
	sort.SliceStable(order, func(i, j int) bool {
		wi := byDomain[order[i]].Request != nil && byDomain[order[i]].Request.State == string(humanWaiting)
		wj := byDomain[order[j]].Request != nil && byDomain[order[j]].Request.State == string(humanWaiting)
		if wi != wj {
			return wi
		}
		return order[i] < order[j]
	})
	out := make([]domainView, 0, len(order))
	for _, d := range order {
		out = append(out, *byDomain[d])
	}
	return out, nil
}

// DomainsHandler handles GET /domains (the list) and DELETE /domains?domain=…
// (Forget).
func DomainsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		domains, err := listDomains(time.Now())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list domains: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"domains": domains})

	case http.MethodDelete:
		domain := normalizeDomain(r.URL.Query().Get("domain"))
		if domain == "" {
			http.Error(w, "domain is required", http.StatusBadRequest)
			return
		}
		if err := forgetDomain(domain); err != nil {
			http.Error(w, fmt.Sprintf("failed to forget domain: %v", err), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// forgetDomain removes the kept clearance and the known-challenge record, and
// stops a window waiting for this domain.
func forgetDomain(domain string) error {
	humanAssistMu.Lock()
	r := humanRequests[domain]
	delete(humanRequests, domain)
	humanAssistMu.Unlock()
	if r != nil && r.state == humanWaiting {
		cancelCrawl(r.requestID)
	}
	if err := shared.DeleteClearances(domain); err != nil {
		return err
	}
	_, err := shared.DB.Exec(`DELETE FROM cloudflare_domains WHERE domain = ?`, domain)
	return err
}

// DomainsClearHandler handles POST /domains/clear {"domain": "..."}: open the
// visible window for a person to clear that site now.
func DomainsClearHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	domain := normalizeDomain(body.Domain)
	if domain == "" {
		http.Error(w, "a domain such as example.com is required", http.StatusBadRequest)
		return
	}
	rawURL := "https://" + domain + "/"
	// The same guard every crawl URL gets: no private or internal address.
	if err := validateCrawlURL(rawURL); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	requestID, already := startHumanRequestNow(domain, rawURL)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"domain": domain, "requestId": requestID, "state": "waiting", "alreadyWaiting": already})
}
