// paginate_advance.go (step 95) is how the paginated crawl moves to the next
// page. A real mouse click is tried first, as always; but a cookie-consent
// banner or any overlay sitting on top of the control swallows the click (the
// click lands on the overlay, the page does not move), and the loop used to
// stop there with "url_unchanged". Found live on a job portal: the right
// selector was found, the click went to the consent banner, while the control's
// own script click and its link both worked. So when the URL has not changed
// after a click (and a further wait, for a slow single-page app), the loop
// tries, in order, the control's own script click, then navigating to the
// control's own link. It still only ever acts on the one control the
// instructions name. See plan/ai/tools/career/step-95-saved-run-is-a-success-and-pagination.md.
package crawler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
)

// How the loop reached a page.
const (
	advancedByClick   = "click"
	advancedByJSClick = "js_click"
	advancedByHref    = "href"
)

// advanceExtraWait is the further wait, after the first settle delay, before a
// click that changed nothing is treated as swallowed: a slow single-page app
// may update the URL a little late, and a second click would then skip a page.
var advanceExtraWait = SettleDelay

// advanceResult is what advanceToNextPage reports.
type advanceResult struct {
	URL      string // the page URL after the attempt
	By       string // how it moved; "" when it did not
	ClickErr error  // the real click's error, if it had one
}

// validateAdvanceURL is the URL guard for following a control's link; the same
// check every crawl's own URL gets. A var so tests can allow their loopback server.
var validateAdvanceURL = validateCrawlURL

func jsClickScript(selector string) string {
	return fmt.Sprintf(`(function(){var el=document.querySelector(%s);if(!el)return false;el.click();return true;})()`, jsStringLiteral(selector))
}

func controlHrefScript(selector string) string {
	return fmt.Sprintf(`(function(){var el=document.querySelector(%s);if(!el)return "";var a=el.tagName&&el.tagName.toUpperCase()==="A"?el:el.closest("a[href]");return a&&a.href?a.href:"";})()`, jsStringLiteral(selector))
}

// advanceToNextPage clicks the control and, when the page did not move,
// retries through the control's own script click and its link. A context
// deadline or cancellation is returned as the error so the caller can stop.
func advanceToNextPage(ctx context.Context, nextSelector, currentURL string) (advanceResult, error) {
	where := func() (string, error) {
		var u string
		err := chromedp.Run(ctx, chromedp.Location(&u))
		return u, err
	}
	moved := func(u string) bool { return u != currentURL }
	fatal := func(err error) bool { return err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) }

	var res advanceResult
	res.ClickErr = chromedp.Run(ctx, chromedp.Click(nextSelector), chromedp.Sleep(SettleDelay))
	if fatal(res.ClickErr) {
		return res, res.ClickErr
	}
	u, err := where()
	if err != nil {
		return res, err
	}
	if res.ClickErr == nil && moved(u) {
		res.URL, res.By = u, advancedByClick
		return res, nil
	}

	// Not moved yet: give a slow page one more chance before clicking again.
	if res.ClickErr == nil {
		if err := chromedp.Run(ctx, chromedp.Sleep(advanceExtraWait)); err != nil {
			return res, err
		}
		if u, err = where(); err != nil {
			return res, err
		}
		if moved(u) {
			res.URL, res.By = u, advancedByClick
			return res, nil
		}
	}

	// The control's own script click: reaches it through any overlay.
	var clicked bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(jsClickScript(nextSelector), &clicked)); err != nil {
		if fatal(err) {
			return res, err
		}
	} else if clicked {
		if err := chromedp.Run(ctx, chromedp.Sleep(SettleDelay)); err != nil {
			return res, err
		}
		if u, err = where(); err != nil {
			return res, err
		}
		if moved(u) {
			res.URL, res.By = u, advancedByJSClick
			return res, nil
		}
	}

	// The control's own link, when it is one: the same place a click would go,
	// refused when it points somewhere a crawl must not go.
	var href string
	if err := chromedp.Run(ctx, chromedp.Evaluate(controlHrefScript(nextSelector), &href)); err == nil {
		href = strings.TrimSpace(href)
		if href != "" && href != currentURL && validateAdvanceURL(href) == nil {
			if err := chromedp.Run(ctx, chromedp.Navigate(href), chromedp.Sleep(SettleDelay)); err != nil {
				if fatal(err) {
					return res, err
				}
			} else if u, err = where(); err == nil && moved(u) {
				res.URL, res.By = u, advancedByHref
				return res, nil
			}
		}
	} else if fatal(err) {
		return res, err
	}

	res.URL = u
	return res, nil
}

// anyFallback reports whether any page was reached by something other than a
// plain click.
func anyFallback(methods []string) bool {
	for _, m := range methods {
		if m != advancedByClick {
			return true
		}
	}
	return false
}
