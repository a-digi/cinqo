package cvbuilder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/cv/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/media"
	"career-tool-backend/persona"
)

// save_persona_cv: how the AI hands over the CV it made for a persona (step 115). It takes the run id,
// so the persona comes from the run, never from the AI, and it saves at most one CV per run.

const maxCvAiTitle = 200

var ErrCvAiRunNotGenerating = errors.New("this run is not generating")
var ErrCvAiAlreadySaved = errors.New("this run already saved its CV")

// SaveCvAiDocument records the CV the AI made in a generating run, on the run's own persona (linked
// as the persona it was built from), and ties it to the run. The checks on the content come first.
func SaveCvAiDocument(runID, templateID, title string, data templates.CvData, mediaFileID string) (CvDocument, error) {
	title, err := checkCvContent(templateID, title, data)
	if err != nil {
		return CvDocument{}, err
	}

	var personaID, status string
	var saved sql.NullString
	switch err := db.CareerDB.QueryRow(`SELECT persona_id, status, cv_document_id FROM cv_ai_runs WHERE id = ?`, runID).Scan(&personaID, &status, &saved); {
	case errors.Is(err, sql.ErrNoRows):
		return CvDocument{}, ErrUnknownCvAiRun
	case err != nil:
		return CvDocument{}, err
	}
	if status != cvAiRunning {
		return CvDocument{}, ErrCvAiRunNotGenerating
	}
	if saved.Valid {
		return CvDocument{}, ErrCvAiAlreadySaved
	}

	if err := checkCvPdfShape(mediaFileID); err != nil {
		return CvDocument{}, err
	}
	if err := claimCvPdf(mediaFileID, title); err != nil {
		return CvDocument{}, err
	}

	doc, err := InsertCvDocument(personaID, templateID, title, data, mediaFileID, "")
	if err != nil {
		return CvDocument{}, err
	}
	// The run takes the CV only while it still works and has none (two calls at once: one wins).
	res, err := db.CareerDB.Exec(
		`UPDATE cv_ai_runs SET cv_document_id = ? WHERE id = ? AND status = 'generating' AND cv_document_id IS NULL`, doc.ID, runID)
	if err != nil {
		_, _ = DeleteCvDocument(doc.ID)
		return CvDocument{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		_, _ = DeleteCvDocument(doc.ID)
		return CvDocument{}, ErrCvAiAlreadySaved
	}
	return doc, nil
}

// checkCvContent is the check of what the AI sends as a CV that does not depend on a run or a persona: the title (trimmed, returned), the
// person's name, the template and the data. Shared by the run's save and the save for a named persona.
func checkCvContent(templateID, title string, data templates.CvData) (string, error) {
	title, err := checkCvTitle(title)
	if err != nil {
		return "", err
	}
	switch {
	case strings.TrimSpace(data.FullName) == "":
		return "", errors.New("data.fullName is required")
	case !templates.IsKnownTemplate(templateID):
		return "", unknownTemplateError(templateID)
	}
	if err := ValidateCvData(data); err != nil {
		return "", err
	}
	return title, nil
}

// checkCvTitle is the title's part of the content check: trimmed, not empty, not too long.
func checkCvTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return "", errors.New("title is required")
	case utf8.RuneCountInString(title) > maxCvAiTitle:
		return "", fmt.Errorf("title is too long (max %d characters)", maxCvAiTitle)
	}
	return title, nil
}

func unknownTemplateError(templateID string) error {
	return fmt.Errorf("unknown template id %q — call list_cv_templates for valid ids", templateID)
}

// checkCvPdfShape refuses a pdfResource that is not the id of a promoted Media file (a raw resource link, or nothing).
func checkCvPdfShape(mediaFileID string) error {
	if strings.Contains(mediaFileID, "://") || strings.HasPrefix(mediaFileID, "/") {
		return errors.New("pdfResource still looks like a raw resource link (" + mediaFileID + "), not a promoted Media file id — " +
			"call generate_pdf again and pass its own returned 'uri' field straight into this call's pdfResource argument, exactly as returned")
	}
	if strings.TrimSpace(mediaFileID) == "" {
		return errors.New("pdfResource is required")
	}
	return nil
}

// claimCvPdf gives the PDF its title in Media, which also proves it is a real file this tool owns (Media answers 404 or 403 otherwise).
// It changes the file in Media, so it comes after every check that could still refuse the save.
func claimCvPdf(mediaFileID, title string) error {
	if err := media.SetTitle(mediaFileID, title+".pdf"); err != nil {
		if errors.Is(err, media.ErrMediaNotOwned) {
			return errors.New("pdfResource (" + mediaFileID + ") does not correspond to a real, already-promoted Media file owned by " +
				"this tool — pass generate_pdf's own returned 'uri' field unmodified")
		}
	}
	return nil
}

// ErrCvPdfInUse is returned when the PDF already belongs to another CV: one file belongs to one CV only, so that deleting a CV (which
// deletes its file) can never take the file of another.
var ErrCvPdfInUse = errors.New("this PDF file already belongs to another CV: render the CV and call generate_pdf again to get its own file")

