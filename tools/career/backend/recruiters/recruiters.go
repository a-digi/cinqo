// recruiters.go is the AI-facing (and HTTP-mirrored) surface over
// jobs.db's own recruiters table — a contact at a company, forced to
// linked to existing Companies (zero or more, many to many, never implicit) (the same
// "forced to be mapped" pattern profile.go/persona.go already use for
// their own parent references). See
// plan/ai/tools/career/step-16-recruiters.md.
package recruiters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/companies"
	"career-tool-backend/db"
)

// Company is one company a recruiter works for.
type Company struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type recruiter struct {
	ID string `json:"id"`
	// CompanyID is the company linked first, or "" (kept so older callers keep working; plan step-169 decision 7).
	CompanyID  string    `json:"companyId"`
	CompanyIDs []string  `json:"companyIds"`
	Companies  []Company `json:"companies"`
	FirstName  string    `json:"firstName"`
	LastName   string    `json:"lastName"`
	Email      string    `json:"email"`
	CreatedAt  string    `json:"createdAt"`
	UpdatedAt  string    `json:"updatedAt,omitempty"`
}

// cleanCompanyIDs trims, drops blanks and repeats, and keeps the order.
func cleanCompanyIDs(ids []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// requireCompaniesExist checks every id before anything is written; the first unknown one is companies.ErrUnknownCompany.
func requireCompaniesExist(ids []string) error {
	for _, id := range ids {
		if err := companies.RequireCompanyExists(id); err != nil {
			return err
		}
	}
	return nil
}

// CreateRecruiter creates a recruiter linked to zero or more existing companies (plan step-169): every company is checked first
// and the recruiter and its links are written in one transaction, so nothing is left half done.
func CreateRecruiter(companyIDs []string, firstName, lastName, email string) (string, error) {
	companyIDs = cleanCompanyIDs(companyIDs)
	if err := requireCompaniesExist(companyIDs); err != nil {
		return "", err
	}
	id := uuid.NewString()
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO recruiters (id, first_name, last_name, email, created_at) VALUES (?, ?, ?, ?, datetime('now'))`,
		id, firstName, lastName, email,
	); err != nil {
		return "", err
	}
	for _, c := range companyIDs {
		if _, err := tx.Exec(`INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES (?, ?)`, id, c); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

// SetCompanies replaces the companies of a recruiter with exactly these (none is allowed). Companies that stay keep their
// link (and its time); the others are removed and the new ones added, in one transaction. An unknown recruiter is
// ErrUnknownRecruiter, an unknown company companies.ErrUnknownCompany, and nothing changes then.
func SetCompanies(id string, companyIDs []string) error {
	companyIDs = cleanCompanyIDs(companyIDs)
	if err := requireRecruiterExists(id); err != nil {
		return err
	}
	if err := requireCompaniesExist(companyIDs); err != nil {
		return err
	}
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM recruiter_companies WHERE recruiter_id = ?`+notIn(companyIDs), withArgs(id, companyIDs)...); err != nil {
		return err
	}
	for _, c := range companyIDs {
		if _, err := tx.Exec(`INSERT INTO recruiter_companies (recruiter_id, company_id) VALUES (?, ?) ON CONFLICT(recruiter_id, company_id) DO NOTHING`, id, c); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE recruiters SET updated_at = datetime('now') WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func notIn(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ` AND company_id NOT IN (` + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + `)`
}

func withArgs(id string, ids []string) []any {
	args := []any{id}
	for _, c := range ids {
		args = append(args, c)
	}
	return args
}

// ListRecruiters returns every recruiter, oldest first, each with the companies it works for (in the order they were linked);
// companyId, if non-empty, restricts to recruiters linked to that company (through recruiter_companies).
func ListRecruiters(companyId string) ([]recruiter, error) {
	where := `WHERE 1=1`
	args := []any{}
	if companyId != "" {
		where += ` AND EXISTS (SELECT 1 FROM recruiter_companies rc WHERE rc.recruiter_id = recruiters.id AND rc.company_id = ?)`
		args = append(args, companyId)
	}

	rows, err := db.JobsDB.Query(
		`SELECT id, first_name, last_name, email, created_at, updated_at
		 FROM recruiters `+where+` ORDER BY created_at ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	recruiters := []recruiter{}
	byID := map[string]int{}
	for rows.Next() {
		var r recruiter
		var updatedAt sql.NullString
		if err := rows.Scan(&r.ID, &r.FirstName, &r.LastName, &r.Email, &r.CreatedAt, &updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		r.UpdatedAt = updatedAt.String
		r.CompanyIDs, r.Companies = []string{}, []Company{}
		byID[r.ID] = len(recruiters)
		recruiters = append(recruiters, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close() // the connection is the only one: read the links after this result set is closed

	links, err := db.JobsDB.Query(
		`SELECT rc.recruiter_id, c.id, c.name
		 FROM recruiter_companies rc JOIN companies c ON c.id = rc.company_id
		 ORDER BY rc.created_at ASC, rc.rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer links.Close()
	for links.Next() {
		var rid string
		var c Company
		if err := links.Scan(&rid, &c.ID, &c.Name); err != nil {
			return nil, err
		}
		if i, ok := byID[rid]; ok {
			recruiters[i].Companies = append(recruiters[i].Companies, c)
			recruiters[i].CompanyIDs = append(recruiters[i].CompanyIDs, c.ID)
			if recruiters[i].CompanyID == "" {
				recruiters[i].CompanyID = c.ID
			}
		}
	}
	return recruiters, links.Err()
}

// ErrUnknownRecruiter is exported so http.go (package main) can still
// map it to a 400.
var ErrUnknownRecruiter = errors.New("unknown recruiter id")

func requireRecruiterExists(id string) error {
	var exists int
	err := db.JobsDB.QueryRow(`SELECT 1 FROM recruiters WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return ErrUnknownRecruiter
	case err != nil:
		return err
	}
	return nil
}

// UpdateRecruiter changes the name and email only; the companies are changed by SetCompanies (plan step-169).
func UpdateRecruiter(id string, firstName, lastName, email *string) error {
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

	_, err := db.JobsDB.Exec(`UPDATE recruiters SET `+setClauses+` WHERE id = ?`, args...)
	return err
}

func DeleteRecruiter(id string) error {
	_, err := db.JobsDB.Exec(`DELETE FROM recruiters WHERE id = ?`, id)
	return err
}

// --- MCP registration ---

type createRecruiterArgs struct {
	CompanyID  string   `json:"companyId,omitempty" jsonschema:"one company this recruiter works at — must already exist (see create_company/list_companies)"`
	CompanyIDs []string `json:"companyIds,omitempty" jsonschema:"the companies this recruiter works at (a recruiter can work for several) — each must already exist; may be combined with companyId, or both omitted for none"`
	FirstName  string   `json:"firstName" jsonschema:"the recruiter's own first name"`
	LastName   string   `json:"lastName" jsonschema:"the recruiter's own last name"`
	Email      string   `json:"email" jsonschema:"the recruiter's own email address"`
}

func RegisterCreateRecruiter(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_recruiter",
		Description: "Create a recruiter contact linked to zero or more existing companies (companyId and/or companyIds). Deleting a company never deletes the recruiter.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createRecruiterArgs) (*mcp.CallToolResult, any, error) {
		id, err := CreateRecruiter(append([]string{args.CompanyID}, args.CompanyIDs...), args.FirstName, args.LastName, args.Email)
		if err != nil {
			if errors.Is(err, companies.ErrUnknownCompany) {
				return db.ErrResult("unknown company id: every companyId must be an existing company"), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to create recruiter: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]string{"id": id})
	})
}

type listRecruitersArgs struct {
	CompanyID string `json:"companyId,omitempty" jsonschema:"restrict to recruiters linked to this company; omit to list every recruiter"`
}

func RegisterListRecruiters(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_recruiters",
		Description: "List recruiter contacts with the companies each works at (companyIds, companies), optionally restricted to one company.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listRecruitersArgs) (*mcp.CallToolResult, any, error) {
		recruiters, err := ListRecruiters(args.CompanyID)
		if err != nil {
			return db.ErrResult(fmt.Sprintf("failed to list recruiters: %v", err)), nil, nil
		}
		return db.JSONResult(map[string]any{"recruiters": recruiters})
	})
}

type updateRecruiterArgs struct {
	ID        string  `json:"id" jsonschema:"the recruiter's own id, from create_recruiter or list_recruiters"`
	FirstName *string `json:"firstName,omitempty" jsonschema:"the recruiter's own first name"`
	LastName  *string `json:"lastName,omitempty" jsonschema:"the recruiter's own last name"`
	Email     *string `json:"email,omitempty" jsonschema:"the recruiter's own email address"`
	// CompanyIDs replaces the whole set of companies; an empty list removes them all; omitted leaves them as they are.
	CompanyIDs *[]string `json:"companyIds,omitempty" jsonschema:"replace the companies this recruiter works at with exactly these (existing company ids); an empty list removes them all; omit to leave them unchanged"`
}

func RegisterUpdateRecruiter(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_recruiter",
		Description: "Update a recruiter's first/last name or email, and/or replace the companies they work at (companyIds). Only the fields provided are changed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updateRecruiterArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if args.CompanyIDs != nil {
			if err := SetCompanies(args.ID, *args.CompanyIDs); err != nil {
				if errors.Is(err, companies.ErrUnknownCompany) {
					return db.ErrResult("unknown company id: every companyId must be an existing company"), nil, nil
				}
				if errors.Is(err, ErrUnknownRecruiter) {
					return db.ErrResult(fmt.Sprintf("unknown recruiter id %q", args.ID)), nil, nil
				}
				return db.ErrResult(fmt.Sprintf("failed to update recruiter: %v", err)), nil, nil
			}
		}
		if err := UpdateRecruiter(args.ID, args.FirstName, args.LastName, args.Email); err != nil {
			if errors.Is(err, ErrUnknownRecruiter) {
				return db.ErrResult(fmt.Sprintf("unknown recruiter id %q", args.ID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to update recruiter: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deleteRecruiterArgs struct {
	ID string `json:"id" jsonschema:"the recruiter's own id"`
}

func RegisterDeleteRecruiter(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_recruiter",
		Description: "Delete a recruiter contact.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deleteRecruiterArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return db.ErrResult("id is required"), nil, nil
		}
		if err := DeleteRecruiter(args.ID); err != nil {
			return db.ErrResult(fmt.Sprintf("failed to delete recruiter: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}
