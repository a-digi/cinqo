package convert

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// pdfCacheMaxEntries is the only bound this cache needs — it is
// content-addressed (keyed by the source PDF's own MD5), so a given
// key can never map to different content over time and there is
// nothing for a cached entry to expire against. See
// plan/ai/tools/pdf-tools/step-03-content-addressed-cache.md's own
// "Why no TTL" section. Plain constant, not user-configurable, same
// convention as fetch_cache.go's own maxEntries.
const pdfCacheMaxEntries = 500

// resolvedCacheDir is set once by Init, mirroring
// tools/browser/backend/shared/db.go's own InitDB()-sets-a-package-level-var
// convention (there: FetchCacheDir).
var resolvedCacheDir string

// pdfCacheMu guards the one shared index.json — write volume is tiny,
// same reasoning fetch_cache.go documents for its own single mutex.
var pdfCacheMu sync.Mutex

type pdfCacheIndexEntry struct {
	File string `json:"file"` // "<md5>.md", relative to resolvedCacheDir
	// CachedAt is set once, on write, and never bumped on a lookup hit
	// — eviction order is therefore "oldest ever written first", not
	// true least-recently-used. Deliberate simplicity choice, flagged
	// in the design doc.
	CachedAt  int64  `json:"cachedAt"`
	SourceURL string `json:"sourceUrl"` // diagnostic aid only — not part of the key
	Bytes     int    `json:"bytes"`     // size of the source PDF, diagnostic aid only
}

// Init creates this capability's own cache subdirectory inside the
// tool-wide TOOL_CACHE_DIR (<TOOL_CACHE_DIR>/pdf_to_md/) and records it
// for pdfCacheLookup/pdfCacheStore. Must be called once at startup,
// before ToMarkdown is ever called.
func Init(toolCacheDir string) error {
	if toolCacheDir == "" {
		return fmt.Errorf("TOOL_CACHE_DIR is not set")
	}
	dir := filepath.Join(toolCacheDir, "pdf_to_md")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create pdf_to_md cache directory: %w", err)
	}
	resolvedCacheDir = dir
	return nil
}

func pdfCacheIndexPath() string {
	return filepath.Join(resolvedCacheDir, "index.json")
}

// readPDFCacheIndex treats a missing or corrupt index as empty rather
// than a fatal error — same "missing file -> empty map" convention as
// fetch_cache.go, extended here to "unreadable JSON -> empty map" too,
// since this is a pure optimization layer that should never block a
// real conversion.
func readPDFCacheIndex() map[string]pdfCacheIndexEntry {
	data, err := os.ReadFile(pdfCacheIndexPath())
	if err != nil {
		return map[string]pdfCacheIndexEntry{}
	}
	var index map[string]pdfCacheIndexEntry
	if err := json.Unmarshal(data, &index); err != nil {
		return map[string]pdfCacheIndexEntry{}
	}
	return index
}

// writePDFCacheIndex writes atomically (temp file + rename), same
// convention as fetch_cache.go's own index.json write.
func writePDFCacheIndex(index map[string]pdfCacheIndexEntry) error {
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := pdfCacheIndexPath() + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, pdfCacheIndexPath())
}

// pdfCacheLookup returns (markdown, true) on any hit; (\"\", false) on
// a miss or any read failure — no expiry check of any kind, since a
// hit is always fresh by construction (content-addressed key).
// Deliberately does NOT touch index.json on a hit — a lookup is
// read-only, full stop; CachedAt is write-time-only.
func pdfCacheLookup(md5Hex string) (string, bool) {
	pdfCacheMu.Lock()
	defer pdfCacheMu.Unlock()

	entry, ok := readPDFCacheIndex()[md5Hex]
	if !ok {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(resolvedCacheDir, entry.File))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// pdfCacheStore writes <md5Hex>.md into the cache dir, sets
// CachedAt = time.Now().UnixNano() for this entry, updates and
// persists index.json, and prunes beyond pdfCacheMaxEntries. Because
// the filename is deterministically <md5Hex>.md, a re-store for the
// same key is a plain overwrite of byte-identical content (the same
// key can only ever mean the same source bytes) — harmless.
func pdfCacheStore(md5Hex, sourceURL string, sourceBytes int, markdown string) error {
	pdfCacheMu.Lock()
	defer pdfCacheMu.Unlock()

	fileName := md5Hex + ".md"
	if err := os.WriteFile(filepath.Join(resolvedCacheDir, fileName), []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	index := readPDFCacheIndex()
	index[md5Hex] = pdfCacheIndexEntry{
		File:      fileName,
		CachedAt:  time.Now().UnixNano(),
		SourceURL: sourceURL,
		Bytes:     sourceBytes,
	}

	removed := prunePDFCacheBeyondLimit(index)
	if err := writePDFCacheIndex(index); err != nil {
		return fmt.Errorf("failed to write cache index: %w", err)
	}
	deletePDFCacheFilesAsync(removed)
	return nil
}

// prunePDFCacheBeyondLimit keeps the pdfCacheMaxEntries entries with
// the most recent CachedAt, mutates index in place, and returns the
// filenames removed (for the caller to delete from disk).
func prunePDFCacheBeyondLimit(index map[string]pdfCacheIndexEntry) []string {
	if len(index) <= pdfCacheMaxEntries {
		return nil
	}

	type keyedEntry struct {
		key   string
		entry pdfCacheIndexEntry
	}
	entries := make([]keyedEntry, 0, len(index))
	for k, v := range index {
		entries = append(entries, keyedEntry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].entry.CachedAt < entries[j].entry.CachedAt
	})

	excess := len(entries) - pdfCacheMaxEntries
	removed := make([]string, 0, excess)
	for i := 0; i < excess; i++ {
		removed = append(removed, entries[i].entry.File)
		delete(index, entries[i].key)
	}
	return removed
}

// deletePDFCacheFilesAsync fires the actual unlinks in a goroutine,
// never blocking the caller — best-effort, same convention as
// fetch_cache.go's own deleteFilesAsync.
func deletePDFCacheFilesAsync(fileNames []string) {
	if len(fileNames) == 0 {
		return
	}
	go func() {
		for _, name := range fileNames {
			if err := os.Remove(filepath.Join(resolvedCacheDir, name)); err != nil && !os.IsNotExist(err) {
				log.Printf("pdf_tools: failed to remove pruned cache file %s: %v", name, err)
			}
		}
	}()
}
