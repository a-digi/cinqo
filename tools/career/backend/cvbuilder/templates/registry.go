package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

// TemplateMeta describes one selectable design — returned as-is by
// ListTemplates for the frontend's own template picker.
//
// Category is one fixed style bucket (Classic/Modern/ATS-Friendly/
// Creative/Executive/Minimalist) — the frontend's own single-select
// style filter. BestFor is a list of profession tags drawn from a
// small SHARED vocabulary (see professionTags below) — the frontend's
// own multi-select profession filter groups by exact string match, so
// every entry below reuses those exact strings rather than inventing
// its own free-text variant per template.
type TemplateMeta struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	BestFor     []string `json:"bestFor"`
}

// professionTags is the fixed, shared vocabulary every metas[].BestFor
// entry below draws from — kept as one list here (not enforced at
// compile time, just documented) so the frontend's own profession
// filter groups templates correctly instead of silently missing one
// because of a typo'd near-duplicate tag.
//
//	Software & IT, Marketing & Sales, Finance & Accounting, Healthcare,
//	Design & Creative, Executive & Management, Education & Academia,
//	Customer Service, Legal, Engineering, Entry-Level & Internships,
//	Human Resources, Project Management
//
// The 15 ats-* designs are all deliberately single-column, plain-text
// layouts — no tables, no CSS `columns`, no floats, no absolute/fixed
// positioning, no profile photo — since any of those can reorder or
// drop content in an ATS's own PDF text-layer extraction relative to
// what's visually shown. They differ from each other only in
// typography, accent color, and (for ats-skills-first) section order.
// The 8 templates after them (modern-banner onward) trade that
// constraint for real visual flair — color blocks, accent rules, a
// timeline marker — since they're explicitly categorized Modern/
// Creative/Executive/Minimalist, not ATS-Friendly; they still stay
// single-column/normal-flow throughout (no position:fixed, no tables),
// since modern-sidebar already proved that kind of layout risks real
// pagination bugs for a purely decorative gain. See
// plan/ai/career/cv-builder/step-05-ats-templates.md and
// plan/ai/career/cv-builder/step-06-template-gallery-and-filters.md.
var metas = []TemplateMeta{
	{ID: "modern-mono", Name: "Modern Mono", Description: "Single accent color, minimal, one column.",
		Category: "Modern", BestFor: []string{"Software & IT", "Marketing & Sales", "Design & Creative"}},
	{ID: "modern-sidebar", Name: "Modern Sidebar", Description: "Two-column layout with a colored sidebar for contact info and skills.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Design & Creative", "Project Management"}},
	{ID: "classic", Name: "Classic", Description: "Traditional serif layout, closest to a conventional printed resume.",
		Category: "Classic", BestFor: []string{"Finance & Accounting", "Legal", "Education & Academia"}},
	{ID: "ats-clean", Name: "ATS Clean", Description: "Single-column sans-serif with minimal navy accents — built for maximum ATS parsing reliability.",
		Category: "ATS-Friendly", BestFor: []string{"Software & IT", "Engineering", "Customer Service"}},
	{ID: "ats-serif-classic", Name: "ATS Serif Classic", Description: "Traditional serif resume in black text only, no color at all — the most conservative ATS-safe option.",
		Category: "ATS-Friendly", BestFor: []string{"Legal", "Finance & Accounting", "Education & Academia"}},
	{ID: "ats-minimal-gray", Name: "ATS Minimal Gray", Description: "Uppercase section headers, generous whitespace, no rules or color accents anywhere.",
		Category: "ATS-Friendly", BestFor: []string{"Human Resources", "Customer Service", "Entry-Level & Internships"}},
	{ID: "ats-executive", Name: "ATS Executive", Description: "Serif headings with a single rule under the name, charcoal accents, formal tone.",
		Category: "ATS-Friendly", BestFor: []string{"Executive & Management", "Finance & Accounting"}},
	{ID: "ats-compact", Name: "ATS Compact", Description: "Smaller type and tighter spacing to fit more content on a single page.",
		Category: "ATS-Friendly", BestFor: []string{"Engineering", "Software & IT"}},
	{ID: "ats-modern-sans", Name: "ATS Modern Sans", Description: "Clean sans-serif body with thin teal underlines on section headings.",
		Category: "ATS-Friendly", BestFor: []string{"Software & IT", "Project Management"}},
	{ID: "ats-times", Name: "ATS Times", Description: "Pure Times New Roman with maroon section headings — the most traditional printed-resume look.",
		Category: "ATS-Friendly", BestFor: []string{"Legal", "Education & Academia"}},
	{ID: "ats-verdana", Name: "ATS Verdana", Description: "Verdana-based body text, slate-blue accents, wide letter-spacing on headings.",
		Category: "ATS-Friendly", BestFor: []string{"Human Resources", "Customer Service"}},
	{ID: "ats-tahoma", Name: "ATS Tahoma", Description: "Tahoma-based body text, burgundy accents, compact single-line contact header.",
		Category: "ATS-Friendly", BestFor: []string{"Finance & Accounting", "Project Management"}},
	{ID: "ats-calibri", Name: "ATS Calibri Style", Description: "Calibri-style sans-serif with warm brown accents and a soft, rounded feel.",
		Category: "ATS-Friendly", BestFor: []string{"Marketing & Sales", "Human Resources"}},
	{ID: "ats-garamond", Name: "ATS Garamond Style", Description: "Garamond-style serif with deep green accents — understated and elegant.",
		Category: "ATS-Friendly", BestFor: []string{"Education & Academia", "Legal"}},
	{ID: "ats-bold-headers", Name: "ATS Bold Headers", Description: "Grayscale only — bold uppercase headings with a bottom rule, zero color risk.",
		Category: "ATS-Friendly", BestFor: []string{"Engineering", "Software & IT"}},
	{ID: "ats-two-line-header", Name: "ATS Two-Line Header", Description: "Name and contact details centered on their own lines, gray accents.",
		Category: "ATS-Friendly", BestFor: []string{"Customer Service", "Entry-Level & Internships"}},
	{ID: "ats-skills-first", Name: "ATS Skills First", Description: "Leads with Skills before Experience — suited to skill-heavy or career-change resumes.",
		Category: "ATS-Friendly", BestFor: []string{"Software & IT", "Entry-Level & Internships", "Engineering"}},
	{ID: "ats-academic", Name: "ATS Academic", Description: "Centered header, small-caps-style section titles, black-only styling for academic or formal CVs.",
		Category: "ATS-Friendly", BestFor: []string{"Education & Academia"}},
	{ID: "modern-banner", Name: "Modern Banner", Description: "A bold colored header banner behind your name and contact details.",
		Category: "Modern", BestFor: []string{"Marketing & Sales", "Design & Creative"}},
	{ID: "modern-timeline", Name: "Modern Timeline", Description: "Experience entries connected by a vertical timeline line and markers.",
		Category: "Modern", BestFor: []string{"Software & IT", "Project Management", "Engineering"}},
	{ID: "creative-accent", Name: "Creative Accent", Description: "Playful purple accents, italic headline, rounded skill pills.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "creative-bold", Name: "Creative Bold", Description: "Oversized name, a bold color block behind your headline, bold section rules.",
		Category: "Creative", BestFor: []string{"Design & Creative", "Marketing & Sales"}},
	{ID: "executive-elegant", Name: "Executive Elegant", Description: "Centered, refined serif layout with a rule above and below the contact line.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Finance & Accounting", "Legal"}},
	{ID: "executive-serif-bold", Name: "Executive Serif Bold", Description: "Bold serif headings with a double rule under your name — formal and assertive.",
		Category: "Executive", BestFor: []string{"Executive & Management", "Legal"}},
	{ID: "minimalist-lines", Name: "Minimalist Lines", Description: "Hairline rules, light section labels, generous whitespace throughout.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Education & Academia", "Engineering"}},
	{ID: "minimalist-airy", Name: "Minimalist Airy", Description: "No rules at all — just spacing and weight contrast carrying the whole design.",
		Category: "Minimalist", BestFor: []string{"Design & Creative", "Software & IT"}},
}

