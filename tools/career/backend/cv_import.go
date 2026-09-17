// cv_import.go — Import CV (step 2): accepting an uploaded CV PDF and
// serving it back via a capability token instead of the normal
// scope-gated proxy auth. pdf_tools' own fetch has no session to
// present (it's a stateless subprocess), so the usual "caller must
// hold tool:career:cv_import" check can never pass for that one fetch
// — a random, unguessable, short-lived token substitutes for it on
// that one route only. See
// plan/ai/tools/career/import-cv/step-02-cv-upload-and-capability-token-serving.md.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// cvImportsDir holds uploaded CV files, under TOOL_UPLOADS_DIR — not
// pre-created by the host, same convention every other tool's own
// main() already follows.
var cvImportsDir string

// maxCVUploadBytes bounds the uploaded file — a resume PDF has no
// business being larger. Plain constant, not configurable, matching
// this codebase's own "plain constants over premature configurability"
// convention.
const maxCVUploadBytes = 10 * 1024 * 1024

// cvImportTokenTTL bounds how long a capability URL stays servable —
// long enough to cover a full AI turn (fetch + extraction + duplicate
// comparison + JSON composition), short enough to bound how long a
// leaked URL would expose real PII.
const cvImportTokenTTL = 30 * time.Minute

// initCVImportsDir creates this feature's own upload subdirectory —
// called once from runHTTPServer, mirroring initDatabases' own
// TOOL_DB_DIR pattern.
func initCVImportsDir() error {
	uploadsDir := os.Getenv("TOOL_UPLOADS_DIR")
	if uploadsDir == "" {
		return fmt.Errorf("TOOL_UPLOADS_DIR is not set")
	}
	dir := filepath.Join(uploadsDir, "cv_imports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create cv_imports directory: %w", err)
	}
	cvImportsDir = dir
	return nil
}

type uploadCVResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// uploadCVHandler handles POST cv-import/upload — multipart, single
// field "cv". Generates a random token (crypto/rand, never
// math/rand — this gates a real capability, not a cosmetic id),
// stores its hash (never the raw token) alongside the file, and
// returns a ready-to-use absolute URL the frontend embeds verbatim
// into the AI's own initial instruction message — the AI never has to
// construct this URL itself, only copy it into its own
// pdf_to_markdown tool call.
func uploadCVHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseMultipartForm(maxCVUploadBytes); err != nil {
		http.Error(w, "cv file is required and must be under 10MB", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("cv")
	if err != nil {
		http.Error(w, "cv file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Fast-fail UX check only, not a security boundary — pdf_tools'
	// own parser will fail on a non-PDF regardless.
	if ext := filepath.Ext(header.Filename); ext != "" && ext != ".pdf" {
		http.Error(w, "cv must be a PDF file", http.StatusBadRequest)
		return
	}

	id := uuid.NewString()
	token, err := generateCVImportToken()
	if err != nil {
		http.Error(w, "failed to generate upload token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	destPath := filepath.Join(cvImportsDir, id+".pdf")
	dest, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "failed to store cv: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(dest, file); err != nil {
		dest.Close()
		http.Error(w, "failed to store cv: "+err.Error(), http.StatusInternalServerError)
		return
	}
	dest.Close()

	tokenHash := hashCVImportToken(token)
	expiresAt := time.Now().Add(cvImportTokenTTL).UTC().Format(time.RFC3339)
	if _, err := careerDB.Exec(
		`INSERT INTO cv_import_files (id, token_hash, file_path, expires_at) VALUES (?, ?, ?, ?)`,
		id, tokenHash, destPath, expiresAt,
	); err != nil {
		http.Error(w, "failed to record cv upload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// CORE_API_URL is the same fixed, host-injected env var every tool
	// subprocess receives — used here (not just by the host itself) so
	// the URL handed back is genuinely reachable by another tool's own
	// subprocess (pdf_tools), not just by a human's browser.
	url := os.Getenv("CORE_API_URL") + "/api/v1/tools/career/proxy/cv-import/file?id=" + id + "&token=" + token

	writeJSON(w, uploadCVResponse{ID: id, URL: url})
}

// serveCVFileHandler handles GET cv-import/file. Declared scope is
// tool:career:cv_import (manifest.json), but this handler checks the
// capability token FIRST and, on a valid match, serves the file
// without the caller needing to hold that scope at all — the token is
// the substitute security boundary for this one route, since
// pdf_tools' own fetch presents no Authorization/cookie to check
// against. An invalid/missing/expired token is indistinguishable from
// a real 404, not a 401/403 — an expired capability URL should look
// exactly like one that never existed.
func serveCVFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	token := r.URL.Query().Get("token")
	if id == "" || token == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var tokenHash, filePath, expiresAt string
	err := careerDB.QueryRow(
		`SELECT token_hash, file_path, expires_at FROM cv_import_files WHERE id = ?`, id,
	).Scan(&tokenHash, &filePath, &expiresAt)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if subtle.ConstantTimeCompare([]byte(hashCVImportToken(token)), []byte(tokenHash)) != 1 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil || time.Now().UTC().After(expiry) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Write(data)
}

// generateCVImportToken returns a real-random (crypto/rand), 32-byte,
// base64url-encoded token — high-entropy, unguessable, the substitute
// security boundary serveCVFileHandler checks.
func generateCVImportToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// hashCVImportToken — the raw token is never stored, only its hash;
// this isn't a low-entropy secret needing bcrypt's slow-hash treatment
// (unlike a human password), it's a high-entropy random value being
// protected against DB-read disclosure, so a plain SHA-256 is enough.
func hashCVImportToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
