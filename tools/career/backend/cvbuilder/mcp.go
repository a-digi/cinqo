// mcp.go exposes the CV Builder feature (templates.Render + its 26
// fixed designs) to the AI, so an AI-driven CV generation (e.g. Jobs
// page's own "Generate CV PDF", jobs/cv_pdf.go's save_cv_document) can
// render a real, professionally-designed CV from structured data
// instead of hand-authoring raw XHTML/CSS itself — the AI's own job
// becomes picking a template and filling in/tailoring CvData, not
// designing a page. Nothing here talks to Media or pdf_tools directly
// (render_cv_document is a pure, in-process render — no HTTP, no auth
// needed, per api/src/conversation/chat.go's own "MCP tool-to-tool
// calls need no auth" model); the AI still calls pdf_tools' own
// generate_pdf itself with the returned XHTML, exactly as before. See
// plan/ai/career/cv-builder/step-08-ai-generated-cv-documents.md.
package cvbuilder

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
	"career-tool-backend/persona"
)

type listCvTemplatesArgs struct{}

// RegisterListCvTemplates adds list_cv_templates — the AI's own way to
// discover which designs exist before calling render_cv_document.
func RegisterListCvTemplates(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_cv_templates",
		Description: "List every available CV template design (id, name, description, style category, and which professions " +
			"it's best suited for). Pick one of these ids for render_cv_document — choose based on the persona/job at hand, " +
			"e.g. an 'ATS-Friendly' category template for a large-company application likely using automated screening, or a " +
			"'Modern'/'Creative' one for a design-facing role.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args listCvTemplatesArgs) (*mcp.CallToolResult, any, error) {
		return db.JSONResult(map[string]any{"templates": templates.ListTemplates()})
	})
}

type getPersonaCvDefaultsArgs struct {
	PersonaID string `json:"personaId" jsonschema:"the persona's own id, from list_personas"`
}

