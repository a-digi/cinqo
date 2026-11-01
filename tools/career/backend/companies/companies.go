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
	"strings"

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

// FindCompanyByName looks up a company by a case-insensitive, trimmed
// exact name match — the same matching rule save_portal_job already
// uses for its own (title, company) dedup (jobs.go's own toLower(
// strings.TrimSpace(...)) convention, mirrored here in SQL since this
// package has no reason to share that file's own ASCII-only micro-
// optimization). found=false (not an error) means no match exists —
// the Domain Events listener that calls this (jobs/events.go's
// JobCreatedEventHandler) creates a new company in that case. See
// plan/ai/tools/career/step-83-job-created-company-linking.md.
func FindCompanyByName(name string) (id string, found bool, err error) {
	err = db.JobsDB.QueryRow(
		`SELECT id FROM companies WHERE LOWER(TRIM(name)) = ?`,
		strings.ToLower(strings.TrimSpace(name)),
	).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		return "", false, nil
	case err != nil:
		return "", false, err
	}
	return id, true, nil
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

// ListCompanies returns every company with its own linked job count and recruiter count — both plain COUNT(*) subqueries,
// recomputed every call (no caching/denormalization needed at this tool's realistic scale). Same "list is enough, no
// standalone getter" reasoning listProfiles/listPersonas already use. The POST and PUT answers and the MCP tool use it.
func ListCompanies() ([]company, error) {
	r, err := ListCompaniesPage("", 0, 0)
	if err != nil {
		return nil, err
	}
	return r.Companies, nil
}

// The limits of a page of companies (step 137): a limit that is given is clamped to 1 through MaxCompaniesLimit.
const (
	MinCompaniesLimit = 1
	MaxCompaniesLimit = 200
)

// CompanySummary is the overview of ALL companies, whatever the search or the page: how many there are, how many have a
// linked job and how many a recruiter, and the sums of the two counts.
type CompanySummary struct {
	Companies      int `json:"companies"`
	WithJobs       int `json:"withJobs"`
	WithRecruiters int `json:"withRecruiters"`
	Jobs           int `json:"jobs"`
	Recruiters     int `json:"recruiters"`
}

// CompaniesPage is one page of the list: the companies on it, how many companies match the search (the pager's total),
// and the overview of all of them.
type CompaniesPage struct {
	Companies []company      `json:"companies"`
	Total     int            `json:"total"`
	Summary   CompanySummary `json:"summary"`
}

// likePattern turns typed text into a LIKE pattern that matches it literally anywhere: the text is trimmed and lower-cased
// (SQLite folds ASCII letters only, like the Jobs search) and its `\`, `%` and `_` are escaped, so a `%` or `_` the person
// typed is a character, not a wildcard. "" means no search.
func likePattern(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return ""
	}
	q = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	return "%" + q + "%"
}

// ClampCompaniesLimit applies the range to a limit that was given.
func ClampCompaniesLimit(limit int) int {
	return min(max(limit, MinCompaniesLimit), MaxCompaniesLimit)
}

// ListCompaniesPage lists the companies whose name or description contains query (all of them for an empty query),
// oldest first with the id as a tie-break (so a page never repeats or skips a company created in the same second). limit
// <= 0 means no limit, which is what the other pages' dropdowns and the MCP tool use; a positive limit is clamped to the
// range and offset (negative counts as 0) says how many matching companies to skip. Total counts every match, and the
// summary counts every company, neither depends on limit or offset.
func ListCompaniesPage(query string, limit, offset int) (CompaniesPage, error) {
	pattern := likePattern(query)
	where := ` WHERE (? = '' OR LOWER(c.name) LIKE ? ESCAPE '\' OR LOWER(c.description) LIKE ? ESCAPE '\')`
	args := []any{pattern, pattern, pattern}

	var page CompaniesPage
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM companies c`+where, args...).Scan(&page.Total); err != nil {
		return CompaniesPage{}, err
	}

	q := `SELECT c.id, c.name, c.description, c.created_at, c.updated_at,
		        (SELECT COUNT(*) FROM jobs WHERE jobs.company_id = c.id),
		        (SELECT COUNT(*) FROM recruiters WHERE recruiters.company_id = c.id)
		 FROM companies c` + where + ` ORDER BY c.created_at ASC, c.id ASC`
	if limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, ClampCompaniesLimit(limit), max(offset, 0))
	}
	rows, err := db.JobsDB.Query(q, args...)
	if err != nil {
		return CompaniesPage{}, err
	}
	page.Companies = []company{}
	for rows.Next() {
		var c company
		var updatedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.CreatedAt, &updatedAt, &c.JobCount, &c.RecruiterCount); err != nil {
			rows.Close()
			return CompaniesPage{}, err
		}
		c.UpdatedAt = updatedAt.String
		page.Companies = append(page.Companies, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CompaniesPage{}, err
	}
	rows.Close() // jobs.db has a single connection: close before the next query

	err = db.JobsDB.QueryRow(
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN x.jc > 0 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN x.rc > 0 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(x.jc), 0),
		        COALESCE(SUM(x.rc), 0)
		 FROM (SELECT (SELECT COUNT(*) FROM jobs WHERE jobs.company_id = c.id) AS jc,
		              (SELECT COUNT(*) FROM recruiters WHERE recruiters.company_id = c.id) AS rc
		       FROM companies c) x`,
	).Scan(&page.Summary.Companies, &page.Summary.WithJobs, &page.Summary.WithRecruiters, &page.Summary.Jobs, &page.Summary.Recruiters)
	if err != nil {
		return CompaniesPage{}, err
	}
	return page, nil
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
//
// When a company was really removed, "career.company.deleted" is announced with its id and name (step 139); the
// name is read first, because the row is gone afterwards. A delete that removes nothing announces nothing.
func DeleteCompany(id string) error {
	var name string
	// Best effort: an unknown id or a failed read leaves the name empty and never stops the delete.
	_ = db.JobsDB.QueryRow(`SELECT name FROM companies WHERE id = ?`, id).Scan(&name)
	res, err := db.JobsDB.Exec(`DELETE FROM companies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		PublishCompanyDeleted(id, name)
	}
	return nil
}

// DeleteCompanyIfUnused deletes a company only if no job is linked to it and no recruiter belongs to it, and says whether
// it did (step 139, the CompanyDelete consumer). Both conditions are checked by the one DELETE statement itself, so a job
// linked or a recruiter added a moment before wins and the company stays (a check followed by a delete could not promise
// that). An unknown id, or a company that is still in use, is no error: deleted is false. When a row was removed,
// "career.company.deleted" is announced, as for any other delete of a company.
func DeleteCompanyIfUnused(id string) (deleted bool, err error) {
	var name string
	// Best effort, as in DeleteCompany: only the announcement's name depends on it.
	_ = db.JobsDB.QueryRow(`SELECT name FROM companies WHERE id = ?`, id).Scan(&name)
	res, err := db.JobsDB.Exec(
		`DELETE FROM companies WHERE id = ?
		   AND NOT EXISTS (SELECT 1 FROM jobs WHERE company_id = companies.id)
		   AND NOT EXISTS (SELECT 1 FROM recruiters WHERE company_id = companies.id)`, id)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	PublishCompanyDeleted(id, name)
	return true, nil
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
