// persona.go is the AI-facing (and HTTP-mirrored) surface over
// career.db's own personas table, plus the shared requirePersonaExists
// guard every profile/skill/experience operation in profile.go now
// calls first. A Persona is a named "hat" the user wears — each one
// owns exactly one profile, one skills set, one experience list (see
// db.go's own schema). See plan/ai/tools/career/step-08-persona.md.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errUnknownPersona is returned by requirePersonaExists — and
// wrapped into every profile/skill/experience function's own error —
// whenever a caller supplies a personaId that doesn't exist. Treated
// as a real, rejected error everywhere (never a silent no-op, never
// an auto-created persona) — the literal enforcement of "forced to be
// mapped to an existing Persona."
var errUnknownPersona = errors.New("unknown persona id")

// requirePersonaExists is the shared guard every profile/skill/
// experience function (profile.go) calls before doing anything else.
func requirePersonaExists(id string) error {
	var exists int
	err := careerDB.QueryRow(`SELECT 1 FROM personas WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownPersona
	case err != nil:
		return err
	}
	return nil
}

type persona struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

func createPersona(name, description string) (string, error) {
	id := uuid.NewString()
	_, err := careerDB.Exec(
		`INSERT INTO personas (id, name, description, created_at) VALUES (?, ?, ?, datetime('now'))`,
		id, name, description,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

func listPersonas() ([]persona, error) {
	rows, err := careerDB.Query(
		`SELECT id, name, description, created_at, updated_at FROM personas ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	personas := []persona{}
	for rows.Next() {
		var p persona
		var description, updatedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &description, &p.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		p.Description = description.String
		p.UpdatedAt = updatedAt.String
		personas = append(personas, p)
	}
	return personas, rows.Err()
}

func updatePersona(id string, name, description *string) error {
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

	_, err := careerDB.Exec(`UPDATE personas SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

// deletePersona cascades (ON DELETE CASCADE, db.go's own schema) to
// that persona's own profile row, every skill, every experience entry
// — a genuinely larger blast radius than any other destructive
// operation this tool has. A benign no-op if id is unknown, matching
// this tool's existing posture for every other delete-by-id operation.
func deletePersona(id string) error {
	_, err := careerDB.Exec(`DELETE FROM personas WHERE id = ?`, id)
	return err
}

// --- MCP registration ---

type createPersonaArgs struct {
	Name        string `json:"name" jsonschema:"a short name for this persona, e.g. \"Backend Engineer\""`
	Description string `json:"description,omitempty" jsonschema:"what this persona is for"`
}

func registerCreatePersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_persona",
		Description: "Create a new persona — a named \"hat\" the user wears, each owning its own profile, skills, and experience. Profile/skill/experience operations require an existing persona's id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createPersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.Name == "" {
			return errResult("name is required"), nil, nil
		}
		id, err := createPersona(args.Name, args.Description)
		if err != nil {
			return errResult(fmt.Sprintf("failed to create persona: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id, "name": args.Name, "description": args.Description})
	})
}

type listPersonasArgs struct{}

func registerListPersonas(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_personas",
		Description: "List every persona the user has created.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listPersonasArgs) (*mcp.CallToolResult, any, error) {
		personas, err := listPersonas()
		if err != nil {
			return errResult(fmt.Sprintf("failed to list personas: %v", err)), nil, nil
		}
		return jsonResult(map[string]any{"personas": personas})
	})
}

type updatePersonaArgs struct {
	ID          string  `json:"id" jsonschema:"the persona's own id, from create_persona or list_personas"`
	Name        *string `json:"name,omitempty" jsonschema:"a short name for this persona"`
	Description *string `json:"description,omitempty" jsonschema:"what this persona is for"`
}

func registerUpdatePersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_persona",
		Description: "Update a persona's name or description. Only the fields provided are changed. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := updatePersona(args.ID, args.Name, args.Description); err != nil {
			if errors.Is(err, errUnknownPersona) {
				return errResult(fmt.Sprintf("unknown persona id %q", args.ID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update persona: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deletePersonaArgs struct {
	ID string `json:"id" jsonschema:"the persona's own id — deleting it also deletes its own profile, every skill, and every experience entry"`
}

func registerDeletePersona(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_persona",
		Description: "Delete a persona. This also permanently deletes that persona's own profile, every skill, and every experience entry — there is no separate confirmation step.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deletePersonaArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := deletePersona(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to delete persona: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}
