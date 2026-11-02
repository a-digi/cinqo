// Package education is the profile's education (plan/ai/tools/career/step-173-persona-skills-experience-and-education.md, step 6): the
// schools, degrees and courses of the job seeker. It is global to the profile (many per profile, deleted with it) and a persona neither
// chooses nor changes it. Every check comes before the write. Dates are free text of the shapes YYYY, YYYY-MM or YYYY-MM-DD, like the
// work experience dates; an empty end date means the person is still studying there.
package education

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

var (
	ErrUnknownProfile = errors.New("unknown profile id")
	// ErrUnknownEducation is an education entry that does not exist.
	ErrUnknownEducation    = errors.New("unknown education id")
	ErrInstitutionRequired = errors.New("an education needs an institution")
	// ErrInvalidDate: a date that is not YYYY, YYYY-MM or YYYY-MM-DD (or not a real one).
	ErrInvalidDate = errors.New("a date must be YYYY, YYYY-MM or YYYY-MM-DD")
	// ErrEndBeforeStart: the end date is before the start date.
	ErrEndBeforeStart = errors.New("the end date cannot be before the start date")
)

// Education is one entry of a profile's education.
type Education struct {
	ID           string `json:"id"`
	ProfileID    string `json:"profileId"`
	Institution  string `json:"institution"`
	Degree       string `json:"degree"`
	FieldOfStudy string `json:"fieldOfStudy"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"`
	Description  string `json:"description"`
	CreatedAt    string `json:"createdAt"`
}

// Input is what a new entry is made of.
type Input struct {
	Institution  string
	Degree       string
	FieldOfStudy string
	StartDate    string
	EndDate      string
	Description  string
}

// Patch changes only the fields that are set.
type Patch struct {
	Institution, Degree, FieldOfStudy, StartDate, EndDate, Description *string
}

var dateShape = regexp.MustCompile(`^(\d{4})(?:-(\d{2})(?:-(\d{2}))?)?$`)

// dateKey is a sortable number for a valid date (empty is valid and has none), or ok false when it is not one.
func dateKey(text string) (key int, set bool, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false, true
	}
	m := dateShape.FindStringSubmatch(text)
	if m == nil {
		return 0, false, false
	}
	year, _ := strconv.Atoi(m[1])
	month, day := 1, 1
	if m[2] != "" {
		month, _ = strconv.Atoi(m[2])
	}
	if m[3] != "" {
		day, _ = strconv.Atoi(m[3])
	}
	if year < 1900 || year > 2100 || month < 1 || month > 12 || day < 1 || day > 31 {
		return 0, false, false
	}
	return year*10000 + month*100 + day, true, true
}

// validate checks a whole entry: an institution, valid dates, the end not before the start.
func validate(in Input) error {
	if strings.TrimSpace(in.Institution) == "" {
		return ErrInstitutionRequired
	}
	start, startSet, ok := dateKey(in.StartDate)
	if !ok {
		return fmt.Errorf("start date %q: %w", in.StartDate, ErrInvalidDate)
	}
	end, endSet, ok := dateKey(in.EndDate)
	if !ok {
		return fmt.Errorf("end date %q: %w", in.EndDate, ErrInvalidDate)
	}
	if startSet && endSet && end < start {
		return ErrEndBeforeStart
	}
	return nil
}

func clean(in Input) Input {
	return Input{
		Institution:  strings.TrimSpace(in.Institution),
		Degree:       strings.TrimSpace(in.Degree),
		FieldOfStudy: strings.TrimSpace(in.FieldOfStudy),
		StartDate:    strings.TrimSpace(in.StartDate),
		EndDate:      strings.TrimSpace(in.EndDate),
		Description:  in.Description,
	}
}

