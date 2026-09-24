package cvbuilder

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"regexp"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
)

const (
	maxSkills            = 100
	maxExperienceEntries = 50
	maxFieldLength       = 10_000
	cvDocumentTitleField = "title"
	cvDocumentFileField  = "file"
	cvDocumentToolSlug   = "career"
)

// requirePersonaExists mirrors persona.go's own unexported
// requirePersonaExists exactly (no exported equivalent exists to reuse
// — confirmed by reading persona.go directly) — used by the POST
// handler below, which (per step 1's own addendum) no longer calls
// AssembleCvData at all, so it needs its own lightweight existence
// check.
func requirePersonaExists(personaID string) error {
	var exists int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM personas WHERE id = ?`, personaID).Scan(&exists); {
	case err == sql.ErrNoRows:
		return persona.ErrUnknownPersona
	case err != nil:
		return err
	}
	return nil
}

// htmlTagPattern matches a known, common HTML tag name (<strong>,
// </div>, <br>, <p class="x">, ...), NOT "any letter right after '<'"
// — a real CV can legitimately contain that shape without meaning HTML
// at all (a live-caught false positive: "vector<int>", a C++ template,
// matched a naive "any tag-shaped bracket" pattern). Restricting to an
// explicit allowlist of actual tag names (word-bounded, so "bold" can
// never match tag "b") keeps single/short letters like generic type
// parameters (T, K, V, int) out of scope while still catching the
// realistic failure mode: an AI reaching for basic HTML formatting
// (bold/italic/paragraph/line-break/list) in what should be plain text.
// A bare '<'/'>' used as a comparison ("<5ms", "revenue < $10M") never
// matches this at all, and html/template already safely escapes it into
// a literal "<" in the rendered PDF regardless, which is correct.
var htmlTagPattern = regexp.MustCompile(`(?i)<\/?(?:strong|em|div|span|br|ul|ol|li|blockquote|code|pre|p|h[1-6]|table|tr|td|th|font|center|sub|sup)\b[^<>]*>`)

// hasHTMLTag is ValidateCvData's own live-observed-bug guard: an AI
// generating a CV occasionally reaches for HTML tags in a free-text
// field (e.g. "<b>Led a team</b>" in a summary), thinking it's writing
// into a web page — html/template then does exactly what it's supposed
// to and ESCAPES that literal text, so the rendered PDF shows the ugly,
// literal "&lt;b&gt;Led a team&lt;/b&gt;" instead of either bold text or
// plain text. Rejecting it here, with a clear and immediately
// actionable error, catches this in one fast round trip — no PDF is
// ever generated from data that would produce this — rather than a
// human discovering visibly broken text in an already-downloaded CV.
func hasHTMLTag(s string) bool {
	return htmlTagPattern.MatchString(s)
}

// ValidateCvData bounds a caller-submitted CvData (step 1's own
// addendum made this frontend-assembled, not server-derived) — html/
// template (templates.Render) already neutralizes injection, this
// only guards against an absurdly large payload wasting render time or
// storage, and against markup that would render as ugly escaped text
// (see hasHTMLTag above) rather than actually failing to render.
// Exported: jobs/cv_pdf.go's own save_cv_document reuses this exact
// check for AI-submitted CvData too, rather than duplicating it.
func ValidateCvData(data templates.CvData) error {
	if len(data.FullName) > maxFieldLength || len(data.Headline) > maxFieldLength ||
		len(data.Summary) > maxFieldLength || len(data.Location) > maxFieldLength {
		return fmt.Errorf("a field is too long")
	}
	if hasHTMLTag(data.FullName) || hasHTMLTag(data.Headline) || hasHTMLTag(data.Summary) || hasHTMLTag(data.Location) {
		return fmt.Errorf("fields must be plain text — no HTML/markup tags (found one in fullName, headline, summary, or location)")
	}
	if len(data.Skills) > maxSkills {
		return fmt.Errorf("too many skills (max %d)", maxSkills)
	}
	for _, s := range data.Skills {
		if len(s) > maxFieldLength {
			return fmt.Errorf("a skill is too long")
		}
		if hasHTMLTag(s) {
			return fmt.Errorf("skills must be plain text — no HTML/markup tags (found one in %q)", s)
		}
	}
	if len(data.Experience) > maxExperienceEntries {
		return fmt.Errorf("too many experience entries (max %d)", maxExperienceEntries)
	}
	for _, e := range data.Experience {
		if len(e.Company) > maxFieldLength || len(e.Title) > maxFieldLength ||
			len(e.StartDate) > maxFieldLength || len(e.EndDate) > maxFieldLength || len(e.Description) > maxFieldLength {
			return fmt.Errorf("an experience field is too long")
		}
		if hasHTMLTag(e.Company) || hasHTMLTag(e.Title) || hasHTMLTag(e.StartDate) || hasHTMLTag(e.EndDate) || hasHTMLTag(e.Description) {
			return fmt.Errorf("experience fields must be plain text — no HTML/markup tags (found one in the %q entry)", e.Title)
		}
	}
	for _, l := range data.ExternalLinks {
		if len(l.Platform) > maxFieldLength || len(l.URL) > maxFieldLength {
			return fmt.Errorf("an external link field is too long")
		}
		if hasHTMLTag(l.Platform) {
			return fmt.Errorf("external link platform names must be plain text — no HTML/markup tags (found one in %q)", l.Platform)
		}
	}
	return nil
}

// TemplatesHandler handles GET cv-documents/templates.
func TemplatesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	db.WriteJSON(w, map[string]any{"templates": templates.ListTemplates()})
}

// PersonaDefaultsHandler handles GET cv-documents/persona-defaults?personaId=
// — the ONE place AssembleCvData is called from an HTTP request; the
// frontend's own builder page uses this to pre-fill its editable
// sidebar (step 1's own addendum), never to re-derive data at
// generation time.
func PersonaDefaultsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	personaID := r.URL.Query().Get("personaId")
	if personaID == "" {
		http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
		return
	}
	data, err := AssembleCvData(personaID)
	if err != nil {
		if errors.Is(err, persona.ErrUnknownPersona) {
			http.Error(w, "unknown persona id", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to assemble persona defaults: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, data)
}

type photoStatusResponse struct {
	HasPhoto bool `json:"hasPhoto"`
}

// PhotoStatusHandler handles GET cv-documents/photo-status?personaId=
// — a separate, deliberately tiny endpoint rather than folding this
// into PersonaDefaultsHandler's own response: it's called from BOTH the
// "pick a persona for a new CV" step AND the "load an existing CV to
// edit" path (CvBuilderPage.tsx), and the latter must NOT re-fetch/
// overwrite the just-loaded document's own CvData the way calling
// persona-defaults again would.
func PhotoStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	personaID := r.URL.Query().Get("personaId")
	if personaID == "" {
		http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
		return
	}
	hasPhoto, err := personaHasProfilePhoto(personaID)
	if err != nil {
		if errors.Is(err, persona.ErrUnknownPersona) {
			http.Error(w, "unknown persona id", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to check profile photo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, photoStatusResponse{HasPhoto: hasPhoto})
}

type previewCvDocumentRequest struct {
	PersonaID   string           `json:"personaId"`
	TemplateID  string           `json:"templateId"`
	Data        templates.CvData `json:"data"`
	AttachPhoto bool             `json:"attachPhoto"`
}

type previewCvDocumentResponse struct {
	PreviewURL string `json:"previewUrl"`
}

// PreviewHandler handles POST cv-documents/preview — renders and
// generates a real PDF via pdf_tools, EXACTLY like the first half of
// DocumentsHandler's own POST (create) case, but deliberately stops
// there: no forwardUploadToMedia call, no InsertCvDocument call, no
// permanent record created anywhere. This lets the wizard's own Review
// step show the user a real rendered preview before they commit to
// actually creating a CV document.
//
// The returned previewUrl points directly at pdf_tools' own temporary
// files route (GET .../proxy/files?id=<resourceId>) — the exact same
// transient reference generate_pdf's own MCP tool already hands back
// mid-conversation, before an AI turn calls save_cv_document. Nothing new
// about this resource's own lifecycle: it lives in pdf_tools' own
// uploads dir, is never linked to any cv_documents row or Media file,
// and is cleaned up on whatever schedule pdf_tools already applies to
// every other unsaved generate_pdf result — no new cleanup mechanism
// needed here. See plan/ai/career/cv-builder/step-04-frontend-cv-builder.md.
func PreviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body previewCvDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if body.PersonaID == "" {
		http.Error(w, "personaId is required", http.StatusBadRequest)
		return
	}
	if body.TemplateID == "" {
		http.Error(w, "templateId is required", http.StatusBadRequest)
		return
	}
	if err := ValidateCvData(body.Data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var photoDataURI string
	if body.AttachPhoto {
		var photoErr error
		photoDataURI, photoErr = resolveProfilePhoto(r, body.PersonaID)
		if photoErr != nil {
			if errors.Is(photoErr, persona.ErrUnknownPersona) {
				http.Error(w, "unknown persona id", http.StatusBadRequest)
				return
			}
			http.Error(w, "failed to resolve profile photo: "+photoErr.Error(), http.StatusInternalServerError)
			return
		}
	}

	xhtml, err := templates.Render(body.TemplateID, body.Data, photoDataURI)
	if err != nil {
		http.Error(w, "unknown template id", http.StatusBadRequest)
		return
	}

	resourceID, err := forwardGeneratePdf(r, body.TemplateID, xhtml)
	if err != nil {
		http.Error(w, "failed to generate preview: "+err.Error(), http.StatusBadGateway)
		return
	}

	db.WriteJSON(w, previewCvDocumentResponse{PreviewURL: "/api/v1/tools/pdf_tools/proxy/files?id=" + resourceID})
}

type createCvDocumentRequest struct {
	PersonaID   string           `json:"personaId"`
	TemplateID  string           `json:"templateId"`
	Title       string           `json:"title"`
	Data        templates.CvData `json:"data"`
	AttachPhoto bool             `json:"attachPhoto"`
}

type updateCvDocumentTitleRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// DocumentsHandler handles every method on cv-documents itself — this
// tool's own router has no path-parameter matching (main.go's plain
// http.HandleFunc calls, confirmed by reading routeHandler.go
// directly), so a single row is addressed via ?id=, exactly like every
// other by-id operation in this tool (e.g. personasHandler's own
// DELETE), not a `{id}` URL segment.
func DocumentsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if id := r.URL.Query().Get("id"); id != "" {
			doc, err := GetCvDocument(id)
			if err != nil {
				writeCvDocumentAwareError(w, "read cv document", err)
				return
			}
			db.WriteJSON(w, doc)
			return
		}
		personaID := r.URL.Query().Get("personaId")
		if personaID == "" {
			http.Error(w, "personaId query parameter is required", http.StatusBadRequest)
			return
		}
		docs, err := ListCvDocuments(personaID)
		if err != nil {
			http.Error(w, "failed to list cv documents: "+err.Error(), http.StatusInternalServerError)
			return
		}
		db.WriteJSON(w, map[string]any{"cvDocuments": docs})

	case http.MethodPost:
		var body createCvDocumentRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.PersonaID == "" {
			http.Error(w, "personaId is required", http.StatusBadRequest)
			return
		}
		if body.TemplateID == "" {
			http.Error(w, "templateId is required", http.StatusBadRequest)
			return
		}
		if body.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		if err := requirePersonaExists(body.PersonaID); err != nil {
			if errors.Is(err, persona.ErrUnknownPersona) {
				http.Error(w, "unknown persona id", http.StatusNotFound)
				return
			}
			http.Error(w, "failed to validate persona: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := ValidateCvData(body.Data); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var photoDataURI string
		if body.AttachPhoto {
			var photoErr error
			photoDataURI, photoErr = resolveProfilePhoto(r, body.PersonaID)
			if photoErr != nil {
				http.Error(w, "failed to resolve profile photo: "+photoErr.Error(), http.StatusInternalServerError)
				return
			}
		}

		xhtml, err := templates.Render(body.TemplateID, body.Data, photoDataURI)
		if err != nil {
			http.Error(w, "unknown template id", http.StatusBadRequest)
			return
		}

		resourceID, err := forwardGeneratePdf(r, body.TemplateID, xhtml)
		if err != nil {
			http.Error(w, "failed to generate pdf: "+err.Error(), http.StatusBadGateway)
			return
		}
		pdfBytes, err := forwardFetchPdfBytes(r, resourceID)
		if err != nil {
			http.Error(w, "failed to fetch generated pdf: "+err.Error(), http.StatusBadGateway)
			return
		}
		mediaFileID, err := forwardUploadToMedia(r, pdfBytes, body.Title)
		if err != nil {
			http.Error(w, "failed to store pdf: "+err.Error(), http.StatusBadGateway)
			return
		}

		doc, err := InsertCvDocument(body.PersonaID, body.TemplateID, body.Title, body.Data, mediaFileID, "")
		if err != nil {
			http.Error(w, "cv generated but failed to record: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		db.WriteJSON(w, doc)

	case http.MethodPut:
		var body updateCvDocumentTitleRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		if body.Title == "" {
			http.Error(w, "title is required", http.StatusBadRequest)
			return
		}
		doc, err := UpdateCvDocumentTitle(body.ID, body.Title)
		if err != nil {
			writeCvDocumentAwareError(w, "update cv document", err)
			return
		}
		db.WriteJSON(w, doc)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		mediaFileID, err := DeleteCvDocument(id)
		if err != nil {
			writeCvDocumentAwareError(w, "delete cv document", err)
			return
		}
		if err := forwardDeleteToMedia(r, mediaFileID); err != nil {
			log.Printf("cvbuilder: failed to delete media file %q for cv document %q: %v", mediaFileID, id, err)
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeCvDocumentAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, ErrUnknownCvDocument) {
		http.Error(w, "unknown cv document id", http.StatusNotFound)
		return
	}
	http.Error(w, fmt.Sprintf("failed to %s: %v", action, err), http.StatusInternalServerError)
}

// --- forwarding helpers ---
//
// All three mirror cv_import.go's own forwardToMedia/forwardDeleteToMedia
// shape exactly: re-attach the ORIGINAL caller's own Authorization/
// Cookie headers (already sitting in r.Header, since ProxyHandler
// forwards the browser's own request through to this backend
// unmodified) onto a second outbound call, rather than inventing a new
// credential. See plan/ai/career/cv-builder/step-01-overview-and-data-model.md.

// CV PDFs ask for top/bottom page margin on every template EXCEPT
// modern-sidebar, a deliberate per-template split, not an oversight:
//   - Plain-text templates (classic, modern-mono) get a 0.4in top/
//     bottom margin. Plain CSS padding on body only ever appears once,
//     at the very start/end of the whole document flow, so a
//     multi-page CV would otherwise have content running edge-to-edge
//     on every page break. Page.printToPDF's own margin (unlike body
//     padding) genuinely repeats on every physical page, which is
//     exactly what pagination needs here. Left/right stay 0 regardless
//     (no full-bleed background to protect, but no reason to add one).
//   - modern-sidebar gets ZERO margin on every side instead — verified
//     live (real generated PDF, not just page 1) that Chrome insets a
//     `position: fixed` element's own containing block by whatever
//     @page margin is set, on EVERY side, not just left/right. Any
//     nonzero top/bottom here would reintroduce the exact white
//     band-around-the-colored-sidebar look this template's whole fixed-
//     sidebar redesign exists to eliminate. Its own template.html/
//     style.css instead uses `break-inside: avoid` on each entry to
//     keep page breaks from looking bad without relying on page margin.
const cvPageMarginTopBottomInches = 0.4
const cvSidebarTemplateID = "modern-sidebar"

type generatePdfRequest struct {
	Xhtml          string  `json:"xhtml"`
	MarginTopIn    float64 `json:"marginTopIn,omitempty"`
	MarginBottomIn float64 `json:"marginBottomIn,omitempty"`
}

type generatePdfResponse struct {
	ID    string `json:"id"`
	Bytes int    `json:"bytes"`
}

func forwardGeneratePdf(originalReq *http.Request, templateID, xhtml string) (string, error) {
	reqBody := generatePdfRequest{Xhtml: xhtml}
	if templateID != cvSidebarTemplateID {
		reqBody.MarginTopIn = cvPageMarginTopBottomInches
		reqBody.MarginBottomIn = cvPageMarginTopBottomInches
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	genURL := os.Getenv("CORE_API_URL") + "/api/v1/tools/pdf_tools/proxy/generate"
	req, err := http.NewRequest(http.MethodPost, genURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	forwardAuthHeaders(originalReq, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("generate failed: %s: %s", resp.Status, string(respBody))
	}

	var parsed generatePdfResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", err
	}
	return parsed.ID, nil
}

func forwardFetchPdfBytes(originalReq *http.Request, resourceID string) ([]byte, error) {
	filesURL := os.Getenv("CORE_API_URL") + "/api/v1/tools/pdf_tools/proxy/files?id=" + resourceID
	req, err := http.NewRequest(http.MethodGet, filesURL, nil)
	if err != nil {
		return nil, err
	}
	forwardAuthHeaders(originalReq, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch generated pdf failed: %s: %s", resp.Status, string(respBody))
	}
	return io.ReadAll(resp.Body)
}

type mediaUploadEnvelope struct {
	Message struct {
		FileID string `json:"fileId"`
	} `json:"message"`
}

func forwardUploadToMedia(originalReq *http.Request, pdfBytes []byte, title string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// CreateFormFile always sets the part's own Content-Type to
	// "application/octet-stream" (Go stdlib default, not something this
	// tool can override afterward) — Media's own UploadHandler reads
	// THIS header, not the file's own extension, to decide
	// content_type. Building the part manually via CreatePart, with an
	// explicit "application/pdf" Content-Type, is what makes the
	// resulting Media row's own content_type (and therefore admin
	// preview/download behavior) actually correct — confirmed by a
	// live test that caught this exact gap.
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", `form-data; name="`+cvDocumentFileField+`"; filename="cv.pdf"`)
	partHeader.Set("Content-Type", "application/pdf")
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(pdfBytes); err != nil {
		return "", err
	}
	if err := writer.WriteField("toolSlug", cvDocumentToolSlug); err != nil {
		return "", err
	}
	if err := writer.WriteField(cvDocumentTitleField, title); err != nil {
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
	forwardAuthHeaders(originalReq, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("media upload failed: %s: %s", resp.Status, string(respBody))
	}

	var envelope mediaUploadEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", err
	}
	return envelope.Message.FileID, nil
}

func forwardDeleteToMedia(originalReq *http.Request, mediaFileID string) error {
	deleteURL := os.Getenv("CORE_API_URL") + "/api/v1/media/" + mediaFileID
	req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	forwardAuthHeaders(originalReq, req)

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

func forwardAuthHeaders(originalReq, req *http.Request) {
	if auth := originalReq.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cookie := originalReq.Header.Get("Cookie"); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

// profileImageMediaFileID looks up personaID's own profile's current
// photo (profileimage package, image_media_file_id column) — "" (no
// error) if the profile has none. Shared by resolveProfilePhoto (needs
// the actual bytes) and personaHasProfilePhoto (only needs to know
// whether one exists, for the wizard's own "attach photo?" checkbox —
// no reason to fetch Media bytes just to answer that). Queries
// career.db directly rather than importing the profileimage package:
// that package's own HTTP handlers aren't needed here, and a two-line
// SELECT is simpler than exporting a getter whose only other callers
// are these two.
func profileImageMediaFileID(personaID string) (string, error) {
	var profileID string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID); {
	case err == sql.ErrNoRows:
		return "", persona.ErrUnknownPersona
	case err != nil:
		return "", err
	}

	var mediaFileID sql.NullString
	if err := db.CareerDB.QueryRow(`SELECT image_media_file_id FROM profiles WHERE id = ?`, profileID).Scan(&mediaFileID); err != nil {
		return "", err
	}
	if !mediaFileID.Valid {
		return "", nil
	}
	return mediaFileID.String, nil
}

// personaHasProfilePhoto answers the wizard's own "does this persona's
// profile have a photo at all?" question — the "attach photo?" checkbox
// (PersonaAndTemplateStep.tsx) only ever renders when this is true, per
// your own instruction ("the checkbox will show up only if the user has
// a profile image uploaded").
func personaHasProfilePhoto(personaID string) (bool, error) {
	mediaFileID, err := profileImageMediaFileID(personaID)
	if err != nil {
		return false, err
	}
	return mediaFileID != "", nil
}

// resolveProfilePhoto returns personaID's own profile photo as a
// ready-to-use "data:<content-type>;base64,<...>" string for
// templates.Render's own photoDataURI parameter — "" (no error) if the
// profile has no photo, which every caller here treats as "just don't
// show one," not a failure. A genuinely unknown personaID is the one
// real caller mistake this surfaces as an error; a Media-side fetch
// failure is deliberately swallowed (logged, not returned) — a photo
// Media can't currently serve must never block generating or
// previewing the rest of the CV.
func resolveProfilePhoto(r *http.Request, personaID string) (string, error) {
	mediaFileID, err := profileImageMediaFileID(personaID)
	if err != nil {
		return "", err
	}
	if mediaFileID == "" {
		return "", nil
	}

	dataURI, err := fetchMediaImageDataURI(r, mediaFileID)
	if err != nil {
		log.Printf("cvbuilder: failed to fetch profile photo %q for cv render: %v", mediaFileID, err)
		return "", nil
	}
	return dataURI, nil
}

// fetchMediaImageDataURI downloads a Media file's own bytes (forwarding
// the original caller's own auth, same convention as every other
// forward* helper in this file) and base64-encodes them into a data:
// URI — pdf_tools' headless-Chrome renderer has no session/cookie of
// its own to fetch an authenticated Media URL with, so the image has to
// travel inside the XHTML itself as text, not as a src="/api/..." the
// renderer would have to fetch separately.
func fetchMediaImageDataURI(originalReq *http.Request, mediaFileID string) (string, error) {
	downloadURL := os.Getenv("CORE_API_URL") + "/api/v1/media/" + mediaFileID + "/download"
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", err
	}
	forwardAuthHeaders(originalReq, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