// SaveCvForPersona records a CV the AI made for a named persona, outside a "create CV with AI" run: the same content checks as
// SaveCvAiDocument, plus that the persona exists and that the PDF belongs to no other CV. The CV is linked to the persona as the one it
// was built from. Several CVs per persona are fine; the run's one-CV limit has no equivalent here.
func SaveCvForPersona(personaID, templateID, title string, data templates.CvData, mediaFileID string) (CvDocument, error) {
	title, err := checkCvContent(templateID, title, data)
	if err != nil {
		return CvDocument{}, err
	}
	if err := requirePersonaExists(personaID); err != nil {
		return CvDocument{}, err
	}
	if err := checkCvPdfShape(mediaFileID); err != nil {
		return CvDocument{}, err
	}
	var used int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents WHERE media_file_id = ?`, mediaFileID).Scan(&used); err != nil {
		return CvDocument{}, err
	}
	if used > 0 {
		return CvDocument{}, ErrCvPdfInUse
	}
	if err := claimCvPdf(mediaFileID, title); err != nil {
		return CvDocument{}, err
	}
	return InsertCvDocument(personaID, templateID, title, data, mediaFileID, "")
}

type saveCvForPersonaArgs struct {
	PersonaID   string             `json:"personaId" jsonschema:"the persona this CV is for, from list_personas"`
	TemplateID  string             `json:"templateId" jsonschema:"the same templateId you passed to render_cv_document for this CV"`
	Title       string             `json:"title" jsonschema:"a short, human-readable name for the CV, e.g. 'Jane Doe - Backend Engineer'"`
	Data        CvDocumentDataArgs `json:"data" jsonschema:"the exact SAME data object you passed to render_cv_document for this CV"`
	PdfResource string             `json:"pdfResource" jsonschema:"the exact 'uri' field from generate_pdf's own returned resource link, unmodified"`
}

// RegisterSaveCvForPersona adds save_cv_for_persona: save_persona_cv for a normal chat, where there is no run, so the AI names the persona.
func RegisterSaveCvForPersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_cv_for_persona",
		Description: "Save the CV PDF you just made for a persona, so that it appears under that persona's CVs: call get_persona_cv_defaults, tailor the data, " +
			"render_cv_document, generate_pdf, and then call this ONCE, right after generate_pdf returns its resource link, passing that link's own 'uri' " +
			"field unmodified as pdfResource. templateId and data must be the EXACT same values you passed to render_cv_document. data's free-text fields are " +
			"PLAIN TEXT ONLY, no HTML or markup. Use this in a normal chat; the run-based save_persona_cv is only for a \"create CV with AI\" run. " +
			"Returns the new CV's id. Fails if the persona is unknown, the template is unknown, data is invalid, pdfResource is not a real promoted Media " +
			"file, or that PDF file already belongs to another CV (each CV needs its own generate_pdf call).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveCvForPersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		doc, err := SaveCvForPersona(args.PersonaID, args.TemplateID, args.Title, args.Data.ToCvData(), args.PdfResource)
		switch {
		case errors.Is(err, persona.ErrUnknownPersona):
			return db.ErrResult(fmt.Sprintf("unknown persona id %q", args.PersonaID)), nil, nil
		case err != nil:
			return db.ErrResult(err.Error()), nil, nil
		}
		return db.JSONResult(map[string]string{"cvDocumentId": doc.ID, "status": "saved"})
	})
}

type savePersonaCvArgs struct {
	RunID       string             `json:"runId" jsonschema:"the run id given in the request"`
	TemplateID  string             `json:"templateId" jsonschema:"the same templateId you passed to render_cv_document for this CV"`
	Title       string             `json:"title" jsonschema:"a short, human-readable name for the CV, e.g. 'Jane Doe - Backend Engineer'"`
	Data        CvDocumentDataArgs `json:"data" jsonschema:"the exact SAME data object you passed to render_cv_document for this CV"`
	PdfResource string             `json:"pdfResource" jsonschema:"the exact 'uri' field from generate_pdf's own returned resource link, unmodified"`
}

func RegisterSavePersonaCv(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_persona_cv",
		Description: "Record the CV PDF you just rendered for the persona of this run (render_cv_document, then generate_pdf) — call " +
			"this ONCE, right after generate_pdf returns its resource link, passing that link's own 'uri' field unmodified. " +
			"templateId and data must be the EXACT same values you passed to render_cv_document. data's free-text fields are " +
			"PLAIN TEXT ONLY, no HTML or markup. The CV is saved on the run's own persona; you cannot choose another. " +
			"Returns the new CV's id. Fails if the run is unknown or no longer running, it already saved a CV, the template " +
			"is unknown, data is invalid, or pdfResource is not a real promoted Media file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args savePersonaCvArgs) (*mcp.CallToolResult, any, error) {
		if args.RunID == "" {
			return db.ErrResult("runId is required"), nil, nil
		}
		doc, err := SaveCvAiDocument(args.RunID, args.TemplateID, args.Title, args.Data.ToCvData(), args.PdfResource)
		switch {
		case errors.Is(err, ErrUnknownCvAiRun):
			return db.ErrResult(fmt.Sprintf("unknown run id %q", args.RunID)), nil, nil
		case err != nil:
			return db.ErrResult(err.Error()), nil, nil
		}
		return db.JSONResult(map[string]string{"cvDocumentId": doc.ID, "status": "saved"})
	})
}
