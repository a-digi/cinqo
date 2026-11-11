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
	// The four sections below (plan 251) come from the profile, are all optional and are left out of the JSON when empty: a CV stored before they existed has
	// none of these keys and renders exactly as it did.
	Education []EducationEntry `json:"education,omitempty"`
	Courses   []CourseEntry    `json:"courses,omitempty"`
	Languages []LanguageEntry  `json:"languages,omitempty"`
	Hobbies   []HobbyEntry     `json:"hobbies,omitempty"`
}

// EducationEntry is one school, degree or study of the person. The dates are free text like ExperienceEntry's; an empty EndDate means still studying.
type EducationEntry struct {
	Institution  string `json:"institution"`
	Degree       string `json:"degree"`
	FieldOfStudy string `json:"fieldOfStudy"`
	StartDate    string `json:"startDate"`
	EndDate      string `json:"endDate"` // "" means "present"
	Description  string `json:"description"`
}

// CourseEntry is one course or certificate; only what a CV line shows (the profile's description and credential address are not carried over).
type CourseEntry struct {
	Title         string `json:"title"`
	Provider      string `json:"provider"`
	CompletedDate string `json:"completedDate"`
}

// LanguageEntry is one language with its level as it is shown ("Native", "C1", "B2"...); the level is free text, so the person can write "fluent".
type LanguageEntry struct {
	Language string `json:"language"`
	Level    string `json:"level"`
}

// HobbyEntry is one hobby or interest, by name.
type HobbyEntry struct {
	Name string `json:"name"`
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
