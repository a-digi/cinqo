// katana_fetch.go (plan 184) — fetchWithKatana, the Katana counterpart of
// fetchWithRod/fetchWithPlaywright: the PRIMARY single-page fetch behind
// fetch_page_html. Same signature, same result (a crawlResponse), same
// contract: a *crawlError is a verdict (a Cloudflare/bot wall, or a refused
// redirect) the caller acts on; any other error means "this engine failed"
// and the caller falls through to the unchanged chromedp primary tab.
//
// What Katana is used for, and what it is NOT allowed to do. Katana is a
// crawler; this tool needs exactly one page, read-only. So the crawler is
// locked down to that (see katanaOptions): attached to OUR Chrome
// (katana_engine.go), depth 1, a scope of exactly the one URL so no link it
// finds is ever queued, no form filling, no clicking of onclick links, no
// robots/sitemap/known-files, no JavaScript endpoint crawling, no response
// storage, no technology detection, no proxy, no custom headers, no cookies
// of any login. The first result ends the crawl.
//
// The page is read through Katana's AfterLoad hook, on the live tab, with
// the SAME steps and JavaScript the rod engine uses (challenge probe, the
// DOM-settle wait, RemoveElementsJS, removeAttributesJS, MaxHTMLBytes), so
// the HTML a model receives is identical whichever engine served the page.
// The main document's status and headers come from the browser's own
// network events, as in the rod engine — never from Katana's response
// object, which is the first hop (a 301) when the URL redirects.
package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/projectdiscovery/katana/pkg/engine/hybrid"
	"github.com/projectdiscovery/katana/pkg/navigation"
	"github.com/projectdiscovery/katana/pkg/output"
	"github.com/projectdiscovery/katana/pkg/types"
)

// codeRedirectRefused: the page redirected to an address this tool never
// crawls. A final answer for the request (not an engine failure).
const codeRedirectRefused = "redirect_refused"

// katanaNoDataDir is only a non-empty value for Options.ChromeDataDir: with
// one set, Katana neither creates a temporary profile folder nor deletes
// one. It never writes there, because no Chrome is started by Katana.
const katanaNoDataDir = "katana-attached-to-our-chrome"

// katanaNoOutput is Katana's output writer replaced by one that writes
// nothing. Katana's own writer prints every result URL to standard output;
// this tool's output is its own, and the result reaches us through OnResult.
type katanaNoOutput struct{}

func (katanaNoOutput) Close() error                 { return nil }
func (katanaNoOutput) Write(*output.Result) error   { return nil }
func (katanaNoOutput) WriteErr(*output.Error) error { return nil }
func (katanaNoOutput) GetResultCount() int64        { return 0 }

// katanaCloseWait bounds how long a finished fetch waits for Katana to wind
// down after its context was cancelled.
const katanaCloseWait = 5 * time.Second

// katanaOptions builds the locked-down Katana options for ONE page. Every
// field that could make Katana do more than load and read this one URL is set
// explicitly, so a change of Katana's defaults can never widen what it does.
// A test pins every one of them.
func katanaOptions(ctx context.Context, wsURL, rawURL string, onResult func(output.Result)) *types.Options {
	return &types.Options{
		// attach to OUR Chrome; never launch, download or find one
		HeadlessHybrid:     true,
		Headless:           true,
		ChromeWSUrl:        wsURL,
		UseInstalledChrome: false,
		SystemChromePath:   "",
		// ChromeDataDir set → Katana creates no temp folder and removes none
		// (it is a folder it never writes to: it does not start a Chrome).
		ChromeDataDir: katanaNoDataDir,
		ShowBrowser:   false,
		Proxy:         "",

		// exactly this page: seed only, nothing it links to is in scope
		MaxDepth:        1,
		Scope:           []string{"^" + regexp.QuoteMeta(rawURL) + "$"},
		OutOfScope:      nil,
		NoScope:         false,
		FieldScope:      "",
		Strategy:        "depth-first",
		Concurrency:     1,
		Parallelism:     1,
		RateLimit:       0,
		MaxOnclickLinks: -1, // never click anything on the page (0 would become Katana's default of 10)

		// read-only: no forms, no files, no scripts crawled, no storing
		AutomaticFormFill:         false,
		FormExtraction:            false,
		FormConfig:                "",
		KnownFiles:                "",
		ScrapeJSResponses:         false,
		ScrapeJSLuiceResponses:    false,
		XhrExtraction:             false,
		StoreResponse:             false,
		StoreFields:               "",
		StoreFieldDir:             "",
		OutputFile:                "",
		TechDetect:                false,
		CustomHeaders:             nil,
		HeadlessOptionalArguments: nil,
		DisableRedirects:          false,
		TlsImpersonate:            false,
		HealthCheck:               false,
		DisableUpdateCheck:        true,
		HeadlessNoIncognito:       false, // its own incognito context inside our Chrome

		// the page is loaded by Katana's "load" strategy; waiting for the
		// page to stop changing is done by waitForDOMToSettle in the hook
		PageLoadStrategy: "load",
		Timeout:          int(crawlTimeout / time.Second),
		BodyReadSize:     MaxHTMLBytes,
		Context:          ctx,

		Silent:   true,
		NoColors: true,
		OnResult: onResult,
	}
}

