// persona_details.go is the AI-facing read/write surface over
// career.db's own persona_details/career_skills/career_experience
// tables — a persona's own career positioning (headline, summary,
// location, desired titles/locations, min salary), skills, and
// experience. Renamed from profile.go/career_profile as of step 10:
// this was never the job seeker's own profile, it was that persona's
// own career details — "Profile" now means the job seeker (see
// profile.go, package main). Unlike browser's own login_credentials.go,
// there is no isolation boundary here at all — the AI reads and writes
// this data directly and freely, by design: this isn't a secret, it's
// the thing the AI is meant to help build and use. See
// plan/ai/tools/career/step-03-career-profile-tools.md,
// plan/ai/tools/career/step-08-persona.md, and
// plan/ai/tools/career/step-10-job-seeker-profile.md.
package persona

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

type personaDetails struct {
	PersonaID        string `json:"personaId"`
	Headline         string `json:"headline"`
	Summary          string `json:"summary"`
	Location         string `json:"location"`
	DesiredTitles    string `json:"desiredTitles"`
	DesiredLocations string `json:"desiredLocations"`
	MinSalary        int64  `json:"minSalary,omitempty"`
}

type careerExperience struct {
	ID          string `json:"id"`
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	Description string `json:"description,omitempty"`
}

type getPersonaDetailsResult struct {
	PersonaDetails *personaDetails    `json:"personaDetails"`
	Skills         []string           `json:"skills"`
	Experience     []careerExperience `json:"experience"`
}

// FetchPersonaDetails reads the given persona's own details row (nil,
// not an error, if none has been set yet on an otherwise-valid
// persona — "not configured yet" is a normal, representable state,
// matching browser's own lookupCredential convention), every skill,
// and every experience entry, all scoped to personaId. Requires
// personaId to already exist — an unknown one is a real error, not an
// empty result, per step 8's own "forced to be mapped" design.
func FetchPersonaDetails(personaId string) (getPersonaDetailsResult, error) {
	var result getPersonaDetailsResult

	if err := requirePersonaExists(personaId); err != nil {
		return result, err
	}

	var d personaDetails
	var minSalary sql.NullInt64
	err := db.CareerDB.QueryRow(
		`SELECT headline, summary, location, desired_titles, desired_locations, min_salary
		 FROM persona_details WHERE persona_id = ?`, personaId,
	).Scan(&d.Headline, &d.Summary, &d.Location, &d.DesiredTitles, &d.DesiredLocations, &minSalary)
	switch {
	case err == sql.ErrNoRows:
		result.PersonaDetails = nil
	case err != nil:
		return result, err
	default:
		d.PersonaID = personaId
		d.MinSalary = minSalary.Int64
		result.PersonaDetails = &d
	}

	skillRows, err := db.CareerDB.Query(`SELECT skill FROM career_skills WHERE persona_id = ? ORDER BY skill ASC`, personaId)
	if err != nil {
		return result, err
	}
	defer skillRows.Close()
	result.Skills = []string{}
	for skillRows.Next() {
		var s string
		if err := skillRows.Scan(&s); err != nil {
			return result, err
		}
		result.Skills = append(result.Skills, s)
	}
	if err := skillRows.Err(); err != nil {
		return result, err
	}

	expRows, err := db.CareerDB.Query(
		`SELECT id, company, title, start_date, end_date, description
		 FROM career_experience WHERE persona_id = ? ORDER BY created_at DESC`, personaId)
	if err != nil {
		return result, err
	}
	defer expRows.Close()
	result.Experience = []careerExperience{}
	for expRows.Next() {
		var e careerExperience
		var startDate, endDate, description sql.NullString
		if err := expRows.Scan(&e.ID, &e.Company, &e.Title, &startDate, &endDate, &description); err != nil {
			return result, err
		}
		e.StartDate = startDate.String
		e.EndDate = endDate.String
		e.Description = description.String
		result.Experience = append(result.Experience, e)
	}
	return result, expRows.Err()
}

