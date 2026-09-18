// Package companies is the AI-facing (and HTTP-mirrored) surface over
// jobs.db's own companies table — a small, human-curated directory of
// companies, separate from the AI-crawled, free-text jobs.company
// column. jobs.company_id (db.CareerDB/JobsDB's own schema) is an
// optional link on top of a job; deleting a company un-links its jobs
// (ON DELETE SET NULL) rather than deleting them. Extracted into its
// own package (step XX) as part of this backend's split into
// subpackages mirroring tools/browser/backend's own auth/crawler/
// shared layout — RequireCompanyExists/ErrUnknownCompany are exported
// specifically because the jobs package (linkJobToCompany) and this
// tool's own package main (recruiters.go, http.go) both need them, and
// package main can never be imported back. See
// plan/ai/tools/career/step-14-companies.md and
// plan/ai/tools/career/step-XX-package-split.md.
package companies

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// ErrUnknownCompany is returned by RequireCompanyExists — the same
// "forced to be mapped to an existing [parent], never implicit"
// enforcement the profile/persona domain already uses for its own
// parent references.
var ErrUnknownCompany = errors.New("unknown company id")

func RequireCompanyExists(id string) error {
	var exists int
	err := db.JobsDB.QueryRow(`SELECT 1 FROM companies WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return ErrUnknownCompany
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

func CreateCompany(name, description string) (string, error) {
	id := uuid.NewString()
	_, err := db.JobsDB.Exec(
		`INSERT INTO companies (id, name, description, created_at) VALUES (?, ?, ?, datetime('now'))`,
		id, name, description,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// ListCompanies returns every company with its own linked job count
// and recruiter count — both plain COUNT(*) subqueries, recomputed
// every call (no caching/denormalization needed at this tool's
// realistic scale). Same "list is enough, no standalone getter"
// reasoning listProfiles/listPersonas already use.
func ListCompanies() ([]company, error) {
	rows, err := db.JobsDB.Query(
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

func UpdateCompany(id string, name, description *string) error {
	if err := RequireCompanyExists(id); err != nil {
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

	_, err := db.JobsDB.Exec(`UPDATE companies SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

// DeleteCompany un-links (never deletes) any jobs that pointed at it
// — ON DELETE SET NULL, db's own schema. A benign no-op if id is
// unknown, matching every other delete-by-id operation in this tool.
func DeleteCompany(id string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM companies WHERE id = ?`, id)
	return err
}

// WriteCompanyAwareError maps ErrUnknownCompany to a 400 (a caller
// mistake) and everything else to a 500 — moved here from this tool's
// own http.go (package main) since it's fundamentally companies-domain
// error mapping, not generic.
func WriteCompanyAwareError(w http.ResponseWriter, action string, err error) {
	if errors.Is(err, ErrUnknownCompany) {
		http.Error(w, "unknown company id", http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

// --- MCP registration ---

type createCompanyArgs struct {
	Name        string `json:"name" jsonschema:"the company's own name"`
	Description string `json:"description,omitempty" jsonschema:"free text describing the company"`
}

func RegisterCreateCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_company",
		Description: "Create a new company in this tool's own curated company directory.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.Name == "" {
			return db.ErrResult("name is required"), nil, nil
		}
		id, err := CreateCompany(args.Name, args.Description)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to create company: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]string{"id": id, "name": args.Name, "description": args.Description})
	})
}

type listCompaniesArgs struct{}

func RegisterListCompanies(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_companies",
		Description: "List every company in this tool's own curated company directory, including each one's own linked job count and recruiter count.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listCompaniesArgs) (*mcp.CallToolResult, any, error) {
		companies, err := ListCompanies()
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to list companies: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"companies": companies})
	})
}

type updateCompanyArgs struct {
	ID          string  `json:"id" jsonschema:"the company's own id, from create_company or list_companies"`
	Name        *string `json:"name,omitempty" jsonschema:"the company's own name"`
	Description *string `json:"description,omitempty" jsonschema:"free text describing the company"`
}

func RegisterUpdateCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_company",
		Description: "Update a company's name/description. Only the fields provided are changed. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := UpdateCompany(args.ID, args.Name, args.Description); err != nil {
			if errors.Is(err, ErrUnknownCompany) {
				return db.ErrResult(fmt.Sprintf("unknown company id %q", args.ID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to update company: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deleteCompanyArgs struct {
	ID string `json:"id" jsonschema:"the company's own id — deleting it un-links (never deletes) any jobs pointed at it"`
}

func RegisterDeleteCompany(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_company",
		Description: "Delete a company. Any jobs linked to it are un-linked, never deleted.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteCompanyArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := DeleteCompany(args.ID); err != nil {
			return db.ErrResult(fmt.Sprintf("failed to delete company: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}
