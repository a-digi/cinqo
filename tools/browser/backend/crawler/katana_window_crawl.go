package crawler

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
)

// katana_window_crawl.go (plan 198, step V4): the paginated crawl of a Katana link's job pages, read in the Katana window.
//
// The same loop as every other tab (runPaginatedCrawlLoop: extraction, the Cloudflare tracker), but on a NEW TAB OF THE KATANA WINDOW'S
// Chrome (a chromedp remote allocator on its DevTools address; a new tab is created in the Chrome's default context), so it carries the
// cookies the person earned there. Only for a request that says `engine: "katana"`, for a site that is a window site, with no kept login, no
// login window and no AI context. Unattended reads (the recrawl workers) use it only while the window is open; a person's run may open it.
// A check met here is handled like in the single-page reads: the person is asked in the window and the crawl goes on. Everything else (a Rod
// link, any other site) keeps its old path unchanged.

// katanaWindowCrawlWanted: does this paginated request belong in the Katana window?
func katanaWindowCrawlWanted(body *paginatedCrawlRequest) bool {
	if body.Engine != "katana" || body.URL == "" || body.LoginSessionID != "" || body.UseLoginWindow || body.AIContextID != "" {
		return false
	}
	host, err := hostnameOf(body.URL)
	if err != nil || host == "" || !isWindowHost(host) {
		return false
	}
	return katanaWindowOpen() || body.Attended
}

// crawlInKatanaWindow runs the loop once on a fresh tab of the window.
func crawlInKatanaWindow(parent context.Context, url string, args paginatedArgs) (paginatedCrawlResponse, []string, error) {
	ws, err := ensureKatanaWindow()
	if err != nil {
		return paginatedCrawlResponse{}, nil, fmt.Errorf("katana window unavailable: %w", err)
	}
	awaitPortalCrawlPacing(parent, args.rateLimitKey)
	if parent.Err() != nil {
		return paginatedCrawlResponse{}, nil, newCrawlCancelledError()
	}
	reqCtx, reqCancel := context.WithCancel(parent)
	defer reqCancel()
	if args.requestID != "" {
		registerCrawlCancel(args.requestID, reqCancel)
		defer unregisterCrawlCancel(args.requestID)
	}
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(reqCtx, ws)
	defer allocCancel()
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel() // closes this tab only; the window and the other tabs stay

	timeout := paginatedCrawlTimeout
	if args.rateLimitKey != "" {
		timeout = paginatedCrawlTimeoutWithPacing
	}
	ctx, cancel := context.WithTimeout(tabCtx, timeout)
	defer cancel()

	signals := watchChallengeSignals(ctx)
	setCrawlPhase(args.requestID, phaseNavigating, "navigating to "+url+" (katana window)")
	if err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.Sleep(SettleDelay)); err != nil {
		return paginatedCrawlResponse{}, nil, classifyCancellation(err, reqCtx, tabCtx)
	}
	resp, html, err := runPaginatedCrawlLoop(ctx, loopTab{signals: signals, autoWait: cloudflareAutoWait}, args)
	return resp, html, classifyCancellation(err, reqCtx, tabCtx)
}

// performPaginatedCrawlViaKatanaWindow reads in the window and, for a person's run that meets a check, asks the person in the window and
// reads once more.
func performPaginatedCrawlViaKatanaWindow(parent context.Context, url string, args paginatedArgs, attended bool) (paginatedCrawlResponse, []string, error) {
	resp, html, err := crawlInKatanaWindow(parent, url, args)
	cfErr, clearable := isClearableCheck(err)
	if !clearable || !attended || !humanAssistEnabled() {
		return resp, html, err
	}
	host, _ := hostnameOf(url)
	if args.requestID == "" {
		return resp, html, windowRequestInBackground(host, url, cfErr)
	}
	if werr := waitForPersonInWindow(parent, url, args.requestID); werr != nil {
		if parent.Err() != nil {
			return resp, html, parent.Err()
		}
		out := *cfErr
		out.Outcome = outcomeHumanGaveUp
		out.Advice = "The browser window was opened but the check was not cleared. Open the link again and clear it, or use Rod for this link."
		return resp, html, &out
	}
	resp, html, err = crawlInKatanaWindow(parent, url, args)
	if again, still := isClearableCheck(err); still {
		out := *again
		out.Outcome = outcomeHumanGaveUp
		out.Advice = "The browser window was opened but the site still shows its check. Open the link again and clear it, or use Rod for this link."
		return resp, html, &out
	}
	return resp, html, err
}
