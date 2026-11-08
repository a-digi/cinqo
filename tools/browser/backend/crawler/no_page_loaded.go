package crawler

import (
	"context"
	"errors"
	"strings"

	"github.com/chromedp/chromedp"
)

// errNoPageLoaded is what the tools that read "the page already loaded in the
// browser" (crawl_paginated without a url, extract_page_data) answer when the
// shared tab is blank. Without it they ran against about:blank and returned an
// ordinary-looking empty result, which an AI took for "the selectors found
// nothing" and retried (the Indeed run lost five of its ten rounds that way).
// find_repeating_items loads its page in a separate worker tab, so it never
// leaves the shared tab loaded for the next call.
var errNoPageLoaded = errors.New("no page is loaded in the browser: add `url: <the page's address>` at the top of the crawl instructions " +
	"(crawl_paginated then opens it itself), or call fetch_page_html with the address first, and then call this again")

// isBlankPageURL reports whether a tab's location means "nothing was loaded".
func isBlankPageURL(location string) bool {
	l := strings.TrimSpace(location)
	return l == "" || l == "about:blank"
}

// ensurePageLoaded checks the shared tab's current location. Assumes shared.Mu
// is held and ctx is the shared tab's context.
func ensurePageLoaded(ctx context.Context) error {
	var location string
	if err := chromedp.Run(ctx, chromedp.Location(&location)); err != nil {
		return err
	}
	if isBlankPageURL(location) {
		return errNoPageLoaded
	}
	return nil
}
