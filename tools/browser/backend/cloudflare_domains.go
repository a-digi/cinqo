// cloudflare_domains.go implements a persistent, per-domain cache of
// "Cloudflare was seen here" — recorded the moment the shared headless
// session gives up on a Cloudflare challenge, so a future crawl
// against the same domain can skip straight to the headed fallback
// (step 29) instead of re-discovering the same failure every time via
// a slow, doomed headless attempt. See
// plan/ai/tools/browser/step-28-cloudflare-domain-cache.md.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// hostnameOf extracts and lowercases rawURL's own hostname — the
// cache key every function below uses. Returns an error for anything
// validateCrawlURL (crawl.go) would already have rejected upstream;
// callers here treat that as "can't determine a domain, don't touch
// the cache" rather than a hard failure.
func hostnameOf(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("no hostname in %q", rawURL)
	}
	return strings.ToLower(u.Hostname()), nil
}

// isDomainKnownCloudflare reports whether domain has ever been
// recorded as Cloudflare-protected.
func isDomainKnownCloudflare(domain string) (bool, error) {
	var exists int
	err := browserDB.QueryRow(`SELECT 1 FROM cloudflare_domains WHERE domain = ?`, domain).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// recordCloudflareDomain records domain the first time it's seen —
// idempotent (INSERT OR IGNORE, matching this same package's own
// browser_settings seeding convention in login_credentials.go): a
// later re-detection with a different reason does not overwrite the
// first one.
func recordCloudflareDomain(domain, reason string) error {
	_, err := browserDB.Exec(`INSERT OR IGNORE INTO cloudflare_domains (domain, reason) VALUES (?, ?)`, domain, reason)
	return err
}

// cloudflareDomainRecord is the read-only, human-facing view of one
// cloudflare_domains row (step 30) — lets a person see what step 29
// has been silently deciding on their behalf, since otherwise this
// cache changes crawl behavior per domain with zero visibility.
type cloudflareDomainRecord struct {
	Domain          string `json:"domain"`
	Reason          string `json:"reason"`
	FirstDetectedAt string `json:"firstDetectedAt"`
}

// listCloudflareDomains returns every recorded domain, most recently
// detected first.
func listCloudflareDomains() ([]cloudflareDomainRecord, error) {
	rows, err := browserDB.Query(`SELECT domain, reason, first_detected_at FROM cloudflare_domains ORDER BY first_detected_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]cloudflareDomainRecord, 0)
	for rows.Next() {
		var d cloudflareDomainRecord
		if err := rows.Scan(&d.Domain, &d.Reason, &d.FirstDetectedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// cloudflareDomainsHandler handles GET /cloudflare-domains — a
// read-only diagnostic list, same shape of route as crawl_logs.go's
// own crawlLogsHandler (a {"domains": [...]} envelope, matching that
// handler's own {"logs": [...]} convention).
func cloudflareDomainsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	domains, err := listCloudflareDomains()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list cloudflare domains: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"domains": domains})
}