// List is the profile's education, the entry still going on first, then the latest end, then the latest start. An unknown profile has none.
func List(profileID string) ([]Education, error) {
	rows, err := db.CareerDB.Query(
		`SELECT id, profile_id, institution, degree, field_of_study, start_date, end_date, description, created_at
		 FROM profile_education WHERE profile_id = ?
		 ORDER BY CASE WHEN COALESCE(end_date, '') = '' THEN 0 ELSE 1 END ASC, COALESCE(end_date, '') DESC, COALESCE(start_date, '') DESC, created_at DESC, id ASC`,
		profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Education{}
	for rows.Next() {
		var e Education
		var start, end, description sql.NullString
		if err := rows.Scan(&e.ID, &e.ProfileID, &e.Institution, &e.Degree, &e.FieldOfStudy, &start, &end, &description, &e.CreatedAt); err != nil {
			return nil, err
		}
		e.StartDate, e.EndDate, e.Description = start.String, end.String, description.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// ByID is one entry, or ErrUnknownEducation.
func ByID(id string) (Education, error) {
	var profileID string
	switch err := db.CareerDB.QueryRow(`SELECT profile_id FROM profile_education WHERE id = ?`, id).Scan(&profileID); {
	case err == sql.ErrNoRows:
		return Education{}, ErrUnknownEducation
	case err != nil:
		return Education{}, err
	}
	all, err := List(profileID)
	if err != nil {
		return Education{}, err
	}
	for _, e := range all {
		if e.ID == id {
			return e, nil
		}
	}
	return Education{}, ErrUnknownEducation
}

func requireProfile(id string) error {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, id).Scan(&one); {
	case err == sql.ErrNoRows:
		return ErrUnknownProfile
	case err != nil:
		return err
	}
	return nil
}

// Add adds an entry to a profile. Every field is checked first (ErrInstitutionRequired, ErrInvalidDate, ErrEndBeforeStart) and the
// profile must exist (ErrUnknownProfile); nothing is written on a refusal.
func Add(profileID string, in Input) (Education, error) {
	in = clean(in)
	if err := validate(in); err != nil {
		return Education{}, err
	}
	if err := requireProfile(profileID); err != nil {
		return Education{}, err
	}
	id := uuid.NewString()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO profile_education (id, profile_id, institution, degree, field_of_study, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, profileID, in.Institution, in.Degree, in.FieldOfStudy, in.StartDate, in.EndDate, in.Description,
	); err != nil {
		return Education{}, err
	}
	return ByID(id)
}

// Update changes the fields that are set; the whole entry as it would be is checked before the write (so a change of one date is
// checked against the other). An unknown entry is ErrUnknownEducation.
func Update(id string, p Patch) (Education, error) {
	current, err := ByID(id)
	if err != nil {
		return Education{}, err
	}
	in := Input{
		Institution: current.Institution, Degree: current.Degree, FieldOfStudy: current.FieldOfStudy,
		StartDate: current.StartDate, EndDate: current.EndDate, Description: current.Description,
	}
	if p.Institution != nil {
		in.Institution = *p.Institution
	}
	if p.Degree != nil {
		in.Degree = *p.Degree
	}
	if p.FieldOfStudy != nil {
		in.FieldOfStudy = *p.FieldOfStudy
	}
	if p.StartDate != nil {
		in.StartDate = *p.StartDate
	}
	if p.EndDate != nil {
		in.EndDate = *p.EndDate
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	in = clean(in)
	if err := validate(in); err != nil {
		return Education{}, err
	}
	if _, err := db.CareerDB.Exec(
		`UPDATE profile_education SET institution = ?, degree = ?, field_of_study = ?, start_date = ?, end_date = ?, description = ? WHERE id = ?`,
		in.Institution, in.Degree, in.FieldOfStudy, in.StartDate, in.EndDate, in.Description, id,
	); err != nil {
		return Education{}, err
	}
	return ByID(id)
}

// Delete removes an entry, or ErrUnknownEducation.
func Delete(id string) error {
	res, err := db.CareerDB.Exec(`DELETE FROM profile_education WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUnknownEducation
	}
	return nil
}

// --- AI tools ---

func errResult(action string, err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, ErrUnknownProfile):
		return db.ErrResult("unknown profile id")
	case errors.Is(err, ErrUnknownEducation):
		return db.ErrResult("unknown education id (see list_education)")
	case errors.Is(err, ErrInstitutionRequired), errors.Is(err, ErrInvalidDate), errors.Is(err, ErrEndBeforeStart):
		return db.ErrResult(err.Error())
	}
	return db.ErrResult(fmt.Sprintf("failed to %s: %v", action, err))
}

type listArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile (from list_profiles)"`
}

func RegisterListEducation(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_education",
		Description: "List a profile's education (schools, degrees, courses), the one still going on first. Education is global to the profile; personas do not choose it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		list, err := List(args.ProfileID)
		if err != nil {
			return errResult("list education", err), nil, nil
		}
		return db.JSONResult(map[string]any{"education": list})
	})
}

type addArgs struct {
	ProfileID    string `json:"profileId" jsonschema:"the profile to add the education to"`
	Institution  string `json:"institution" jsonschema:"the school, university or course provider"`
	Degree       string `json:"degree,omitempty" jsonschema:"the degree or certificate, e.g. \"B.Sc.\""`
	FieldOfStudy string `json:"fieldOfStudy,omitempty" jsonschema:"what was studied"`
	StartDate    string `json:"startDate,omitempty" jsonschema:"when it started: YYYY, YYYY-MM or YYYY-MM-DD"`
	EndDate      string `json:"endDate,omitempty" jsonschema:"when it ended: YYYY, YYYY-MM or YYYY-MM-DD; omit while still studying"`
	Description  string `json:"description,omitempty" jsonschema:"anything worth saying about it"`
}

func RegisterAddEducation(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_education",
		Description: "Add an education entry to a profile. An institution is required; dates are YYYY, YYYY-MM or YYYY-MM-DD and the end cannot be before the start. Returns the entry's id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		e, err := Add(args.ProfileID, Input{
			Institution: args.Institution, Degree: args.Degree, FieldOfStudy: args.FieldOfStudy,
			StartDate: args.StartDate, EndDate: args.EndDate, Description: args.Description,
		})
		if err != nil {
			return errResult("add education", err), nil, nil
		}
		return db.JSONResult(map[string]string{"id": e.ID})
	})
}

