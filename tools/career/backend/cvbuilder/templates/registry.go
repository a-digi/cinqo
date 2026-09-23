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

var metas = []TemplateMeta{
	{ID: "modern-mono", Name: "Modern Mono", Description: "Single accent color, minimal, one column."},
	{ID: "modern-sidebar", Name: "Modern Sidebar", Description: "Two-column layout with a colored sidebar for contact info and skills."},
	{ID: "classic", Name: "Classic", Description: "Traditional serif layout, closest to a conventional printed resume."},
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
