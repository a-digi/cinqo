// profile.go is the AI-facing (and HTTP-mirrored) surface over
// career.db's own profiles/profile_external_links tables — the actual
// job seeker using this tool. A Profile owns one-to-many Personas
// (persona package); every Persona belongs to exactly one Profile. See
// plan/ai/tools/career/step-10-job-seeker-profile.md.
package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// ErrUnknownProfile is returned by RequireProfileExists — the literal
// enforcement of "a Persona can be linked only to 1 [existing]
// profile," the same "forced to be mapped, never implicit" pattern
// step 8 already established one level down for Persona itself.
// Exported so both http.go (package main) and the persona package can
// map it to a real error.
var ErrUnknownProfile = errors.New("unknown profile id")

// RequireProfileExists is the shared guard every profile-scoped
// operation calls before doing anything else — including the persona
// package's own CreatePersona, which requires an existing Profile.
func RequireProfileExists(id string) error {
	var exists int
	err := db.CareerDB.QueryRow(`SELECT 1 FROM profiles WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return ErrUnknownProfile
	case err != nil:
		return err
	}
	return nil
}

type profileExternalLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

type profile struct {
	ID            string                `json:"id"`
	FirstName     string                `json:"firstName"`
	LastName      string                `json:"lastName"`
	CreatedAt     string                `json:"createdAt"`
	UpdatedAt     string                `json:"updatedAt,omitempty"`
	ExternalLinks []profileExternalLink `json:"externalLinks"`
}

func CreateProfile(firstName, lastName string) (string, error) {
	id := uuid.NewString()
	_, err := db.CareerDB.Exec(
		`INSERT INTO profiles (id, first_name, last_name, created_at) VALUES (?, ?, ?, datetime('now'))`,
		id, firstName, lastName,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ListProfiles returns every profile with its own external links
// nested — small expected count per profile, same "list is enough, no
// standalone getter" reasoning step 8 used for Persona.
func ListProfiles() ([]profile, error) {
	rows, err := db.CareerDB.Query(
		`SELECT id, first_name, last_name, created_at, updated_at FROM profiles ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	profiles := []profile{}
	for rows.Next() {
		var p profile
		var updatedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.CreatedAt, &updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.UpdatedAt = updatedAt.String
		p.ExternalLinks = []profileExternalLink{}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	linkRows, err := db.CareerDB.Query(
		`SELECT profile_id, platform, url FROM profile_external_links ORDER BY platform ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()
	byProfile := make(map[string][]profileExternalLink, len(profiles))
	for linkRows.Next() {
		var profileID string
		var link profileExternalLink
		if err := linkRows.Scan(&profileID, &link.Platform, &link.URL); err != nil {
			return nil, err
		}
		byProfile[profileID] = append(byProfile[profileID], link)
	}
	if err := linkRows.Err(); err != nil {
		return nil, err
	}
	for i := range profiles {
		if links, ok := byProfile[profiles[i].ID]; ok {
			profiles[i].ExternalLinks = links
		}
	}
	return profiles, nil
}

func UpdateProfile(id string, firstName, lastName *string) error {
	if err := RequireProfileExists(id); err != nil {
		return err
	}
	if firstName == nil && lastName == nil {
		return nil
	}

	setClauses := ""
	args := []any{}
	if firstName != nil {
		setClauses += "first_name = ?, "
		args = append(args, *firstName)
	}
	if lastName != nil {
		setClauses += "last_name = ?, "
		args = append(args, *lastName)
	}
	setClauses += "updated_at = datetime('now')"
	args = append(args, id)

	_, err := db.CareerDB.Exec(`UPDATE profiles SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

// DeleteProfile cascades (ON DELETE CASCADE, db.go's own schema) to
// every Persona that profile owns, and — transitively, via step 8's
// own cascades — every one of those Personas' own Persona
// Details/Skills/Experience rows. The largest blast radius this tool
// has: a single call can delete an entire job seeker's whole
// footprint. A benign no-op if id is unknown, matching every other
// delete-by-id operation in this tool.
func DeleteProfile(id string) error {
	_, err := db.CareerDB.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}

// UpsertProfileExternalLink is an upsert, not a no-op-on-conflict like
// AddCareerSkill (persona package) — a URL is correction-prone content,
// so re-adding an already-present platform updates it in place rather
// than being silently ignored.
func UpsertProfileExternalLink(profileId, platform, url string) error {
	if err := RequireProfileExists(profileId); err != nil {
		return err
	}
	_, err := db.CareerDB.Exec(
		`INSERT INTO profile_external_links (id, profile_id, platform, url) VALUES (?, ?, ?, ?)
		 ON CONFLICT(profile_id, platform) DO UPDATE SET url = excluded.url`,
		uuid.NewString(), profileId, platform, url,
	)
	return err
}

func RemoveProfileExternalLink(profileId, platform string) error {
	if err := RequireProfileExists(profileId); err != nil {
		return err
	}
	_, err := db.CareerDB.Exec(`DELETE FROM profile_external_links WHERE profile_id = ? AND platform = ?`, profileId, platform)
	return err
}

// --- MCP registration ---

type createProfileArgs struct {
	FirstName string `json:"firstName" jsonschema:"the job seeker's own first name"`
	LastName  string `json:"lastName" jsonschema:"the job seeker's own last name"`
}

func RegisterCreateProfile(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_profile",
		Description: "Create a new profile — represents one job seeker using this tool. A profile owns one or more personas; every persona must belong to an existing profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createProfileArgs) (*mcp.CallToolResult, any, error) {
		id, err := CreateProfile(args.FirstName, args.LastName)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to create profile: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]string{"id": id, "firstName": args.FirstName, "lastName": args.LastName})
	})
}

