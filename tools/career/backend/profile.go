// profile.go is the AI-facing read/write surface over career.db's own
// career_profile/career_skills/career_experience tables. Unlike
// browser's own login_credentials.go, there is no isolation boundary
// here at all — the AI reads and writes this data directly and
// freely, by design: this isn't a secret, it's the thing the AI is
// meant to help build and use. See
// plan/ai/tools/career/step-03-career-profile-tools.md.
package main

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// defaultProfileID is the fixed key for the single logical profile
// row this tool ever holds — one installation, one user, one profile.
// See this step's own design doc for why a keyed row (not a
// schema-less singleton table) was chosen anyway.
const defaultProfileID = "default"

type careerProfile struct {
	FullName         string `json:"fullName"`
	Headline         string `json:"headline"`
	Summary          string `json:"summary"`
	Location         string `json:"location"`
	DesiredTitles    string `json:"desiredTitles"`
	DesiredLocations string `json:"desiredLocations"`
	MinSalary        int64  `json:"minSalary,omitempty"`
}

type careerSkill struct {
	ID    string `json:"id"`
	Skill string `json:"skill"`
}

type careerExperience struct {
	ID          string `json:"id"`
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	Description string `json:"description,omitempty"`
}

type getCareerProfileResult struct {
	Profile    *careerProfile     `json:"profile"`
	Skills     []string           `json:"skills"`
	Experience []careerExperience `json:"experience"`
}