// ListTemplates returns every available design, in a stable, fixed
// order — a plain Go function, not its own HTTP route; step 3's own
// GET .../cv-documents/templates handler just calls this and returns
// the result as JSON.
func ListTemplates() []TemplateMeta {
	return metas
}

// Each id below is both a metas[].ID value AND a real subdirectory
// here (modern-mono/, modern-sidebar/, classic/) — go:embed can't use
// a variable/wildcard tied to that Go slice, so this list is
// necessarily its own small, parallel source of truth; adding a
// template means updating both this directive and metas above (a
// mismatch is caught immediately: Render's own isKnownTemplate check
// or ParseFS's own file-not-found error, never a silent bad render).
// See this directory's own sync-from-frontend.sh — the actual
// template.html/style.css files embedded here are COPIES; the real
// source of truth is
// tools/career/frontend-app/src/Components/CvBuilder/Template/<id>/,
// synced into these folders by that script before every `go build`
// (Go's own //go:embed can only reach files inside this package's own
// directory tree, never across into frontend-app/).
//
//go:embed modern-mono/template.html modern-mono/style.css modern-sidebar/template.html modern-sidebar/style.css classic/template.html classic/style.css
//go:embed ats-clean/template.html ats-clean/style.css ats-serif-classic/template.html ats-serif-classic/style.css
//go:embed ats-minimal-gray/template.html ats-minimal-gray/style.css ats-executive/template.html ats-executive/style.css
//go:embed ats-compact/template.html ats-compact/style.css ats-modern-sans/template.html ats-modern-sans/style.css
//go:embed ats-times/template.html ats-times/style.css ats-verdana/template.html ats-verdana/style.css
//go:embed ats-tahoma/template.html ats-tahoma/style.css ats-calibri/template.html ats-calibri/style.css
//go:embed ats-garamond/template.html ats-garamond/style.css ats-bold-headers/template.html ats-bold-headers/style.css
//go:embed ats-two-line-header/template.html ats-two-line-header/style.css ats-skills-first/template.html ats-skills-first/style.css
//go:embed ats-academic/template.html ats-academic/style.css
//go:embed modern-banner/template.html modern-banner/style.css modern-timeline/template.html modern-timeline/style.css
//go:embed creative-accent/template.html creative-accent/style.css creative-bold/template.html creative-bold/style.css
//go:embed executive-elegant/template.html executive-elegant/style.css executive-serif-bold/template.html executive-serif-bold/style.css
//go:embed minimalist-lines/template.html minimalist-lines/style.css minimalist-airy/template.html minimalist-airy/style.css
var templateFS embed.FS

