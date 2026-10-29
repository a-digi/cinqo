// access_outcome.go (step 96) classifies what a site did when a page was
// requested, and what the caller should do about it. Until now a challenge or
// block reached an agent as one error string ("Cloudflare challenge could not
// be cleared (cloudflare_challenge_unresolved: ...)"), with nothing saying
// whether retrying made sense, and a challenge from anything other than
// Cloudflare (or a rate limit) was not recognized at all: the interstitial came
// back as if it were the page, which an agent sees as "0 items".
//
// This file only DESCRIBES: it never solves, waits through, or evades anything.
// See plan/ai/tools/browser/step-96-cloudflare-outcomes-and-kept-clearance.md.
package crawler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Outcome kinds. A kind is what the site did, in terms an agent can act on.
const (
	outcomeClearedAutomatically = "cleared_automatically" // challenged, cleared by the browser itself
	outcomeInteractiveRequired  = "interactive_required"  // a challenge that needs a person
	outcomeBlocked              = "blocked"               // the site denies this visitor
	outcomeRateLimited          = "rate_limited"          // too many requests
	outcomeOtherChallenge       = "other_challenge"       // a bot-protection wall that is not Cloudflare
)

// Error codes for the outcomes that had no code before.
const (
	codeRateLimited        = "rate_limited"
	codeBotProtectionFound = "bot_protection_detected"
)

// accessEvidence is the wire-level evidence for one main-frame document.
type accessEvidence struct {
	DocSeen    bool
	Status     int64
	Mitigated  string // cf-mitigated, lower-cased
	Server     string
	CFRay      string
	RetryAfter string // raw header value
}

// accessVerdict is a FINAL classification: one that no wait changes. Empty
// Kind means "nothing final recognized" (a normal page, or a Cloudflare
// challenge that the existing tracker handles).
type accessVerdict struct {
	Kind       string
	Reason     string
	RetryAfter time.Duration
	Vendor     string
}

// cloudflareDenyCodes are Cloudflare error-page codes that mean the visitor is
// denied (firewall rule, IP/ASN/country ban, banned browser signature).
var cloudflareDenyCodes = map[string]bool{
	"1005": true, "1006": true, "1007": true, "1008": true, "1009": true,
	"1010": true, "1012": true, "1020": true,
}

// classifyAccess returns the final verdict for a page, from the wire evidence
// and the page probe, or the zero verdict. The caller's own expected content
// being visible always wins: a page that shows what was asked for is not a wall.
func classifyAccess(ev accessEvidence, p challengeProbe) accessVerdict {
	if p.Expected {
		return accessVerdict{}
	}
	switch {
	case ev.DocSeen && ev.Mitigated == "block", p.BlockPage:
		return accessVerdict{Kind: outcomeBlocked, Reason: "cloudflare-block"}
	case p.ErrorCode != "" && cloudflareDenyCodes[p.ErrorCode]:
		return accessVerdict{Kind: outcomeBlocked, Reason: "cloudflare error " + p.ErrorCode}
	case p.ErrorCode == "1015":
		return accessVerdict{Kind: outcomeRateLimited, Reason: "cloudflare error 1015", RetryAfter: parseRetryAfter(ev.RetryAfter, time.Now())}
	case ev.DocSeen && ev.Mitigated == "challenge", p.ChallengePage:
		// A Cloudflare challenge: the tracker decides (it can clear by itself).
		return accessVerdict{}
	case ev.DocSeen && ev.Status == http.StatusTooManyRequests:
		return accessVerdict{Kind: outcomeRateLimited, Reason: "HTTP 429", RetryAfter: parseRetryAfter(ev.RetryAfter, time.Now())}
	case p.Vendor != "":
		return accessVerdict{Kind: outcomeOtherChallenge, Reason: p.Vendor + " bot protection", Vendor: p.Vendor}
	}
	return accessVerdict{}
}

// maxRetryAfter caps a Retry-After we repeat to an agent: a site asking for
// days is better reported as "much later".
const maxRetryAfter = time.Hour

