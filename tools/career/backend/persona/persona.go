// persona.go is the AI-facing (and HTTP-mirrored) surface over
// career.db's own personas table, plus the shared requirePersonaExists
// guard every persona-scoped operation calls first. A Persona is a
// named "hat" the user wears — each one owns exactly one set of
// persona details, one skills set, one experience list (see db.go's
// own schema). As of step 10, every Persona belongs to exactly one
// Profile (profile.go, package main) — profileId is required and
// validated the same "forced to be mapped, never implicit" way
// personaId already is everywhere else in this tool. See
// plan/ai/tools/career/step-08-persona.md and
// plan/ai/tools/career/step-10-job-seeker-profile.md.
//
// As of the package-split refactor (step XX), this package cannot
// import package main (profile.go, where Profile itself lives) — Go
// forbids importing package main from anywhere. requireProfileExistsRef
// below is a deliberate independent duplicate of profile.go's own
// requireProfileExists, exactly the same "two separate, independently
// declared copies kept in sync by hand" convention already used for
// jobs' own errUnknownPortalLinkRef and portal's own
// activeCrawlRunPortalLinkIDsRef.
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

// ErrUnknownPersona is returned by requirePersonaExists — and
// wrapped into every persona-scoped operation's own error — whenever
// a caller supplies a personaId that doesn't exist. Treated as a
// real, rejected error everywhere (never a silent no-op, never an
// auto-created persona) — the literal enforcement of "forced to be
// mapped to an existing Persona." Exported so http.go (package main)
// can still map it to a 400.
var ErrUnknownPersona = errors.New("unknown persona id")

// requirePersonaExists is the shared guard every persona-scoped
// function (persona_details.go) calls before doing anything else.
func requirePersonaExists(id string) error {
	var exists int
	err := db.CareerDB.QueryRow(`SELECT 1 FROM personas WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return ErrUnknownPersona
	case err != nil:
		return err
	}
	return nil
}

// ErrUnknownProfile mirrors package main's own errUnknownProfile
// (profile.go) — a deliberate independent duplicate, not an import,
// since this package can never import package main. Exported so
// http.go can still map it to a 400.
var ErrUnknownProfile = errors.New("unknown profile id")

// requireProfileExistsRef is CreatePersona's own copy of profile.go's
// requireProfileExists — see this file's own doc comment for why.
func requireProfileExistsRef(id string) error {
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

type persona struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profileId"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

func CreatePersona(profileId, name, description string) (string, error) {
	if err := requireProfileExistsRef(profileId); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err := db.CareerDB.Exec(
		`INSERT INTO personas (id, profile_id, name, description, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		id, profileId, name, description,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ListPersonas optionally filters by profileId — omitted (empty
// string) lists every persona across every profile, matching this
// tool's own "omitting an optional filter means unfiltered" convention
// (search_jobs's own query/location params).
func ListPersonas(profileId string) ([]persona, error) {
	query := `SELECT id, profile_id, name, description, created_at, updated_at FROM personas`
	args := []any{}
	if profileId != "" {
		query += ` WHERE profile_id = ?`
		args = append(args, profileId)
	}
	query += ` ORDER BY created_at ASC`

	rows, err := db.CareerDB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	personas := []persona{}
	for rows.Next() {
		var p persona
		var description, updatedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.ProfileID, &p.Name, &description, &p.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		p.Description = description.String
		p.UpdatedAt = updatedAt.String
		personas = append(personas, p)
	}
	return personas, rows.Err()
}

func UpdatePersona(id string, name, description *string) error {
	if err := requirePersonaExists(id); err != nil {
		return err
	}
	if name == nil && description == nil {
		return nil
	}

	setClauses := ""
	args := []any{}
	if name != nil {
		setClauses += "name = ?, "
		args = append(args, *name)
	}
	if description != nil {
		setClauses += "description = ?, "
		args = append(args, *description)
	}
	setClauses += "updated_at = datetime('now')"
	args = append(args, id)

	_, err := db.CareerDB.Exec(`UPDATE personas SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

// DeletePersona cascades (ON DELETE CASCADE, db.go's own schema) to
// that persona's own details row, every skill, every experience entry
// — a genuinely larger blast radius than a plain single-row delete.
// A benign no-op if id is unknown, matching this tool's existing
// posture for every other delete-by-id operation.
func DeletePersona(id string) error {
	_, err := db.CareerDB.Exec(`DELETE FROM personas WHERE id = ?`, id)
	return err
}

// --- MCP registration ---

type createPersonaArgs struct {
	ProfileID   string `json:"profileId" jsonschema:"the profile (job seeker) this persona belongs to, from create_profile or list_profiles"`
	Name        string `json:"name" jsonschema:"a short name for this persona, e.g. \"Backend Engineer\""`
	Description string `json:"description,omitempty" jsonschema:"what this persona is for"`
}

func RegisterCreatePersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_persona",
		Description: "Create a new persona — a named \"hat\" a job seeker (profile) wears, each owning its own details, skills, and experience. Must belong to an existing profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createPersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.ProfileID == "" {
			return db.ErrResult("profileId is required"), nil, nil
		}
		if args.Name == "" {
			return db.ErrResult("name is required"), nil, nil
		}
		id, err := CreatePersona(args.ProfileID, args.Name, args.Description)
		if err != nil {
			if errors.Is(err, ErrUnknownProfile) {
				return db.ErrResult(fmt.Sprintf("unknown profile id %q", args.ProfileID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to create persona: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]string{"id": id, "profileId": args.ProfileID, "name": args.Name, "description": args.Description})
	})
}

type listPersonasArgs struct {
	ProfileID string `json:"profileId,omitempty" jsonschema:"limit to personas belonging to this profile; omit to list every persona"`
}

func RegisterListPersonas(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_personas",
		Description: "List personas, optionally limited to one profile.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listPersonasArgs) (*mcp.CallToolResult, any, error) {
		personas, err := ListPersonas(args.ProfileID)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to list personas: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"personas": personas})
	})
}

type updatePersonaArgs struct {
	ID          string  `json:"id" jsonschema:"the persona's own id, from create_persona or list_personas"`
	Name        *string `json:"name,omitempty" jsonschema:"a short name for this persona"`
	Description *string `json:"description,omitempty" jsonschema:"what this persona is for"`
}

func RegisterUpdatePersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_persona",
		Description: "Update a persona's name or description. Only the fields provided are changed. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := UpdatePersona(args.ID, args.Name, args.Description); err != nil {
			if errors.Is(err, ErrUnknownPersona) {
				return db.ErrResult(fmt.Sprintf("unknown persona id %q", args.ID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to update persona: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deletePersonaArgs struct {
	ID string `json:"id" jsonschema:"the persona's own id — deleting it also deletes its own details, every skill, and every experience entry"`
}

func RegisterDeletePersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_persona",
		Description: "Delete a persona. This also permanently deletes that persona's own details, every skill, and every experience entry — there is no separate confirmation step.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deletePersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := DeletePersona(args.ID); err != nil {
			return db.ErrResult(fmt.Sprintf("failed to delete persona: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}
