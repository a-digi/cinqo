package cvbuilder

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/media"
)

// save_persona_cv: how the AI hands over the CV it made for a persona (step 115). It takes the run id,
// so the persona comes from the run, never from the AI, and it saves at most one CV per run.

const maxCvAiTitle = 200

var ErrCvAiRunNotGenerating = errors.New("this run is not generating")
var ErrCvAiAlreadySaved = errors.New("this run already saved its CV")

// SaveCvAiDocument records the CV the AI made in a generating run, on the run's own persona (linked
// as the persona it was built from), and ties it to the run. The checks on the content come first.
func SaveCvAiDocument(runID, templateID, title string, data templates.CvData, mediaFileID string) (CvDocument, error) {
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return CvDocument{}, errors.New("title is required")
	case utf8.RuneCountInString(title) > maxCvAiTitle:
		return CvDocument{}, fmt.Errorf("title is too long (max %d characters)", maxCvAiTitle)
	case strings.TrimSpace(data.FullName) == "":
		return CvDocument{}, errors.New("data.fullName is required")
	case !templates.IsKnownTemplate(templateID):
		return CvDocument{}, fmt.Errorf("unknown template id %q — call list_cv_templates for valid ids", templateID)
	}
	if err := ValidateCvData(data); err != nil {
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

	if strings.Contains(mediaFileID, "://") || strings.HasPrefix(mediaFileID, "/") {
		return CvDocument{}, errors.New("pdfResource still looks like a raw resource link (" + mediaFileID + "), not a promoted Media file id — " +
			"call generate_pdf again and pass its own returned 'uri' field straight into this call's pdfResource argument, exactly as returned")
	}
	if strings.TrimSpace(mediaFileID) == "" {
		return CvDocument{}, errors.New("pdfResource is required")
	}
	if err := media.SetTitle(mediaFileID, title+".pdf"); err != nil {
		if errors.Is(err, media.ErrMediaNotOwned) {
			return CvDocument{}, errors.New("pdfResource (" + mediaFileID + ") does not correspond to a real, already-promoted Media file owned by " +
				"this tool — pass generate_pdf's own returned 'uri' field unmodified")
		}
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