// fetchCareerProfile reads the single profile row (nil, not an error,
// if none exists yet — "not configured yet" is a normal,
// representable state, matching browser's own lookupCredential
// convention), every skill, and every experience entry.
func fetchCareerProfile() (getCareerProfileResult, error) {
	var result getCareerProfileResult

	var p careerProfile
	var minSalary sql.NullInt64
	err := careerDB.QueryRow(
		`SELECT full_name, headline, summary, location, desired_titles, desired_locations, min_salary
		 FROM career_profile WHERE id = ?`, defaultProfileID,
	).Scan(&p.FullName, &p.Headline, &p.Summary, &p.Location, &p.DesiredTitles, &p.DesiredLocations, &minSalary)
	switch {
	case err == sql.ErrNoRows:
		result.Profile = nil
	case err != nil:
		return result, err
	default:
		p.MinSalary = minSalary.Int64
		result.Profile = &p
	}

	skillRows, err := careerDB.Query(`SELECT skill FROM career_skills ORDER BY skill ASC`)
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

	expRows, err := careerDB.Query(
		`SELECT id, company, title, start_date, end_date, description
		 FROM career_experience ORDER BY created_at DESC`)
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

// updateCareerProfile is a partial update — only fields present in
// args (via the has* flags) are touched. Upserts the single
// defaultProfileID row.
type updateCareerProfileArgs struct {
	FullName         *string `json:"fullName,omitempty"`
	Headline         *string `json:"headline,omitempty"`
	Summary          *string `json:"summary,omitempty"`
	Location         *string `json:"location,omitempty"`
	DesiredTitles    *string `json:"desiredTitles,omitempty"`
	DesiredLocations *string `json:"desiredLocations,omitempty"`
	MinSalary        *int64  `json:"minSalary,omitempty"`
}

func updateCareerProfile(args updateCareerProfileArgs) error {
	// A plain upsert seeded from whatever's already there, then
	// overwritten field-by-field only where args supplied a value —
	// simplest correct way to express "partial update" against
	// SQLite's own INSERT ... ON CONFLICT without hand-building a
	// dynamic SET clause.
	existing, err := fetchCareerProfile()
	if err != nil {
		return err
	}
	p := careerProfile{}
	if existing.Profile != nil {
		p = *existing.Profile
	}
	if args.FullName != nil {
		p.FullName = *args.FullName
	}
	if args.Headline != nil {
		p.Headline = *args.Headline
	}
	if args.Summary != nil {
		p.Summary = *args.Summary
	}
	if args.Location != nil {
		p.Location = *args.Location
	}
	if args.DesiredTitles != nil {
		p.DesiredTitles = *args.DesiredTitles
	}
	if args.DesiredLocations != nil {
		p.DesiredLocations = *args.DesiredLocations
	}
	if args.MinSalary != nil {
		p.MinSalary = *args.MinSalary
	}

	_, err = careerDB.Exec(
		`INSERT INTO career_profile (id, full_name, headline, summary, location, desired_titles, desired_locations, min_salary, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(id) DO UPDATE SET
			full_name = excluded.full_name,
			headline = excluded.headline,
			summary = excluded.summary,
			location = excluded.location,
			desired_titles = excluded.desired_titles,
			desired_locations = excluded.desired_locations,
			min_salary = excluded.min_salary,
			updated_at = excluded.updated_at`,
		defaultProfileID, p.FullName, p.Headline, p.Summary, p.Location, p.DesiredTitles, p.DesiredLocations, p.MinSalary,
	)
	return err
}

func addCareerSkill(skill string) error {
	_, err := careerDB.Exec(
		`INSERT INTO career_skills (id, skill) VALUES (?, ?) ON CONFLICT(skill) DO NOTHING`,
		uuid.NewString(), skill,
	)
	return err
}

func removeCareerSkill(skill string) error {
	_, err := careerDB.Exec(`DELETE FROM career_skills WHERE skill = ?`, skill)
	return err
}

func addCareerExperience(company, title, startDate, endDate, description string) (string, error) {
	id := uuid.NewString()
	_, err := careerDB.Exec(
		`INSERT INTO career_experience (id, company, title, start_date, end_date, description) VALUES (?, ?, ?, ?, ?, ?)`,
		id, company, title, startDate, endDate, description,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func removeCareerExperience(id string) error {
	_, err := careerDB.Exec(`DELETE FROM career_experience WHERE id = ?`, id)
	return err
}

// updateCareerExperienceArgs mirrors updateCareerProfileArgs's own
// partial-update shape — only fields provided are changed. id is
// required; updating an unknown id is a benign no-op (0 rows
// affected), the same posture removeCareerExperience already has.
type updateCareerExperienceArgs struct {
	ID          string
	Company     *string
	Title       *string
	StartDate   *string
	EndDate     *string
	Description *string
}

func updateCareerExperience(args updateCareerExperienceArgs) error {
	var e careerExperience
	var startDate, endDate, description sql.NullString
	err := careerDB.QueryRow(
		`SELECT id, company, title, start_date, end_date, description FROM career_experience WHERE id = ?`,
		args.ID,
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

	_, err = careerDB.Exec(
		`UPDATE career_experience SET company = ?, title = ?, start_date = ?, end_date = ?, description = ? WHERE id = ?`,
		e.Company, e.Title, e.StartDate, e.EndDate, e.Description, args.ID,
	)
	return err
}

// --- MCP registration ---

type getCareerProfileArgs struct{}

func registerGetCareerProfile(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_career_profile",
		Description: "Read the user's own career profile, skills, and experience. profile is null if none has been set yet.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getCareerProfileArgs) (*mcp.CallToolResult, any, error) {
		result, err := fetchCareerProfile()
		if err != nil {
			return errResult(fmt.Sprintf("failed to read career profile: %v", err)), nil, nil
		}
		return jsonResult(result)
	})
}

type updateCareerProfileToolArgs struct {
	FullName         *string `json:"fullName,omitempty" jsonschema:"the user's own full name"`
	Headline         *string `json:"headline,omitempty" jsonschema:"a short headline, e.g. \"Senior Backend Engineer\""`
	Summary          *string `json:"summary,omitempty" jsonschema:"a free-text bio/summary"`
	Location         *string `json:"location,omitempty" jsonschema:"where the user is currently based"`
	DesiredTitles    *string `json:"desiredTitles,omitempty" jsonschema:"job titles the user is looking for, comma-separated"`
	DesiredLocations *string `json:"desiredLocations,omitempty" jsonschema:"locations the user is willing to work in, comma-separated"`
	MinSalary        *int64  `json:"minSalary,omitempty" jsonschema:"the user's own minimum acceptable salary"`
}

func registerUpdateCareerProfile(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_career_profile",
		Description: "Update the user's own career profile. Only the fields provided are changed — omitted fields keep their current value.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateCareerProfileToolArgs) (*mcp.CallToolResult, any, error) {
		err := updateCareerProfile(updateCareerProfileArgs{
			FullName:         args.FullName,
			Headline:         args.Headline,
			Summary:          args.Summary,
			Location:         args.Location,
			DesiredTitles:    args.DesiredTitles,
			DesiredLocations: args.DesiredLocations,
			MinSalary:        args.MinSalary,
		})
		if err != nil {
			return errResult(fmt.Sprintf("failed to update career profile: %v", err)), nil, nil
		}
		result, err := fetchCareerProfile()
		if err != nil {
			return errResult(fmt.Sprintf("profile updated but failed to reload: %v", err)), nil, nil
		}
		return jsonResult(result)
	})
}