// katanaCapture carries what the hooks learned about the one page.
type katanaCapture struct {
	mu   sync.Mutex
	resp *crawlResponse
	err  error
}

func (c *katanaCapture) set(resp *crawlResponse, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resp == nil && c.err == nil {
		c.resp, c.err = resp, err
	}
}

func (c *katanaCapture) get() (*crawlResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resp, c.err
}

// fetchWithKatana — see the file comment.
func fetchWithKatana(ctx context.Context, rawURL, requestID string, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (crawlResponse, error) {
	slots := rodPageSlots() // the same bound on simultaneously open pages
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	case <-ctx.Done():
		return crawlResponse{}, ctx.Err()
	}

	wsURL, err := ensureKatanaEngine()
	if err != nil {
		return crawlResponse{}, fmt.Errorf("katana engine unavailable: %w", err)
	}

	// One context for the whole fetch: the caller's cancellation, the
	// whole-fetch budget, and our own cancel once the first result is in.
	fctx, cancel := context.WithTimeout(ctx, crawlTimeout)
	defer cancel()

	capture := &katanaCapture{}
	main := &mainDocumentResponse{}
	results := make(chan output.Result, 1)
	var once sync.Once
	opts := katanaOptions(fctx, wsURL, rawURL, func(r output.Result) {
		once.Do(func() { results <- r })
	})

	crawlerOptions, err := types.NewCrawlerOptions(opts)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("katana: options refused: %w", err)
	}
	_ = crawlerOptions.OutputWriter.Close()
	crawlerOptions.OutputWriter = katanaNoOutput{}
	kat, err := hybrid.New(crawlerOptions)
	if err != nil {
		return crawlResponse{}, fmt.Errorf("katana: could not attach to Chrome: %w", err)
	}
	defer func() {
		cancel()
		_ = kat.Close()
		_ = crawlerOptions.Close()
	}()

	kat.SetHooks(&hybrid.Hooks{
		BeforeNavigate: func(hctx context.Context, page *rod.Page, req *navigation.Request) error {
			return katanaBeforeNavigate(fctx, page, rawURL, requestID, main)
		},
		AfterLoad: func(hctx context.Context, page *rod.Page, req *navigation.Request, resp *navigation.Response) error {
			r, err := katanaReadPage(fctx, page, rawURL, requestID, main, expectedSelectors, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
			capture.set(r, err)
			return nil
		},
	})

	setCrawlPhase(requestID, phaseNavigating, "navigating to "+rawURL+" (katana)")
	crawlDone := make(chan error, 1)
	go func() { crawlDone <- kat.Crawl(rawURL) }()

	var result output.Result
	select {
	case result = <-results:
	case err := <-crawlDone:
		// The crawl ended without a result: it failed, or the context ended.
		if ctx.Err() != nil {
			return crawlResponse{}, ctx.Err()
		}
		if err != nil {
			return crawlResponse{}, fmt.Errorf("katana: crawl failed: %w", err)
		}
		return crawlResponse{}, fmt.Errorf("katana: no page was returned")
	case <-fctx.Done():
		if ctx.Err() != nil {
			return crawlResponse{}, ctx.Err()
		}
		return crawlResponse{}, fmt.Errorf("katana: timed out after %s", crawlTimeout)
	}

	// The first result is the page. Stop Katana now rather than let it wait
	// out its idle time; it would find nothing else in scope anyway.
	cancel()
	select {
	case <-crawlDone:
	case <-time.After(katanaCloseWait):
	}

	if resp, verdict := capture.get(); verdict != nil {
		return crawlResponse{}, rodCtxOr(ctx, verdict)
	} else if resp != nil {
		return *resp, nil
	}
	if result.Error != "" {
		return crawlResponse{}, fmt.Errorf("katana: %s", result.Error)
	}
	return crawlResponse{}, fmt.Errorf("katana: the page could not be read")
}

