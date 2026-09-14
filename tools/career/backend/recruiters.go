// recruiters.go is the AI-facing (and HTTP-mirrored) surface over
// jobs.db's own recruiters table — a contact at a company, forced to
// be mapped to an existing Company, never implicit (the same
// "forced to be mapped" pattern profile.go/persona.go already use for
// their own parent references). See
// plan/ai/tools/career/step-16-recruiters.md.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type recruiter struct {
	ID        string `json:"id"`
	CompanyID string `json:"companyId"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func createRecruiter(companyId, firstName, lastName, email string) (string, error) {
	if err := requireCompanyExists(companyId); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err := jobsDB.Exec(
		`INSERT INTO recruiters (id, company_id, first_name, last_name, email, created_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
		id, companyId, firstName, lastName, email,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// listRecruiters returns every recruiter, newest first; companyId, if
// non-empty, restricts to recruiters at that company — the same
// optional-filter shape listPersonas(profileId string) already uses.
func listRecruiters(companyId string) ([]recruiter, error) {
	where := `WHERE 1=1`
	args := []any{}
	if companyId != "" {
		where += ` AND company_id = ?`
		args = append(args, companyId)
	}

	rows, err := jobsDB.Query(
		`SELECT id, company_id, first_name, last_name, email, created_at, updated_at
		 FROM recruiters `+where+` ORDER BY created_at ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	recruiters := []recruiter{}
	for rows.Next() {
		var r recruiter
		var updatedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.CompanyID, &r.FirstName, &r.LastName, &r.Email, &r.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		r.UpdatedAt = updatedAt.String
		recruiters = append(recruiters, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recruiters, nil
}

var errUnknownRecruiter = errors.New("unknown recruiter id")

func requireRecruiterExists(id string) error {
	var exists int
	err := jobsDB.QueryRow(`SELECT 1 FROM recruiters WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownRecruiter
	case err != nil:
		return err
	}
	return nil
}

// updateRecruiter never reassigns company_id — see this step's own
// open question 1; only delete-and-recreate moves a recruiter to a
// different company.
func updateRecruiter(id string, firstName, lastName, email *string) error {
	if err := requireRecruiterExists(id); err != nil {
		return err
	}
	if firstName == nil && lastName == nil && email == nil {
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
	if email != nil {
		setClauses += "email = ?, "
		args = append(args, *email)
	}
	setClauses += "updated_at = datetime('now')"
	args = append(args, id)

	_, err := jobsDB.Exec(`UPDATE recruiters SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

func deleteRecruiter(id string) error {
	_, err := jobsDB.Exec(`DELETE FROM recruiters WHERE id = ?`, id)
	return err
}

// --- MCP registration ---

type createRecruiterArgs struct {
	CompanyID string `json:"companyId" jsonschema:"the company this recruiter works at — must already exist (see create_company/list_companies)"`
	FirstName string `json:"firstName" jsonschema:"the recruiter's own first name"`
	LastName  string `json:"lastName" jsonschema:"the recruiter's own last name"`
	Email     string `json:"email" jsonschema:"the recruiter's own email address"`
}

func registerCreateRecruiter(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_recruiter",
		Description: "Create a recruiter contact at an existing company.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createRecruiterArgs) (*mcp.CallToolResult, any, error) {
		if args.CompanyID == "" {
			return errResult("companyId is required"), nil, nil
		}
		id, err := createRecruiter(args.CompanyID, args.FirstName, args.LastName, args.Email)
		if err != nil {
			if errors.Is(err, errUnknownCompany) {
				return errResult(fmt.Sprintf("unknown company id %q", args.CompanyID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to create recruiter: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id})
	})
}

type listRecruitersArgs struct {
	CompanyID string `json:"companyId,omitempty" jsonschema:"restrict to recruiters at this company; omit to list every recruiter"`
}

func registerListRecruiters(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_recruiters",
		Description: "List recruiter contacts, optionally restricted to one company.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listRecruitersArgs) (*mcp.CallToolResult, any, error) {
		recruiters, err := listRecruiters(args.CompanyID)
		if err != nil {
			return errResult(fmt.Sprintf("failed to list recruiters: %v", err)), nil, nil
		}
		return jsonResult(map[string]any{"recruiters": recruiters})
	})
}

type updateRecruiterArgs struct {
	ID        string  `json:"id" jsonschema:"the recruiter's own id, from create_recruiter or list_recruiters"`
	FirstName *string `json:"firstName,omitempty" jsonschema:"the recruiter's own first name"`
	LastName  *string `json:"lastName,omitempty" jsonschema:"the recruiter's own last name"`
	Email     *string `json:"email,omitempty" jsonschema:"the recruiter's own email address"`
}

func registerUpdateRecruiter(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_recruiter",
		Description: "Update a recruiter's first/last name or email. Only the fields provided are changed. The recruiter's own company cannot be reassigned this way — delete and recreate instead.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateRecruiterArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := updateRecruiter(args.ID, args.FirstName, args.LastName, args.Email); err != nil {
			if errors.Is(err, errUnknownRecruiter) {
				return errResult(fmt.Sprintf("unknown recruiter id %q", args.ID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update recruiter: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deleteRecruiterArgs struct {
	ID string `json:"id" jsonschema:"the recruiter's own id"`
}

func registerDeleteRecruiter(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_recruiter",
		Description: "Delete a recruiter contact.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteRecruiterArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := deleteRecruiter(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to delete recruiter: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}
