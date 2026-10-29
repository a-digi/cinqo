package crawler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"browser-tool-backend/shared"
)

func TestWindowHTML(t *testing.T) {
	html := strings.Repeat("a", 40_000)
	got, w := windowHTML(html, 0)
	if len(got) != fetchWindowChars || w != (htmlWindow{Start: 0, End: fetchWindowChars, Total: 40_000}) {
		t.Errorf("first window: %d chars, %+v", len(got), w)
	}
	got, w = windowHTML(html, 36_000)
	if len(got) != 4_000 || w != (htmlWindow{Start: 36_000, End: 40_000, Total: 40_000}) {
		t.Errorf("last window: %d chars, %+v", len(got), w)
	}
	got, w = windowHTML(html, 40_000)
	if got != "" || w != (htmlWindow{Start: 40_000, End: 40_000, Total: 40_000}) {
		t.Errorf("at the end: %q %+v", got, w)
	}
	got, w = windowHTML(html, 99_999)
	if got != "" || w.Start != 40_000 || w.End != 40_000 {
		t.Errorf("past the end must clamp: %q %+v", got, w)
	}
	got, w = windowHTML(html, -5)
	if len(got) != fetchWindowChars || w.Start != 0 {
		t.Errorf("a negative offset clamps to 0: %d %+v", len(got), w)
	}
}

// Windows count characters, so a multi-byte character is never cut in half.
func TestWindowHTMLNeverSplitsACharacter(t *testing.T) {
	html := strings.Repeat("ä€😀", 10_000) // 30,000 characters, 2-, 3- and 4-byte
	first, w := windowHTML(html, 0)
	rest, w2 := windowHTML(html, w.End)
	if w.Total != 30_000 || w2.Total != 30_000 {
		t.Fatalf("totals: %+v %+v", w, w2)
	}
	if first+rest != html[:len(first)+len(rest)] || strings.ContainsRune(first+rest, '�') {
		t.Error("a window boundary split a character")
	}
	if len([]rune(first)) != fetchWindowChars {
		t.Errorf("window is %d characters", len([]rune(first)))
	}
}

func TestFetchMemoKeyDependsOnEverythingThatShapesTheHTML(t *testing.T) {
	base := crawlRequest{URL: "https://x.test/", RemoveSelectors: []string{"header"}, RemoveAttributes: []string{"style"}, MaxAttributeLength: 200, IgnoreAttributesForMaxLength: []string{"poster"}}
	same := base
	if fetchMemoKey(base) != fetchMemoKey(same) {
		t.Error("identical requests must share a key")
	}
	withOffset := base
	zero := 0
	withOffset.Offset = &zero
	if fetchMemoKey(base) != fetchMemoKey(withOffset) {
		t.Error("the offset is not part of the key: every window of one fetch shares it")
	}
	for name, mutate := range map[string]func(*crawlRequest){
		"url":              func(r *crawlRequest) { r.URL = "https://x.test/other" },
		"removeSelectors":  func(r *crawlRequest) { r.RemoveSelectors = []string{"footer"} },
		"removeAttributes": func(r *crawlRequest) { r.RemoveAttributes = nil },
		"maxAttributeLen":  func(r *crawlRequest) { r.MaxAttributeLength = 100 },
		"ignoreAttributes": func(r *crawlRequest) { r.IgnoreAttributesForMaxLength = nil },
	} {
		other := base
		mutate(&other)
		if fetchMemoKey(base) == fetchMemoKey(other) {
			t.Errorf("a different %s must give a different key", name)
		}
	}
}

func TestFetchMemoExpiresAndIsBounded(t *testing.T) {
	resetFetchMemo()
	t.Cleanup(resetFetchMemo)
	now := time.Now()
	fetchMemoNow = func() time.Time { return now }
	t.Cleanup(func() { fetchMemoNow = time.Now })

	fetchMemoStore("a", crawlResponse{HTML: "A"})
	if r, ok := fetchMemoLookup("a"); !ok || r.HTML != "A" {
		t.Fatalf("fresh entry: %v %v", r, ok)
	}
	now = now.Add(fetchCacheTTL + time.Second)
	if _, ok := fetchMemoLookup("a"); ok {
		t.Error("an entry older than the TTL must not be served")
	}

	for i := 0; i < fetchMemoMaxEntries+3; i++ {
		now = now.Add(time.Second)
		fetchMemoStore(string(rune('a'+i)), crawlResponse{HTML: "x"})
	}
	fetchMemoMu.Lock()
	n := len(fetchMemoEntries)
	fetchMemoMu.Unlock()
	if n != fetchMemoMaxEntries {
		t.Errorf("entries = %d, want the bound %d", n, fetchMemoMaxEntries)
	}
	if _, ok := fetchMemoLookup("a"); ok {
		t.Error("the oldest entry should have been evicted")
	}
	if _, ok := fetchMemoLookup(string(rune('a' + fetchMemoMaxEntries + 2))); !ok {
		t.Error("the newest entry must still be there")
	}
}