// katanaBeforeNavigate prepares Katana's tab so it presents the same browser
// the rod engine does. Katana creates its tabs through a rod browser that
// emulates a built-in laptop device (a stale CDP user-agent override, a
// fixed viewport, touch settings); those are cleared again here, and the
// same stealth script every other engine injects is added on every new
// document.
func katanaBeforeNavigate(fctx context.Context, page *rod.Page, rawURL, requestID string, main *mainDocumentResponse) error {
	if err := (proto.NetworkSetUserAgentOverride{UserAgent: ""}).Call(page); err != nil {
		return fmt.Errorf("could not clear the user agent override: %w", err)
	}
	if err := (proto.EmulationClearDeviceMetricsOverride{}).Call(page); err != nil {
		return fmt.Errorf("could not clear the device metrics: %w", err)
	}
	_ = (proto.EmulationSetTouchEmulationEnabled{Enabled: false}).Call(page)
	if _, err := page.EvalOnNewDocument(stealth.JS); err != nil {
		return fmt.Errorf("could not inject the stealth script: %w", err)
	}

	// A JavaScript dialog blocks a page's load forever; dismiss every one.
	// And record the main document's own status and headers.
	mainFrame := proto.PageFrameID(page.TargetID)
	go page.Context(fctx).EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false}.Call(page)
		},
		func(e *proto.NetworkResponseReceived) {
			if e.Type == proto.NetworkResourceTypeDocument && e.FrameID == mainFrame && e.Response != nil {
				main.record(e.Response.Status, e.Response.Headers)
			}
		},
	)()

	seedStoredClearanceRod(page.Context(fctx), rawURL) // a cf_clearance a person earned; nothing else
	return nil
}

// katanaReadPage runs on the live tab after Katana loaded it: every verdict
// first (exactly the rod engine's order), then the DOM-settle wait, then the
// cleaned HTML. It returns either a page or a verdict/error, never both.
func katanaReadPage(fctx context.Context, page *rod.Page, rawURL, requestID string, main *mainDocumentResponse, expectedSelectors, removeSelectors, removeAttributes []string, maxAttributeLength int, ignoreAttributesForMaxLength []string) (*crawlResponse, error) {
	p := page.Context(fctx)

	setCrawlPhase(requestID, phaseCheckingCloudflare, "checking for a Cloudflare challenge (katana)")
	if value, status := main.header("cf-mitigated"); value != "" {
		switch strings.ToLower(value) {
		case "block":
			return nil, newCloudflareBlockedError(fmt.Sprintf("cf-mitigated: block (HTTP %d)", status))
		case "challenge":
			return nil, newCloudflareUnresolvedError(fmt.Sprintf("cf-mitigated: challenge (HTTP %d)", status), nil)
		}
	}
	probeJS, err := buildChallengeProbeJS(expectedSelectors)
	if err != nil {
		return nil, err
	}
	raw, err := rodEvalExpression(p, probeJS)
	if err != nil {
		return nil, fmt.Errorf("katana: challenge probe failed: %w", err)
	}
	var probe challengeProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("katana: challenge probe decode failed: %w", err)
	}
	if probe.BlockPage {
		return nil, newCloudflareBlockedError("cloudflare block page")
	}
	if probe.ChallengePage && !probe.Expected {
		return nil, newCloudflareUnresolvedError("cloudflare challenge page", nil)
	}
	status, headers := main.statusAndHeaders()
	if cfErr := accessErrorFromSingleShot(status, headers, probe); cfErr != nil {
		return nil, cfErr
	}

	// A redirect to a private/internal address is never returned.
	if finalURL, err := rodEvalString(p, `document.location.toString()`); err == nil {
		if err := refuseRedirectToPrivate(rawURL, finalURL); err != nil {
			return nil, err
		}
	}

	if err := waitForDOMToSettle(fctx, p); err != nil {
		return nil, err
	}
	resp, err := readCrawlResponseWithRod(p, removeSelectors, removeAttributes, maxAttributeLength, ignoreAttributesForMaxLength)
	if err != nil {
		return nil, err
	}
	// The address may have changed while the page settled (a script redirect).
	if err := refuseRedirectToPrivate(rawURL, resp.FinalURL); err != nil {
		return nil, err
	}
	return &resp, nil
}

// refuseRedirectToPrivate applies the crawl's own address rule to where the
// page ended up, when that is another host than the one asked for.
func refuseRedirectToPrivate(rawURL, finalURL string) *crawlError {
	asked, err1 := url.Parse(rawURL)
	final, err2 := url.Parse(finalURL)
	if err1 != nil || err2 != nil || final.Hostname() == "" || strings.EqualFold(asked.Hostname(), final.Hostname()) {
		return nil
	}
	if err := validateCrawlURL(finalURL); err != nil {
		return &crawlError{
			Code:    codeRedirectRefused,
			Message: fmt.Sprintf("the page redirected to %s, which this tool does not crawl (%v)", final.Hostname(), err),
			Reason:  "redirect to an address the crawler refuses",
		}
	}
	return nil
}
