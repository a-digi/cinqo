// Package detailpick is the pure core of Career's "pick the data of a job page" screen (plan 186, step 4): it turns what the Browser's
// fetch-detail-candidates route found on a sample page into ranked suggestions, turns a person's choices into the stored job-detail
// instructions, and checks what the real extraction returned for sample jobs.
//
// It does no I/O. Nothing here fetches, stores or logs anything, and the page text it is given is untrusted: it is bounded, never
// executed, never put into YAML by concatenation (the instructions are marshalled), and a flag never blocks a save except an empty
// description.
//
// The Browser returns raw candidates with features; the ranking lives here (decision D6) so it can be tested with real pages and fuzzed.
package detailpick

// Field is one thing a job page can tell about a job.
type Field string

const (
	FieldTitle       Field = "title"
	FieldCompany     Field = "company"
	FieldLocation    Field = "location"
	FieldPostedAt    Field = "postedAt"
	FieldDescription Field = "description"
)

// Fields is every field, in the order the screen shows them.
var Fields = []Field{FieldTitle, FieldCompany, FieldLocation, FieldPostedAt, FieldDescription}

// Furniture is an element inside a description box that looks like page furniture (a share bar, an apply button, a cookie notice).
type Furniture struct {
	Selector string `json:"selector"`
	Unique   bool   `json:"unique"`
	Kind     string `json:"kind"`
	Tag      string `json:"tag"`
	Chars    int    `json:"chars"`
	Value    string `json:"value"`
}

// Candidate is one place on the page a field could be read from (the Browser's detail candidate).
type Candidate struct {
	Selector  string `json:"selector"`
	Unique    bool   `json:"unique"`
	Matches   int    `json:"matches"`
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Chars     int    `json:"chars"`
	Tag       string `json:"tag"`
	Reason    string `json:"reason"`
	InMain    bool   `json:"inMain"`
	// PlacedIn names the page furniture the element sits in (nav, header, footer, aside, consent); "" when none.
	PlacedIn string `json:"placedIn"`
	Order    int    `json:"order"`
	Source   string `json:"source"`
	// Description candidates only.
	ShareOfPage float64     `json:"shareOfPage,omitempty"`
	ByAttr      bool        `json:"byAttr,omitempty"`
	Headings    int         `json:"headings,omitempty"`
	ListItems   int         `json:"listItems,omitempty"`
	Paragraphs  int         `json:"paragraphs,omitempty"`
	Furniture   []Furniture `json:"furniture,omitempty"`
	LineSignals []string    `json:"lineSignals,omitempty"`
}

// Candidates are the candidates of every field.
type Candidates struct {
	Title       []Candidate `json:"title"`
	Company     []Candidate `json:"company"`
	Location    []Candidate `json:"location"`
	PostedAt    []Candidate `json:"postedAt"`
	Description []Candidate `json:"description"`
}

// For returns the candidates of one field.
func (c Candidates) For(f Field) []Candidate {
	switch f {
	case FieldTitle:
		return c.Title
	case FieldCompany:
		return c.Company
	case FieldLocation:
		return c.Location
	case FieldPostedAt:
		return c.PostedAt
	case FieldDescription:
		return c.Description
	}
	return nil
}

// JobPosting is what a page's JobPosting JSON-LD states about the job.
type JobPosting struct {
	Type             string `json:"type"`
	Title            string `json:"title"`
	Company          string `json:"company"`
	Location         string `json:"location"`
	DatePosted       string `json:"datePosted"`
	DescriptionChars int    `json:"descriptionChars"`
	DescriptionPrev  string `json:"descriptionPreview"`
}

// Meta are the page's meta tags that name the job.
type Meta struct {
	OgTitle      string `json:"ogTitle"`
	OgSiteName   string `json:"ogSiteName"`
	OgDescrip    string `json:"ogDescription"`
	TwitterTitle string `json:"twitterTitle"`
}

// Structured is the structured data a page states about the job.
type Structured struct {
	JSONLD    []JobPosting `json:"jsonld"`
	Meta      Meta         `json:"meta"`
	Microdata int          `json:"microdata"`
}

// Element is a text element of the page, for picking something the heuristics missed.
type Element struct {
	Selector string `json:"selector"`
	Unique   bool   `json:"unique"`
	Tag      string `json:"tag"`
	Text     string `json:"text"`
	Chars    int    `json:"chars"`
}

// ViewElement is one element of the page copy the person clicks in (plan 249): its number (data-cinqo-e in the copy), the selector the Browser verified on
// the live page, its tag, how much text it holds and the start of it, and the attribute that holds a machine date when it has one. Carried to the screen as it
// came; the ranking never reads it.
type ViewElement struct {
	N         int    `json:"n"`
	Selector  string `json:"selector"`
	Unique    bool   `json:"unique"`
	Tag       string `json:"tag"`
	Chars     int    `json:"chars"`
	Text      string `json:"text"`
	Attribute string `json:"attribute,omitempty"`
}

// SnapshotInfo is what the Browser tool reports about the page copy it made (plan 247).
type SnapshotInfo struct {
	ID        string `json:"id"`
	Bytes     int    `json:"bytes"`
	CreatedAt string `json:"createdAt"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Page is the Browser's fetch-detail-candidates answer.
type Page struct {
	FinalURL      string     `json:"finalUrl"`
	Title         string     `json:"title"`
	Candidates    Candidates `json:"candidates"`
	Structured    Structured `json:"structured"`
	Elements      []Element  `json:"elements"`
	PageTextChars int        `json:"pageTextChars"`
	Truncated     bool       `json:"truncated"`
	// ViewElements, Snapshot and SnapshotError (plan 249): only when a page copy was asked for.
	ViewElements  []ViewElement `json:"viewElements,omitempty"`
	Snapshot      *SnapshotInfo `json:"snapshot,omitempty"`
	SnapshotError string        `json:"snapshotError,omitempty"`
}
