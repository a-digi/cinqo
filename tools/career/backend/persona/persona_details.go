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
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// The values of a persona's remote-work preference (step 130): empty means not set.
const (
	RemoteOnSite         = "onsite"      // on-site only: remote work does not help
	RemoteOpen           = "open"        // open to remote or hybrid as well as on-site
	RemoteOnly           = "remote_only" // only remote work
	remotePreferenceHelp = "remotePreference must be empty, \"onsite\", \"open\" or \"remote_only\""
)

// ErrInvalidRemotePreference is returned for a remote preference that is not one of the values above.
var ErrInvalidRemotePreference = errors.New(remotePreferenceHelp)

// ValidRemotePreference is validRemotePreference for the profile's own goals (plan 174).
func ValidRemotePreference(v string) bool { return validRemotePreference(v) }

// validRemotePreference reports whether v is a remote preference (case-sensitive; empty is "not set").
func validRemotePreference(v string) bool {
	switch v {
	case "", RemoteOnSite, RemoteOpen, RemoteOnly:
		return true
	}
	return false
}

type personaDetails struct {
	PersonaID        string `json:"personaId"`
	Headline         string `json:"headline"`
	Summary          string `json:"summary"`
	Location         string `json:"location"`
	DesiredTitles    string `json:"desiredTitles"`
	DesiredLocations string `json:"desiredLocations"`
	MinSalary        int64  `json:"minSalary,omitempty"`
	// RemotePreference (step 130): "", "onsite", "open" or "remote_only"; "" means not set.
	RemotePreference string `json:"remotePreference"`
	// Overridden (plan 174) names the values among summary, desiredTitles, desiredLocations, minSalary and remotePreference that are
	// the persona's own; the others are the profile's, which the persona follows. Headline and location are always the persona's.
	Overridden []string `json:"overridden"`
}

// Where a skill or an experience of a persona comes from (plan 173): the profile's library (the persona uses it) or the persona's own.
const (
	SourceProfile = "profile"
	SourcePersona = "persona"
)

type careerExperience struct {
	ID          string `json:"id"`
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate,omitempty"`
	EndDate     string `json:"endDate,omitempty"`
	Description string `json:"description,omitempty"`
	// Source is "profile" (a library experience the persona uses; ID is the library item's id) or "persona" (its own; ID is its own).
	Source string `json:"source"`
	// TitleOverridden/DescriptionOverridden: the persona shows its own title/description instead of the profile's (profile items only).
	TitleOverridden       bool `json:"titleOverridden,omitempty"`
	DescriptionOverridden bool `json:"descriptionOverridden,omitempty"`
}

// skillItem is one skill of a persona with where it comes from: ID is the library skill's id for source "profile", the persona's own
// row id for "persona".
type skillItem struct {
	ID     string `json:"id"`
	Skill  string `json:"skill"`
	Source string `json:"source"`
}

// getPersonaDetailsResult: Skills and Experience are what the persona shows (the profile items it uses with its overrides, plus its
// own), in the shapes the job match, the CV builder and the pages always read; SkillItems says where each skill comes from.
type getPersonaDetailsResult struct {
	PersonaDetails *personaDetails    `json:"personaDetails"`
	Skills         []string           `json:"skills"`
	SkillItems     []skillItem        `json:"skillItems"`
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

	// Plan 174: the persona's own row (headline, location and the values it words itself) and the profile's summary and goals it follows.
	row, err := readDetailsRow(db.CareerDB, personaId)
	if err != nil {
		return result, err
	}
	pv, err := profileValuesOfPersona(db.CareerDB, personaId)
	if err != nil {
		return result, err
	}
	// "Not configured yet" (nil) only when the persona has no row and the profile has nothing to follow either.
	if row.exists || !pv.empty() {
		d := effectiveDetails(personaId, row, pv)
		result.PersonaDetails = &d
	}

	items, err := personaSkillItems(personaId)
	if err != nil {
		return result, err
	}
	result.SkillItems = items
	result.Skills = make([]string, 0, len(items))
	for _, it := range items {
		result.Skills = append(result.Skills, it.Skill)
	}

	experience, err := personaExperience(personaId)
	if err != nil {
		return result, err
	}
	result.Experience = experience
	return result, nil
}

// OwnSkillItems are the persona's own custom skills (not the library skills it uses) of a details result.
func (r getPersonaDetailsResult) OwnSkillItems() []skillItem {
	out := []skillItem{}
	for _, it := range r.SkillItems {
		if it.Source == SourcePersona {
			out = append(out, it)
		}
	}
	return out
}

// OwnExperience are the persona's own custom experiences of a details result.
func (r getPersonaDetailsResult) OwnExperience() []careerExperience {
	out := []careerExperience{}
	for _, e := range r.Experience {
		if e.Source == SourcePersona {
			out = append(out, e)
		}
	}
	return out
}

