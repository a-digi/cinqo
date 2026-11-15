// capture_store.go (Namako step 1) — where the evidence captures and the media downloads live, and how they are read back and released.
//
// One folder per capture, shared.CapturesDir/<id>, inside the tool's cache folder (so the temporary-files cleanup covers it). The id is 16 random
// bytes in hex and the ONLY thing a route takes from the caller; a file is addressed as <id> + a name that matches a fixed pattern and is resolved
// inside that folder (no ".." and no links followed). Everything is written with mode 0600/0700 and served only as an attachment of an opaque type.
package crawler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"browser-tool-backend/shared"
)

// capturesQuotaBytes is the most the captures folder may hold (a var so a test can shrink it).
var capturesQuotaBytes int64 = 20 << 30

var (
	captureIDPattern   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	captureNamePattern = regexp.MustCompile(`^((media|segments)/)?[A-Za-z0-9][A-Za-z0-9._-]{0,119}$`)
)

var errCapturesUnavailable = errors.New("the captures folder is not set up")

func capturesRoot() (string, error) {
	if shared.CapturesDir == "" {
		return "", errCapturesUnavailable
	}
	return shared.CapturesDir, nil
}

// newCapture creates an empty capture folder and returns its id and path, refusing when the folder is over its quota.
func newCapture() (id, dir string, err error) {
	root, err := capturesRoot()
	if err != nil {
		return "", "", err
	}
	if used, _ := measure(root); used >= capturesQuotaBytes {
		return "", "", fmt.Errorf("the temporary captures folder is full (%d bytes used); release old captures first", used)
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	id = hex.EncodeToString(buf)
	dir = filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	return id, dir, nil
}

// captureDir resolves a capture id to its folder; it must be a well-formed id and an existing real folder.
func captureDir(id string) (string, error) {
	if !captureIDPattern.MatchString(id) {
		return "", errors.New("invalid capture id")
	}
	root, err := capturesRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, id)
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return "", os.ErrNotExist
	}
	return dir, nil
}

// captureFile resolves id + name to a regular file inside the capture's folder.
func captureFile(id, name string) (string, error) {
	dir, err := captureDir(id)
	if err != nil {
		return "", err
	}
	if !captureNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return "", errors.New("invalid file name")
	}
	path := filepath.Join(dir, filepath.FromSlash(name))
	if !within(dir, path) {
		return "", errors.New("invalid file name")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", os.ErrNotExist
	}
	return path, nil
}

// artifact is one file of a capture as the routes report it.
type artifact struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// writeArtifact writes data to dir/name (0600), and returns its description with the SHA-256 of exactly the bytes written.
func writeArtifact(dir, name, kind string, data []byte) (artifact, error) {
	path := filepath.Join(dir, filepath.FromSlash(name))
	if !within(dir, path) {
		return artifact{}, errors.New("invalid file name")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return artifact{}, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return artifact{}, err
	}
	sum := sha256.Sum256(data)
	return artifact{Name: name, Kind: kind, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}, nil
}

// hashFile returns the size and SHA-256 of a file on disk.
func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return n, hex.EncodeToString(h.Sum(nil)), err
}

// isMediaName says whether a capture file name is under the media download folders (media/ or segments/).
func isMediaName(name string) bool {
	return strings.HasPrefix(name, "media/") || strings.HasPrefix(name, "segments/")
}

// CaptureFileHandler handles GET /capture-file?id=&name= — an evidence file (screenshot, page copy, network log, frames, meta) as a download. It
// refuses the media download folders: those are served only by MediaFileHandler, which has the stricter scope.
func CaptureFileHandler(w http.ResponseWriter, r *http.Request) { serveCaptureFile(w, r, false) }

// MediaFileHandler handles GET /media-file?id=&name= — a downloaded media file or segment (name under media/ or segments/), as a download.
func MediaFileHandler(w http.ResponseWriter, r *http.Request) { serveCaptureFile(w, r, true) }

func serveCaptureFile(w http.ResponseWriter, r *http.Request, media bool) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.URL.Query().Get("name")
	if media != isMediaName(name) {
		http.Error(w, "invalid file name", http.StatusBadRequest)
		return
	}
	path, err := captureFile(r.URL.Query().Get("id"), name)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, os.ErrNotExist) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(path)+`"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, filepath.Base(path), info.ModTime(), f)
}

// CaptureHandler handles DELETE /capture?id= — releases (deletes) one capture. Releasing one that is already gone is fine.
func CaptureHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if !captureIDPattern.MatchString(id) {
		http.Error(w, "invalid capture id", http.StatusBadRequest)
		return
	}
	dir, err := captureDir(id)
	if errors.Is(err, os.ErrNotExist) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		http.Error(w, "could not release the capture", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeCaptureDir deletes a capture folder created by newCapture (used when a capture fails half way).
func removeCaptureDir(dir string) error {
	root, err := capturesRoot()
	if err != nil || !within(root, dir) || dir == root {
		return errors.New("refusing to delete outside the captures folder")
	}
	return os.RemoveAll(dir)
}
