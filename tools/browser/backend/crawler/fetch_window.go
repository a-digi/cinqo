// fetch_window.go (step 94) lets fetch_page_html be read in parts: an optional
// offset returns a window of the page's HTML, so an agent whose tool results
// are capped (the conversation layer keeps only the first 20,000 characters of
// each) can walk through a long page instead of guessing selectors to strip.
//
// The parts come from a short-lived in-memory copy of the last AI fetch, keyed
// by the URL AND the strip options (identical options give identical HTML, so
// no HTML parsing is needed — which is why fetch_cache.go, keyed by URL alone,
// cannot serve this). A request without offset behaves exactly as before and
// never reads the copy. See
// plan/ai/tools/browser/step-94-find-repeating-items.md.
package crawler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// fetchWindowChars is the size of one window. Below the conversation layer's
// 20,000-character cap, so the text around it is never what gets cut.
const fetchWindowChars = 18_000

// fetchMemoMaxEntries bounds the in-memory copies: at most this many pages of
// up to MaxHTMLBytes each.
const fetchMemoMaxEntries = 8

// htmlWindow says which part of the page's HTML a response carries.
// Start/End/Total count characters (runes), not bytes.
type htmlWindow struct {
	Start int `json:"start"`
	End   int `json:"end"`
	Total int `json:"total"`
}

// windowHTML returns the characters [offset, offset+fetchWindowChars) of html.
// An offset at or past the end yields an empty window at the end.
func windowHTML(html string, offset int) (string, htmlWindow) {
	runes := []rune(html)
	total := len(runes)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + fetchWindowChars
	if end > total {
		end = total
	}
	return string(runes[offset:end]), htmlWindow{Start: offset, End: end, Total: total}
}

type fetchMemoEntry struct {
	resp crawlResponse
	at   time.Time
}

var (
	fetchMemoMu      sync.Mutex
	fetchMemoEntries = map[string]fetchMemoEntry{}
	// fetchMemoNow is the clock; a var so tests can move it.
	fetchMemoNow = time.Now
)

// fetchMemoKey identifies a fetch by everything that shapes its HTML.
func fetchMemoKey(req crawlRequest) string {
	raw, _ := json.Marshal([]any{req.URL, req.RemoveSelectors, req.RemoveAttributes, req.MaxAttributeLength, req.IgnoreAttributesForMaxLength})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// fetchMemoStore keeps resp for fetchCacheTTL, evicting the oldest entry when
// full.
func fetchMemoStore(key string, resp crawlResponse) {
	fetchMemoMu.Lock()
	defer fetchMemoMu.Unlock()
	now := fetchMemoNow()
	for k, e := range fetchMemoEntries {
		if now.Sub(e.at) > fetchCacheTTL {
			delete(fetchMemoEntries, k)
		}
	}
	if _, exists := fetchMemoEntries[key]; !exists && len(fetchMemoEntries) >= fetchMemoMaxEntries {
		oldestKey, oldest := "", now
		for k, e := range fetchMemoEntries {
			if oldestKey == "" || e.at.Before(oldest) {
				oldestKey, oldest = k, e.at
			}
		}
		delete(fetchMemoEntries, oldestKey)
	}
	fetchMemoEntries[key] = fetchMemoEntry{resp: resp, at: now}
}

// fetchMemoLookup returns the kept copy while it is still fresh.
func fetchMemoLookup(key string) (crawlResponse, bool) {
	fetchMemoMu.Lock()
	defer fetchMemoMu.Unlock()
	e, ok := fetchMemoEntries[key]
	if !ok {
		return crawlResponse{}, false
	}
	if fetchMemoNow().Sub(e.at) > fetchCacheTTL {
		delete(fetchMemoEntries, key)
		return crawlResponse{}, false
	}
	return e.resp, true
}

// resetFetchMemo empties the copies (tests).
func resetFetchMemo() {
	fetchMemoMu.Lock()
	defer fetchMemoMu.Unlock()
	fetchMemoEntries = map[string]fetchMemoEntry{}
}
