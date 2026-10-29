package crawler

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"browser-tool-backend/shared"
)

// clearance_seed.go (step 96) puts a kept cf_clearance cookie (shared/clearance.go)
// into a browser tab before it navigates, so a site a person cleared once stays
// reachable for agents and auto-discovery until the cookie expires — across
// restarts. A kept cookie is only ever used by a browser reporting the same user
// agent it was earned with (Cloudflare binds it to that), and only for the
// domain it belongs to. Best effort: a failure to seed never fails a crawl.

// usableClearances returns the kept clearances for rawURL's host that match ua.
func usableClearances(rawURL, ua string) []shared.Clearance {
	host, err := hostnameOf(rawURL)
	if err != nil || shared.DB == nil {
		return nil
	}
	all, err := shared.ClearancesFor(host, time.Now())
	if err != nil {
		return nil
	}
	var out []shared.Clearance
	for _, c := range all {
		if c.UserAgent == ua {
			out = append(out, c)
		}
	}
	return out
}

// seedDomain is the cookie domain to set: a registrable domain gets the leading
// dot (so subdomains share it, as the site's own cookie does); an IP or a
// single-label host cannot take one.
func seedDomain(domain string) string {
	if net.ParseIP(domain) != nil || !strings.Contains(domain, ".") {
		return domain
	}
	return "." + domain
}

func hasKeptClearance(rawURL string) bool {
	host, err := hostnameOf(rawURL)
	if err != nil || shared.DB == nil {
		return false
	}
	all, err := shared.ClearancesFor(host, time.Now())
	return err == nil && len(all) > 0
}

// seedStoredClearance seeds a chromedp tab (before it navigates).
func seedStoredClearance(ctx context.Context, rawURL string) {
	if !hasKeptClearance(rawURL) {
		return
	}
	var ua string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua)); err != nil || ua == "" {
		return
	}
	for _, c := range usableClearances(rawURL, ua) {
		c := c
		_ = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			exp := cdp.TimeSinceEpoch(c.ExpiresAt)
			call := network.SetCookie(c.Name, c.Value).
				WithDomain(seedDomain(c.Domain)).
				WithPath(c.Path).
				WithSecure(c.Secure).
				WithHTTPOnly(c.HTTPOnly).
				WithExpires(&exp)
			if ss := network.CookieSameSite(c.SameSite); c.SameSite != "" {
				call = call.WithSameSite(ss)
			}
			return call.Do(ctx)
		}))
	}
}

// seedStoredClearanceRod seeds a rod page (before it navigates).
func seedStoredClearanceRod(p *rod.Page, rawURL string) {
	if !hasKeptClearance(rawURL) {
		return
	}
	res, err := p.Eval(`() => navigator.userAgent`)
	if err != nil {
		return
	}
	ua := res.Value.String()
	var params []*proto.NetworkCookieParam
	for _, c := range usableClearances(rawURL, ua) {
		params = append(params, &proto.NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   seedDomain(c.Domain),
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: proto.NetworkCookieSameSite(c.SameSite),
			Expires:  proto.TimeSinceEpoch(float64(c.ExpiresAt.Unix())),
		})
	}
	if len(params) > 0 {
		_ = p.SetCookies(params)
	}
}

// storeHumanClearance keeps the cf_clearance cookie a headed tab holds for
// rawURL, with the user agent it was earned under. Called when a person has
// cleared a challenge (importHeadedCookies). Only that one cookie is read for
// storing; every other cookie stays where it is.
func storeHumanClearance(ctx context.Context, rawURL string, cookies []*network.Cookie) {
	var ua string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua)); err != nil || ua == "" {
		return
	}
	for _, c := range cookies {
		if c.Name != shared.ClearanceCookieName || c.Session || c.Expires <= 0 {
			continue
		}
		sec := int64(c.Expires)
		_ = shared.StoreClearance(shared.Clearance{
			Domain:    c.Domain,
			Name:      c.Name,
			Value:     c.Value,
			Path:      c.Path,
			Secure:    c.Secure,
			HTTPOnly:  c.HTTPOnly,
			SameSite:  string(c.SameSite),
			ExpiresAt: time.Unix(sec, int64((c.Expires-float64(sec))*1e9)),
			UserAgent: ua,
			Source:    "human",
		})
	}
}
