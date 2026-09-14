// portals.go is the AI-facing (and HTTP-mirrored) surface over
// jobs.db's own portals/portal_links tables — a small, curated list of
// job-board sources the AI should crawl, and the URLs on each one to
// visit. A Portal Link is forced to be mapped to an existing Portal,
// never implicit — the same pattern profile.go/persona.go/
// recruiters.go already use for their own parent references.
//
// Step 19 added crawl_instructions read/write — an AI-authored YAML
// document in the exact same shape browser's own crawl_paginated MCP
// tool expects (fields[] + pagination), so the AI's own next step is
// handing it straight to that tool. career cannot import browser's own
// validation (two fully separate Go modules/binaries, no shared
// package) — validateCrawlInstructions below is a small, independent
// reimplementation of the *shape* check only, never a guarantee the
// instruction actually works against a real page. See
// plan/ai/tools/career/step-18-portals.md and
// plan/ai/tools/career/step-19-portal-crawl-instructions.md.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

var errUnknownPortal = errors.New("unknown portal id")

func requirePortalExists(id string) error {
	var exists int
	err := jobsDB.QueryRow(`SELECT 1 FROM portals WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownPortal
	case err != nil:
		return err
	}
	return nil
}

var errUnknownPortalLink = errors.New("unknown portal link id")

func requirePortalLinkExists(id string) error {
	var exists int
	err := jobsDB.QueryRow(`SELECT 1 FROM portal_links WHERE id = ?`, id).Scan(&exists)
	switch {
	case err == sql.ErrNoRows:
		return errUnknownPortalLink
	case err != nil:
		return err
	}
	return nil
}

var errDuplicatePortalLink = errors.New("this url is already a link on this portal")

type portalLink struct {
	ID                string `json:"id"`
	PortalID          string `json:"portalId"`
	URL               string `json:"url"`
	CrawlInstructions string `json:"crawlInstructions,omitempty"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt,omitempty"`
}

type portal struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Links     []portalLink `json:"links"`
	CreatedAt string       `json:"createdAt"`
	UpdatedAt string       `json:"updatedAt,omitempty"`
}