// stubFetch replaces the real browser fetch and counts how often it runs.
func stubFetch(t *testing.T, html string) *int {
	t.Helper()
	resetFetchMemo()
	t.Cleanup(resetFetchMemo)
	calls := 0
	old := crawlPageFn
	crawlPageFn = func(_ context.Context, rawURL, _ string, _, _, _ []string, _ int, _ []string) (crawlResponse, error) {
		calls++
		return crawlResponse{HTML: html, Title: "T", FinalURL: rawURL}, nil
	}
	t.Cleanup(func() { crawlPageFn = old })
	return &calls
}

func crawlViaHandler(t *testing.T, body string) (int, crawlResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	CrawlHandler(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	var resp crawlResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, resp
}

const windowURL = `http://93.184.216.34/` // an IP literal: the URL guard needs no DNS lookup

func TestWithoutOffsetTheResponseIsTheWholePageAsBefore(t *testing.T) {
	stubFetch(t, strings.Repeat("a", 50_000))
	code, resp := crawlViaHandler(t, `{"url":"`+windowURL+`","maxAttributeLength":200}`)
	if code != 200 || len(resp.HTML) != 50_000 || resp.Window != nil {
		t.Fatalf("no offset must change nothing: %d, %d chars, window %v", code, len(resp.HTML), resp.Window)
	}
}

func TestOffsetWindowsComeFromOneNavigation(t *testing.T) {
	calls := stubFetch(t, strings.Repeat("a", 40_000))
	opts := `"url":"` + windowURL + `","maxAttributeLength":200`

	// The agent's usual first call (no offset) is what the windows reuse.
	if code, _ := crawlViaHandler(t, `{`+opts+`}`); code != 200 {
		t.Fatal(code)
	}
	code, w1 := crawlViaHandler(t, `{`+opts+`,"offset":0}`)
	if code != 200 || w1.Window == nil || *w1.Window != (htmlWindow{0, fetchWindowChars, 40_000}) || len(w1.HTML) != fetchWindowChars {
		t.Fatalf("window 1: %d %+v", code, w1.Window)
	}
	_, w2 := crawlViaHandler(t, `{`+opts+`,"offset":18000}`)
	if w2.Window == nil || *w2.Window != (htmlWindow{18_000, 36_000, 40_000}) {
		t.Fatalf("window 2: %+v", w2.Window)
	}
	_, w3 := crawlViaHandler(t, `{`+opts+`,"offset":36000}`)
	if w3.Window == nil || *w3.Window != (htmlWindow{36_000, 40_000, 40_000}) || len(w3.HTML) != 4_000 {
		t.Fatalf("window 3: %+v", w3.Window)
	}
	if *calls != 1 {
		t.Errorf("navigations = %d, want 1 for the first call and all its windows", *calls)
	}
}

func TestOffsetWithDifferentOptionsFetchesAgain(t *testing.T) {
	calls := stubFetch(t, strings.Repeat("a", 30_000))
	crawlViaHandler(t, `{"url":"`+windowURL+`","maxAttributeLength":200}`)
	crawlViaHandler(t, `{"url":"`+windowURL+`","maxAttributeLength":200,"removeSelectors":["header"],"offset":18000}`)
	if *calls != 2 {
		t.Errorf("other strip options give other HTML, so another fetch: calls = %d", *calls)
	}
}

func TestOffsetWithNothingKeptFetchesAndStillWindows(t *testing.T) {
	calls := stubFetch(t, strings.Repeat("a", 30_000))
	code, resp := crawlViaHandler(t, `{"url":"`+windowURL+`","offset":18000}`)
	if code != 200 || *calls != 1 || resp.Window == nil || resp.Window.Start != 18_000 {
		t.Fatalf("%d calls=%d window=%v", code, *calls, resp.Window)
	}
}

func TestAnOffsetPastTheEndIsAnEmptyWindowNotAnError(t *testing.T) {
	stubFetch(t, strings.Repeat("a", 1_000))
	code, resp := crawlViaHandler(t, `{"url":"`+windowURL+`","offset":5000}`)
	if code != 200 || resp.HTML != "" || resp.Window == nil || *resp.Window != (htmlWindow{1_000, 1_000, 1_000}) {
		t.Fatalf("%d %q %+v", code, resp.HTML, resp.Window)
	}
}

func TestANegativeOffsetIsRejected(t *testing.T) {
	calls := stubFetch(t, "x")
	if code, _ := crawlViaHandler(t, `{"url":"`+windowURL+`","offset":-1}`); code != http.StatusBadRequest || *calls != 0 {
		t.Errorf("status %d, fetches %d", code, *calls)
	}
}

// Career's own calls carry a requestId: they never use or fill the copies.
func TestCallsWithARequestIDNeverTouchTheCopies(t *testing.T) {
	calls := stubFetch(t, strings.Repeat("a", 30_000))
	crawlViaHandler(t, `{"url":"`+windowURL+`","requestId":"r1"}`)
	fetchMemoMu.Lock()
	n := len(fetchMemoEntries)
	fetchMemoMu.Unlock()
	if n != 0 {
		t.Errorf("a request-id call must not store a copy, %d stored", n)
	}
	crawlViaHandler(t, `{"url":"`+windowURL+`","requestId":"r2","offset":0}`)
	if *calls != 2 {
		t.Errorf("fetches = %d, want a fresh one each time", *calls)
	}
}

func TestFetchPageText(t *testing.T) {
	short := fetchPageText(crawlResponse{HTML: "<p>hi</p>", Title: "T", FinalURL: "https://x/"})
	if strings.Contains(short, "characters") || !strings.HasSuffix(short, "<p>hi</p>") || !strings.HasPrefix(short, "Title: T\nURL: https://x/\n") {
		t.Errorf("a short page reads as before: %q", short)
	}

	long := fetchPageText(crawlResponse{HTML: strings.Repeat("a", 50_000), Title: "T", FinalURL: "https://x/"})
	head := long[:strings.Index(long, "aaaa")]
	if !strings.Contains(head, "50000 characters long") || !strings.Contains(head, "offset=18000") {
		t.Errorf("a long page read without offset must say how to read on, in the head that stays visible: %q", head)
	}

	mid := fetchPageText(crawlResponse{HTML: "x", Window: &htmlWindow{18_000, 36_000, 50_000}})
	if !strings.Contains(mid, "Showing characters 18000-36000 of 50000") || !strings.Contains(mid, "offset=36000") {
		t.Errorf("middle window: %q", mid)
	}
	last := fetchPageText(crawlResponse{HTML: "x", Window: &htmlWindow{36_000, 50_000, 50_000}})
	if !strings.Contains(last, "the end of the page") || strings.Contains(last, "offset=") {
		t.Errorf("last window must say it is the end and give no next offset: %q", last)
	}
	past := fetchPageText(crawlResponse{HTML: "", Window: &htmlWindow{50_000, 50_000, 50_000}})
	if !strings.Contains(past, "past the end") {
		t.Errorf("past the end: %q", past)
	}
}

func TestTheFetchToolDescriptionAndSchemaMentionOffset(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterFetchPageHTML(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 1 {
		t.Fatalf("tools: %v %v", tools, err)
	}
	tool := tools.Tools[0]
	if !strings.Contains(tool.Description, "offset") || !strings.Contains(tool.Description, "find_repeating_items") {
		t.Error("the tool description must tell the model about offset and the listing tool")
	}
	raw, _ := json.Marshal(tool.InputSchema)
	if !strings.Contains(string(raw), `"offset"`) {
		t.Errorf("the input schema must offer offset: %s", raw)
	}
}

// TestLiveFetchWindows walks a live page through the real handler and reports
// where the first job card appears, as evidence that the windows reach content
// the first 20,000 characters hide. Skipped unless FETCH_WINDOW_LIVE_URL is set.
func TestLiveFetchWindows(t *testing.T) {
	url := os.Getenv("FETCH_WINDOW_LIVE_URL")
	if url == "" {
		t.Skip("set FETCH_WINDOW_LIVE_URL to run against a live page")
	}
	if _, err := shared.ChromePath(); err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	useBrowserSettings(t, shared.FetchEngineRod) // the real default engine, with a temp database
	resetFetchMemo()
	t.Cleanup(func() { shared.StopSharedSession(); StopRodEngine(); resetFetchMemo() })
	opts := `"url":"` + url + `","maxAttributeLength":200`
	code, first := crawlViaHandler(t, `{`+opts+`}`)
	if code != 200 {
		t.Fatalf("first fetch: %d", code)
	}
	t.Logf("first fetch (no offset): %d characters of HTML", len([]rune(first.HTML)))
	marker := `data-testid="job-item"`
	for offset := 0; ; {
		code, w := crawlViaHandler(t, fmt.Sprintf(`{%s,"offset":%d}`, opts, offset))
		if code != 200 || w.Window == nil {
			t.Fatalf("offset %d: %d", offset, code)
		}
		if i := strings.Index(w.HTML, marker); i >= 0 {
			t.Logf("first %q is in the window %d-%d of %d (at character %d)", marker, w.Window.Start, w.Window.End, w.Window.Total, w.Window.Start+len([]rune(w.HTML[:i])))
			return
		}
		if w.Window.End >= w.Window.Total {
			t.Logf("marker not found in any window (total %d)", w.Window.Total)
			return
		}
		offset = w.Window.End
	}
}
