package crawler

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"browser-tool-backend/shared"
)

// --- pure classification ------------------------------------------------------

func TestClassifyAccess(t *testing.T) {
	cases := []struct {
		name string
		ev   accessEvidence
		p    challengeProbe
		want string
	}{
		{"an ordinary page", accessEvidence{DocSeen: true, Status: 200}, challengeProbe{}, ""},
		{"cf-mitigated block", accessEvidence{DocSeen: true, Status: 403, Mitigated: "block"}, challengeProbe{}, outcomeBlocked},
		{"the block page", accessEvidence{}, challengeProbe{BlockPage: true}, outcomeBlocked},
		{"error 1020 access denied", accessEvidence{DocSeen: true, Status: 403}, challengeProbe{ErrorCode: "1020"}, outcomeBlocked},
		{"error 1010 banned browser signature", accessEvidence{}, challengeProbe{ErrorCode: "1010"}, outcomeBlocked},
		{"error 1015 rate limited", accessEvidence{DocSeen: true, Status: 429}, challengeProbe{ErrorCode: "1015"}, outcomeRateLimited},
		{"HTTP 429", accessEvidence{DocSeen: true, Status: 429, RetryAfter: "30"}, challengeProbe{}, outcomeRateLimited},
		{"a Cloudflare challenge is left to the tracker", accessEvidence{DocSeen: true, Status: 429, Mitigated: "challenge"}, challengeProbe{}, ""},
		{"the challenge page markers are left to the tracker", accessEvidence{DocSeen: true, Status: 403}, challengeProbe{ChallengePage: true, Vendor: "datadome"}, ""},
		{"another vendor's wall", accessEvidence{DocSeen: true, Status: 403}, challengeProbe{Vendor: "datadome"}, outcomeOtherChallenge},
		{"expected content wins over everything", accessEvidence{DocSeen: true, Status: 429}, challengeProbe{Expected: true, Vendor: "datadome", ErrorCode: "1020"}, ""},
		{"an unknown error code is not a verdict", accessEvidence{DocSeen: true, Status: 200}, challengeProbe{ErrorCode: "9999"}, ""},
	}
	for _, c := range cases {
		if got := classifyAccess(c.ev, c.p).Kind; got != c.want {
			t.Errorf("%s: kind = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Duration{
		"":                              30 * 0,
		"30":                            30 * time.Second,
		" 120 ":                         120 * time.Second,
		"-5":                            0,
		"garbage":                       0,
		"Thu, 29 Oct 2026 12:01:30 GMT": 90 * time.Second,
		"Thu, 29 Oct 2026 11:00:00 GMT": 0,
		"999999":                        maxRetryAfter,
		"Fri, 30 Oct 2026 12:00:00 GMT": maxRetryAfter,
	} {
		if got := parseRetryAfter(in, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestEveryAccessErrorCarriesItsOutcomeAndAdvice(t *testing.T) {
	for name, c := range map[string]struct {
		err      *crawlError
		code     string
		outcome  string
		contains string
	}{
		"unresolved challenge":  {newCloudflareUnresolvedError("x", nil), codeCloudflareUnresolved, outcomeInteractiveRequired, "Do not retry. Ask the user to clear it once"},
		"block":                 {newCloudflareBlockedError("x"), codeCloudflareBlocked, outcomeBlocked, "official feed or API"},
		"rate limit with delay": {newAccessError(accessVerdict{Kind: outcomeRateLimited, Reason: "HTTP 429", RetryAfter: 45 * time.Second}), codeRateLimited, outcomeRateLimited, "at least 45 seconds"},
		"rate limit no delay":   {newAccessError(accessVerdict{Kind: outcomeRateLimited, Reason: "HTTP 429"}), codeRateLimited, outcomeRateLimited, "a few minutes"},
		"other vendor":          {newAccessError(accessVerdict{Kind: outcomeOtherChallenge, Reason: "datadome bot protection", Vendor: "datadome"}), codeBotProtectionFound, outcomeOtherChallenge, "DataDome"},
	} {
		if c.err.Code != c.code || c.err.Outcome != c.outcome || !strings.Contains(c.err.Advice, c.contains) {
			t.Errorf("%s: %+v", name, c.err)
		}
		raw, _ := json.Marshal(c.err)
		if !strings.Contains(string(raw), `"outcome":"`+c.outcome+`"`) || !strings.Contains(string(raw), `"advice":`) {
			t.Errorf("%s: outcome/advice must be in the error JSON: %s", name, raw)
		}
	}
	if newAccessError(accessVerdict{}) != nil {
		t.Error("no verdict, no error")
	}
}

func TestAdviceIsBuiltOnlyFromTheKind(t *testing.T) {
	for _, kind := range []string{outcomeInteractiveRequired, outcomeBlocked, outcomeRateLimited, outcomeOtherChallenge} {
		a := adviceFor(kind, 10*time.Second, "perimeterx")
		if a == "" || strings.ContainsAny(a, "<>{}") {
			t.Errorf("%s advice: %q", kind, a)
		}
	}
	if adviceFor(outcomeClearedAutomatically, 0, "") != "" || adviceFor("nonsense", 0, "") != "" {
		t.Error("nothing to advise when nothing is wrong")
	}
}

func TestSuccessNoteOnlyWhenSomethingHappened(t *testing.T) {
	if noteClearedAutomatically("not-detected", time.Second) != nil || noteClearedAutomatically("", 0) != nil {
		t.Error("an unchallenged page gets no note")
	}
	n := noteClearedAutomatically("clean document (HTTP 200)", 1200*time.Millisecond)
	if n == nil || n.Kind != outcomeClearedAutomatically || n.WaitedMS != 1200 {
		t.Fatalf("%+v", n)
	}
	text := fetchPageText(crawlResponse{HTML: "<p>x</p>", Title: "T", FinalURL: "https://x/", Access: n})
	if !strings.Contains(text, "cleared it by itself after 1.2 s") || !strings.HasSuffix(text, "<p>x</p>") {
		t.Errorf("note must show above the HTML: %q", text)
	}
	if plain := fetchPageText(crawlResponse{HTML: "<p>x</p>", Title: "T", FinalURL: "https://x/"}); strings.Contains(plain, "Cloudflare") {
		t.Errorf("no note, no line: %q", plain)
	}
}

func TestAccessErrorFromSingleShot(t *testing.T) {
	if err := accessErrorFromSingleShot(200, map[string]string{"server": "cloudflare"}, challengeProbe{}); err != nil {
		t.Errorf("a normal page: %v", err)
	}
	err := accessErrorFromSingleShot(429, map[string]string{"retry-after": "30"}, challengeProbe{})
	if err == nil || err.Code != codeRateLimited || !strings.Contains(err.Advice, "30 seconds") {
		t.Errorf("429: %+v", err)
	}
	err = accessErrorFromSingleShot(403, map[string]string{}, challengeProbe{Vendor: "akamai"})
	if err == nil || err.Code != codeBotProtectionFound || !strings.Contains(err.Advice, "Akamai") {
		t.Errorf("vendor wall: %+v", err)
	}
	err = accessErrorFromSingleShot(403, map[string]string{}, challengeProbe{ErrorCode: "1020"})
	if err == nil || err.Code != codeCloudflareBlocked {
		t.Errorf("1020 must reuse the existing blocked error: %+v", err)
	}
}

// What an agent reads: CallSibling puts the advice after the message.
func TestTheAgentReadsTheAdvice(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(newCloudflareUnresolvedError("cf-mitigated: challenge (HTTP 403)", nil))
	}))
	defer stub.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(stub.URL, "http://"))
	t.Setenv("TOOL_OWN_PORT", port)

	_, err := shared.CallSibling("crawl", []byte(`{}`))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"Cloudflare challenge could not be cleared", "cloudflare_challenge_unresolved", "Do not retry", "Domains page"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q lacks %q", err.Error(), want)
		}
	}
}

