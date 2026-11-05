// Package templates holds the CV Builder's own JSON-schema-shaped
// input contract (CvData) and the fixed, code-defined set of HTML/CSS
// designs that render it into XHTML pdf_tools can turn into a PDF. See
// plan/ai/career/cv-builder/step-02-templates-and-html-generator.md.
//
// CvData lives in this package, not cvbuilder's own top-level package
// (as step 2's own plan doc first sketched it) — Render below needs
// the type, and cvbuilder/assemble.go (step 3) needs to both build a
// CvData and call Render, so CvData has to live wherever Render does
// to avoid an import cycle.
package templates

// CvData is the one stable shape every template renders against —
// adding a new template later never touches step 3's own data
// assembly code, and every template stays visually different but
// data-identical. Assembling a real CvData from a persona's own DB
// rows is cvbuilder/assemble.go's job (step 3), not this package's.
type CvData struct {
	FullName      string            `json:"fullName"`
	Headline      string            `json:"headline"` // persona_details.headline
	Summary       string            `json:"summary"`  // persona_details.summary
	Location      string            `json:"location"` // persona_details.location
	ExternalLinks []ExternalLink    `json:"externalLinks"`
	Skills        []string          `json:"skills"`
	Experience    []ExperienceEntry `json:"experience"`
}

type ExternalLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

type ExperienceEntry struct {
	Company     string `json:"company"`
	Title       string `json:"title"`
	StartDate   string `json:"startDate"`
	EndDate     string `json:"endDate"` // "" means "present"
	Description string `json:"description"`
}
