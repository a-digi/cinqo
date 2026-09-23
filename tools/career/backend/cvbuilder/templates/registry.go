package templates

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

// TemplateMeta describes one selectable design — returned as-is by
// ListTemplates for the frontend's own template picker (step 4).
type TemplateMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// The 15 ats-* designs below are all deliberately single-column, plain-
// text layouts — no tables, no CSS `columns`, no floats, no absolute/
// fixed positioning, no profile photo — since any of those can reorder
// or drop content in an ATS's own PDF text-layer extraction relative to
// what's visually shown. They differ from each other only in
// typography, accent color, and (for ats-skills-first) section order —
// the safe axes of variation once the layout itself is constrained to
// "real, linear, single-column text." See
// plan/ai/career/cv-builder/step-05-ats-templates.md.
var metas = []TemplateMeta{
	{ID: "modern-mono", Name: "Modern Mono", Description: "Single accent color, minimal, one column."},
	{ID: "modern-sidebar", Name: "Modern Sidebar", Description: "Two-column layout with a colored sidebar for contact info and skills."},
	{ID: "classic", Name: "Classic", Description: "Traditional serif layout, closest to a conventional printed resume."},
	{ID: "ats-clean", Name: "ATS Clean", Description: "Single-column sans-serif with minimal navy accents — built for maximum ATS parsing reliability."},
	{ID: "ats-serif-classic", Name: "ATS Serif Classic", Description: "Traditional serif resume in black text only, no color at all — the most conservative ATS-safe option."},
	{ID: "ats-minimal-gray", Name: "ATS Minimal Gray", Description: "Uppercase section headers, generous whitespace, no rules or color accents anywhere."},
	{ID: "ats-executive", Name: "ATS Executive", Description: "Serif headings with a single rule under the name, charcoal accents, formal tone."},
	{ID: "ats-compact", Name: "ATS Compact", Description: "Smaller type and tighter spacing to fit more content on a single page."},
	{ID: "ats-modern-sans", Name: "ATS Modern Sans", Description: "Clean sans-serif body with thin teal underlines on section headings."},
	{ID: "ats-times", Name: "ATS Times", Description: "Pure Times New Roman with maroon section headings — the most traditional printed-resume look."},
	{ID: "ats-verdana", Name: "ATS Verdana", Description: "Verdana-based body text, slate-blue accents, wide letter-spacing on headings."},
	{ID: "ats-tahoma", Name: "ATS Tahoma", Description: "Tahoma-based body text, burgundy accents, compact single-line contact header."},
	{ID: "ats-calibri", Name: "ATS Calibri Style", Description: "Calibri-style sans-serif with warm brown accents and a soft, rounded feel."},
	{ID: "ats-garamond", Name: "ATS Garamond Style", Description: "Garamond-style serif with deep green accents — understated and elegant."},
	{ID: "ats-bold-headers", Name: "ATS Bold Headers", Description: "Grayscale only — bold uppercase headings with a bottom rule, zero color risk."},
	{ID: "ats-two-line-header", Name: "ATS Two-Line Header", Description: "Name and contact details centered on their own lines, gray accents."},
	{ID: "ats-skills-first", Name: "ATS Skills First", Description: "Leads with Skills before Experience — suited to skill-heavy or career-change resumes."},
	{ID: "ats-academic", Name: "ATS Academic", Description: "Centered header, small-caps-style section titles, black-only styling for academic or formal CVs."},
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
var templateFS embed.FS

func isKnownTemplate(id string) bool {
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
	if !isKnownTemplate(templateID) {
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