// --- the page probe, in a real browser ----------------------------------------

const longPage = `<html><body><main>` +
	`<p>This is an ordinary page with plenty of real content about jobs, companies, locations and salaries, written out at enough length to be a genuine page and not a near-empty wall. ` +
	`It goes on for a while so that its visible text is comfortably longer than any challenge interstitial could be, which is exactly what separates it from one. ` +
	`More words here, and more still, because a real page has many paragraphs, headings, lists and links that a wall never has, however it is built.</p>`

func probeOfPage(t *testing.T, html string, expected ...string) challengeProbe {
	t.Helper()
	ctx := pageCtx(t, html)
	js, err := buildChallengeProbeJS(expected)
	if err != nil {
		t.Fatal(err)
	}
	var p challengeProbe
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &p)); err != nil {
		t.Fatalf("probe: %v", err)
	}
	return p
}

func TestProbeRecognizesCloudflareErrorPages(t *testing.T) {
	deny := probeOfPage(t, `<html><head><title>Access denied</title></head><body><div id="cf-error-details"><h1><span class="code-label">Error code <span>1020</span></span></h1><p>Access denied</p><p>Cloudflare Ray ID: 8a1b2c3d4e5f</p></div></body></html>`)
	if deny.ErrorCode != "1020" || classifyAccess(accessEvidence{DocSeen: true, Status: 403}, deny).Kind != outcomeBlocked {
		t.Errorf("1020: %+v", deny)
	}
	rate := probeOfPage(t, `<html><body><div id="cf-wrapper"><h1>Error code 1015</h1><p>You are being rate limited.</p><p>Cloudflare Ray ID: abc</p></div></body></html>`)
	if rate.ErrorCode != "1015" || classifyAccess(accessEvidence{DocSeen: true, Status: 429}, rate).Kind != outcomeRateLimited {
		t.Errorf("1015: %+v", rate)
	}
	// An article that merely mentions an error code is not a Cloudflare page.
	if p := probeOfPage(t, longPage+`<p>If you ever see error code 1020 somewhere, it means access denied.</p></main></body></html>`); p.ErrorCode != "" {
		t.Errorf("a page that only mentions a code must not be classified: %+v", p)
	}
}