// IsKnownTemplate is exported so callers that need to validate a
// caller-submitted templateID WITHOUT actually rendering (e.g.
// jobs/cv_pdf.go's own save_cv_document, defensively re-checking a
// value it trusts was already used successfully by an earlier
// render_cv_document call in the same conversation) don't have to
// duplicate this list.
func IsKnownTemplate(id string) bool {
	for _, m := range metas {
		if m.ID == id {
			return true
		}
	}
	return false
}

// xmlDeclaration is prepended in Go code, AFTER html/template has
// already rendered the rest of the document — never written literally
// inside a .html.tmpl file. html/template parses its input as an HTML5
// document tree and re-serializes it; a leading "<?xml ...?>" appears
// before any real tag, so its own tokenizer treats it as a stray text
// node and HTML-escapes it on the way back out ("&lt;?xml...&gt;"),
// which would render as literal garbage text at the top of the PDF
// instead of a real (harmless, Chrome-ignored) XML declaration. The
// existing, already-proven AI-driven flow
// (buildGenerateCvMessage.ts's own XHTML_SKELETON) sends this exact
// declaration as a plain, non-templated string — this constant
// reproduces the same final byte sequence, just assembled at a point
// html/template's own parser never sees it. Confirmed by rendering
// and inspecting actual output before this fix was added.
const xmlDeclaration = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