type listProfilesArgs struct{}

func RegisterListProfiles(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_profiles",
		Description: "List every profile (job seeker) using this tool, including each one's own external platform links.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listProfilesArgs) (*mcp.CallToolResult, any, error) {
		profiles, err := ListProfiles()
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to list profiles: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"profiles": profiles})
	})
}

type updateProfileArgs struct {
	ID        string  `json:"id" jsonschema:"the profile's own id, from create_profile or list_profiles"`
	FirstName *string `json:"firstName,omitempty" jsonschema:"the job seeker's own first name"`
	LastName  *string `json:"lastName,omitempty" jsonschema:"the job seeker's own last name"`
}

func RegisterUpdateProfile(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_profile",
		Description: "Update a profile's first/last name. Only the fields provided are changed. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateProfileArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := UpdateProfile(args.ID, args.FirstName, args.LastName); err != nil {
			if errors.Is(err, ErrUnknownProfile) {
				return db.ErrResult(fmt.Sprintf("unknown profile id %q", args.ID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to update profile: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deleteProfileArgs struct {
	ID string `json:"id" jsonschema:"the profile's own id — deleting it also deletes every persona it owns, and everything those personas own (persona details, skills, experience)"`
}

func RegisterDeleteProfile(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_profile",
		Description: "Delete a profile. This permanently deletes every persona this profile owns, and everything those personas own in turn (persona details, skills, experience) — the largest deletion this tool can perform in one call. There is no separate confirmation step.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteProfileArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := DeleteProfile(args.ID); err != nil {
			return db.ErrResult(fmt.Sprintf("failed to delete profile: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}

type addProfileExternalLinkArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile to add this external link to"`
	Platform  string `json:"platform" jsonschema:"the platform name, e.g. \"linkedin\", \"github\", \"portfolio\" — free text, not a fixed list"`
	URL       string `json:"url" jsonschema:"the profile's own URL on that platform"`
}

func RegisterAddProfileExternalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_profile_external_link",
		Description: "Add or update an external platform link (e.g. LinkedIn) on a profile. Adding the same platform again updates its URL rather than duplicating it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addProfileExternalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		if args.Platform == "" || args.URL == "" {
			return db.ErrResult("platform and url are both required"), nil, nil
		}
		if err := UpsertProfileExternalLink(args.ProfileID, args.Platform, args.URL); err != nil {
			if errors.Is(err, ErrUnknownProfile) {
				return db.ErrResult(fmt.Sprintf("unknown profile id %q", args.ProfileID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to add external link: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("saved %s link", args.Platform)}}}, nil, nil
	})
}

type removeProfileExternalLinkArgs struct {
	ProfileID string `json:"profileId" jsonschema:"the profile to remove this external link from"`
	Platform  string `json:"platform" jsonschema:"the platform name to remove"`
}

func RegisterRemoveProfileExternalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_profile_external_link",
		Description: "Remove an external platform link from a profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removeProfileExternalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		if args.Platform == "" {
			return db.ErrResult("platform is required"), nil, nil
		}
		if err := RemoveProfileExternalLink(args.ProfileID, args.Platform); err != nil {
			if errors.Is(err, ErrUnknownProfile) {
				return db.ErrResult(fmt.Sprintf("unknown profile id %q", args.ProfileID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to remove external link: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed %s link", args.Platform)}}}, nil, nil
	})
}
