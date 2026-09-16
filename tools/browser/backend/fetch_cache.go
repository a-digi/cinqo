// fetch_cache.go implements a short-lived, on-disk cache of
// fetch_page_html's own results — a repeated call for the same URL
// within fetchCacheTTL, made without the shared session having
// navigated anywhere else in between, is served from disk instead of
// re-navigating. Wired into crawlPage (crawl.go). Deliberately keyed
// by the URL alone — a call with non-default removeSelectors bypasses
// this cache entirely, never reads or writes it, since correctly
// honoring arbitrary caller-supplied strip rules on a cache hit would
// need an HTML-parsing dependency this module doesn't have, for a
// case real AI-driven calls essentially never hit. See
// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md.
package main

import (
	"crypto/md5" //nolint:gosec // cache key, not a security boundary — no collision-resistance requirement
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// fetchCacheTTL bounds how long a cached fetch_page_html result stays
// fresh — a plain constant, matching this codebase's own established
// "plain constants over premature configurability" convention (e.g.
// crawl.go's own maxHTMLBytes/crawlTimeout).
const fetchCacheTTL = 5 * time.Minute

// fetchCacheMaxEntries bounds this cache's own growth — mirrors
// crawl_logs' own maxCrawlLogEntries (step 39) exactly in spirit: a
// URL fetched once and never fetched again would otherwise keep its
// own .cache file on disk forever, since fetchCacheStore's own
// superseded-file cleanup only ever fires on the *next* store for the
// *same* URL. This is a short-lived optimization cache, not a record
// anyone needs kept past its own fetchCacheTTL, let alone
// indefinitely. See
// plan/ai/tools/browser/step-45-fetch-html-caching-plan.md's own Step
// 7.
const fetchCacheMaxEntries = 200

// fetchCacheIndexEntry is one manifest row, keyed by md5(url) in the
// index map itself — File is a bare filename (no directory), relative
// to fetchCacheDir, and holds only the HTML itself (per this
// feature's own spec: "cache the HTML in a {file}.cache"). Title/
// FinalURL/Truncated are the small remainder of crawlResponse that
// can't be re-derived from the HTML alone once it's read back off
// disk, so they live here instead of inside the .cache file.
type fetchCacheIndexEntry struct {
	File string `json:"file"`
	// CachedAt is nanosecond-precision (time.Now().UnixNano()) —
	// deliberately not the same second-precision value the .cache
	// file's own name embeds (this feature's own spec calls for a
	// plain unix-seconds timestamp there): two entries stored within
	// the same second would otherwise tie under pruneFetchCacheBeyondLimit's
	// own "keep the N most recent" ordering, and Go map iteration order
	// (which feeds that sort) is unspecified, making a tie's own
	// outcome arbitrary rather than actually oldest-first. Caught
	// directly by a real test with 201 rapid stores, not assumed.
	CachedAt  int64  `json:"cachedAt"`
	Title     string `json:"title"`
	FinalURL  string `json:"finalUrl"`
	Truncated bool   `json:"truncated"`
}

// fetchCacheMu guards the one shared manifest file every
// fetch_page_html call reads/writes — a single mutex, not per-key
// locking: this cache's own write volume (one entry per distinct URL
// fetched, at most once every fetchCacheTTL) is tiny, same reasoning
// login_credentials.go's own single sqlite connection already
// documents for browser.db.
var fetchCacheMu sync.Mutex

func fetchCacheIndexPath() string {
	return filepath.Join(fetchCacheDir, "index.json")
}

// fetchCacheKey is the manifest key and the first half of a stored
// file's own name — never a security boundary, purely a stable,
// fixed-length identifier for an arbitrary URL string.
func fetchCacheKey(url string) string {
	sum := md5.Sum([]byte(url)) //nolint:gosec // see package doc comment
	return hex.EncodeToString(sum[:])
}

// readFetchCacheIndex reads the manifest, tolerating a missing file
// (first run, or every cache file having been pruned externally) as
// an empty index rather than an error.
func readFetchCacheIndex() (map[string]fetchCacheIndexEntry, error) {
	data, err := os.ReadFile(fetchCacheIndexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]fetchCacheIndexEntry{}, nil
		}
		return nil, err
	}
	var index map[string]fetchCacheIndexEntry
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}
	if index == nil {
		index = map[string]fetchCacheIndexEntry{}
	}
	return index, nil
}

// writeFetchCacheIndex writes the manifest atomically — a temp file +
// rename, so a crash/kill mid-write can never leave index.json
// truncated/corrupted and strand every existing entry, not just the
// one currently being updated.
func writeFetchCacheIndex(index map[string]fetchCacheIndexEntry) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := fetchCacheIndexPath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, fetchCacheIndexPath())
}