func TestProbeRecognizesOtherVendorsWalls(t *testing.T) {
	for name, c := range map[string]struct{ html, vendor string }{
		"datadome":            {`<html><body><iframe src="https://geo.captcha-delivery.com/captcha/?initialCid=x" style="width:420px;height:320px"></iframe></body></html>`, "datadome"},
		"perimeterx":          {`<html><body><div id="px-captcha" style="width:320px;height:90px">Press &amp; Hold to confirm you are a human</div></body></html>`, "perimeterx"},
		"akamai":              {`<html><head><title>Access Denied</title></head><body><h1>Access Denied</h1><p>You don't have permission to access this server.</p><p>Reference #18.2d351ab8.1557333295.a4e16ab</p></body></html>`, "akamai"},
		"imperva":             {`<html><body>Request unsuccessful. Incapsula incident ID: 1234000670123456-98765432</body></html>`, "imperva"},
		"a captcha-only page": {`<html><body><iframe src="https://www.google.com/recaptcha/api2/anchor?k=x" style="width:300px;height:80px"></iframe></body></html>`, "captcha"},
	} {
		p := probeOfPage(t, c.html)
		if p.Vendor != c.vendor {
			t.Errorf("%s: vendor = %q, want %q (%+v)", name, p.Vendor, c.vendor, p)
		}
		if classifyAccess(accessEvidence{DocSeen: true, Status: 403}, p).Kind != outcomeOtherChallenge {
			t.Errorf("%s: must classify as another vendor's wall", name)
		}
	}
}

// Vendors' scripts also load on perfectly normal pages: they must not be taken
// for walls.
func TestProbeDoesNotMistakeNormalPagesForWalls(t *testing.T) {
	for name, html := range map[string]string{
		"datadome script on a real page":     `<html><head><script src="https://js.datadome.co/tags.js"></script><script src="https://geo.captcha-delivery.com/x.js"></script></head>` + longPage + `</main></body></html>`,
		"a recaptcha on a long contact form": longPage + `<form><iframe src="https://www.google.com/recaptcha/api2/anchor?k=x" style="width:300px;height:80px"></iframe></form></main></body></html>`,
		"Access Denied in a long article":    `<html><head><title>Access Denied pages explained</title></head>` + longPage + `</main></body></html>`,
		"a Turnstile widget on a form":       longPage + `<form><div class="cf-turnstile" style="width:300px;height:65px"></div></form></main></body></html>`,
	} {
		if p := probeOfPage(t, html); p.Vendor != "" || p.ErrorCode != "" {
			t.Errorf("%s was taken for a wall: %+v", name, p)
		}
	}
	// A short login page with a CAPTCHA is a page with a form, not a wall.
	if p := probeOfPage(t, `<html><body><form><input type="text" name="user"><input type="password" name="pw"><iframe src="https://www.google.com/recaptcha/api2/anchor?k=x" style="width:300px;height:80px"></iframe></form></body></html>`); p.Vendor != "" {
		t.Errorf("a short form page was taken for a CAPTCHA wall: %+v", p)
	}
	// The caller's own expected content beats a wall's markup.
	if p := probeOfPage(t, `<html><body><div class="job">Real job</div><div id="px-captcha" style="width:300px;height:80px"></div></body></html>`, ".job"); p.Vendor != "" {
		t.Errorf("expected content must win: %+v", p)
	}
}

