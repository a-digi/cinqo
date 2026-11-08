// cv_import_text.go — the text of an uploaded CV, for a page that does the import in code (plan 209, step 2).
//
// The pdf_to_markdown tool is for the AI: the core resolves "media:<id>" for it, and nothing hands its answer to a page. So the page asks the
// Career backend, which fetches the stored PDF from the core with the caller's own login (the same way the upload is forwarded) and reads it
// with the same library and the same cleaning as the tool (package pdftext), so the code sees what a model would see.
package cv

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"

	"career-tool-backend/cv/pdftext"
	"career-tool-backend/db"
)

type cvTextResponse struct {
	Text       string `json:"text"`
	Characters int    `json:"characters"`
}

// CVImportTextHandler handles GET /cv-import/text?fileId=<media id>: {text, characters}. 404 for a file the caller cannot download or that
// is gone, 422 for a PDF with no readable text, 502 when the core cannot be reached.
func CVImportTextHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fileID := strings.TrimSpace(r.URL.Query().Get("fileId"))
	if _, err := uuid.Parse(fileID); err != nil {
		http.Error(w, "fileId is required and must be a uuid", http.StatusBadRequest)
		return
	}
	data, status, err := downloadMedia(r, fileID)
	switch {
	case err != nil:
		http.Error(w, "failed to fetch the CV: "+err.Error(), http.StatusBadGateway)
		return
	case status == http.StatusNotFound || status == http.StatusForbidden || status == http.StatusUnauthorized:
		http.Error(w, "the CV file was not found (it may have expired); upload it again", http.StatusNotFound)
		return
	case status != http.StatusOK:
		http.Error(w, fmt.Sprintf("failed to fetch the CV: the core answered %d", status), http.StatusBadGateway)
		return
	}
	text, err := pdftext.ExtractText(data)
	if err != nil {
		http.Error(w, "the CV could not be read as text: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}
	db.WriteJSON(w, cvTextResponse{Text: text, Characters: len([]rune(text))})
}

// downloadMedia fetches the stored file from the core with the caller's own Authorization and Cookie (as forwardToMedia does for the upload).
// The body is bounded: a CV is at most maxCVUploadBytes.
func downloadMedia(originalReq *http.Request, fileID string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(originalReq.Context(), http.MethodGet, os.Getenv("CORE_API_URL")+"/api/v1/media/"+fileID+"/download", nil)
	if err != nil {
		return nil, 0, err
	}
	if auth := originalReq.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cookie := originalReq.Header.Get("Cookie"); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCVUploadBytes+1))
	if err != nil {
		return nil, 0, err
	}
	if len(data) > maxCVUploadBytes {
		return nil, 0, fmt.Errorf("the file is larger than %d bytes", maxCVUploadBytes)
	}
	return data, http.StatusOK, nil
}
