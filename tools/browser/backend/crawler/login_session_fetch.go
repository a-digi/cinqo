package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"browser-tool-backend/shared"
)

// login_session_fetch.go (plan 195, step 3): Katana fetches a page with a login session a person chose to keep (shared/loginsession.go).
//
//   - A request names the session by id (loginSessionId on /fetch-links, /fetch-detail-candidates and /crawl; never an MCP tool: the AI has
//     no way to name or learn one). The id is a random 128-bit value, so it works like a key: only the Career tool, which got it from the
//     person's own confirmation, knows it. The tool cannot see the caller's scopes, so this is the guard instead of a second scope.
//   - The session is opened for the page's OWN host only (a session never goes to another site), and a session that is gone, expired or for
//     another host is refused before any browser work starts.
//   - The cookies are set in the fetch's own tab, which Katana makes in its own incognito context, so five fetches with different sessions
//     never mix (BeforeNavigate, like the stored cf_clearance).
//   - After the page loaded, a visible password field means the site no longer accepts the session: it is marked expired and the fetch
//     answers login_expired. A good page records last_ok_at and writes back the cookies the site changed (a rotated session cookie).
//   - Never cached (the on-disk fetch cache and the window memo are not used on this path) and never logged: no cookie value reaches a log,
//     an error text or a response.

const (
	// codeLoginExpired: the site no longer accepts the kept login (or the session is gone/expired): the person must log in again.
	codeLoginExpired = "login_expired"
	// codeLoginSessionOtherSite: the session was made for another site than the page asked for.
	codeLoginSessionOtherSite = "login_session_other_site"
	// codeLoginSessionUnreadable: the sealed session cannot be opened (a lost or replaced key): the person must log in again.
	codeLoginSessionUnreadable = "login_session_unreadable"
)

const loginExpiredMessage = "The kept login no longer works: log in again (and keep the login) to crawl this link"

// loginExpiredAdvice is for a caller that is an AI: it cannot log in and must not retry in a loop.
const loginExpiredAdvice = "Do not fetch this site again until the person has logged in again: tell them the kept login expired and wait for their answer"

var loginSessionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func newLoginExpiredError(reason string) *crawlError {
	return &crawlError{Code: codeLoginExpired, Message: loginExpiredMessage, Reason: reason, Advice: loginExpiredAdvice}
}

type loginSessionCtxKey struct{}

// withLoginSession returns a context that makes Katana fetches use the saved login session id ("" = none).
func withLoginSession(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, loginSessionCtxKey{}, id)
}

func loginSessionFrom(ctx context.Context) string {
	id, _ := ctx.Value(loginSessionCtxKey{}).(string)
	return id
}

// checkLoginSessionID validates a request's session id; "" is fine (no session).
func checkLoginSessionID(id string) error {
	if id != "" && !loginSessionIDPattern.MatchString(id) {
		return errors.New("loginSessionId must be 32 lower-case hex characters")
	}
	return nil
}

// sessionCookiesFor opens the session of ctx (if any) for rawURL's host and returns the cookies to set in the fetch's tab, or a verdict
// (*crawlError) when the session cannot be used. No session in ctx: nil, nil.
func sessionCookiesFor(ctx context.Context, rawURL string) ([]*proto.NetworkCookieParam, error) {
	id := loginSessionFrom(ctx)
	if id == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return nil, fmt.Errorf("the address has no host")
	}
	cookies, err := shared.OpenLoginSession(id, u.Hostname())
	switch {
	case errors.Is(err, shared.ErrLoginSessionNotFound):
		return nil, newLoginExpiredError("the kept login no longer exists")
	case errors.Is(err, shared.ErrLoginSessionExpired):
		return nil, newLoginExpiredError("the kept login has expired")
	case errors.Is(err, shared.ErrLoginSessionWrongHost):
		return nil, &crawlError{Code: codeLoginSessionOtherSite, Message: "The kept login belongs to another site than this page", Reason: "login session for another site"}
	case errors.Is(err, shared.ErrLoginSessionUnreadable):
		return nil, &crawlError{Code: codeLoginSessionUnreadable, Message: "The kept login cannot be opened any more: log in again and keep the login", Reason: "login session unreadable", Advice: loginExpiredAdvice}
	case err != nil:
		return nil, fmt.Errorf("could not open the kept login")
	}
	if len(cookies) == 0 {
		return nil, newLoginExpiredError("the kept login has no live cookie left")
	}
	params := make([]*proto.NetworkCookieParam, 0, len(cookies))
	for _, c := range cookies {
		p := &proto.NetworkCookieParam{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HTTPOnly}
		if c.Expires > 0 {
			p.Expires = proto.TimeSinceEpoch(c.Expires)
		}
		switch strings.ToLower(c.SameSite) {
		case "strict":
			p.SameSite = proto.NetworkCookieSameSiteStrict
		case "lax":
			p.SameSite = proto.NetworkCookieSameSiteLax
		case "none":
			p.SameSite = proto.NetworkCookieSameSiteNone
		}
		params = append(params, p)
	}
	return params, nil
}

