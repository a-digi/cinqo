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
package cv

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"career-tool-backend/db"
)

// maxCVUploadBytes bounds the uploaded file — a resume PDF has no
// business being larger. Plain constant, not configurable, matching
// this codebase's own "plain constants over premature configurability"
// convention.
const maxCVUploadBytes = 10 * 1024 * 1024

// cvImportTTLSeconds bounds how long the uploaded CV stays resolvable
// by the AI's own pdf_to_markdown call — and now also how long
// "process again" (cv_import_runs.go) can reuse the same upload
// instead of asking for a fresh one, now that reprocessing is a real
// feature. 24h, not indefinite: long enough that revisiting an import
// a day later still works, short enough to still bound how long a CV's
// own PII sits on disk. Passed straight to Media's own ttlSeconds
// field.
const cvImportTTLSeconds = 24 * 60 * 60

type uploadCVResponse struct {
	FileID string `json:"fileId"`
}

// mediaUploadEnvelope mirrors the core app's own response.SuccessResponse
// shape ({"success":true,"message":<payload>}) — every core API
// response is wrapped this way, unlike this tool's own plain writeJSON
// convention, so the real payload has to be unwrapped from "message"
// rather than decoded as if it were top-level.
type mediaUploadEnvelope struct {
	Message struct {
		FileID string `json:"fileId"`
	} `json:"message"`
}

// UploadCVHandler handles POST cv-import/upload — multipart, single
// field "cv". Forwards the file to the core Media feature
// (POST {CORE_API_URL}/api/v1/media/upload, toolSlug="career"),
// re-presenting the ORIGINAL caller's own Authorization/Cookie header
// on that outbound call: the reverse proxy already forwarded it onto
// this handler unmodified (httputil.ReverseProxy preserves it by
// default), and Media's own authorization check needs to see the same
// real, currently-authenticated user's scopes — this handler never
// invents its own service-to-service credential for that call.
func UploadCVHandler(w http.ResponseWriter, r *http.Request) {
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

	db.WriteJSON(w, uploadCVResponse{FileID: fileID})
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

	var result mediaUploadEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Message.FileID, nil
}

// --- cv-import/runs — history: what the AI suggested, what was saved ---
//
// Superseded this feature's own previous GET cv-import/uploads (a bare
// Media-file listing with no domain knowledge of what an upload was
// even for) — see plan/ai/media/step-05-career-history.md. One
// handler, dispatched by method, matching profilesHandler's own
// established convention elsewhere in this tool.

type createCVImportRunRequest struct {
	FileID           string          `json:"fileId"`
	OriginalFilename string          `json:"originalFilename"`
	ConversationID   string          `json:"conversationId"`
	AIProposal       json.RawMessage `json:"aiProposal"`
}

type updateCVImportRunRequest struct {
	ID          string          `json:"id"`
	SaveSummary json.RawMessage `json:"saveSummary"`
}

// CVImportRunsHandler handles GET/POST/PUT/DELETE cv-import/runs.
func CVImportRunsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		runs, err := listCVImportRuns()
		if err != nil {
			http.Error(w, "failed to list cv import runs: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"runs": runs})

	case http.MethodPost:
		var body createCVImportRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.FileID == "" || body.ConversationID == "" || len(body.AIProposal) == 0 {
			http.Error(w, "fileId, conversationId, and aiProposal are all required", http.StatusBadRequest)
			return
		}
		id, err := createCVImportRun(body.FileID, body.OriginalFilename, body.ConversationID, body.AIProposal)
		if err != nil {
			http.Error(w, "failed to record cv import run: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"id": id})

	case http.MethodPut:
		var body updateCVImportRunRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" || len(body.SaveSummary) == 0 {
			http.Error(w, "id and saveSummary are both required", http.StatusBadRequest)
			return
		}
		if err := updateCVImportRunSaveSummary(body.ID, body.SaveSummary); err != nil {
			if errors.Is(err, errUnknownCVImportRun) {
				http.Error(w, "unknown cv import run id", http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to update cv import run: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		mediaFileID, err := deleteCVImportRun(id)
		if err != nil {
			if errors.Is(err, errUnknownCVImportRun) {
				http.Error(w, "unknown cv import run id", http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to delete cv import run: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Best-effort — the history row is gone either way; a lingering
		// orphaned Media file just expires on its own TTL if this fails.
		// Forwards the ORIGINAL caller's own Authorization/Cookie so
		// Media's own ownership check (DeleteHandler, api/src/media) sees
		// the same real, currently-authenticated user.
		if err := forwardDeleteToMedia(r, mediaFileID); err != nil {
			log.Printf("cv_import_runs: failed to delete media file %q for run %q: %v", mediaFileID, id, err)
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func forwardDeleteToMedia(originalReq *http.Request, mediaFileID string) error {
	deleteURL := os.Getenv("CORE_API_URL") + "/api/v1/media/" + mediaFileID
	req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	if auth := originalReq.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cookie := originalReq.Header.Get("Cookie"); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("media delete failed: %s: %s", resp.Status, string(respBody))
	}
	return nil
}