// personaSkillItems is what a persona shows as skills: the library skills it uses first, then its own, without a skill twice (same
// text, any case: the library one wins), sorted by text like the list always was.
func personaSkillItems(personaId string) ([]skillItem, error) {
	items := []skillItem{}
	seen := map[string]bool{}
	read := func(query, source string) error {
		rows, err := db.CareerDB.Query(query, personaId)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it skillItem
			if err := rows.Scan(&it.ID, &it.Skill); err != nil {
				return err
			}
			it.Source = source
			key := strings.ToLower(strings.TrimSpace(it.Skill))
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, it)
		}
		return rows.Err()
	}
	if err := read(`SELECT k.id, k.skill FROM persona_skills ps JOIN profile_skills k ON k.id = ps.profile_skill_id WHERE ps.persona_id = ? ORDER BY k.skill ASC`, SourceProfile); err != nil {
		return nil, err
	}
	if err := read(`SELECT id, skill FROM career_skills WHERE persona_id = ? ORDER BY skill ASC`, SourcePersona); err != nil {
		return nil, err
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Skill < items[j].Skill })
	return items, nil
}

// personaExperience is what a persona shows as experience: the library experiences it uses (its title and description overrides
// applied) and its own, newest first.
func personaExperience(personaId string) ([]careerExperience, error) {
	type row struct {
		careerExperience
		createdAt string
	}
	rows := []row{}
	profileRows, err := db.CareerDB.Query(
		`SELECT x.id, x.company, x.title, x.start_date, x.end_date, x.description, x.created_at, pe.title_override, pe.description_override
		 FROM persona_experience pe JOIN profile_experience x ON x.id = pe.profile_experience_id WHERE pe.persona_id = ?`, personaId)
	if err != nil {
		return nil, err
	}
	for profileRows.Next() {
		var r row
		var start, end, description, titleOverride, descriptionOverride sql.NullString
		if err := profileRows.Scan(&r.ID, &r.Company, &r.Title, &start, &end, &description, &r.createdAt, &titleOverride, &descriptionOverride); err != nil {
			profileRows.Close()
			return nil, err
		}
		r.StartDate, r.EndDate, r.Description = start.String, end.String, description.String
		r.Source = SourceProfile
		if titleOverride.Valid {
			r.Title, r.TitleOverridden = titleOverride.String, true
		}
		if descriptionOverride.Valid {
			r.Description, r.DescriptionOverridden = descriptionOverride.String, true
		}
		rows = append(rows, r)
	}
	if err := profileRows.Err(); err != nil {
		profileRows.Close()
		return nil, err
	}
	profileRows.Close()

	own, err := db.CareerDB.Query(
		`SELECT id, company, title, start_date, end_date, description, created_at FROM career_experience WHERE persona_id = ?`, personaId)
	if err != nil {
		return nil, err
	}
	defer own.Close()
	for own.Next() {
		var r row
		var start, end, description sql.NullString
		if err := own.Scan(&r.ID, &r.Company, &r.Title, &start, &end, &description, &r.createdAt); err != nil {
			return nil, err
		}
		r.StartDate, r.EndDate, r.Description = start.String, end.String, description.String
		r.Source = SourcePersona
		rows = append(rows, r)
	}
	if err := own.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].createdAt != rows[j].createdAt {
			return rows[i].createdAt > rows[j].createdAt
		}
		return rows[i].ID < rows[j].ID
	})
	out := make([]careerExperience, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.careerExperience)
	}
	return out, nil
}

// UpdatePersonaDetailsArgs is a partial update — only fields present
// (via the *string/*int64 pointers) are touched. Upserts the row
// keyed by PersonaID, which must already exist.
type UpdatePersonaDetailsArgs struct {
	PersonaID string  `json:"personaId"`
	Headline  *string `json:"headline,omitempty"`
	Location  *string `json:"location,omitempty"`
	// Summary and the four goals below are the PERSONA's own values (plan 174): they replace the profile's for this persona only.
	Summary          *string `json:"summary,omitempty"`
	DesiredTitles    *string `json:"desiredTitles,omitempty"`
	DesiredLocations *string `json:"desiredLocations,omitempty"`
	MinSalary        *int64  `json:"minSalary,omitempty"`
	// RemotePreference (step 130) is "", "onsite", "open" or "remote_only"; any other value is refused before anything
	// is written.
	RemotePreference *string `json:"remotePreference,omitempty"`
	// FollowProfile sends the named values (summary, desiredTitles, desiredLocations, minSalary, remotePreference) back to following the
	// profile's: the persona's own value is dropped. A name that is also set in the same request is refused.
	FollowProfile []string `json:"followProfile,omitempty"`
}

