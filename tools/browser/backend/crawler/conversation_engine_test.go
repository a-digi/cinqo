package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

func resetConversationEngines(t *testing.T) {
	t.Helper()
	conversationEngines.mu.Lock()
	conversationEngines.m = map[string]conversationEngineEntry{}
	conversationEngines.now = time.Now
	conversationEngines.mu.Unlock()
	t.Cleanup(func() {
		conversationEngines.mu.Lock()
		conversationEngines.m = map[string]conversationEngineEntry{}
		conversationEngines.now = time.Now
		conversationEngines.mu.Unlock()
	})
}

func TestAConversationsEngineIsRememberedForAnHourAndOnlyForThatConversation(t *testing.T) {
	resetConversationEngines(t)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	conversationEngines.mu.Lock()
	conversationEngines.now = func() time.Time { return now }
	conversationEngines.mu.Unlock()

	if !setConversationEngine("conv-1", "rod") {
		t.Fatal("a good id and engine must be accepted")
	}
	if got := conversationEngine("conv-1"); got != "rod" {
		t.Errorf("remembered: %q", got)
	}
	if got := conversationEngine("conv-2"); got != "" {
		t.Errorf("another conversation must be unaffected: %q", got)
	}
	if got := conversationEngine(""); got != "" {
		t.Errorf("no conversation: %q", got)
	}
	now = now.Add(59 * time.Minute)
	if got := conversationEngine("conv-1"); got != "rod" {
		t.Errorf("still valid after 59 minutes: %q", got)
	}
	now = now.Add(2 * time.Minute)
	if got := conversationEngine("conv-1"); got != "" {
		t.Errorf("expired after an hour: %q", got)
	}
}

func TestOnlyAWellFormedIdAndAKnownEngineAreAccepted(t *testing.T) {
	resetConversationEngines(t)
	for _, bad := range [][2]string{{"", "rod"}, {"has space", "rod"}, {"../x", "rod"}, {strings.Repeat("a", 101), "rod"}, {"ok", ""}, {"ok", "playwright"}, {"ok", "Rod"}} {
		if setConversationEngine(bad[0], bad[1]) {
			t.Errorf("%q / %q must be refused", bad[0], bad[1])
		}
	}
	if !setConversationEngine("0a60cef4-5079-41ee-b6ad-8d93effe1893", "katana") {
		t.Error("a conversation uuid and a known engine must be accepted")
	}
}

func TestTheStoreIsBoundedAndForgetsExpiredEntriesToMakeRoom(t *testing.T) {
	resetConversationEngines(t)
	now := time.Now()
	conversationEngines.mu.Lock()
	conversationEngines.now = func() time.Time { return now }
	conversationEngines.mu.Unlock()
	for i := 0; i < conversationEngineMax; i++ {
		if !setConversationEngine("c"+strings.Repeat("x", 1)+itoa(i), "rod") {
			t.Fatalf("entry %d refused before the limit", i)
		}
	}
	if setConversationEngine("one-too-many", "rod") {
		t.Error("a full store of live entries must refuse a new conversation")
	}
	if !setConversationEngine("cx0", "rod") {
		t.Error("an existing conversation can still be updated when the store is full")
	}
	now = now.Add(2 * time.Hour) // everything expired
	if !setConversationEngine("one-too-many", "rod") {
		t.Error("expired entries must make room")
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

func TestTheConversationEngineRouteAcceptsAPutAndRefusesTheRest(t *testing.T) {
	resetConversationEngines(t)
	put := func(method, body string) int {
		rec := httptest.NewRecorder()
		ConversationEngineHandler(rec, httptest.NewRequest(method, "/conversation-engine", strings.NewReader(body)))
		return rec.Code
	}
	if code := put(http.MethodPut, `{"conversationId":"conv-a","engine":"rod"}`); code != http.StatusNoContent || conversationEngine("conv-a") != "rod" {
		t.Errorf("good PUT: %d", code)
	}
	for _, c := range []struct{ method, body string }{
		{http.MethodPut, ``}, {http.MethodPut, `nope`}, {http.MethodPut, `{"conversationId":"x y","engine":"rod"}`},
		{http.MethodPut, `{"conversationId":"conv-b","engine":"selenium"}`}, {http.MethodPut, `{"conversationId":"conv-b"}`},
	} {
		if code := put(c.method, c.body); code != http.StatusBadRequest {
			t.Errorf("%s %q: %d", c.method, c.body, code)
		}
	}
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		if code := put(m, `{"conversationId":"conv-c","engine":"rod"}`); code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, code)
		}
	}
	if conversationEngine("conv-b") != "" || conversationEngine("conv-c") != "" {
		t.Error("a refused request must remember nothing")
	}
}