func createPortal(name string) (string, error) {
	id := uuid.NewString()
	_, err := jobsDB.Exec(
		`INSERT INTO portals (id, name, created_at) VALUES (?, ?, datetime('now'))`,
		id, name,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// listPortals returns every portal with its own links nested — same
// "list is enough, no standalone getter" reasoning listProfiles/
// listCompanies already use; a portal's own realistic link count is
// small.
func listPortals() ([]portal, error) {
	rows, err := jobsDB.Query(
		`SELECT id, name, created_at, updated_at FROM portals ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	portals := []portal{}
	for rows.Next() {
		var p portal
		var updatedAt sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.CreatedAt, &updatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		p.UpdatedAt = updatedAt.String
		p.Links = []portalLink{}
		portals = append(portals, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	linkRows, err := jobsDB.Query(
		`SELECT id, portal_id, url, crawl_instructions, created_at, updated_at
		 FROM portal_links ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer linkRows.Close()
	byPortal := make(map[string][]portalLink, len(portals))
	for linkRows.Next() {
		var l portalLink
		var crawlInstructions, updatedAt sql.NullString
		if err := linkRows.Scan(&l.ID, &l.PortalID, &l.URL, &crawlInstructions, &l.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		l.CrawlInstructions = crawlInstructions.String
		l.UpdatedAt = updatedAt.String
		byPortal[l.PortalID] = append(byPortal[l.PortalID], l)
	}
	if err := linkRows.Err(); err != nil {
		return nil, err
	}
	for i := range portals {
		if links, ok := byPortal[portals[i].ID]; ok {
			portals[i].Links = links
		}
	}
	return portals, nil
}

func updatePortal(id, name string) error {
	if err := requirePortalExists(id); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE portals SET name = ?, updated_at = datetime('now') WHERE id = ?`,
		name, id,
	)
	return err
}

// deletePortal cascades (ON DELETE CASCADE, db.go's own schema) to
// every link it owns; step 20 makes this also un-link (never delete)
// any jobs those links produced. A benign no-op if id is unknown,
// matching every other delete-by-id operation in this tool.
func deletePortal(id string) error {
	_, err := jobsDB.Exec(`DELETE FROM portals WHERE id = ?`, id)
	return err
}

func addPortalLink(portalId, url string) (string, error) {
	if err := requirePortalExists(portalId); err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err := jobsDB.Exec(
		`INSERT INTO portal_links (id, portal_id, url, created_at) VALUES (?, ?, ?, datetime('now'))`,
		id, portalId, url,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return "", errDuplicatePortalLink
		}
		return "", err
	}
	return id, nil
}

// updatePortalLink corrects a link's own url. crawl_instructions
// (step 19) is deliberately not touched by this function — that
// column's own validated read/write path is step 19's own
// contribution, kept separate so this step can't accidentally store
// an unvalidated YAML document through a generic update path.
func updatePortalLink(id, url string) error {
	if err := requirePortalLinkExists(id); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE portal_links SET url = ?, updated_at = datetime('now') WHERE id = ?`,
		url, id,
	)
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return errDuplicatePortalLink
	}
	return err
}

func removePortalLink(id string) error {
	_, err := jobsDB.Exec(`DELETE FROM portal_links WHERE id = ?`, id)
	return err
}

// --- crawl instructions (step 19) ---

// crawlInstructionsField/crawlInstructionsPagination mirror only the
// keys this step actually validates — browser's own crawl_paginated
// tolerates (and this step ignores) any other field (e.g. `attribute`/
// `multiple` on a field entry), since re-validating those isn't this
// step's job; yaml.v3 silently ignores unknown keys on unmarshal by
// default, so a real, fuller instruction still round-trips through
// this shape check untouched.
type crawlInstructionsField struct {
	Label    string `yaml:"label"`
	Selector string `yaml:"selector"`
}

type crawlInstructionsPagination struct {
	NextSelector string `yaml:"nextSelector"`
	MaxPages     int    `yaml:"maxPages"`
}

type crawlInstructionsDoc struct {
	Fields     []crawlInstructionsField     `yaml:"fields"`
	Pagination *crawlInstructionsPagination `yaml:"pagination"`
}

// errInvalidCrawlInstructions wraps a specific, actionable detail
// message (via fmt.Errorf's own %w) — a shape check, not a
// correctness check: this only confirms the required keys are present,
// never that the instruction actually works against a real page (that
// can only be proven by the AI itself later calling browser's own
// crawl_paginated). See
// plan/ai/tools/career/step-19-portal-crawl-instructions.md.
var errInvalidCrawlInstructions = errors.New("invalid crawl instructions")

func validateCrawlInstructions(yamlText string) error {
	var doc crawlInstructionsDoc
	if err := yaml.Unmarshal([]byte(yamlText), &doc); err != nil {
		return fmt.Errorf("%w: not valid YAML: %v", errInvalidCrawlInstructions, err)
	}
	if len(doc.Fields) == 0 {
		return fmt.Errorf("%w: fields must have at least one entry", errInvalidCrawlInstructions)
	}
	for i, f := range doc.Fields {
		if f.Label == "" {
			return fmt.Errorf("%w: fields[%d].label is required", errInvalidCrawlInstructions, i)
		}
		if f.Selector == "" {
			return fmt.Errorf("%w: fields[%d].selector is required", errInvalidCrawlInstructions, i)
		}
	}
	if doc.Pagination == nil {
		return fmt.Errorf("%w: pagination is required", errInvalidCrawlInstructions)
	}
	if doc.Pagination.NextSelector == "" {
		return fmt.Errorf("%w: pagination.nextSelector is required", errInvalidCrawlInstructions)
	}
	if doc.Pagination.MaxPages <= 0 {
		return fmt.Errorf("%w: pagination.maxPages is required", errInvalidCrawlInstructions)
	}
	return nil
}

func getPortalLinkCrawlInstructions(id string) (string, error) {
	if err := requirePortalLinkExists(id); err != nil {
		return "", err
	}
	var crawlInstructions sql.NullString
	err := jobsDB.QueryRow(`SELECT crawl_instructions FROM portal_links WHERE id = ?`, id).Scan(&crawlInstructions)
	if err != nil {
		return "", err
	}
	return crawlInstructions.String, nil
}

// updatePortalLinkCrawlInstructions is the one shared persistent-layer
// function both the HTTP human-editing path (PUT /portal-links'
// own optional crawlInstructions field) and the AI-facing
// set_portal_link_crawl_instructions MCP tool call — validation lives
// here, once, so neither path can bypass it.
func updatePortalLinkCrawlInstructions(id, yamlText string) error {
	if err := requirePortalLinkExists(id); err != nil {
		return err
	}
	if err := validateCrawlInstructions(yamlText); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE portal_links SET crawl_instructions = ?, updated_at = datetime('now') WHERE id = ?`,
		yamlText, id,
	)
	return err
}

// --- MCP registration ---

type createPortalArgs struct {
	Name string `json:"name" jsonschema:"the portal's own name, e.g. the job board's name"`
}

func registerCreatePortal(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_portal",
		Description: "Create a new job portal — a source to crawl for job postings.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args createPortalArgs) (*mcp.CallToolResult, any, error) {
		if args.Name == "" {
			return errResult("name is required"), nil, nil
		}
		id, err := createPortal(args.Name)
		if err != nil {
			return errResult(fmt.Sprintf("failed to create portal: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id, "name": args.Name})
	})
}

type listPortalsArgs struct{}

func registerListPortals(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_portals",
		Description: "List every job portal, including each one's own crawl target links.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listPortalsArgs) (*mcp.CallToolResult, any, error) {
		portals, err := listPortals()
		if err != nil {
			return errResult(fmt.Sprintf("failed to list portals: %v", err)), nil, nil
		}
		return jsonResult(map[string]any{"portals": portals})
	})
}

type updatePortalArgs struct {
	ID   string `json:"id" jsonschema:"the portal's own id, from create_portal or list_portals"`
	Name string `json:"name" jsonschema:"the portal's own new name"`
}

func registerUpdatePortal(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_portal",
		Description: "Rename a job portal. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePortalArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if args.Name == "" {
			return errResult("name is required"), nil, nil
		}
		if err := updatePortal(args.ID, args.Name); err != nil {
			if errors.Is(err, errUnknownPortal) {
				return errResult(fmt.Sprintf("unknown portal id %q", args.ID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update portal: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type deletePortalArgs struct {
	ID string `json:"id" jsonschema:"the portal's own id — deleting it also deletes every link it owns"`
}

func registerDeletePortal(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_portal",
		Description: "Delete a job portal. This permanently deletes every crawl target link it owns.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args deletePortalArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := deletePortal(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to delete portal: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}

type addPortalLinkArgs struct {
	PortalID string `json:"portalId" jsonschema:"the portal to add this crawl target to — must already exist"`
	URL      string `json:"url" jsonschema:"the URL to crawl for job postings"`
}

func registerAddPortalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "add_portal_link",
		Description: "Add a URL to crawl to an existing job portal.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args addPortalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalID == "" {
			return errResult("portalId is required"), nil, nil
		}
		if args.URL == "" {
			return errResult("url is required"), nil, nil
		}
		id, err := addPortalLink(args.PortalID, args.URL)
		if err != nil {
			switch {
			case errors.Is(err, errUnknownPortal):
				return errResult(fmt.Sprintf("unknown portal id %q", args.PortalID)), nil, nil
			case errors.Is(err, errDuplicatePortalLink):
				return errResult("this url is already a link on this portal"), nil, nil
			}
			return errResult(fmt.Sprintf("failed to add portal link: %v", err)), nil, nil
		}
		return jsonResult(map[string]string{"id": id})
	})
}

type updatePortalLinkArgs struct {
	ID  string `json:"id" jsonschema:"the portal link's own id, from add_portal_link or list_portals"`
	URL string `json:"url" jsonschema:"the link's own new URL"`
}

func registerUpdatePortalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_portal_link",
		Description: "Correct a portal link's own URL. Fails if id is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args updatePortalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if args.URL == "" {
			return errResult("url is required"), nil, nil
		}
		if err := updatePortalLink(args.ID, args.URL); err != nil {
			switch {
			case errors.Is(err, errUnknownPortalLink):
				return errResult(fmt.Sprintf("unknown portal link id %q", args.ID)), nil, nil
			case errors.Is(err, errDuplicatePortalLink):
				return errResult("this url is already a link on this portal"), nil, nil
			}
			return errResult(fmt.Sprintf("failed to update portal link: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}

type removePortalLinkArgs struct {
	ID string `json:"id" jsonschema:"the portal link's own id"`
}

func registerRemovePortalLink(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remove_portal_link",
		Description: "Remove a crawl target link from a portal.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args removePortalLinkArgs) (*mcp.CallToolResult, any, error) {
		if args.ID == "" {
			return errResult("id is required"), nil, nil
		}
		if err := removePortalLink(args.ID); err != nil {
			return errResult(fmt.Sprintf("failed to remove portal link: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "deleted"}}}, nil, nil
	})
}

type getPortalLinkCrawlInstructionsArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link's own id, from add_portal_link or list_portals"`
}

func registerGetPortalLinkCrawlInstructions(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_portal_link_crawl_instructions",
		Description: "Read the currently saved YAML crawl instructions for one portal link (null if none saved yet).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getPortalLinkCrawlInstructionsArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalLinkID == "" {
			return errResult("portalLinkId is required"), nil, nil
		}
		instructions, err := getPortalLinkCrawlInstructions(args.PortalLinkID)
		if err != nil {
			if errors.Is(err, errUnknownPortalLink) {
				return errResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to read crawl instructions: %v", err)), nil, nil
		}
		if instructions == "" {
			return jsonResult(map[string]any{"crawlInstructions": nil})
		}
		return jsonResult(map[string]any{"crawlInstructions": instructions})
	})
}

type setPortalLinkCrawlInstructionsArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link's own id, from add_portal_link or list_portals"`
	Instructions string `json:"instructions" jsonschema:"YAML: fields (>=1 entry, each with label+selector) and pagination (nextSelector+maxPages), the exact shape crawl_paginated expects"`
}

func registerSetPortalLinkCrawlInstructions(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "set_portal_link_crawl_instructions",
		Description: "Save the YAML crawl instructions for one portal link — the exact same shape crawl_paginated " +
			"expects (fields: [...] + pagination: {nextSelector, maxPages}), so it can be handed to that tool " +
			"directly later. Example:\nfields:\n  - label: title\n    selector: h1\npagination:\n  " +
			"nextSelector: a.next-page\n  maxPages: 5\nRejected if it doesn't parse or is missing required keys " +
			"— this only checks shape, not that it actually works against the real page.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args setPortalLinkCrawlInstructionsArgs) (*mcp.CallToolResult, any, error) {
		if args.PortalLinkID == "" {
			return errResult("portalLinkId is required"), nil, nil
		}
		if args.Instructions == "" {
			return errResult("instructions is required"), nil, nil
		}
		if err := updatePortalLinkCrawlInstructions(args.PortalLinkID, args.Instructions); err != nil {
			if errors.Is(err, errUnknownPortalLink) {
				return errResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
			}
			// errInvalidCrawlInstructions and any other failure already
			// carry a specific, actionable message of their own.
			return errResult(err.Error()), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "updated"}}}, nil, nil
	})
}