func UpdatePersonaDetails(args UpdatePersonaDetailsArgs) error {
	if err := requirePersonaExists(args.PersonaID); err != nil {
		return err
	}
	if args.RemotePreference != nil {
		trimmed := strings.TrimSpace(*args.RemotePreference)
		if !validRemotePreference(trimmed) {
			return ErrInvalidRemotePreference
		}
		args.RemotePreference = &trimmed
	}
	set := map[string]bool{
		FieldSummary: args.Summary != nil, FieldDesiredTitles: args.DesiredTitles != nil, FieldDesiredLocations: args.DesiredLocations != nil,
		FieldMinSalary: args.MinSalary != nil, FieldRemotePreference: args.RemotePreference != nil,
	}
	for _, f := range args.FollowProfile {
		known := false
		for _, k := range detailFields {
			known = known || k == f
		}
		if !known {
			return ErrUnknownFollowField
		}
		if set[f] {
			return ErrSetAndFollow
		}
	}

	// Plan 174: the persona's own row is read, changed field by field where args supplied a value, and written back whole (a plain
	// upsert); the old summary/goal columns are no longer read or written.
	row, err := readDetailsRow(db.CareerDB, args.PersonaID)
	if err != nil {
		return err
	}
	pv, err := profileValuesOfPersona(db.CareerDB, args.PersonaID)
	if err != nil {
		return err
	}
	before := effectiveDetails(args.PersonaID, row, pv)

	if args.Headline != nil {
		row.headline = *args.Headline
	}
	if args.Location != nil {
		row.location = *args.Location
	}
	if args.Summary != nil {
		row.summary = sql.NullString{String: *args.Summary, Valid: true}
	}
	if args.DesiredTitles != nil {
		row.titles = sql.NullString{String: *args.DesiredTitles, Valid: true}
	}
	if args.DesiredLocations != nil {
		row.locations = sql.NullString{String: *args.DesiredLocations, Valid: true}
	}
	if args.MinSalary != nil {
		row.salary = sql.NullInt64{Int64: *args.MinSalary, Valid: true}
	}
	if args.RemotePreference != nil {
		row.remote = sql.NullString{String: *args.RemotePreference, Valid: true}
	}
	if err := row.follow(args.FollowProfile); err != nil {
		return err
	}
	after := effectiveDetails(args.PersonaID, row, pv)

	_, err = db.CareerDB.Exec(
		`INSERT INTO persona_details (persona_id, headline, location, summary_override, desired_titles_override, desired_locations_override,
		   min_salary_override, remote_preference_override, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(persona_id) DO UPDATE SET
			headline = excluded.headline,
			location = excluded.location,
			summary_override = excluded.summary_override,
			desired_titles_override = excluded.desired_titles_override,
			desired_locations_override = excluded.desired_locations_override,
			min_salary_override = excluded.min_salary_override,
			remote_preference_override = excluded.remote_preference_override,
			updated_at = excluded.updated_at`,
		args.PersonaID, row.headline, row.location, row.summary, row.titles, row.locations, row.salary, row.remote,
	)
	if err == nil && scoredDetailsDiffer(before, after) {
		publishPersonaChanged(args.PersonaID, ChangeDetails)
	}
	return err
}

// scoredDetailsDiffer reports whether two versions of a persona's details differ in something the job matcher reads
// (not the minimum salary, which it does not use): only such a change can move a score.
func scoredDetailsDiffer(a, b personaDetails) bool {
	return a.Headline != b.Headline || a.Summary != b.Summary || a.Location != b.Location ||
		a.DesiredTitles != b.DesiredTitles || a.DesiredLocations != b.DesiredLocations || a.RemotePreference != b.RemotePreference
}

// ErrProfileItem is returned when a persona's own route is asked to change an experience that comes from the profile's library
// (plan 173): the persona only uses it. It is changed in the library, or overridden for the persona by profileitems.
var ErrProfileItem = errors.New("this experience comes from the profile: change it in the profile's library, or override its title and description for this target role")