// fetchCacheLookup returns the cached crawlResponse for url and true
// if a fresh (< fetchCacheTTL) entry exists; (zero value, false) for a
// cache miss, an expired entry, or any read failure along the way — a
// broken cache is treated identically to "no cache" in every case,
// since this is purely an optimization, never a correctness
// requirement: the caller always falls back to a real fetch.
func fetchCacheLookup(url string) (crawlResponse, bool) {
	fetchCacheMu.Lock()
	defer fetchCacheMu.Unlock()

	index, err := readFetchCacheIndex()
	if err != nil {
		return crawlResponse{}, false
	}
	entry, ok := index[fetchCacheKey(url)]
	if !ok {
		return crawlResponse{}, false
	}
	if time.Since(time.Unix(0, entry.CachedAt)) > fetchCacheTTL {
		return crawlResponse{}, false
	}
	html, err := os.ReadFile(filepath.Join(fetchCacheDir, entry.File))
	if err != nil {
		return crawlResponse{}, false
	}
	return crawlResponse{
		HTML:      string(html),
		Title:     entry.Title,
		FinalURL:  entry.FinalURL,
		Truncated: entry.Truncated,
	}, true
}

// fetchCacheStore writes a fresh cache entry for url, deleting the
// on-disk file for whatever was previously cached under the same
// url's key first. Without that cleanup, every re-fetch of the same
// URL would leave a permanently growing pile of stale .cache files
// behind, since each write's own filename embeds the current unix
// timestamp (per this feature's own spec, so a cache file is
// self-describing/inspectable without opening index.json at all).
// Returns an error, but every real call site treats a failure here as
// best-effort — the caller already has its real, fresh HTML
// regardless of whether caching it for next time succeeds.
func fetchCacheStore(url string, resp crawlResponse) error {
	fetchCacheMu.Lock()
	defer fetchCacheMu.Unlock()

	index, err := readFetchCacheIndex()
	if err != nil {
		index = map[string]fetchCacheIndexEntry{}
	}

	key := fetchCacheKey(url)
	now := time.Now()
	fileName := fmt.Sprintf("%s_%d.cache", key, now.Unix())

	if previous, ok := index[key]; ok && previous.File != fileName {
		_ = os.Remove(filepath.Join(fetchCacheDir, previous.File))
	}

	if err := os.WriteFile(filepath.Join(fetchCacheDir, fileName), []byte(resp.HTML), 0o644); err != nil {
		return err
	}

	index[key] = fetchCacheIndexEntry{
		File:      fileName,
		CachedAt:  now.UnixNano(),
		Title:     resp.Title,
		FinalURL:  resp.FinalURL,
		Truncated: resp.Truncated,
	}

	prunedFiles := pruneFetchCacheBeyondLimit(index)

	if err := writeFetchCacheIndex(index); err != nil {
		return err
	}
	fetchCacheDeleteFilesAsync(prunedFiles)
	return nil
}

// pruneFetchCacheBeyondLimit removes the oldest entries (by CachedAt)
// once index holds more than fetchCacheMaxEntries — mirrors
// crawl_logs' own crawlLogPathsBeyondLimit, just operating on an
// in-memory map instead of a SQL query, since this cache's own
// manifest is a JSON file, not a table. Mutates index in place
// (removing the pruned keys) and returns the filenames removed, for
// the caller to delete asynchronously — called only from within
// fetchCacheStore, which already holds fetchCacheMu.
func pruneFetchCacheBeyondLimit(index map[string]fetchCacheIndexEntry) []string {
	if len(index) <= fetchCacheMaxEntries {
		return nil
	}

	type keyedEntry struct {
		key   string
		entry fetchCacheIndexEntry
	}
	entries := make([]keyedEntry, 0, len(index))
	for k, e := range index {
		entries = append(entries, keyedEntry{k, e})
	}
	// Newest first, so entries[fetchCacheMaxEntries:] is exactly the
	// oldest excess — same "keep the N most recent" shape
	// crawlLogPathsBeyondLimit's own ORDER BY created_at DESC LIMIT ?
	// already uses.
	sort.Slice(entries, func(i, j int) bool { return entries[i].entry.CachedAt > entries[j].entry.CachedAt })

	removedFiles := make([]string, 0, len(entries)-fetchCacheMaxEntries)
	for _, e := range entries[fetchCacheMaxEntries:] {
		removedFiles = append(removedFiles, e.entry.File)
		delete(index, e.key)
	}
	return removedFiles
}

// fetchCacheDeleteFilesAsync mirrors deleteCrawlLogFilesAsync
// (crawl_log.go) exactly — never blocks the caller (index.json is
// already rewritten without these entries by the time this is
// called), and a slow or failed filesystem delete must never turn an
// otherwise-successful crawl into a failure.
func fetchCacheDeleteFilesAsync(fileNames []string) {
	if len(fileNames) == 0 {
		return
	}
	go func(fileNames []string) {
		for _, name := range fileNames {
			_ = os.Remove(filepath.Join(fetchCacheDir, name))
		}
	}(fileNames)
}