type updateArgs struct {
	ID           string  `json:"id" jsonschema:"the education entry's id (from list_education)"`
	Institution  *string `json:"institution,omitempty" jsonschema:"the school, university or course provider"`
	Degree       *string `json:"degree,omitempty" jsonschema:"the degree or certificate"`
	FieldOfStudy *string `json:"fieldOfStudy,omitempty" jsonschema:"what was studied"`
	StartDate    *string `json:"startDate,omitempty" jsonschema:"YYYY, YYYY-MM or YYYY-MM-DD; an empty text clears it"`
	EndDate      *string `json:"endDate,omitempty" jsonschema:"YYYY, YYYY-MM or YYYY-MM-DD; an empty text means still studying"`
	Description  *string `json:"description,omitempty" jsonschema:"anything worth saying about it"`
}

func RegisterUpdateEducation(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_education",
		Description: "Update an education entry: only the fields provided change. The whole entry must stay valid (an institution, valid dates, the end not before the start).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if _, err := Update(args.ID, Patch{
			Institution: args.Institution, Degree: args.Degree, FieldOfStudy: args.FieldOfStudy,
			StartDate: args.StartDate, EndDate: args.EndDate, Description: args.Description,
		}); err != nil {
			return errResult("update education", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type removeArgs struct {
	ID string `json:"id" jsonschema:"the education entry's id (from list_education)"`
}

func RegisterRemoveEducation(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_education",
		Description: "Remove an education entry from a profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := Delete(args.ID); err != nil {
			return errResult("remove education", err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "removed"}}}, nil, nil
	})
}