// usesProfileExperience reports whether the persona uses the library experience with this id.
func usesProfileExperience(personaId, id string) (bool, error) {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM persona_experience WHERE persona_id = ? AND profile_experience_id = ?`, personaId, id).Scan(&one); {
	case err == sql.ErrNoRows:
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

func AddCareerSkill(personaId, skill string) error {
	if err := requirePersonaExists(personaId); err != nil {
		return err
	}
	res, err := db.CareerDB.Exec(
		`INSERT INTO career_skills (id, persona_id, skill) VALUES (?, ?, ?) ON CONFLICT(persona_id, skill) DO NOTHING`,
		uuid.NewString(), personaId, skill,
	)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 { // a skill that was there already changes nothing
			publishPersonaChanged(personaId, ChangeSkillAdded)
		}
	}
	return err
}

func RemoveCareerSkill(personaId, skill string) error {
	if err := requirePersonaExists(personaId); err != nil {
		return err
	}
	res, err := db.CareerDB.Exec(`DELETE FROM career_skills WHERE persona_id = ? AND skill = ?`, personaId, skill)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			publishPersonaChanged(personaId, ChangeSkillRemoved)
		}
	}
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
	publishPersonaChanged(personaId, ChangeExperienceAdded)
	return id, nil
}

func RemoveCareerExperience(personaId, id string) error {
	if err := requirePersonaExists(personaId); err != nil {
		return err
	}
	if uses, err := usesProfileExperience(personaId, id); err != nil {
		return err
	} else if uses {
		return ErrProfileItem
	}
	res, err := db.CareerDB.Exec(`DELETE FROM career_experience WHERE id = ? AND persona_id = ?`, id, personaId)
	if err == nil {
		if n, _ := res.RowsAffected(); n > 0 {
			publishPersonaChanged(personaId, ChangeExperienceRemoved)
		}
	}
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
	if uses, err := usesProfileExperience(args.PersonaID, args.ID); err != nil {
		return err
	} else if uses {
		return ErrProfileItem
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
	before := e

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
	// the matcher reads the title, the dates and the description, not the company
	if err == nil && (before.Title != e.Title || before.StartDate != e.StartDate || before.EndDate != e.EndDate || before.Description != e.Description) {
		publishPersonaChanged(args.PersonaID, ChangeExperienceUpdated)
	}
	return err
}

// --- MCP registration ---

type getPersonaDetailsArgs struct {
	PersonaID string `json:"personaId" jsonschema:"the persona whose details to read, from create_persona or list_personas"`
}

func RegisterGetPersonaDetails(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_persona_details",
		Description: "Read what a persona shows: its career details (headline/summary/location/desired titles/desired locations/min salary/remote preference), skills, and experience. The summary and the goals are the profile's unless the persona has its own; overridden lists the ones that are its own. personaDetails is null if neither the persona nor its profile has any yet. remotePreference is empty (not set), \"onsite\" (on-site only), \"open\" (open to remote or hybrid) or \"remote_only\".",
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
	PersonaID        string   `json:"personaId" jsonschema:"the persona whose details to update, from create_persona or list_personas"`
	Headline         *string  `json:"headline,omitempty" jsonschema:"a short headline, e.g. \"Senior Backend Engineer\""`
	Summary          *string  `json:"summary,omitempty" jsonschema:"a free-text bio/summary"`
	Location         *string  `json:"location,omitempty" jsonschema:"where the user is currently based"`
	DesiredTitles    *string  `json:"desiredTitles,omitempty" jsonschema:"job titles the user is looking for, comma-separated"`
	DesiredLocations *string  `json:"desiredLocations,omitempty" jsonschema:"locations the user is willing to work in, comma-separated"`
	MinSalary        *int64   `json:"minSalary,omitempty" jsonschema:"the user's own minimum acceptable salary"`
	RemotePreference *string  `json:"remotePreference,omitempty" jsonschema:"whether the user wants remote work: \"onsite\" (on-site only), \"open\" (open to remote or hybrid as well) or \"remote_only\"; an empty string clears it (not set)"`
	FollowProfile    []string `json:"followProfile,omitempty" jsonschema:"values to send back to following the profile's own: any of summary, desiredTitles, desiredLocations, minSalary, remotePreference"`
}

func RegisterUpdatePersonaDetails(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_persona_details",
		Description: "Update a persona's own career details. Only the fields provided are changed — omitted fields keep their current value. The summary and the goals (desiredTitles, desiredLocations, minSalary, remotePreference) normally come from the profile, which every persona follows; setting one here gives THIS persona its own value and changes nothing for the other personas. followProfile sends such a value back to the profile's.",
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
			RemotePreference: args.RemotePreference,
			FollowProfile:    args.FollowProfile,
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
		Description: "Add a skill that belongs only to this persona (a custom skill of its own, not a skill of the profile's library; add those with add_profile_skill and choose them with set_persona_skills). A no-op if it's already there.",
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
		Description: "Remove one of this persona's own custom skills. A skill the persona uses from the profile's library is not removed here: change the persona's choice with set_persona_skills.",
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
		Description: "Add a work experience entry that belongs only to this persona (a custom entry of its own, not an experience of the profile's library; add those with add_profile_experience and choose them with set_persona_experience). Returns the new entry's own id.",
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
		Description: "Remove one of this persona's own custom work experience entries. An entry that comes from the profile's library is refused: change the persona's choice with set_persona_experience, or delete it from the library.",
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
		Description: "Update a work experience entry on a persona's own career details. Only the fields provided are changed — omitted fields keep their current value. A no-op if id is unknown or belongs to a different persona; an entry that comes from the profile's library is refused (change it in the library, or override its title and description for this persona with set_persona_experience).",
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