// UpdatePersonaDetailsArgs is a partial update — only fields present
// (via the *string/*int64 pointers) are touched. Upserts the row
// keyed by PersonaID, which must already exist.
type UpdatePersonaDetailsArgs struct {
	PersonaID        string  `json:"personaId"`
	Headline         *string `json:"headline,omitempty"`
	Summary          *string `json:"summary,omitempty"`
	Location         *string `json:"location,omitempty"`
	DesiredTitles    *string `json:"desiredTitles,omitempty"`
	DesiredLocations *string `json:"desiredLocations,omitempty"`
	MinSalary        *int64  `json:"minSalary,omitempty"`
}

func UpdatePersonaDetails(args UpdatePersonaDetailsArgs) error {
	if err := requirePersonaExists(args.PersonaID); err != nil {
		return err
	}

	// A plain upsert seeded from whatever's already there, then
	// overwritten field-by-field only where args supplied a value —
	// simplest correct way to express "partial update" against
	// SQLite's own INSERT ... ON CONFLICT without hand-building a
	// dynamic SET clause.
	existing, err := FetchPersonaDetails(args.PersonaID)
	if err != nil {
		return err
	}
	d := personaDetails{}
	if existing.PersonaDetails != nil {
		d = *existing.PersonaDetails
	}
	if args.Headline != nil {
		d.Headline = *args.Headline
	}
	if args.Summary != nil {
		d.Summary = *args.Summary
	}
	if args.Location != nil {
		d.Location = *args.Location
	}
	if args.DesiredTitles != nil {
		d.DesiredTitles = *args.DesiredTitles
	}
	if args.DesiredLocations != nil {
		d.DesiredLocations = *args.DesiredLocations
	}
	if args.MinSalary != nil {
		d.MinSalary = *args.MinSalary
	}

	_, err = db.CareerDB.Exec(
		`INSERT INTO persona_details (persona_id, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(persona_id) DO UPDATE SET
			headline = excluded.headline,
			summary = excluded.summary,
			location = excluded.location,
			desired_titles = excluded.desired_titles,
			desired_locations = excluded.desired_locations,
			min_salary = excluded.min_salary,
			updated_at = excluded.updated_at`,
		args.PersonaID, d.Headline, d.Summary, d.Location, d.DesiredTitles, d.DesiredLocations, d.MinSalary,
	)
	return err
}

func AddCareerSkill(personaId, skill string) error {
	if err := requirePersonaExists(personaId); err != nil {
		return err
	}
	_, err := db.CareerDB.Exec(
		`INSERT INTO career_skills (id, persona_id, skill) VALUES (?, ?, ?) ON CONFLICT(persona_id, skill) DO NOTHING`,
		uuid.NewString(), personaId, skill,
	)
	return err
}

func RemoveCareerSkill(personaId, skill string) error {
	if err := requirePersonaExists(personaId); err != nil {
		return err
	}
	_, err := db.CareerDB.Exec(`DELETE FROM career_skills WHERE persona_id = ? AND skill = ?`, personaId, skill)
	return err
}

