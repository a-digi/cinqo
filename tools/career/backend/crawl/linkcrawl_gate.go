// linkcrawl_gate.go (plan 185, step 2) — is the "link crawler" (jobs found by
// URL pattern, without AI) available for a portal link right now?
//
// It is available only while the Browser tool's selected engine is Katana,
// and never for a link that needs a login. The engine is READ from the
// Browser tool on every call (its stored setting, through core's proxy with
// the person's own session, exactly like every other Browser call of this
// backend) — never cached, never assumed: switching the engine on the
// Browser & Engine page changes the answer on the next request.
//
// Fail closed: when the engine cannot be read (the Browser tool is not
// installed, the session expired, the call failed) the feature is NOT
// available. The frontend uses the answer to hide the feature; the later
// steps (save, test, run) call requireKatanaSelected themselves, so hiding
// is never the only enforcement.
package crawl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"career-tool-backend/portal"
)

// browserSettingsPath is the Browser tool's settings route behind core's proxy.
const browserSettingsPath = "/api/v1/tools/browser/proxy/browser-settings"

// browserEngineKatana is the stored value of the Katana engine.
const browserEngineKatana = "katana"

// engineReadTimeout bounds the one small call that reads the engine setting.
const engineReadTimeout = 10 * time.Second

// Why the link crawler is not available (the stable codes the frontend reads).
const (
	linkCrawlAvailable        = "available"
	linkCrawlReasonLoginLink  = "login_link"
	linkCrawlReasonNotKatana  = "katana_not_selected"
	linkCrawlReasonEngineRead = "engine_unknown"
)

var (
	// errKatanaNotSelected: another engine is selected.
	errKatanaNotSelected = errors.New("the link crawler needs the Katana engine: select Katana on the Browser & Engine page")
	// errBrowserEngineUnknown: the selected engine could not be read.
	errBrowserEngineUnknown = errors.New("the Browser tool's engine could not be read, so the link crawler is not available")
)

// readBrowserEngine reads the Browser tool's selected engine. A var so tests
// can script the Browser without a core.
var readBrowserEngine = readBrowserEngineViaCore

func readBrowserEngineViaCore(ctx context.Context, coreURL, accessToken string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, engineReadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, coreURL+browserSettingsPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := crawlNowHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the Browser tool answered %d", resp.StatusCode)
	}
	var body struct {
		FetchEngine string `json:"fetchEngine"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("the Browser tool's settings could not be read: %w", err)
	}
	return body.FetchEngine, nil
}

// requireKatanaSelected is the check every link-crawler action makes: nil
// only when the Browser tool says Katana is the selected engine. An empty or
// missing engine value is what an OLDER Browser sends (its default was rod), so it is not Katana.
func requireKatanaSelected(ctx context.Context, coreURL, accessToken string) error {
	engine, err := readBrowserEngine(ctx, coreURL, accessToken)
	if err != nil {
		return fmt.Errorf("%w: %v", errBrowserEngineUnknown, err)
	}
	if strings.TrimSpace(engine) != browserEngineKatana {
		return errKatanaNotSelected
	}
	return nil
}

// linkCrawlAvailability is the answer of GET /portal-links/link-crawl/availability.
type linkCrawlAvailabilityResult struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"` // "available" or one of the linkCrawlReason* codes
	Message   string `json:"message"`
}

// linkCrawlAvailabilityFor decides for one link. A link that needs a login is
// answered first (it is a local fact and needs no Browser call): login links
// are crawled by hand only and never with Katana. An unknown link is an error
// (portal.ErrUnknownPortalLink).
func linkCrawlAvailabilityFor(ctx context.Context, coreURL, accessToken, portalLinkID string) (linkCrawlAvailabilityResult, error) {
	requiresLogin, _, err := portal.GetPortalLinkLogin(portalLinkID)
	if err != nil {
		return linkCrawlAvailabilityResult{}, err
	}
	if requiresLogin {
		return linkCrawlAvailabilityResult{Reason: linkCrawlReasonLoginLink,
			Message: "This link needs a login, so it is crawled by hand only: the link crawler is not available for it."}, nil
	}
	switch err := requireKatanaSelected(ctx, coreURL, accessToken); {
	case err == nil:
		return linkCrawlAvailabilityResult{Available: true, Reason: linkCrawlAvailable,
			Message: "Katana is the selected engine: this link can be crawled by its job links."}, nil
	case errors.Is(err, errKatanaNotSelected):
		return linkCrawlAvailabilityResult{Reason: linkCrawlReasonNotKatana,
			Message: "The link crawler is available when Katana is the selected engine (Browser & Engine page)."}, nil
	default:
		return linkCrawlAvailabilityResult{Reason: linkCrawlReasonEngineRead,
			Message: "The Browser tool's engine could not be read, so the link crawler is not available right now."}, nil
	}
}

// LinkCrawlAvailabilityHandler handles GET /portal-links/link-crawl/availability?portalLinkId=…
// Read-only: it changes nothing and starts nothing.
// The answers of GET /portal-links/crawl-engine (plan 190).
const (
	crawlEngineKatana  = "katana"
	crawlEngineRod     = "rod"
	crawlEngineUnknown = "unknown"
)

// crawlEngineName turns what the Browser tool answered into the three names the screen knows. An empty value is what an older Browser
// tool sends (its default was Rod); anything else, or a read that failed, is "unknown": the screen then judges nothing by the engine.
func crawlEngineName(engine string, err error) string {
	if err != nil {
		return crawlEngineUnknown
	}
	switch strings.TrimSpace(engine) {
	case browserEngineKatana:
		return crawlEngineKatana
	case "rod", "":
		return crawlEngineRod
	}
	return crawlEngineUnknown
}

// CrawlEngineHandler handles GET /portal-links/crawl-engine: which engine the Browser tool has selected, so the Platforms screen can show
// the instructions of the engine in use (plan 190). It reads the engine with the person's own session, exactly like the availability route.
func CrawlEngineHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}
	coreURL, err := coreAPIURL()
	if err != nil {
		http.Error(w, "core API address is not configured", http.StatusInternalServerError)
		return
	}
	engine, rerr := readBrowserEngine(r.Context(), coreURL, accessCookie.Value)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"engine": crawlEngineName(engine, rerr)})
}

func LinkCrawlAvailabilityHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	portalLinkID := strings.TrimSpace(r.URL.Query().Get("portalLinkId"))
	if portalLinkID == "" {
		http.Error(w, "portalLinkId is required", http.StatusBadRequest)
		return
	}
	// The person's own session, read off this request like Crawl now does.
	accessCookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return
	}
	coreURL, err := coreAPIURL()
	if err != nil {
		http.Error(w, "core API address is not configured", http.StatusInternalServerError)
		return
	}
	result, err := linkCrawlAvailabilityFor(r.Context(), coreURL, accessCookie.Value, portalLinkID)
	if err != nil {
		portal.WritePortalLinkAwareError(w, "read link crawler availability", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(result)
}
