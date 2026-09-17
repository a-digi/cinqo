// cv_import.go — Import CV: accepting an uploaded CV PDF and handing
// it to the core Media feature, which stores it centrally and returns
// an opaque file id. The AI only ever sees that id (embedded as
// "media:<fileId>" in buildImportPrompt.ts's own initial instruction
// message) — never a fetchable URL — so pdf_tools' own pdf_to_markdown
// call never performs an HTTP fetch for it at all, and can never 401.
// Replaces this feature's own previous capability-token mechanism
// (a per-file random token this tool minted, hashed, and checked
// itself against a route the reverse proxy had to specially exempt
// from its normal auth check) — that mechanism is gone; see
// plan/ai/media/step-02-career-media-migration.md.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

// maxCVUploadBytes bounds the uploaded file — a resume PDF has no
// business being larger. Plain constant, not configurable, matching
// this codebase's own "plain constants over premature configurability"
// convention.
const maxCVUploadBytes = 10 * 1024 * 1024

// cvImportTTLSeconds bounds how long the uploaded CV stays resolvable
// by the AI's own pdf_to_markdown call — long enough to cover a full AI
// turn (fetch + extraction + duplicate comparison + JSON composition),
// short enough to bound how long an abandoned upload lingers on disk.
// Passed straight to Media's own ttlSeconds field.
const cvImportTTLSeconds = 30 * 60

type uploadCVResponse struct {
	FileID string `json:"fileId"`
}

type mediaUploadResponse struct {
	FileID string `json:"fileId"`
}

// uploadCVHandler handles POST cv-import/upload — multipart, single
// field "cv". Forwards the file to the core Media feature
// (POST {CORE_API_URL}/api/v1/media/upload, toolSlug="career"),
// re-presenting the ORIGINAL caller's own Authorization/Cookie header
// on that outbound call: the reverse proxy already forwarded it onto
// this handler unmodified (httputil.ReverseProxy preserves it by
// default), and Media's own authorization check needs to see the same
// real, currently-authenticated user's scopes — this handler never
// invents its own service-to-service credential for that call.
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

	fileID, err := forwardToMedia(r, file, header.Filename)
	if err != nil {
		http.Error(w, "failed to store cv: "+err.Error(), http.StatusBadGateway)
		return
	}

	writeJSON(w, uploadCVResponse{FileID: fileID})
}

// forwardToMedia builds a fresh multipart request to Media's own
// upload endpoint and returns the file id it hands back. CORE_API_URL
// is the same fixed, host-injected env var every tool subprocess
// receives — used the same way this file's own previous
// capability-token URL used it.
func forwardToMedia(originalReq *http.Request, file io.Reader, filename string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.WriteField("toolSlug", "career"); err != nil {
		return "", err
	}
	if err := writer.WriteField("ttlSeconds", fmt.Sprintf("%d", cvImportTTLSeconds)); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	uploadURL := os.Getenv("CORE_API_URL") + "/api/v1/media/upload"
	req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if auth := originalReq.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cookie := originalReq.Header.Get("Cookie"); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("media upload failed: %s: %s", resp.Status, string(respBody))
	}

	var result mediaUploadResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.FileID, nil
}
