// page_snapshot_store.go (plan 247, step 1) — where the page copies live and how they go away.
//
// One folder (shared.PageSnapshotsDir, under TOOL_CACHE_DIR), one <id>.html and one <id>.json per snapshot. The id is 16 random bytes in hex and the ONLY
// thing that ever becomes part of a file name, checked against snapshotIDRe before every use. A snapshot goes away when the picker closes it (DELETE
// /page-snapshot), when the person presses "Delete temporary files" (DELETE /page-snapshots), and by an age sweep: 60 minutes, 15 for a page read with a
// kept login (it may hold personal data). Nothing here is needed for anything else: the folder can be emptied at any time.
package crawler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"browser-tool-backend/shared"
)

const (
	snapshotMaxAge      = 60 * time.Minute
	snapshotMaxAgeLogin = 15 * time.Minute
	snapshotSweepEvery  = 10 * time.Minute
)

// snapshotMu serialises the changes of the page copy folder: making a copy (which removes the others), the removal of one, and a clear of the whole folder.
var snapshotMu sync.Mutex

var (
	snapshotIDRe   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	snapshotFileRe = regexp.MustCompile(`^([0-9a-f]{32})\.(html|json)$`)
)

// snapshotInfo is what /fetch-links answers about the copy it made.
type snapshotInfo struct {
	ID        string `json:"id"`
	Bytes     int    `json:"bytes"`
	CreatedAt string `json:"createdAt"`
	Truncated bool   `json:"truncated,omitempty"`
}

type snapshotMeta struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	FinalURL  string `json:"finalUrl"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	Bytes     int    `json:"bytes"`
	Login     bool   `json:"login,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

func snapshotDir() (string, error) {
	if shared.PageSnapshotsDir == "" {
		return "", errors.New("the page snapshot folder is not set up")
	}
	if err := os.MkdirAll(shared.PageSnapshotsDir, 0o700); err != nil {
		return "", err
	}
	return shared.PageSnapshotsDir, nil
}

func newSnapshotID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// writeSnapshot stores the copy and its meta and returns what /fetch-links reports.
func writeSnapshot(htmlText string, meta snapshotMeta) (snapshotInfo, error) {
	dir, err := snapshotDir()
	if err != nil {
		return snapshotInfo{}, err
	}
	id, err := newSnapshotID()
	if err != nil {
		return snapshotInfo{}, err
	}
	snapshotMu.Lock()
	defer snapshotMu.Unlock()
	meta.ID, meta.Bytes, meta.CreatedAt = id, len(htmlText), time.Now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(filepath.Join(dir, id+".html"), []byte(htmlText), 0o600); err != nil {
		return snapshotInfo{}, err
	}
	b, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o600); err != nil {
		_ = os.Remove(filepath.Join(dir, id+".html"))
		return snapshotInfo{}, err
	}
	// Plan 248: one copy at a time. The new copy is written first, so a failed write never leaves the person with none; every other copy goes now. A view
	// that is already open keeps the page it loaded in memory, so only a reload of an old view finds its copy gone.
	removeOtherSnapshots(dir, id)
	return snapshotInfo{ID: id, Bytes: meta.Bytes, CreatedAt: meta.CreatedAt, Truncated: meta.Truncated}, nil
}

// removeOtherSnapshots deletes every snapshot file in dir except the two of the snapshot keep (and nothing that is not named like a snapshot file).
func removeOtherSnapshots(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		m := snapshotFileRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() || m[1] == keep {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
}

// deleteSnapshot removes one copy; a missing one is fine.
func deleteSnapshot(id string) error {
	if !snapshotIDRe.MatchString(id) {
		return fmt.Errorf("not a snapshot id")
	}
	dir, err := snapshotDir()
	if err != nil {
		return err
	}
	for _, ext := range []string{".html", ".json"} {
		if err := os.Remove(filepath.Join(dir, id+ext)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// snapshotStats counts the copies and their size (the .html files).
func snapshotStats() (count int, bytes int64, err error) {
	dir, err := snapshotDir()
	if err != nil {
		return 0, 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, err
	}
	for _, e := range entries {
		m := snapshotFileRe.FindStringSubmatch(e.Name())
		if m == nil || m[2] != "html" || e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil {
			count++
			bytes += info.Size()
		}
	}
	return count, bytes, nil
}

// deleteAllSnapshots empties the folder of the files this feature wrote (names matching snapshotFileRe) and returns how many copies went.
func deleteAllSnapshots() (int, error) {
	dir, err := snapshotDir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, e := range entries {
		m := snapshotFileRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return deleted, err
		}
		if m[2] == "html" {
			deleted++
		}
	}
	return deleted, nil
}

// sweepSnapshots removes the files older than their limit (by modification time) and returns how many copies went.
func sweepSnapshots(now time.Time) (int, error) {
	dir, err := snapshotDir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		m := snapshotFileRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		limit := snapshotMaxAge
		if b, err := os.ReadFile(filepath.Join(dir, m[1]+".json")); err == nil {
			var meta snapshotMeta
			if json.Unmarshal(b, &meta) == nil && meta.Login {
				limit = snapshotMaxAgeLogin
			}
		}
		if now.Sub(info.ModTime()) < limit {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil && m[2] == "html" {
			removed++
		}
	}
	return removed, nil
}

// StartSnapshotSweeper sweeps once now and then every snapshotSweepEvery until ctx ends.
func StartSnapshotSweeper(ctx context.Context) {
	_, _ = sweepSnapshots(time.Now())
	go func() {
		t := time.NewTicker(snapshotSweepEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				_, _ = sweepSnapshots(now)
			}
		}
	}()
}

func writeSnapshotJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// PageSnapshotHandler handles GET /page-snapshot?id= (the html) and DELETE /page-snapshot?id= (204, also when it is gone).
func PageSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !snapshotIDRe.MatchString(id) {
		http.Error(w, "id must be a snapshot id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		dir, err := snapshotDir()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f, err := os.Open(filepath.Join(dir, id+".html"))
		if err != nil {
			http.Error(w, "no such snapshot (it may have been deleted)", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "sandbox; "+snapshotCSP)
		_, _ = io.Copy(w, f)
	case http.MethodDelete:
		if err := deleteSnapshot(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// PageSnapshotsHandler handles GET /page-snapshots ({count, bytes}) and DELETE /page-snapshots ({deleted}).
func PageSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		count, bytes, err := snapshotStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeSnapshotJSON(w, http.StatusOK, map[string]any{"count": count, "bytes": bytes})
	case http.MethodDelete:
		n, err := deleteAllSnapshots()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeSnapshotJSON(w, http.StatusOK, map[string]any{"deleted": n})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
