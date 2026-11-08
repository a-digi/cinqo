// Package autodiscovery is the auto-discovery scheduled-crawling
// feature's own home — a cross-cutting concern over both portal and
// crawl (crawl can't cleanly import portal, per that package's own
// existing cycle-avoidance notes), so it lives as its own top-level
// package rather than being bolted onto either. See
// plan/ai/tools/career/step-84-auto-discovery-scheduled-crawling.md.
//
// token_cache.go solves this feature's own hardest problem: the
// scheduler manager (a later step) fires on a timer with no live HTTP
// request of its own, but the only way this backend can authenticate
// an outbound crawl call into browser's own tool proxy is with a real
// user's own access_token (see crawl_now.go's own CrawlNowHandler,
// which reads it straight off the initiating request's cookie).
// CaptureTokenMiddleware wraps every route this backend serves; every
// request that reaches it has ALREADY passed core's own proxy-layer
// scope/auth check to get here at all (this backend's own routes are
// never reachable directly — only via /api/v1/tools/career/proxy/**),
// so by the time any handler runs, its own access_token cookie (when
// present) is already a genuinely verified, real session credential —
// capturing it here is not a new trust boundary, just remembering the
// most recent one for reuse later when no live request exists to
// borrow one from.
package autodiscovery

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	tokenMu sync.Mutex
	// cachedExpiry's zero value means "no known expiry" (nothing
	// recorded yet, or the last recorded token's own exp claim
	// couldn't be decoded) — CachedToken treats that as NOT fresh,
	// never as "never expires."
	cachedToken  string
	cachedExpiry time.Time
)

// RecordSeenToken stashes token as the most recently seen live session
// credential. A token whose exp claim can't be parsed is recorded with
// a zero-value expiry, so CachedToken's own freshness check on it
// always reports "not fresh" rather than treating a malformed value as
// eternally valid.
func RecordSeenToken(token string) {
	if token == "" {
		return
	}
	exp, ok := decodeJWTExpiry(token)
	tokenMu.Lock()
	defer tokenMu.Unlock()
	cachedToken = token
	if ok {
		cachedExpiry = exp
	} else {
		cachedExpiry = time.Time{}
	}
}

// CachedToken returns the most recently seen access token and whether
// it's still fresh right now — false whenever nothing has ever been
// recorded, the last recorded token's own exp claim couldn't be
// decoded, or that claim has already passed. A "fresh" verdict here is
// only ever a freshness courtesy, never a trust decision: whatever this
// returns still goes through the exact same real verification any
// other caller's own access_token would when it's actually used — see
// this package's own step-84 plan doc, Security considerations.
func CachedToken() (token string, fresh bool) {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	if cachedToken == "" || cachedExpiry.IsZero() || time.Now().After(cachedExpiry) {
		return "", false
	}
	return cachedToken, true
}

// decodeJWTExpiry reads a JWT's own unverified "exp" claim — a plain
// base64url-decode-and-json-unmarshal of the token's middle segment,
// deliberately NOT signature verification: see this file's own top
// comment for why that's not this cache's job.
func decodeJWTExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0), true
}

// CaptureTokenMiddleware wraps this backend's entire HTTP mux (main.go
// wraps it once, at the http.ListenAndServe call site, rather than
// touching every individual route registration) — the one choke point
// every authenticated request already passes through. A request with
// no access_token cookie (the core-dispatched /events/* routes, or
// /healthz) simply records nothing; RecordSeenToken already no-ops on
// an empty string.
func CaptureTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("access_token"); err == nil {
			RecordSeenToken(c.Value)
		}
		next.ServeHTTP(w, r)
	})
}
