package shared

import (
	"errors"
	"strings"
	"time"
)

// clearance.go (step 96) keeps the cf_clearance cookie a person earned by
// clearing a site's challenge, so agents and auto-discovery can reuse it until
// it expires — across restarts, which the in-memory copy into the headless
// browser never did. Only cf_clearance is kept (no login or session cookie);
// the value is as sensitive as a session cookie for that site, so it is never
// returned by any route or written to any log. It is bound by Cloudflare to the
// browser's user agent, so a row is only ever used by a browser reporting the
// same one. See plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.

// ClearanceCookieName is the only cookie ever kept.
const ClearanceCookieName = "cf_clearance"

// maxClearanceValueLen bounds a stored value (real ones are a few hundred bytes).
const maxClearanceValueLen = 4096

// Clearance is one kept cf_clearance cookie.
type Clearance struct {
	Domain    string // the cookie's own domain, without a leading dot
	Name      string
	Value     string
	Path      string
	Secure    bool
	HTTPOnly  bool
	SameSite  string
	ExpiresAt time.Time
	UserAgent string
	Source    string // "human" | "automatic"
}

var ErrNotAClearance = errors.New("only a cf_clearance cookie with a future expiry and a user agent can be kept")

// StoreClearance upserts c. Anything but a non-expired cf_clearance with a
// user agent is refused: a session cookie, or a cookie the headless browser
// could not use, has no business here.
func StoreClearance(c Clearance) error {
	c.Domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(c.Domain)), ".")
	if c.Name != ClearanceCookieName || c.Value == "" || len(c.Value) > maxClearanceValueLen ||
		c.Domain == "" || c.UserAgent == "" || !c.ExpiresAt.After(time.Now()) {
		return ErrNotAClearance
	}
	if c.Path == "" {
		c.Path = "/"
	}
	if c.Source != "human" && c.Source != "automatic" {
		c.Source = "human"
	}
	_, err := DB.Exec(
		`INSERT INTO domain_clearance (domain, name, value, path, secure, http_only, same_site, expires_at, user_agent, source, obtained_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(domain, name, path) DO UPDATE SET
		   value = excluded.value, secure = excluded.secure, http_only = excluded.http_only, same_site = excluded.same_site,
		   expires_at = excluded.expires_at, user_agent = excluded.user_agent, source = excluded.source, obtained_at = datetime('now')`,
		c.Domain, c.Name, c.Value, c.Path, boolToInt(c.Secure), boolToInt(c.HTTPOnly), c.SameSite,
		c.ExpiresAt.UTC().Format(time.RFC3339), c.UserAgent, c.Source,
	)
	return err
}

// deleteExpiredClearances removes every row whose expiry has passed.
func deleteExpiredClearances(now time.Time) error {
	_, err := DB.Exec(`DELETE FROM domain_clearance WHERE expires_at <= ?`, now.UTC().Format(time.RFC3339))
	return err
}

// ClearancesFor returns the unexpired clearances that apply to host: a cookie
// whose domain is host itself or a parent of it ("example.com" serves
// "www.example.com"). Expired rows are deleted on the way.
func ClearancesFor(host string, now time.Time) ([]Clearance, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || DB == nil {
		return nil, nil
	}
	if err := deleteExpiredClearances(now); err != nil {
		return nil, err
	}
	rows, err := DB.Query(`SELECT domain, name, value, path, secure, http_only, COALESCE(same_site, ''), expires_at, user_agent, source FROM domain_clearance`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Clearance
	for rows.Next() {
		var c Clearance
		var secure, httpOnly int
		var expires string
		if err := rows.Scan(&c.Domain, &c.Name, &c.Value, &c.Path, &secure, &httpOnly, &c.SameSite, &expires, &c.UserAgent, &c.Source); err != nil {
			return nil, err
		}
		if host != c.Domain && !strings.HasSuffix(host, "."+c.Domain) {
			continue
		}
		t, err := time.Parse(time.RFC3339, expires)
		if err != nil || !t.After(now) {
			continue
		}
		c.ExpiresAt, c.Secure, c.HTTPOnly = t, secure != 0, httpOnly != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteClearances forgets every clearance kept for domain (and its parents'
// cookies that apply to it are left alone: only exact-domain rows go).
func DeleteClearances(domain string) error {
	domain = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(domain)), ".")
	_, err := DB.Exec(`DELETE FROM domain_clearance WHERE domain = ?`, domain)
	return err
}

// ClearanceSummary is what the Domains page shows about a domain's kept
// clearance: when it expires and how it was earned. Never the value.
type ClearanceSummary struct {
	Domain    string
	ExpiresAt time.Time
	Source    string
}

// ClearanceSummaries lists, per domain, the kept clearance that lasts longest,
// including ones that have just expired (so the page can say "expired"). It
// deletes nothing.
func ClearanceSummaries() ([]ClearanceSummary, error) {
	rows, err := DB.Query(`SELECT domain, expires_at, source FROM domain_clearance ORDER BY domain, expires_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClearanceSummary
	seen := map[string]bool{}
	for rows.Next() {
		var c ClearanceSummary
		var expires string
		if err := rows.Scan(&c.Domain, &expires, &c.Source); err != nil {
			return nil, err
		}
		if seen[c.Domain] {
			continue
		}
		seen[c.Domain] = true
		t, err := time.Parse(time.RFC3339, expires)
		if err != nil {
			continue
		}
		c.ExpiresAt = t
		out = append(out, c)
	}
	return out, rows.Err()
}
