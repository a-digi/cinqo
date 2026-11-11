package cvbuilder

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/cv/cvbuilder/templates"
	"career-tool-backend/db"
)

// hand_over_cv (plan 256, step 4): how a model-driven "create a CV with AI" run hands over its choice without making the PDF. The model reads the persona, chooses the design and
// tailors the data as before, but it can never give the CV a photo: the photo travels inside the XHTML as base64 and the XHTML would have to pass through the model twice. So the
// model hands over the design, the title and the data, the run stores them, and the page makes the CV through the wizard's route (with the profile's photo when it is wanted) and
// ties it to the run. The tool checks the content exactly as save_persona_cv does and stores nothing but this choice.

var ErrCvAiChoiceAlreadyHandedOver = errors.New("this run already handed over its CV")

// CvAiChoice is what a run handed over: the design, the title and the data the CV is made from.
type CvAiChoice struct {
	TemplateID string           `json:"templateId"`
	Title      string           `json:"title"`
	Data       templates.CvData `json:"data"`
}

// StoreCvAiChoice records the choice of a generating run: the same content checks as SaveCvAiDocument (title, name, design, data), the run must exist, still work, have no CV and
// no choice yet. One UPDATE decides a race: the run takes the choice only while it still works and has neither a CV nor a choice.
func StoreCvAiChoice(runID, templateID, title string, data templates.CvData) error {
	title, err := checkCvContent(templateID, title, data)
	if err != nil {
		return err
	}
	var status string
	var cv, choice sql.NullString
	switch err := db.CareerDB.QueryRow(`SELECT status, cv_document_id, choice_json FROM cv_ai_runs WHERE id = ?`, runID).Scan(&status, &cv, &choice); {
	case errors.Is(err, sql.ErrNoRows):
		return ErrUnknownCvAiRun
	case err != nil:
		return err
	}
	if status != cvAiRunning {
		return ErrCvAiRunNotGenerating
	}
	if cv.Valid {
		return ErrCvAiAlreadySaved
	}
	if choice.Valid {
		return ErrCvAiChoiceAlreadyHandedOver
	}
	raw, err := json.Marshal(CvAiChoice{TemplateID: templateID, Title: title, Data: data})
	if err != nil {
		return err
	}
	res, err := db.CareerDB.Exec(
		`UPDATE cv_ai_runs SET choice_json = ? WHERE id = ? AND status = 'generating' AND cv_document_id IS NULL AND choice_json IS NULL`, string(raw), runID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrCvAiChoiceAlreadyHandedOver
	}
	return nil
}

type handOverCvArgs struct {
	RunID      string             `json:"runId" jsonschema:"the run id given in the request"`
	TemplateID string             `json:"templateId" jsonschema:"the id of the ONE design you chose, from list_cv_templates"`
	Title      string             `json:"title" jsonschema:"a short, human-readable name for the CV, e.g. 'Jane Doe - Backend Engineer'"`
	Data       CvDocumentDataArgs `json:"data" jsonschema:"the CV's data: start from get_persona_cv_defaults and tailor it, never invent anything"`
}

// RegisterHandOverCv adds hand_over_cv: the last call of a model-driven run.
func RegisterHandOverCv(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "hand_over_cv",
		Description: "Hand over the CV you decided on for the persona of this run: the ONE design you chose (templateId, from list_cv_templates), a short title and the CV's data " +
			"(start from get_persona_cv_defaults, tailor it, never invent a skill, employer, title, date or language). Call this ONCE, as your last tool call. You do NOT render " +
			"anything and you do NOT make a PDF: the application makes the finished CV (with the person's photo when it is wanted) from what you hand over. data's free-text fields " +
			"are PLAIN TEXT ONLY, no HTML or markup. The CV is made for the run's own persona; you cannot choose another. Fails if the run is unknown or no longer running, it already " +
			"handed over a CV, the template is unknown or data is invalid.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args handOverCvArgs) (*mcp.CallToolResult, any, error) {
		if args.RunID == "" {
			return db.ErrResult("runId is required"), nil, nil
		}
		err := StoreCvAiChoice(args.RunID, args.TemplateID, args.Title, args.Data.ToCvData())
		switch {
		case errors.Is(err, ErrUnknownCvAiRun):
			return db.ErrResult(fmt.Sprintf("unknown run id %q", args.RunID)), nil, nil
		case err != nil:
			return db.ErrResult(err.Error()), nil, nil
		}
		return db.JSONResult(map[string]string{"status": "handed over"})
	})
}
