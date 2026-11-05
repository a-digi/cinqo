// Package profiledetails is the summary and the goals of a profile (plan/ai/tools/career/step-174-split-summary-and-goals.md, step 3):
// the values every persona of the profile follows unless it has its own (see persona.FetchPersonaDetails). Every check comes before the
// write; a profile without a row has empty values, so reading one is never an error (an unknown profile is).
package profiledetails

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/persona"
)

var (
	ErrUnknownProfile          = errors.New("unknown profile id")
	ErrNegativeSalary          = errors.New("minSalary cannot be negative")
	ErrInvalidRemotePreference = persona.ErrInvalidRemotePreference
)

// Values are the profile's summary and goals. MinSalary 0 is "no minimum".
type Values struct {
	ProfileID        string `json:"profileId"`
	Summary          string `json:"summary"`
	DesiredTitles    string `json:"desiredTitles"`
	DesiredLocations string `json:"desiredLocations"`
	MinSalary        int64  `json:"minSalary"`
	RemotePreference string `json:"remotePreference"`
}

// Patch changes only the fields that are set.
type Patch struct {
	Summary          *string `json:"summary,omitempty"`
	DesiredTitles    *string `json:"desiredTitles,omitempty"`
	DesiredLocations *string `json:"desiredLocations,omitempty"`
	MinSalary        *int64  `json:"minSalary,omitempty"`
	RemotePreference *string `json:"remotePreference,omitempty"`
}

func requireProfile(profileID string) error {
	var one int
	switch err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, profileID).Scan(&one); {
	case err == sql.ErrNoRows:
		return ErrUnknownProfile
	case err != nil:
		return err
	}
	return nil
}

// Get returns the profile's values (all empty when none was ever set); an unknown profile is ErrUnknownProfile.
func Get(profileID string) (Values, error) {
	if err := requireProfile(profileID); err != nil {
		return Values{}, err
	}
	v := Values{ProfileID: profileID}
	var salary sql.NullInt64
	err := db.CareerDB.QueryRow(
		`SELECT summary, desired_titles, desired_locations, min_salary, remote_preference FROM profile_details WHERE profile_id = ?`, profileID,
	).Scan(&v.Summary, &v.DesiredTitles, &v.DesiredLocations, &salary, &v.RemotePreference)
	if err != nil && err != sql.ErrNoRows {
		return Values{}, err
	}
	v.MinSalary = salary.Int64
	return v, nil
}

// Update changes the fields that are set and returns the whole values. The remote preference ("", onsite, open, remote_only) and a
// minimum salary of 0 or more are checked before anything is written. When a value changed, every persona of the profile is announced
// (the ones that follow it show something else now).
func Update(profileID string, p Patch) (Values, error) {
	if err := requireProfile(profileID); err != nil {
		return Values{}, err
	}
	if p.RemotePreference != nil {
		trimmed := strings.TrimSpace(*p.RemotePreference)
		if !persona.ValidRemotePreference(trimmed) {
			return Values{}, ErrInvalidRemotePreference
		}
		p.RemotePreference = &trimmed
	}
	if p.MinSalary != nil && *p.MinSalary < 0 {
		return Values{}, ErrNegativeSalary
	}
	before, err := Get(profileID)
	if err != nil {
		return Values{}, err
	}
	v := before
	if p.Summary != nil {
		v.Summary = *p.Summary
	}
	if p.DesiredTitles != nil {
		v.DesiredTitles = *p.DesiredTitles
	}
	if p.DesiredLocations != nil {
		v.DesiredLocations = *p.DesiredLocations
	}
	if p.MinSalary != nil {
		v.MinSalary = *p.MinSalary
	}
	if p.RemotePreference != nil {
		v.RemotePreference = *p.RemotePreference
	}
	if v == before {
		return v, nil
	}
	var salary any
	if v.MinSalary > 0 {
		salary = v.MinSalary
	}
	if _, err := db.CareerDB.Exec(
		`INSERT INTO profile_details (profile_id, summary, desired_titles, desired_locations, min_salary, remote_preference, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, datetime('now'))
		 ON CONFLICT(profile_id) DO UPDATE SET summary = excluded.summary, desired_titles = excluded.desired_titles,
		   desired_locations = excluded.desired_locations, min_salary = excluded.min_salary, remote_preference = excluded.remote_preference,
		   updated_at = excluded.updated_at`,
		profileID, v.Summary, v.DesiredTitles, v.DesiredLocations, salary, v.RemotePreference,
	); err != nil {
		return Values{}, err
	}
	persona.PublishProfileDetailsChanged(profileID)
	return v, nil
}

// --- AI tools ---

func errResult(action string, err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, ErrUnknownProfile):
		return db.ErrResult("unknown profile id (see list_profiles)")
	case errors.Is(err, ErrNegativeSalary), errors.Is(err, ErrInvalidRemotePreference):
		return db.ErrResult(err.Error())
	}
	return db.ErrResult(fmt.Sprintf("failed to %s: %v", action, err))
}

type getArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile (from list_profiles)"`
}

func RegisterGetProfileDetails(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_profile_details",
		Description: "Read the summary and the goals of a profile (desiredTitles, desiredLocations, minSalary, remotePreference): the values every persona of the profile uses " +
			"unless the persona has its own (get_persona_details shows what one persona shows and lists its own values in overridden). remotePreference is empty (not set), \"onsite\", \"open\" or \"remote_only\"; minSalary 0 is no minimum.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		v, err := Get(args.ProfileID)
		if err != nil {
			return errResult("read the profile details", err), nil, nil
		}
		return db.JSONResult(v)
	})
}

type updateArgs struct {
	ProfileID        string  `json:"profileId" jsonschema:"the profile to update (from list_profiles)"`
	Summary          *string `json:"summary,omitempty" jsonschema:"the profile's summary, a free-text bio"`
	DesiredTitles    *string `json:"desiredTitles,omitempty" jsonschema:"job titles the person is looking for, comma-separated"`
	DesiredLocations *string `json:"desiredLocations,omitempty" jsonschema:"locations the person is willing to work in, comma-separated"`
	MinSalary        *int64  `json:"minSalary,omitempty" jsonschema:"the person's own minimum acceptable salary; 0 means no minimum"`
	RemotePreference *string `json:"remotePreference,omitempty" jsonschema:"\"onsite\", \"open\" (open to remote or hybrid as well) or \"remote_only\"; an empty string clears it (not set)"`
}

func RegisterUpdateProfileDetails(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "update_profile_details",
		Description: "Update the summary and the goals of a profile. Only the fields provided change. This changes them for EVERY persona of the profile that follows them (a persona that has its own value keeps it); " +
			"to change one persona only, use update_persona_details.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		v, err := Update(args.ProfileID, Patch{
			Summary: args.Summary, DesiredTitles: args.DesiredTitles, DesiredLocations: args.DesiredLocations,
			MinSalary: args.MinSalary, RemotePreference: args.RemotePreference,
		})
		if err != nil {
			return errResult("update the profile details", err), nil, nil
		}
		return db.JSONResult(v)
	})
}
