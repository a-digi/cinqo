// linkcrawl_api.go (plan 192, step 5) — the few pieces of the link crawler's screen routes that the Katana AI service
// (instruction/katana/service) needs, exported as plain functions: the same page load, the same own-site rule and the same engine gate
// as the screen, so the AI can never do what the screen may not. Nothing here is new behaviour.
package crawl

import (
	"context"
	"errors"
)

// PageLinks is what the Browser's link list answered for a page.
type PageLinks = pageLinks

// PageFetchError is a verdict or refusal of the Browser tool (a check page, a rate limit, a refused redirect, a bad address).
type PageFetchError = linkFetchError

// CoreURL is the core API address every tool subprocess and the HTTP process get.
func CoreURL() (string, error) { return coreAPIURL() }

// LoadPageLinks loads a page's links through core's proxy to the Browser, with the given session token.
func LoadPageLinks(ctx context.Context, coreURL, accessToken, pageURL, portalID string) (PageLinks, error) {
	res, err := fetchPageLinks(ctx, coreURL, accessToken, pageURL, portalID, "")
	// plan 195, step 5: a login_expired answer marks the link the context names (see WithKeptLogin).
	noteLoginVerdict(keptLoginLinkFrom(ctx), err)
	return res, err
}

// WithKeptLogin returns a context whose Browser calls (LoadPageLinks) carry the kept login of the link, and that marks the link when the site
// says the login expired. An empty session id changes nothing. Used by the Katana AI service for a link that needs a login (plan 195, step 5).
func WithKeptLogin(ctx context.Context, linkID, sessionID string) context.Context {
	return withKeptLoginFor(ctx, linkID, sessionID)
}

// OwnSitePage returns the page to load: the link's own address when none is given, else the given address if it is an http(s) address
// on the link's own site.
func OwnSitePage(linkURL, requested string) (string, error) { return ownSitePage(linkURL, requested) }

// The reasons KatanaGate gives.
const (
	GateOK             = ""
	GateNotKatana      = codeKatanaNotSelected
	GateEngineNotKnown = codeEngineUnknown
)

// KatanaGate says whether Katana is the engine that reads the link (its own engine, else the Browser's selected one): GateOK, or the reason
// code with its message (plan 197).
func KatanaGate(ctx context.Context, coreURL, accessToken, linkID string) (code, message string) {
	err := requireKatanaFor(ctx, coreURL, accessToken, linkID)
	switch {
	case err == nil:
		return GateOK, ""
	case errors.Is(err, errKatanaNotSelected):
		return GateNotKatana, err.Error()
	}
	return GateEngineNotKnown, err.Error()
}

// TestPagesLimit is the most pages a test run loads (the screen's limit).
const TestPagesLimit = testPagesLimit

// RunListingCrawl crawls the link's job list now with the given session, as a "Crawl now" press does, and returns when the crawl has ended
// (the run's result is in its crawl_runs row). It refuses what the press refuses (no rule, a login link, another engine selected, a crawl
// already running).
func RunListingCrawl(portalLinkID, accessToken string) (runID string, err error) {
	return RunListingCrawlNow(portalLinkID, accessToken, "manual")
}

// IsCrawlAlreadyRunning says whether an error means that a crawl of the link is already running.
func IsCrawlAlreadyRunning(err error) bool { return errors.Is(err, errCrawlAlreadyRunning) }