// parseRetryAfter reads a Retry-After header: delay-seconds or an HTTP date.
// Zero when absent or unreadable.
func parseRetryAfter(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var d time.Duration
	if secs, err := strconv.Atoi(value); err == nil {
		d = time.Duration(secs) * time.Second
	} else if t, err := http.ParseTime(value); err == nil {
		d = t.Sub(now)
	}
	if d < 0 {
		return 0
	}
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

// vendorLabel names a bot-protection vendor for people.
func vendorLabel(vendor string) string {
	switch vendor {
	case "datadome":
		return "DataDome"
	case "perimeterx":
		return "PerimeterX (HUMAN)"
	case "akamai":
		return "Akamai"
	case "imperva":
		return "Imperva"
	case "captcha":
		return "a CAPTCHA"
	}
	return vendor
}

// adviceFor is the fixed text an agent reads with the outcome. Built only from
// the kind and a number, never from page content.
func adviceFor(kind string, retryAfter time.Duration, vendor string) string {
	switch kind {
	case outcomeInteractiveRequired:
		return "The site asks for an interactive check that this tool does not solve. Do not retry. Ask the user to clear it once (Browser tool, Domains page, Clear now) or use another source for this data."
	case outcomeBlocked:
		return "The site denies this visitor. Do not retry. Use an official feed or API if one exists."
	case outcomeRateLimited:
		if retryAfter > 0 {
			return fmt.Sprintf("The site says too many requests. Wait at least %d seconds before calling this site again, and make fewer calls.", int(retryAfter.Seconds()+0.5))
		}
		return "The site says too many requests. Wait a few minutes before calling this site again, and make fewer calls."
	case outcomeOtherChallenge:
		return fmt.Sprintf("The page is a bot-protection wall (%s), not the content, and this tool does not solve it. Do not retry or read this page as data. Ask the user whether they can open it themselves, or use another source.", vendorLabel(vendor))
	case outcomeClearedAutomatically:
		return ""
	}
	return ""
}

// newAccessError turns a final verdict into the error the callers already
// know how to return (a *crawlError with a code, outcome and advice).
func newAccessError(v accessVerdict) *crawlError {
	switch v.Kind {
	case outcomeBlocked:
		return newCloudflareBlockedError(v.Reason)
	case outcomeRateLimited:
		msg := "The site is rate limiting this tool"
		if v.RetryAfter > 0 {
			msg += fmt.Sprintf(" (retry after %d s)", int(v.RetryAfter.Seconds()+0.5))
		}
		return &crawlError{Code: codeRateLimited, Message: msg, Reason: v.Reason, Outcome: outcomeRateLimited, Advice: adviceFor(outcomeRateLimited, v.RetryAfter, "")}
	case outcomeOtherChallenge:
		return &crawlError{Code: codeBotProtectionFound, Message: "The page is a bot-protection wall, not the content", Reason: v.Reason, Outcome: outcomeOtherChallenge, Advice: adviceFor(outcomeOtherChallenge, 0, v.Vendor)}
	}
	return nil
}

// accessErrorFromSingleShot is the check the single-page engines (Rod,
// Playwright) make after their own Cloudflare checks, with the main document's
// status and headers and the page probe: a final access error, or nil. A
// Cloudflare deny page counts as the existing blocked error.
func accessErrorFromSingleShot(status int, headers map[string]string, p challengeProbe) *crawlError {
	ev := accessEvidence{
		DocSeen:    true,
		Status:     int64(status),
		Mitigated:  strings.ToLower(strings.TrimSpace(headers["cf-mitigated"])),
		Server:     strings.ToLower(strings.TrimSpace(headers["server"])),
		CFRay:      strings.TrimSpace(headers["cf-ray"]),
		RetryAfter: strings.TrimSpace(headers["retry-after"]),
	}
	return newAccessError(classifyAccess(ev, p))
}

// accessNote is attached to a SUCCESSFUL result only when something happened
// that the caller should know about.
type accessNote struct {
	Kind     string `json:"kind"`
	WaitedMS int64  `json:"waitedMs,omitempty"`
}

// noteClearedAutomatically builds the note for a challenge the browser cleared
// by itself; nil when the page was never challenged.
func noteClearedAutomatically(clearedBy string, waited time.Duration) *accessNote {
	if clearedBy == "" || clearedBy == "not-detected" {
		return nil
	}
	return &accessNote{Kind: outcomeClearedAutomatically, WaitedMS: waited.Milliseconds()}
}

// accessNoteText renders the note for the line the model reads.
func accessNoteText(n *accessNote) string {
	if n == nil {
		return ""
	}
	if n.Kind == outcomeClearedAutomatically {
		return fmt.Sprintf("(Cloudflare asked for a check and the browser cleared it by itself after %.1f s; no action needed.)\n", float64(n.WaitedMS)/1000)
	}
	return ""
}
