// temp_files.go (plan 248, step 1) — the Browser tool's temporary files folder: what is in it, and how it is cleared.
//
// The folder is the tool's cache folder (shared.CacheRoot, the TOOL_CACHE_DIR Cinqo gives the tool): page copies (plan 247), the 5-minute page cache
// and the profile folders of the headless Chromes. It holds only what the tool can make again. It is cleared with no person asked, at every start of
// the tool, at a clean stop and whenever a new page copy is made (the other copies go), and a person can clear it from Browser & Engine.
//
// Safety, all of it in this one place:
//   - only shared.CacheRoot is ever touched, a path the tool resolved at start; no route takes a path or a name;
//   - nothing is followed through a symbolic link: a link is removed as a link (os.RemoveAll) or measured as a link (WalkDir uses Lstat);
//   - the Chrome ledger is never deleted, and neither is the profile folder of a Chrome that is running (or about to be registered), so a crawl in
//     progress is never broken; the logs, the database and the visible window's profile are outside the folder.
package crawler

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"browser-tool-backend/shared"
	"browser-tool-backend/shared/chromeproc"
)

// tempPart is one part of the folder as Browser & Engine lists it.
type tempPart struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
	// InUse counts the profile folders of running Chromes (only for the profiles part).
	InUse int `json:"inUse,omitempty"`
}

// tempInventory is the answer of GET /temp-files: the size of the whole folder and its parts.
type tempInventory struct {
	Bytes int64      `json:"bytes"`
	Files int        `json:"files"`
	Parts []tempPart `json:"parts"`
}

// tempClearResult is the answer of DELETE /temp-files.
type tempClearResult struct {
	DeletedBytes int64 `json:"deletedBytes"`
	DeletedFiles int   `json:"deletedFiles"`
	// KeptBytes is what is in the folder after the delete (the ledger and the profiles of running Chromes).
	KeptBytes  int64  `json:"keptBytes"`
	KeptReason string `json:"keptReason"`
}

// tempRoot is the folder, or "" when it is not set up (nothing then is touched).
func tempRoot() string {
	root := shared.CacheRoot
	if root == "" || !filepath.IsAbs(root) || filepath.Dir(root) == root {
		return ""
	}
	return root
}

// within says whether path is root or inside it (by path; links are not resolved here, nothing is followed anywhere in this file).
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// measure sums the sizes of the files below dir and counts them; links are not followed, and a folder that is missing is empty.
func measure(dir string) (bytes int64, files int) {
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, ierr := d.Info(); ierr == nil {
			bytes += info.Size()
			files++
		}
		return nil
	})
	return bytes, files
}

// removeInside deletes every entry directly inside dir for which keep says false, and returns what went. An entry that cannot be removed stays
// (it is counted by the next measurement of what is kept).
func removeInside(dir string, keep func(name string, isDir bool) bool) (bytes int64, files int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if keep != nil && keep(e.Name(), e.IsDir()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, f := measure(path)
		if e.Type()&fs.ModeSymlink != 0 { // a link: measured as nothing, removed as a link
			b, f = 0, 1
		}
		if err := os.RemoveAll(path); err == nil {
			bytes += b
			files += f
		}
	}
	return bytes, files
}

// inventoryTempFiles measures the folder and its parts.
func inventoryTempFiles() tempInventory {
	root := tempRoot()
	inv := tempInventory{Parts: []tempPart{}}
	if root == "" {
		return inv
	}
	inv.Bytes, inv.Files = measure(root)
	pcBytes, pcFiles := measure(shared.PageSnapshotsDir)
	cacheBytes, cacheFiles := measure(shared.FetchCacheDir)
	var profBytes int64
	var profFiles int
	if within(root, chromeproc.ProfileRoot()) {
		profBytes, profFiles = measure(chromeproc.ProfileRoot())
	}
	inUse := 0
	for _, f := range chromeproc.ProfileFolders() {
		if within(root, f.Path) && f.InUse {
			inUse++
		}
	}
	inv.Parts = append(inv.Parts,
		tempPart{ID: "page_copies", Label: "Page copies", Bytes: pcBytes, Files: pcFiles},
		tempPart{ID: "page_cache", Label: "Page cache", Bytes: cacheBytes, Files: cacheFiles},
		tempPart{ID: "chrome_profiles", Label: "Browser profiles", Bytes: profBytes, Files: profFiles, InUse: inUse},
	)
	if ob, of := inv.Bytes-pcBytes-cacheBytes-profBytes, inv.Files-pcFiles-cacheFiles-profFiles; ob > 0 || of > 0 {
		inv.Parts = append(inv.Parts, tempPart{ID: "other", Label: "Other", Bytes: ob, Files: of})
	}
	return inv
}

// ClearTemporaryFiles deletes everything in the temporary files folder that is not in use and reports what it did. It is safe to call at any time:
// the page cache is cleared under its own lock (a lookup then simply misses), the ledger and the profiles of running Chromes stay.
func ClearTemporaryFiles() tempClearResult {
	root := tempRoot()
	res := tempClearResult{}
	if root == "" {
		return res
	}
	add := func(b int64, f int) { res.DeletedBytes += b; res.DeletedFiles += f }

	snapshotMu.Lock()
	add(removeInside(shared.PageSnapshotsDir, nil))
	snapshotMu.Unlock()

	fetchCacheMu.Lock()
	add(removeInside(shared.FetchCacheDir, nil))
	fetchCacheMu.Unlock()

	running := 0
	for _, f := range chromeproc.ProfileFolders() {
		if !within(root, f.Path) {
			continue // not this folder's to touch
		}
		if f.InUse {
			running++
			continue
		}
		b, n := measure(f.Path)
		if chromeproc.RemoveProfileDir(f.Path) == nil {
			add(b, n)
		}
	}

	// everything else directly in the folder, and in the Chrome folder besides the ledger and the profiles
	ledgerDir := chromeproc.LedgerDir()
	if !within(root, ledgerDir) || ledgerDir == root {
		ledgerDir = "" // a ledger folder outside (or being) the temporary folder is not cleaned here
	}
	profileRoot := chromeproc.ProfileRoot()
	known := map[string]bool{shared.FetchCacheDir: true, shared.PageSnapshotsDir: true}
	add(removeInside(root, func(name string, _ bool) bool {
		path := filepath.Join(root, name)
		return known[path] || (ledgerDir != "" && path == ledgerDir)
	}))
	if ledgerDir != "" {
		add(removeInside(ledgerDir, func(name string, _ bool) bool {
			return chromeproc.IsLedgerFile(name) || filepath.Join(ledgerDir, name) == profileRoot
		}))
	}

	res.KeptBytes, _ = measure(root)
	if running > 0 {
		res.KeptReason = fmt.Sprintf("%d browser session(s) are running and keep what they use", running)
	}
	return res
}

// TempFilesHandler handles GET /temp-files (the size of the folder and its parts) and DELETE /temp-files (clear it).
func TempFilesHandler(w http.ResponseWriter, r *http.Request) {
	var v any
	switch r.Method {
	case http.MethodGet:
		v = inventoryTempFiles()
	case http.MethodDelete:
		v = ClearTemporaryFiles()
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// SummarizeTempClear is the one log line of an automatic clear ("" when there was nothing to delete).
func SummarizeTempClear(why string, r tempClearResult) string {
	if r.DeletedFiles == 0 {
		return ""
	}
	return fmt.Sprintf("temporary files cleared %s: %d file(s), %.1f MB", why, r.DeletedFiles, float64(r.DeletedBytes)/1024/1024)
}