type addCareerSkillArgs struct {
	Skill string `json:"skill" jsonschema:"the skill to add, e.g. \"Go\""`
}

func registerAddCareerSkill(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_career_skill",
		Description: "Add a skill to the user's own career profile. A no-op if it's already there.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerSkillArgs) (*mcp.CallToolResult, any, error) {
		if args.Skill == "" {
			return errResult("skill is required"), nil, nil
		}
		if err := addCareerSkill(args.Skill); err != nil {
			return errResult(fmt.Sprintf("failed to add skill: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("added skill %q", args.Skill)}}}, nil, nil
	})
}

type removeCareerSkillArgs struct {
	Skill string `json:"skill" jsonschema:"the skill to remove"`
}

func registerRemoveCareerSkill(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_career_skill",
		Description: "Remove a skill from the user's own career profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeCareerSkillArgs) (*mcp.CallToolResult, any, error) {
		if args.Skill == "" {
			return errResult("skill is required"), nil, nil
		}
		if err := removeCareerSkill(args.Skill); err != nil {
			return errResult(fmt.Sprintf("failed to remove skill: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed skill %q", args.Skill)}}}, nil, nil
	})
}

type addCareerExperienceArgs struct {
	Company     string `json:"company" jsonschema:"the employer's name"`
	Title       string `json:"title" jsonschema:"the job title held there"`
	StartDate   string `json:"startDate,omitempty" jsonschema:"when this position started"`
	EndDate     string `json:"endDate,omitempty" jsonschema:"when this position ended — omit for a current position"`
	Description string `json:"description,omitempty" jsonschema:"what the role involved"`
}

func registerAddCareerExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_career_experience",
		Description: "Add a work experience entry to the user's own career profile. Returns the new entry's own id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addCareerExperienceArgs) (*mcp.CallToolResult, any, error) {
		if args.Company == "" || args.Title == "" {
			return errResult("company and title are both required"), nil, nil
		}
		id, err := addCareerExperience(args.Company, args.Title, args.StartDate, args.EndDate, args.Description)
		if err != nil {
			return errResult(fmt.Sprintf("failed to add experience: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id})
	})
}

type removeCareerExperienceArgs struct {
	ID string `json:"id" jsonschema:"the experience entry's own id, from add_career_experience or get_career_profile"`
}

func registerRemoveCareerExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_career_experience",
		Description: "Remove a work experience entry from the user's own career profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeCareerExperienceArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := removeCareerExperience(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to remove experience: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "removed"}}}, nil, nil
	})
}

type updateCareerExperienceToolArgs struct {
	ID          string  `json:"id" jsonschema:"the experience entry's own id, from add_career_experience or get_career_profile"`
	Company     *string `json:"company,omitempty" jsonschema:"the employer's name"`
	Title       *string `json:"title,omitempty" jsonschema:"the job title held there"`
	StartDate   *string `json:"startDate,omitempty" jsonschema:"when this position started"`
	EndDate     *string `json:"endDate,omitempty" jsonschema:"when this position ended — omit for a current position"`
	Description *string `json:"description,omitempty" jsonschema:"what the role involved"`
}

func registerUpdateCareerExperience(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_career_experience",
		Description: "Update a work experience entry on the user's own career profile. Only the fields provided are changed — omitted fields keep their current value. A no-op if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateCareerExperienceToolArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := updateCareerExperience(updateCareerExperienceArgs{
			ID:          args.ID,
			Company:     args.Company,
			Title:       args.Title,
			StartDate:   args.StartDate,
			EndDate:     args.EndDate,
			Description: args.Description,
		}); err != nil {
			return errResult(fmt.Sprintf("failed to update experience: %v", err)), nil, nil
		}
		result, err := fetchCareerProfile()
		if err != nil {
			return errResult(fmt.Sprintf("experience updated but failed to reload: %v", err)), nil, nil
		}
		return jsonResult(result)
	})
}

// --- small shared helpers (used by jobs.go too) ---

func errResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: true}
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	text, err := jsonMarshalIndent(v)
	if err != nil {
		return errResult(fmt.Sprintf("failed to format result: %v", err)), nil, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil, nil
}