func AddCareerExperience(personaId, company, title, startDate, endDate, description string) (string, error) {
	if err := requirePersonaExists(personaId); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err := db.CareerDB.Exec(
		`INSERT INTO career_experience (id, persona_id, company, title, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, personaId, company, title, startDate, endDate, description,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func RemoveCareerExperience(personaId, id string) error {
	if err := requirePersonaExists(personaId); err != nil {
		return err
	}
	_, err := db.CareerDB.Exec(`DELETE FROM career_experience WHERE id = ? AND persona_id = ?`, id, personaId)
	return err
}

// UpdateCareerExperienceArgs mirrors UpdatePersonaDetailsArgs's own
// partial-update shape — only fields provided are changed. ID is
// required and must belong to PersonaID; updating an unknown id (or
// one belonging to a different persona) is a benign no-op (0 rows
// affected), the same posture RemoveCareerExperience already has.
type UpdateCareerExperienceArgs struct {
	PersonaID   string
	ID          string
	Company     *string
	Title       *string
	StartDate   *string
	EndDate     *string
	Description *string
}

func UpdateCareerExperience(args UpdateCareerExperienceArgs) error {
	if err := requirePersonaExists(args.PersonaID); err != nil {
		return err
	}

	var e careerExperience
	var startDate, endDate, description sql.NullString
	err := db.CareerDB.QueryRow(
		`SELECT id, company, title, start_date, end_date, description FROM career_experience WHERE id = ? AND persona_id = ?`,
		args.ID, args.PersonaID,
	).Scan(&e.ID, &e.Company, &e.Title, &startDate, &endDate, &description)
	switch {
	case err == sql.ErrNoRows:
		return nil
	case err != nil:
		return err
	}
	e.StartDate = startDate.String
	e.EndDate = endDate.String
	e.Description = description.String

	if args.Company != nil {
		e.Company = *args.Company
	}
	if args.Title != nil {
		e.Title = *args.Title
	}
	if args.StartDate != nil {
		e.StartDate = *args.StartDate
	}
	if args.EndDate != nil {
		e.EndDate = *args.EndDate
	}
	if args.Description != nil {
		e.Description = *args.Description
	}

	_, err = db.CareerDB.Exec(
		`UPDATE career_experience SET company = ?, title = ?, start_date = ?, end_date = ?, description = ? WHERE id = ? AND persona_id = ?`,
		e.Company, e.Title, e.StartDate, e.EndDate, e.Description, args.ID, args.PersonaID,
	)
	return err
}

// --- MCP registration ---

type getPersonaDetailsArgs struct {
	PersonaID string `json:"personaId" jsonschema:"the persona whose details to read, from create_persona or list_personas"`
}

func RegisterGetPersonaDetails(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_persona_details",
		Description: "Read a persona's own career details (headline/summary/location/desired titles/desired locations/min salary), skills, and experience. personaDetails is null if none has been set yet.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getPersonaDetailsArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		result, err := FetchPersonaDetails(args.PersonaID)
		if err != nil {
			return personaAwareErrResult("read persona details", args.PersonaID, err), nil, nil
		}
		return db.JSONResult(result)
	})
}

type updatePersonaDetailsToolArgs struct {
	PersonaID        string  `json:"personaId" jsonschema:"the persona whose details to update, from create_persona or list_personas"`
	Headline         *string `json:"headline,omitempty" jsonschema:"a short headline, e.g. \"Senior Backend Engineer\""`
	Summary          *string `json:"summary,omitempty" jsonschema:"a free-text bio/summary"`
	Location         *string `json:"location,omitempty" jsonschema:"where the user is currently based"`
	DesiredTitles    *string `json:"desiredTitles,omitempty" jsonschema:"job titles the user is looking for, comma-separated"`
	DesiredLocations *string `json:"desiredLocations,omitempty" jsonschema:"locations the user is willing to work in, comma-separated"`
	MinSalary        *int64  `json:"minSalary,omitempty" jsonschema:"the user's own minimum acceptable salary"`
}

func RegisterUpdatePersonaDetails(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_persona_details",
		Description: "Update a persona's own career details. Only the fields provided are changed — omitted fields keep their current value.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePersonaDetailsToolArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{
			PersonaID:        args.PersonaID,
			Headline:         args.Headline,
			Summary:          args.Summary,
			Location:         args.Location,
			DesiredTitles:    args.DesiredTitles,
			DesiredLocations: args.DesiredLocations,
			MinSalary:        args.MinSalary,
		})
		if err != nil {
			return personaAwareErrResult("update persona details", args.PersonaID, err), nil, nil
		}
		result, err := FetchPersonaDetails(args.PersonaID)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("persona details updated but failed to reload: %v", err)), nil, nil
		}
		return db.JSONResult(result)
	})
}

type addCareerSkillArgs struct {
	PersonaID string `json:"personaId" jsonschema:"the persona to add this skill to"`
	Skill     string `json:"skill" jsonschema:"the skill to add, e.g. \"Go\""`
}

func RegisterAddCareerSkill(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_career_skill",
		Description: "Add a skill to a persona's own career details. A no-op if it's already there.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerSkillArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if args.Skill == "" {
			return db.ErrResult("skill is required"), nil, nil
		}
		if err := AddCareerSkill(args.PersonaID, args.Skill); err != nil {
			return personaAwareErrResult("add skill", args.PersonaID, err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("added skill %q", args.Skill)}}}, nil, nil
	})
}

type removeCareerSkillArgs struct {
	PersonaID string `json:"personaId" jsonschema:"the persona to remove this skill from"`
	Skill     string `json:"skill" jsonschema:"the skill to remove"`
}

func RegisterRemoveCareerSkill(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_career_skill",
		Description: "Remove a skill from a persona's own career details.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeCareerSkillArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if args.Skill == "" {
			return db.ErrResult("skill is required"), nil, nil
		}
		if err := RemoveCareerSkill(args.PersonaID, args.Skill); err != nil {
			return personaAwareErrResult("remove skill", args.PersonaID, err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed skill %q", args.Skill)}}}, nil, nil
	})
}

type addCareerExperienceArgs struct {
	PersonaID   string `json:"personaId" jsonschema:"the persona to add this experience entry to"`
	Company     string `json:"company" jsonschema:"the employer's name"`
	Title       string `json:"title" jsonschema:"the job title held there"`
	StartDate   string `json:"startDate,omitempty" jsonschema:"when this position started"`
	EndDate     string `json:"endDate,omitempty" jsonschema:"when this position ended — omit for a current position"`
	Description string `json:"description,omitempty" jsonschema:"what the role involved"`
}

func RegisterAddCareerExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_career_experience",
		Description: "Add a work experience entry to a persona's own career details. Returns the new entry's own id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerExperienceArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if args.Company == "" || args.Title == "" {
			return db.ErrResult("company and title are both required"), nil, nil
		}
		id, err := AddCareerExperience(args.PersonaID, args.Company, args.Title, args.StartDate, args.EndDate, args.Description)
		if err != nil {
			return personaAwareErrResult("add experience", args.PersonaID, err), nil, nil
		}
		return db.JSONResult(map[string]string{"id": id})
	})
}

type removeCareerExperienceArgs struct {
	PersonaID string `json:"personaId" jsonschema:"the persona this experience entry belongs to"`
	ID        string `json:"id" jsonschema:"the experience entry's own id, from add_career_experience or get_persona_details"`
}

func RegisterRemoveCareerExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_career_experience",
		Description: "Remove a work experience entry from a persona's own career details.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeCareerExperienceArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := RemoveCareerExperience(args.PersonaID, args.ID); err != nil {
			return personaAwareErrResult("remove experience", args.PersonaID, err), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "removed"}}}, nil, nil
	})
}

type updateCareerExperienceToolArgs struct {
	PersonaID   string  `json:"personaId" jsonschema:"the persona this experience entry belongs to"`
	ID          string  `json:"id" jsonschema:"the experience entry's own id, from add_career_experience or get_persona_details"`
	Company     *string `json:"company,omitempty" jsonschema:"the employer's name"`
	Title       *string `json:"title,omitempty" jsonschema:"the job title held there"`
	StartDate   *string `json:"startDate,omitempty" jsonschema:"when this position started"`
	EndDate     *string `json:"endDate,omitempty" jsonschema:"when this position ended — omit for a current position"`
	Description *string `json:"description,omitempty" jsonschema:"what the role involved"`
}

func RegisterUpdateCareerExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_career_experience",
		Description: "Update a work experience entry on a persona's own career details. Only the fields provided are changed — omitted fields keep their current value. A no-op if id is unknown or belongs to a different persona.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateCareerExperienceToolArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := UpdateCareerExperience(UpdateCareerExperienceArgs{
			PersonaID:   args.PersonaID,
			ID:          args.ID,
			Company:     args.Company,
			Title:       args.Title,
			StartDate:   args.StartDate,
			EndDate:     args.EndDate,
			Description: args.Description,
		}); err != nil {
			return personaAwareErrResult("update experience", args.PersonaID, err), nil, nil
		}
		result, err := FetchPersonaDetails(args.PersonaID)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("experience updated but failed to reload: %v", err)), nil, nil
		}
		return db.JSONResult(result)
	})
}

// --- small shared helpers ---

// personaAwareErrResult gives ErrUnknownPersona a clearer message than
// the bare "unknown persona id" — every persona-scoped MCP tool routes
// its own requirePersonaExists failure through this.
func personaAwareErrResult(action, personaID string, err error) *mcp.CallToolResult {
	if errors.Is(err, ErrUnknownPersona) {
		return db.ErrResult(fmt.Sprintf("failed to %s: unknown persona id %q", action, personaID))
	}
	return db.ErrResult(fmt.Sprintf("failed to %s: %v", action, err))
}
