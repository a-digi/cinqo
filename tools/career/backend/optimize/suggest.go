package optimize

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// What the AI hands over (plan 258, 2.3 and 2.4): one improved text at a time, which is only STORED for the user to approve or decline.
// Nothing in this file writes to a profile or a persona.

const (
	maxText   = 4000
	maxTitle  = 200
	maxReason = 300
	// MaxSuggestionsPerRun bounds one run: a text can be suggested again (that replaces the earlier suggestion), so this is "texts changed".
	MaxSuggestionsPerRun = 100
)

// ErrRunNotGenerating: the AI hands suggestions over only while its run is generating.
var ErrRunNotGenerating = errors.New("this run is not generating: the AI can no longer hand suggestions over")

// fieldsOfKind is which fields a kind of text has.
var fieldsOfKind = map[string][]string{
	KindProfileDetails:   {FieldSummary},
	KindPersonaDetails:   {FieldSummary},
	KindExperience:       {FieldTitle, FieldDescription},
	KindCustomExperience: {FieldTitle, FieldDescription},
	KindEducation:        {FieldDescription},
	KindCourse:           {FieldDescription},
	KindHobby:            {FieldDescription},
}

func validPair(kind, field string) bool {
	for _, f := range fieldsOfKind[kind] {
		if f == field {
			return true
		}
	}
	return false
}

// bullets counts the lines of a text that start with "- ": a suggestion keeps exactly as many (a small model turns an intro line into a bullet, or merges two: the layout stays).
func bullets(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			n++
		}
	}
	return n
}

type runRow struct {
	subjectKind, subjectID, status, focus string
}

func loadRun(q interface {
	QueryRow(string, ...any) *sql.Row
}, runID string) (runRow, error) {
	var r runRow
	switch err := q.QueryRow(`SELECT subject_kind, subject_id, status, focus FROM optimize_runs WHERE id = ?`, runID).Scan(&r.subjectKind, &r.subjectID, &r.status, &r.focus); {
	case errors.Is(err, sql.ErrNoRows):
		return r, ErrUnknownRun
	case err != nil:
		return r, err
	}
	return r, nil
}

// Stored is the answer to a stored suggestion.
type Stored struct {
	SuggestionID string `json:"suggestionId"`
	// Suggested is how many texts this run has suggestions for now.
	Suggested int `json:"suggested"`
	// Replaced is true when an earlier suggestion for the same text was replaced.
	Replaced bool `json:"replaced"`
}