// --- through the engines ------------------------------------------------------

// wallSite serves one fixed response on /.
func wallSite(t *testing.T, status int, headers map[string]string, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}

const dataDomeWall = `<html><body><iframe src="https://geo.captcha-delivery.com/captcha/?x=1" style="width:420px;height:320px"></iframe></body></html>`

func asCrawlError(t *testing.T, err error) *crawlError {
	t.Helper()
	var ce *crawlError
	if !errors.As(err, &ce) {
		t.Fatalf("want a *crawlError, got %v", err)
	}
	return ce
}

func TestRodReportsARateLimitWithItsRetryAfter(t *testing.T) {
	requireRodChrome(t)
	url := wallSite(t, 429, map[string]string{"Retry-After": "30"}, `<html><body>Too many requests</body></html>`)
	_, err := fetchWithRod(context.Background(), url, "", nil, nil, nil, 0, nil)
	ce := asCrawlError(t, err)
	if ce.Code != codeRateLimited || ce.Outcome != outcomeRateLimited || !strings.Contains(ce.Advice, "30 seconds") {
		t.Errorf("%+v", ce)
	}
}

func TestRodReportsAnotherVendorsWallInsteadOfReturningItAsContent(t *testing.T) {
	requireRodChrome(t)
	_, err := fetchWithRod(context.Background(), wallSite(t, 403, nil, dataDomeWall), "", nil, nil, nil, 0, nil)
	ce := asCrawlError(t, err)
	if ce.Code != codeBotProtectionFound || !strings.Contains(ce.Advice, "DataDome") {
		t.Errorf("%+v", ce)
	}
}

func TestRodStillReturnsOrdinaryPagesAndExistingCloudflareVerdicts(t *testing.T) {
	requireRodChrome(t)
	if _, err := fetchWithRod(context.Background(), wallSite(t, 200, map[string]string{"Server": "cloudflare", "CF-RAY": "8a-FRA"}, longPage+`</main></body></html>`), "", nil, nil, nil, 0, nil); err != nil {
		t.Errorf("an ordinary page behind Cloudflare must pass: %v", err)
	}
	_, err := fetchWithRod(context.Background(), wallSite(t, 403, map[string]string{"cf-mitigated": "challenge"}, `<html><body>Just a moment...</body></html>`), "", nil, nil, nil, 0, nil)
	ce := asCrawlError(t, err)
	if ce.Code != codeCloudflareUnresolved || ce.Outcome != outcomeInteractiveRequired || ce.Advice == "" {
		t.Errorf("challenge: %+v", ce)
	}
	_, err = fetchWithRod(context.Background(), wallSite(t, 403, map[string]string{"cf-mitigated": "block"}, `<html><body>blocked</body></html>`), "", nil, nil, nil, 0, nil)
	if ce := asCrawlError(t, err); ce.Code != codeCloudflareBlocked || ce.Outcome != outcomeBlocked {
		t.Errorf("block: %+v", ce)
	}
}

func browserOnly(t *testing.T) context.Context {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	chrome, err := shared.ChromePath()
	if err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome))
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	ctx, cancelTimeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(func() { cancelTimeout(); cancelCtx(); cancelAlloc() })
	return ctx
}

func TestTheTabPathReportsTheSameOutcomes(t *testing.T) {
	for name, c := range map[string]struct {
		status  int
		headers map[string]string
		body    string
		code    string
	}{
		"rate limit":   {429, map[string]string{"Retry-After": "20"}, `<html><body>Too many requests</body></html>`, codeRateLimited},
		"another wall": {403, nil, dataDomeWall, codeBotProtectionFound},
		"deny page":    {403, nil, `<html><body><div id="cf-error-details"><h1>Error code 1020</h1><p>Cloudflare Ray ID: 8a</p></div></body></html>`, codeCloudflareBlocked},
		"CF block":     {403, map[string]string{"cf-mitigated": "block"}, `<html><body>blocked</body></html>`, codeCloudflareBlocked},
	} {
		ctx := browserOnly(t)
		_, err := navigateAndReadWithCloudflareCheck(ctx, wallSite(t, c.status, c.headers, c.body), "", func() {}, 3*time.Second, nil, nil, nil, 0, nil)
		ce := asCrawlError(t, err)
		if ce.Code != c.code || ce.Advice == "" {
			t.Errorf("%s: %+v", name, ce)
		}
	}
}