func TestTheRouteIsDeclaredUnderTheCrawlScopeAndIsNotAnAITool(t *testing.T) {
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method string `json:"method"`
			Path   string `json:"path_suffix"`
			Scope  string `json:"required_scope"`
		} `json:"routes"`
		MCPTools []struct {
			Name string `json:"name"`
		} `json:"mcp_tools"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range m.Routes {
		if r.Path == "conversation-engine" {
			found = r.Method == "PUT" && r.Scope == "tool:browser:crawl"
		}
	}
	if !found {
		t.Error("PUT conversation-engine must be a route of the manifest under tool:browser:crawl")
	}
	for _, tool := range m.MCPTools {
		if strings.Contains(tool.Name, "engine") {
			t.Errorf("MCP tool %s would let the AI choose its engine", tool.Name)
		}
	}
}

// The engine of the call is the conversation's, else the Browser's: the stubs stand in for the two engines.
func TestACrawlOfARodConversationReadsWithRodWhileKatanaIsSelected(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	resetConversationEngines(t)
	var mu sync.Mutex
	var used []string
	origRod, origKatana := rodFetch, katanaFetch
	rodFetch = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		mu.Lock()
		used = append(used, "rod")
		mu.Unlock()
		return crawlResponse{HTML: "<p>rod</p>"}, nil
	}
	katanaFetch = func(context.Context, string, string, []string, []string, []string, int, []string) (crawlResponse, error) {
		mu.Lock()
		used = append(used, "katana")
		mu.Unlock()
		return crawlResponse{HTML: "<p>katana</p>"}, nil
	}
	t.Cleanup(func() { rodFetch, katanaFetch = origRod, origKatana })
	read := func(ctx context.Context) {
		t.Helper()
		if _, err := fetchWithEngine(ctx, "http://93.184.216.34/", "", nil, nil, nil, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	read(context.Background()) // the Browser's engine
	read(withFetchEngine(context.Background(), conversationEngine("conv-unknown")))
	setConversationEngine("conv-rod", "rod")
	read(withFetchEngine(context.Background(), conversationEngine("conv-rod")))
	if got := strings.Join(used, ","); got != "katana,katana,rod" {
		t.Errorf("engines used: %s", got)
	}
}

// The handler passes the conversation's engine to the page fetch for an AI call and nothing for any other call.
func TestTheCrawlRouteHandsTheConversationsEngineToThePageFetch(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	resetConversationEngines(t)
	setConversationEngine("conv-rod", "rod")
	var seen []string
	orig := crawlPageFn
	crawlPageFn = func(ctx context.Context, rawURL, requestID string, _, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		seen = append(seen, fetchEngineFrom(ctx))
		return crawlResponse{HTML: "<p>x</p>"}, nil
	}
	t.Cleanup(func() { crawlPageFn = orig })
	for _, body := range []string{
		`{"url":"` + publicTestURL + `","aiContextId":"conv-rod"}`,   // the Rod conversation
		`{"url":"` + publicTestURL + `","aiContextId":"conv-other"}`, // another conversation
		`{"url":"` + publicTestURL + `"}`,                            // Career's own call: no conversation
	} {
		rec := httptest.NewRecorder()
		CrawlHandler(rec, httptest.NewRequest(http.MethodPost, "/crawl", strings.NewReader(body)))
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	if len(seen) != 3 || seen[0] != "rod" || seen[1] != "" || seen[2] != "" {
		t.Errorf("engine handed to the fetch: %q", seen)
	}
}