// AddSuggestion stores one improved text for a generating run. The text must be one the subject shows (see get_optimize_subject), really
// different, within the limits and keep exactly the number of bullet points. A second suggestion for the same text replaces the first.
func AddSuggestion(runID, kind, itemID, field, text, reason string) (Stored, error) {
	run, err := loadRun(db.CareerDB, runID)
	if err != nil {
		return Stored{}, err
	}
	if run.status != runGenerating {
		return Stored{}, ErrRunNotGenerating
	}
	kind, itemID, field = strings.TrimSpace(kind), strings.TrimSpace(itemID), strings.TrimSpace(field)
	if !validPair(kind, field) {
		return Stored{}, invalid("kind %q with field %q does not exist: the kinds are profileDetails, personaDetails, experience, customExperience, education, course and hobby, with the fields in get_optimize_subject", kind, field)
	}
	subject, err := ReadSubject(run.subjectKind, run.subjectID)
	if err != nil {
		return Stored{}, err
	}
	cur, ok := subject.find(kind, itemID, field)
	if !ok {
		return Stored{}, invalid("there is no %s %s of %q to improve here: use the kind, itemId and field exactly as get_optimize_subject gives them", kind, field, itemID)
	}
	text = strings.TrimSpace(text)
	reason = strings.TrimSpace(reason)
	switch {
	case text == "":
		return Stored{}, invalid("text is empty: send the improved text, or no suggestion")
	case utf8.RuneCountInString(text) > maxText:
		return Stored{}, invalid("text is too long (max %d characters)", maxText)
	case field == FieldTitle && (strings.ContainsAny(text, "\r\n") || utf8.RuneCountInString(text) > maxTitle):
		return Stored{}, invalid("a title is one line of at most %d characters", maxTitle)
	case text == strings.TrimSpace(cur.Text):
		return Stored{}, invalid("the text is the same as it is now: send a suggestion only for a text you really improve")
	case bullets(text) != bullets(cur.Text):
		return Stored{}, invalid("the text has %d bullet points (lines starting with \"- \") and the original has %d: keep exactly the same number: never drop, merge or add a bullet point, and keep a line that is not a bullet point as it is", bullets(text), bullets(cur.Text))
	case utf8.RuneCountInString(reason) > maxReason:
		return Stored{}, invalid("reason is too long (max %d characters): one sentence", maxReason)
	}
	if err := checkLanguage(cur.Text, text, reason); err != nil {
		return Stored{}, err
	}

	tx, err := db.CareerDB.Begin()
	if err != nil {
		return Stored{}, err
	}
	defer func() { _ = tx.Rollback() }()
	// The run may have been finished or removed while the subject was read.
	again, err := loadRun(tx, runID)
	if err != nil {
		return Stored{}, err
	}
	if again.status != runGenerating {
		return Stored{}, ErrRunNotGenerating
	}
	var existing string
	replaced := false
	switch err := tx.QueryRow(`SELECT id FROM optimize_suggestions WHERE run_id = ? AND kind = ? AND item_id = ? AND field = ?`, runID, kind, itemID, field).Scan(&existing); {
	case err == nil:
		replaced = true
	case errors.Is(err, sql.ErrNoRows):
	default:
		return Stored{}, err
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM optimize_suggestions WHERE run_id = ?`, runID).Scan(&count); err != nil {
		return Stored{}, err
	}
	if replaced {
		if _, err := tx.Exec(`UPDATE optimize_suggestions SET before_text = ?, after_text = ?, reason = ? WHERE id = ?`, cur.Text, text, reason, existing); err != nil {
			return Stored{}, err
		}
		return Stored{SuggestionID: existing, Suggested: count, Replaced: true}, tx.Commit()
	}
	if count >= MaxSuggestionsPerRun {
		return Stored{}, invalid("a run holds at most %d suggestions: stop here", MaxSuggestionsPerRun)
	}
	id := uuid.NewString()
	if _, err := tx.Exec(
		`INSERT INTO optimize_suggestions (id, run_id, position, kind, item_id, field, before_text, after_text, reason)
		 VALUES (?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM optimize_suggestions WHERE run_id = ?), ?, ?, ?, ?, ?, ?)`,
		id, runID, runID, kind, itemID, field, cur.Text, text, reason,
	); err != nil {
		return Stored{}, err
	}
	return Stored{SuggestionID: id, Suggested: count + 1}, tx.Commit()
}

// SubjectForAI is what get_optimize_subject answers: the texts, what the user asked for, and how many suggestions the run holds.
type SubjectForAI struct {
	Subject
	Focus     string `json:"focus,omitempty"`
	Suggested int    `json:"suggested"`
}

// GetSubjectForRun reads the texts of the subject of a generating run.
func GetSubjectForRun(runID string) (SubjectForAI, error) {
	run, err := loadRun(db.CareerDB, runID)
	if err != nil {
		return SubjectForAI{}, err
	}
	if run.status != runGenerating {
		return SubjectForAI{}, ErrRunNotGenerating
	}
	s, err := ReadSubject(run.subjectKind, run.subjectID)
	if err != nil {
		return SubjectForAI{}, err
	}
	out := SubjectForAI{Subject: s, Focus: run.focus}
	err = db.CareerDB.QueryRow(`SELECT COUNT(*) FROM optimize_suggestions WHERE run_id = ?`, runID).Scan(&out.Suggested)
	return out, err
}

type getSubjectArgs struct {
	RunID string `json:"runId" jsonschema:"the id of the optimization run (a UUID: it is in the first message, after the word runId)"`
}

type suggestArgs struct {
	RunID  string `json:"runId" jsonschema:"the id of the optimization run"`
	Kind   string `json:"kind" jsonschema:"the kind of the text, exactly as get_optimize_subject gives it"`
	ItemID string `json:"itemId" jsonschema:"the id of the item, exactly as get_optimize_subject gives it (empty for a summary)"`
	Field  string `json:"field" jsonschema:"the field of the text: summary, title or description, as get_optimize_subject gives it"`
	Text   string `json:"text" jsonschema:"the improved text: the whole text, not only the changed part"`
	Reason string `json:"reason" jsonschema:"one short sentence: why this is better"`
}

func toolError(runID string, err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, ErrUnknownRun):
		return db.ErrResult(fmt.Sprintf("unknown optimization run id %q", runID))
	case errors.Is(err, ErrUnknownSubject):
		return db.ErrResult("the profile or target role of this run no longer exists")
	}
	return db.ErrResult(err.Error())
}

// RegisterGetOptimizeSubject registers get_optimize_subject.
func RegisterGetOptimizeSubject(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_optimize_subject",
		Description: "Read the texts of the profile or target role you are asked to optimize: every text that can be improved, each with its kind, itemId, field, a label, some context (dates, company) and its text as it is now. " +
			"Call it once at the start with the run's id (a UUID: it is in the first message, after the word runId). It also tells what the user asked you to focus on. " +
			"It only reads. Fails when the run is unknown or is no longer running.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getSubjectArgs) (*mcp.CallToolResult, any, error) {
		runID := strings.TrimSpace(args.RunID)
		if runID == "" {
			return db.ErrResult("runId is required: the id of the optimization run, a UUID"), nil, nil
		}
		s, err := GetSubjectForRun(runID)
		if err != nil {
			return toolError(runID, err), nil, nil
		}
		return db.JSONResult(s)
	})
}

// RegisterSuggestOptimization registers suggest_optimization.
func RegisterSuggestOptimization(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "suggest_optimization",
		Description: "Hand over ONE improved text to the user, who then approves or declines it. It changes nothing in the profile or target role: only the user's approval does. " +
			"Call it once for each text you really improve, with the runId, the text's kind, itemId and field exactly as get_optimize_subject gave them, the WHOLE improved text and one short sentence saying why it is better. " +
			"Improve the wording only: write the improved text AND the reason in the SAME language as the text you improve (a German text gets a German suggestion, never an English one; never translate), keep every fact, number, name and date, never add anything the text does not say, and keep exactly the same number of bullet points (lines starting with \"- \"): never drop, merge or add one. " +
			"A title stays one line. Suggest nothing for a text that is already good. Sending the same text again replaces your earlier suggestion for it. " +
			"Returns the suggestion's id. Fails (and says why) when the run is unknown or finished, the text does not exist, the improved text is empty, unchanged, too long, changes the number of bullet points or is in another language than the text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args suggestArgs) (*mcp.CallToolResult, any, error) {
		runID := strings.TrimSpace(args.RunID)
		if runID == "" {
			return db.ErrResult("runId is required: the id of the optimization run, a UUID"), nil, nil
		}
		st, err := AddSuggestion(runID, args.Kind, args.ItemID, args.Field, args.Text, args.Reason)
		if err != nil {
			return toolError(runID, err), nil, nil
		}
		return db.JSONResult(st)
	})
}

type addSuggestionRequest struct {
	RunID  string `json:"runId"`
	Kind   string `json:"kind"`
	ItemID string `json:"itemId"`
	Field  string `json:"field"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// SuggestionsHandler handles POST optimize-runs/suggestions: the same store as the tool, for the page's code driver (a small local
// model answers one text at a time and the page hands each answer over).
func SuggestionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body addSuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.RunID) == "" {
		http.Error(w, "runId, kind, itemId, field and text are required", http.StatusBadRequest)
		return
	}
	st, err := AddSuggestion(body.RunID, body.Kind, body.ItemID, body.Field, body.Text, body.Reason)
	if err != nil {
		writeError(w, "store the suggestion", err)
		return
	}
	db.WriteJSON(w, st)
}