func TestTheTabPathStillPassesAnOrdinaryPage(t *testing.T) {
	ctx := browserOnly(t)
	resp, err := navigateAndReadWithCloudflareCheck(ctx, wallSite(t, 200, nil, longPage+`</main></body></html>`), "", func() {}, 3*time.Second, nil, nil, nil, 0, nil)
	if err != nil || !strings.Contains(resp.HTML, "ordinary page") || resp.Access != nil {
		t.Fatalf("%v %+v", err, resp.Access)
	}
}

// --- diagnose -----------------------------------------------------------------

func diagnoseSite(t *testing.T, status int, headers map[string]string, body string) diagnoseReport {
	t.Helper()
	ctx := browserOnly(t)
	url := wallSite(t, status, headers, body)
	signals := watchChallengeSignals(ctx)
	if err := chromedp.Run(ctx, chromedp.Navigate(url), chromedp.Sleep(800*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	rep, err := diagnosePage(ctx, signals)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestDiagnoseReportsWhatTheToolSees(t *testing.T) {
	normal := diagnoseSite(t, 200, map[string]string{"Server": "cloudflare", "CF-RAY": "8a1b-FRA"}, longPage+`</main></body></html>`)
	if normal.Verdict != "none" || normal.HTTPStatus != 200 || normal.Wire.Server != "cloudflare" || normal.Wire.CFRay != "8a1b-FRA" || !normal.Wire.MainDocumentSeen {
		t.Errorf("normal page: %+v", normal)
	}
	if normal.Page.VisibleTextLength < 200 {
		t.Errorf("visible text length = %d", normal.Page.VisibleTextLength)
	}

	limited := diagnoseSite(t, 429, map[string]string{"Retry-After": "30"}, `<html><body>Too many</body></html>`)
	if limited.Verdict != outcomeRateLimited || limited.HTTPStatus != 429 || limited.Wire.RetryAfter != "30" || !strings.Contains(limited.Advice, "30 seconds") {
		t.Errorf("rate limit: %+v", limited)
	}

	chal := diagnoseSite(t, 403, map[string]string{"cf-mitigated": "challenge"}, `<html><body>Just a moment...</body></html>`)
	if chal.Verdict != "cloudflare_challenge" || chal.Wire.CFMitigated != "challenge" {
		t.Errorf("challenge: %+v", chal)
	}

	wall := diagnoseSite(t, 403, nil, dataDomeWall)
	if wall.Verdict != outcomeOtherChallenge || wall.Page.OtherVendor != "datadome" || !strings.Contains(wall.Advice, "DataDome") {
		t.Errorf("another wall: %+v", wall)
	}
}

func TestDiagnoseHandlerRejectsBadRequestsBeforeAnyBrowserWork(t *testing.T) {
	get := func(target string) int {
		rec := httptest.NewRecorder()
		DiagnoseURLHandler(rec, httptest.NewRequest(http.MethodGet, "/diagnose-url"+target, nil))
		return rec.Code
	}
	for name, c := range map[string]struct {
		target string
		want   int
	}{
		"no url":     {"", http.StatusBadRequest},
		"ftp":        {"?url=ftp://example.com/", http.StatusBadRequest},
		"loopback":   {"?url=http://127.0.0.1:1/", http.StatusBadRequest},
		"private IP": {"?url=http://10.0.0.5/", http.StatusBadRequest},
	} {
		if got := get(c.target); got != c.want {
			t.Errorf("%s: status %d, want %d", name, got, c.want)
		}
	}
	rec := httptest.NewRecorder()
	DiagnoseURLHandler(rec, httptest.NewRequest(http.MethodPost, "/diagnose-url", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}

func TestManifestDeclaresTheDiagnoseRouteWithItsScope(t *testing.T) {
	for _, r := range readManifest(t) {
		if r["path_suffix"] == "diagnose-url" {
			if r["method"] != "GET" || r["required_scope"] != "tool:browser:crawl" {
				t.Errorf("route: %v", r)
			}
			return
		}
	}
	t.Error("manifest.json does not declare diagnose-url")
}
