package detailpick

import (
	"sort"
	"strings"
)

// ExcludeSuggestion is an element inside a description box that is probably not part of the description.
type ExcludeSuggestion struct {
	Selector string `json:"selector"`
	Kind     string `json:"kind"`
	Chars    int    `json:"chars"`
	Value    string `json:"value"`
	// Share is the part of the box's text this element holds (0..1).
	Share float64 `json:"share"`
	// Risky says the selector is generic or takes a large part of the box: the person should look at the preview before keeping it.
	Risky bool `json:"risky"`
}

var (
	// genericTags are tag selectors that are safe to exclude inside a box (they are page furniture by definition).
	genericTags = map[string]bool{"nav": true, "footer": true, "aside": true, "button": true, "form": true}
	// excludeKinds are the furniture kinds a person usually wants gone.
	excludeKinds = map[string]bool{
		"cookie": true, "consent": true, "share": true, "social": true, "apply": true, "banner": true, "newsletter": true, "alert": true,
		"breadcrumb": true, "related": true, "similar": true, "recommend": true, "ähnlich": true, "teilen": true, "bewerb": true,
		"nav": true, "footer": true, "aside": true, "button": true, "form": true,
	}
)

func isBareTag(sel string) bool { return !strings.ContainsAny(sel, ".#[ >:") }

// SuggestExcludes lists the furniture inside a description box that is worth excluding, biggest first. A selector that is a bare tag
// other than the page-furniture tags, or that holds more than 40% of the box's text, is marked risky.
func SuggestExcludes(box Candidate) []ExcludeSuggestion {
	var out []ExcludeSuggestion
	seen := map[string]bool{}
	for _, f := range box.Furniture {
		sel := strings.TrimSpace(f.Selector)
		kind := strings.ToLower(f.Kind)
		if sel == "" || seen[sel] || !excludeKinds[kind] {
			continue
		}
		seen[sel] = true
		share := 0.0
		if box.Chars > 0 {
			share = float64(f.Chars) / float64(box.Chars)
			if share > 1 {
				share = 1
			}
		}
		risky := share > 0.4 || (isBareTag(sel) && !genericTags[strings.ToLower(sel)])
		out = append(out, ExcludeSuggestion{Selector: sel, Kind: kind, Chars: f.Chars, Value: f.Value, Share: share, Risky: risky})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Chars > out[j].Chars })
	return out
}

// cutSignals maps what the Browser noticed in a box's text to a text a "cut after" could stop at. "Apply now" is never one: it can stand
// in the middle of a posting, and cutting there would drop the rest.
var cutSignals = map[string]string{
	"similar jobs": "Similar jobs",
	"ähnliche":     "Ähnliche",
	"share this":   "Share this",
	"teilen":       "Teilen",
}

// SuggestCutAfter proposes texts the description could stop at, from the signals found in the box's text.
func SuggestCutAfter(box Candidate) []string {
	var out []string
	for _, s := range box.LineSignals {
		if t, ok := cutSignals[strings.ToLower(strings.TrimSpace(s))]; ok {
			out = append(out, t)
		}
	}
	return out
}

// StructuredSuggestion is a value the page states in structured data (not a selector yet: the structured source is Phase B), shown next
// to the candidates so the person sees what the page itself says.
type StructuredSuggestion struct {
	Field  Field  `json:"field"`
	Value  string `json:"value"`
	Source string `json:"source"` // jsonld | meta
	// Path is what a field reads to get this value (the JSON-LD path, or the meta tag's name).
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// StructuredSuggestions lists what the page's JobPosting JSON-LD and meta tags say.
func StructuredSuggestions(p Page) []StructuredSuggestion {
	// Never nil: the screen reads the JSON list (a page without structured data must give [], not null).
	out := []StructuredSuggestion{}
	if len(p.Structured.JSONLD) > 0 {
		j := p.Structured.JSONLD[0]
		add := func(f Field, v, path, reason string) {
			if strings.TrimSpace(v) != "" {
				out = append(out, StructuredSuggestion{Field: f, Value: v, Source: "jsonld", Path: path, Reason: reason})
			}
		}
		add(FieldTitle, j.Title, "title", "JobPosting title")
		add(FieldCompany, j.Company, "hiringOrganization", "JobPosting hiringOrganization")
		add(FieldLocation, j.Location, "jobLocation", "JobPosting jobLocation")
		add(FieldPostedAt, j.DatePosted, "datePosted", "JobPosting datePosted")
		if j.DescriptionChars > 0 {
			out = append(out, StructuredSuggestion{Field: FieldDescription, Value: j.DescriptionPrev, Source: "jsonld", Path: "description", Reason: "JobPosting description"})
		}
	}
	if m := p.Structured.Meta; strings.TrimSpace(m.OgTitle) != "" {
		out = append(out, StructuredSuggestion{Field: FieldTitle, Value: m.OgTitle, Source: "meta", Path: "og:title", Reason: "og:title"})
	}
	return out
}