// RegisterGetPersonaCvDefaults adds get_persona_cv_defaults — the same
// data source the human-facing CV Builder wizard itself starts from
// (AssembleCvData), given here as a ready-made starting point so the
// AI never has to hand-translate get_persona_details' own shape into
// render_cv_document's own data argument itself. Always fetch this
// FIRST, then edit the result (see render_cv_document's own
// description for what "tailoring" means) before rendering — never
// invent content that isn't already present here.
func RegisterGetPersonaCvDefaults(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_persona_cv_defaults",
		Description: "Get a persona's own current data (name, headline, summary, location, external links, skills, " +
			"experience) already shaped as render_cv_document's own 'data' argument — the exact starting point the human-facing " +
			"CV Builder wizard itself uses. Call this before render_cv_document, then tailor the result (rewrite the headline/" +
			"summary, reorder or trim skills/experience) to fit a specific job before rendering — never invent a skill, " +
			"employer, title, or dates that aren't already present here.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getPersonaCvDefaultsArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		data, err := AssembleCvData(args.PersonaID)
		if err != nil {
			if errors.Is(err, persona.ErrUnknownPersona) {
				return db.ErrResult(fmt.Sprintf("unknown persona id %q", args.PersonaID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to assemble persona defaults: %v", err)), nil, nil
		}
		return db.JSONResult(data)
	})
}

// CvDocumentDataArgs mirrors templates.CvData field-for-field (its own
// list fields reuse templates.ExternalLink/ExperienceEntry directly, so
// building a real templates.CvData from this is a plain field copy,
// never a conversion) — a separate, EXPORTED type purely so every field
// can carry its own jsonschema description; templates.CvData itself has
// none, it's the internal render/storage shape, not an AI-facing
// schema. Exported (not just used internally by render_cv_document's
// own args) because jobs/cv_pdf.go's save_cv_document reuses this exact
// shape for its own 'data' argument — the AI passes the SAME data
// object to both calls, so both tools describing it identically, via
// one shared type, avoids the two ever silently drifting apart.
type CvDocumentDataArgs struct {
	FullName      string                      `json:"fullName" jsonschema:"REQUIRED — the person's full name; every other field here is optional"`
	Headline      string                      `json:"headline,omitempty" jsonschema:"a short professional headline/title, e.g. 'Senior Backend Engineer' — when tailoring for a specific job, rewrite this to match what that job is looking for"`
	Summary       string                      `json:"summary,omitempty" jsonschema:"a 2-4 sentence professional summary — when tailoring for a specific job, rewrite this to foreground exactly the experience/skills that job's own description asks for"`
	Location      string                      `json:"location,omitempty" jsonschema:"city/region, e.g. 'Berlin, Germany'"`
	ExternalLinks []templates.ExternalLink    `json:"externalLinks,omitempty" jsonschema:"list of {platform, url} pairs, e.g. LinkedIn/GitHub/portfolio links"`
	Skills        []string                    `json:"skills,omitempty" jsonschema:"list of skill strings — when tailoring for a specific job, include (and lead with) the ones that job's own description actually asks for"`
	Experience    []templates.ExperienceEntry `json:"experience,omitempty" jsonschema:"list of past roles ({company, title, startDate, endDate, description}) — when tailoring for a specific job, reorder so the most relevant roles lead, and omit ones that don't help; NEVER invent an employer, title, or dates not already present in the persona's own real data"`
}

// ToCvData converts to the internal render/storage shape — a plain
// field copy, never a real conversion (see this type's own doc
// comment).
func (a CvDocumentDataArgs) ToCvData() templates.CvData {
	return templates.CvData{
		FullName:      a.FullName,
		Headline:      a.Headline,
		Summary:       a.Summary,
		Location:      a.Location,
		ExternalLinks: a.ExternalLinks,
		Skills:        a.Skills,
		Experience:    a.Experience,
	}
}

type renderCvDocumentArgs struct {
	PersonaID  string             `json:"personaId" jsonschema:"an existing persona's id, from list_personas — used only to confirm you're generating for a real persona; the CV's own content comes entirely from 'data' below, never re-derived from the persona automatically"`
	TemplateID string             `json:"templateId" jsonschema:"one of the ids returned by list_cv_templates"`
	Data       CvDocumentDataArgs `json:"data" jsonschema:"the CV's own content — call get_persona_cv_defaults first for a real starting point, then tailor it for the job at hand before rendering"`
}

// RegisterRenderCvDocument adds render_cv_document — renders a CV using
// one of this tool's own fixed template designs from structured data,
// so the AI never has to write any HTML/CSS itself. Returns real XHTML
// text, ready to pass directly as generate_pdf's own xhtml argument,
// then call save_cv_document once that returns. No verification step
// in between (there used to be one, calling pdf_to_markdown to check
// for garbled output) — that existed to catch an AI hand-authoring
// malformed markup itself, which can't happen here: every free-text
// field in 'data' goes through Go's html/template, which auto-escapes
// it, so the failure mode that check existed for is structurally
// impossible on this path. Removed per your own instruction, since it
// was adding a real round trip to every single CV generation for a risk
// that no longer exists here. A pure, in-process render — no Media/
// pdf_tools call happens here, and none of this is persisted until
// save_cv_document runs.
func RegisterRenderCvDocument(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "render_cv_document",
		Description: "Render a CV as XHTML using one of this tool's own fixed template designs (see list_cv_templates), from " +
			"structured data you provide — you do NOT write any HTML or CSS yourself, this handles all layout/styling. " +
			"Required: personaId (an existing persona, from list_personas), templateId (from list_cv_templates), and " +
			"data.fullName (every other data field is optional but recommended — call get_persona_cv_defaults first to get a " +
			"real starting point, then tailor it for the job at hand). Every data field is PLAIN TEXT ONLY — never include " +
			"HTML tags or markup of any kind (no <b>, <strong>, <p>, <div>, <br>, etc.) in fullName/headline/summary/location/" +
			"skills/experience; write formatting-free prose, this tool handles all visual styling itself, and a stray tag will " +
			"be rejected. Returns the rendered XHTML as plain text — you MUST pass this exact returned XHTML straight into " +
			"generate_pdf's own xhtml argument, completely UNCHANGED — do not edit, reformat, retype, or write your own XHTML " +
			"for this task under any circumstances; if you find yourself typing an <html> tag by hand here, stop, you're doing " +
			"it wrong. Then call save_cv_document once generate_pdf returns (no verification step needed in between). NOTE: " +
			"this never embeds a profile photo, even if one exists — the photo feature requires a live browser session this " +
			"AI-driven flow doesn't have.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args renderCvDocumentArgs) (*mcp.CallToolResult, any, error) {
		if args.PersonaID == "" {
			return db.ErrResult("personaId is required"), nil, nil
		}
		if args.TemplateID == "" {
			return db.ErrResult("templateId is required"), nil, nil
		}
		if err := requirePersonaExists(args.PersonaID); err != nil {
			if errors.Is(err, persona.ErrUnknownPersona) {
				return db.ErrResult(fmt.Sprintf("unknown persona id %q", args.PersonaID)), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to validate persona: %v", err)), nil, nil
		}
		cvData := args.Data.ToCvData()
		if err := ValidateCvData(cvData); err != nil {
			return db.ErrResult(err.Error()), nil, nil
		}
		xhtml, err := templates.Render(args.TemplateID, cvData, "")
		if err != nil {
			return db.ErrResult(fmt.Sprintf("unknown template id %q — call list_cv_templates for valid ids", args.TemplateID)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: xhtml}}}, nil, nil
	})
}
