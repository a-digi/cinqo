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
	linkCrawlReasonNotKatana  = "katana_not_selected"
	linkCrawlReasonEngineRead = "engine_unknown"
)

var (
	// errKatanaNotSelected: another engine is selected.
	errKatanaNotSelected = errors.New("the link crawler needs the Katana engine: select Katana on the Browser & Engine page")
	// errBrowserEngineUnknown: the selected engine could not be read.
	errBrowserEngineUnknown = errors.New("the Browser tool's engine could not be read, so the link crawler is not available")
)

// linkOnRodError is the refusal for a link that is set to use Rod: it IS a "Katana is not the engine here" (errors.Is errKatanaNotSelected, so
// every caller and code that handles that keeps working), with the words that fit a link-level choice.
type linkOnRodError struct{}

func (linkOnRodError) Error() string {
	return "this link is set to use Rod, so Katana does not read it: set its engine back to the Browser setting to use Katana"
}
func (linkOnRodError) Is(target error) bool { return target == errKatanaNotSelected }

var errLinkUsesRod error = linkOnRodError{}

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

// effectiveEngine (plan 197, step 1) is the engine that reads this link: the link's own engine when it has one, else the Browser's selected
// engine. The Browser's value is read only when the link has none of its own (a link on Rod needs no reading), and a Browser that cannot be
// read is an error for a link that follows it, never a guess.
func effectiveEngine(ctx context.Context, coreURL, accessToken, linkID string) (string, error) {
	if own, err := portal.GetLinkCrawlEngine(linkID); err == nil && own != portal.LinkEngineBrowser {
		return own, nil
	}
	engine, err := readBrowserEngine(ctx, coreURL, accessToken)
	if err != nil {
		return "", err
	}
	return portal.EngineOf(portal.LinkEngineBrowser, engine), nil
}

// requireKatanaFor is requireKatanaSelected for one link: nil only when Katana is the engine that reads THIS link (a link on Rod is refused
// with errKatanaNotSelected whatever the Browser selected). An empty or missing Browser value is what an older Browser sends: not Katana.
func requireKatanaFor(ctx context.Context, coreURL, accessToken, linkID string) error {
	if own, err := portal.GetLinkCrawlEngine(linkID); err == nil && own == portal.LinkEngineRod {
		return errLinkUsesRod
	}
	engine, err := effectiveEngine(ctx, coreURL, accessToken, linkID)
	if err != nil {
		return fmt.Errorf("%w: %v", errBrowserEngineUnknown, err)
	}
	if strings.TrimSpace(engine) != browserEngineKatana {
		return errKatanaNotSelected
	}
	return nil
}

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
