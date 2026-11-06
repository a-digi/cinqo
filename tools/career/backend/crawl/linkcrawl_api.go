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
	return fetchPageLinks(ctx, coreURL, accessToken, pageURL, portalID, "")
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

// KatanaGate says whether Katana is the Browser's selected engine: GateOK, or the reason code with its message.
func KatanaGate(ctx context.Context, coreURL, accessToken string) (code, message string) {
	err := requireKatanaSelected(ctx, coreURL, accessToken)
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
