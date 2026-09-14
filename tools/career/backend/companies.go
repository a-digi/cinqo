// companies.go is the AI-facing (and HTTP-mirrored) surface over
// jobs.db's own companies table — a small, human-curated directory of
// companies, separate from the AI-crawled, free-text jobs.company
// column. jobs.company_id (db.go) is an optional link on top of a
// job; deleting a company un-links its jobs (ON DELETE SET NULL)
// rather than deleting them. See
// plan/ai/tools/career/step-14-companies.md.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// errUnknownCompany is returned by requireCompanyExists — the same
// "forced to be mapped to an existing [parent], never implicit"
// enforcement profile.go/persona.go already use for their own parent
// references.
var errUnknownCompany = errors.New("unknown company id")

func requireCompanyExists(id string) error {
	var exists int
	err := jobsDB.QueryRow(`SELECT 1 FROM companies WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownCompany
	case err != nil:
		return err
	}
	return nil
}

type company struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	JobCount       int    `json:"jobCount"`
	RecruiterCount int    `json:"recruiterCount"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

func createCompany(name, description string) (string, error) {
	id := uuid.NewString()
	_, err := jobsDB.Exec(
		`INSERT INTO companies (id, name, description, created_at) VALUES (?, ?, ?, datetime('now'))`,
		id, name, description,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// listCompanies returns every company with its own linked job count
// and recruiter count — both plain COUNT(*) subqueries, recomputed
// every call (no caching/denormalization needed at this tool's
// realistic scale). Same "list is enough, no standalone getter"
// reasoning listProfiles/listPersonas already use.
func listCompanies() ([]company, error) {
	rows, err := jobsDB.Query(
		`SELECT c.id, c.name, c.description, c.created_at, c.updated_at,
		        (SELECT COUNT(*) FROM jobs WHERE jobs.company_id = c.id),
		        (SELECT COUNT(*) FROM recruiters WHERE recruiters.company_id = c.id)
		 FROM companies c ORDER BY c.created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	companies := []company{}
	for rows.Next() {
		var c company
		var updatedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &updatedAt, &c.JobCount, &c.RecruiterCount); err != nil {
			return nil, err
		}
		c.UpdatedAt = updatedAt.String
		companies = append(companies, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return companies, nil
}

func updateCompany(id string, name, description *string) error {
	if err := requireCompanyExists(id); err != nil {
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

	_, err := jobsDB.Exec(`UPDATE companies SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

// deleteCompany un-links (never deletes) any jobs that pointed at it
// — ON DELETE SET NULL, db.go's own schema. A benign no-op if id is
// unknown, matching every other delete-by-id operation in this tool.
func deleteCompany(id string) error {
	_, err := jobsDB.Exec(`DELETE FROM companies WHERE id = ?`, id)
	return err
}

// --- MCP registration ---

type createCompanyArgs struct {
	Name        string `json:"name" jsonschema:"the company's own name"`
	Description string `json:"description,omitempty" jsonschema:"free text describing the company"`
}

func registerCreateCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_company",
		Description: "Create a new company in this tool's own curated company directory.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.Name == "" {
			return errResult("name is required"), nil, nil
		}
		id, err := createCompany(args.Name, args.Description)
		if err != nil {
			return errResult(fmt.Sprintf("failed to create company: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id, "name": args.Name, "description": args.Description})
	})
}

type listCompaniesArgs struct{}

func registerListCompanies(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_companies",
		Description: "List every company in this tool's own curated company directory, including each one's own linked job count and recruiter count.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listCompaniesArgs) (*mcp.CallToolResult, any, error) {
		companies, err := listCompanies()
		if err != nil {
			return errResult(fmt.Sprintf("failed to list companies: %v", err)), nil, nil
		}
		return jsonResult(map[string]any{"companies": companies})
	})
}

type updateCompanyArgs struct {
	ID          string  `json:"id" jsonschema:"the company's own id, from create_company or list_companies"`
	Name        *string `json:"name,omitempty" jsonschema:"the company's own name"`
	Description *string `json:"description,omitempty" jsonschema:"free text describing the company"`
}

func registerUpdateCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_company",
		Description: "Update a company's name/description. Only the fields provided are changed. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := updateCompany(args.ID, args.Name, args.Description); err != nil {
			if errors.Is(err, errUnknownCompany) {
				return errResult(fmt.Sprintf("unknown company id %q", args.ID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update company: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deleteCompanyArgs struct {
	ID string `json:"id" jsonschema:"the company's own id — deleting it un-links (never deletes) any jobs pointed at it"`
}

func registerDeleteCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_company",
		Description: "Delete a company. Any jobs linked to it are un-linked, never deleted.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := deleteCompany(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to delete company: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}