// katanaSessionCheck runs after the page loaded, when a session was used: a login form means the site refused it; a good page records the
// success and writes back the cookies the site changed. Returns the login_expired verdict or nil.
func katanaSessionCheck(ctx context.Context, p *rod.Page, rawURL string) error {
	id := loginSessionFrom(ctx)
	if id == "" {
		return nil
	}
	raw, err := rodEvalExpression(p, shared.InspectPageScript())
	if err != nil {
		return fmt.Errorf("katana: could not look at the page for a login form: %w", err)
	}
	var page shared.PageInspection
	if err := json.Unmarshal(raw, &page); err != nil {
		return fmt.Errorf("katana: could not read the login check: %w", err)
	}
	if page.PasswordField {
		_ = shared.MarkLoginSessionExpired(id)
		return newLoginExpiredError("the page shows a login form")
	}
	_ = shared.MarkLoginSessionOK(id)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	finalURL, ferr := rodEvalString(p, `document.location.toString()`)
	if ferr != nil {
		finalURL = rawURL
	}
	got, err := p.Cookies([]string{finalURL})
	if err != nil {
		return nil // writing back is best effort: the fetch itself worked
	}
	fresh := make([]shared.SessionCookie, 0, len(got))
	for _, c := range got {
		fresh = append(fresh, shared.SessionCookie{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Secure: c.Secure, HTTPOnly: c.HTTPOnly, SameSite: string(c.SameSite), Expires: float64(c.Expires)})
	}
	_, _ = shared.RefreshLoginSessionCookies(id, u.Hostname(), fresh)
	return nil
}

// crawlWithKeptLogin answers POST /crawl for a request that names a kept login: the Katana engine fetches the page with the session. It is a
// separate path on purpose: the normal crawl caches pages by address and falls back to a headed browser without the session, both wrong for a
// page that only a logged-in visitor sees.
func crawlWithKeptLogin(w http.ResponseWriter, r *http.Request, body crawlRequest) {
	if err := checkLoginSessionID(body.LoginSessionID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.UseLoginWindow || body.AIContextID != "" || body.Offset != nil {
		http.Error(w, "loginSessionId cannot be combined with useLoginWindow, aiContextId or offset", http.StatusBadRequest)
		return
	}
	if selectedFetchEngine() != fetchEngineKatana {
		writeFetchLinksJSON(w, http.StatusConflict, map[string]string{
			"code":    codeEngineNotKatana,
			"message": "fetching with a kept login needs the Katana engine; select Katana on the Browser & Engine page",
		})
		return
	}
	ctx, cancel := context.WithCancel(withLoginSession(r.Context(), body.LoginSessionID))
	registerCrawlCancel(body.RequestID, cancel)
	defer func() {
		unregisterCrawlCancel(body.RequestID)
		cancel()
	}()
	defer beginEngineFetch(fetchEngineKatana)()
	awaitPortalCrawlPacing(ctx, body.RateLimitKey)

	result, err := katanaFetch(ctx, body.URL, body.RequestID, body.ExpectedSelectors, body.RemoveSelectors, body.RemoveAttributes, body.MaxAttributeLength, body.IgnoreAttributesForMaxLength)
	if err != nil {
		setCrawlPhase(body.RequestID, phaseFailed, err.Error())
		var cfErr *crawlError
		if errors.As(err, &cfErr) {
			writeFetchLinksJSON(w, http.StatusBadGateway, cfErr)
			return
		}
		http.Error(w, fmt.Sprintf("crawl failed: %v", err), http.StatusBadGateway)
		return
	}
	setCrawlPhase(body.RequestID, phaseCompleted, "HTML crawled with a kept login")
	writeFetchLinksJSON(w, http.StatusOK, result)
}