// styleMarker is the exact placeholder every template.html's own
// <style> element contains — Render below replaces it with that same
// template's own style.css content, spliced in AFTER html/template has
// already rendered the rest of the document (never fed through
// html/template itself), for the identical reason xmlDeclaration above
// is assembled outside the parser too: html/template re-serializes its
// input as an HTML5 tree, and raw CSS routed through {{.CSS}} would
// either get HTML-escaped (mangling it — the exact same class of bug
// xmlDeclaration's own doc comment describes) or need a
// template.CSS-typed field to opt out of that escaping, which is more
// machinery than a plain string.Replace on a fixed marker needs.
const styleMarker = "<!--STYLE-->"

// renderContext embeds the caller-supplied CvData (so every existing
// template field, e.g. {{.FullName}}, keeps resolving exactly as
// before via Go's normal promoted-field lookup) alongside a photo the
// caller resolved separately. PhotoDataURI is deliberately NOT a field
// of CvData itself: CvData round-trips through the browser (the CV
// Builder wizard edits and re-submits it whole, and it's stored
// verbatim as data_json) — if the photo lived there, a client could
// submit an arbitrary string claiming to be the profile's photo, and
// typing it template.URL to make it render (see below) would render
// that value unvalidated. Keeping it a Render-only parameter means the
// caller (cvbuilder/handler.go) is the only place that can ever set it,
// always resolved server-side from the actual persona's own current
// profile photo — never client-supplied. See
// plan/ai/career/profile-image/step-02-cv-template-integration.md.
type renderContext struct {
	CvData
	// template.URL, not string: html/template's own contextual escaper
	// treats a plain string src="{{.}}" through its "safe URL" filter,
	// which only allow-lists http(s)/mailto schemes and would replace a
	// data: URI with "#ZgotmplZ" — the exact same class of escaping
	// gotcha xmlDeclaration/styleMarker above already ran into for XML
	// declarations and raw CSS. template.URL is how html/template's own
	// API says "this value is already a safe URL, don't filter it" —
	// safe here because it's never attacker-controlled (see above), only
	// ever a data: URI this package base64-encoded itself from Media's
	// own resized/re-encoded image bytes.
	PhotoDataURI template.URL
}

// Render renders data into XHTML using templateID's own design.
// html/template (NOT text/template) is what makes this safe to call
// with real user-entered text (company names, descriptions, summaries,
// all free text the caller never controls the shape of) — its
// automatic contextual escaping is the only thing standing between a
// user's own CV text and malformed or injected markup inside the
// XHTML pdf_tools goes on to render. An unknown templateID returns an
// error — never a silent fallback to some default design.
//
// photoDataURI is a ready-to-use "data:<content-type>;base64,<...>"
// string, or "" if this persona's own profile has no photo uploaded —
// resolved by the caller (cvbuilder/handler.go), never by this
// package, which has no HTTP client of its own and no notion of a
// profile.
func Render(templateID string, data CvData, photoDataURI string) (string, error) {
	if !IsKnownTemplate(templateID) {
		return "", fmt.Errorf("unknown template %q", templateID)
	}

	tmpl, err := template.ParseFS(templateFS, templateID+"/template.html")
	if err != nil {
		return "", err
	}

	ctx := renderContext{CvData: data, PhotoDataURI: template.URL(photoDataURI)}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", err
	}

	css, err := templateFS.ReadFile(templateID + "/style.css")
	if err != nil {
		return "", err
	}

	rendered := strings.Replace(buf.String(), styleMarker, string(css), 1)
	return xmlDeclaration + rendered, nil
}
