package detailpick

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// Ranked is a candidate with its score and the reasons behind it (short phrases a screen can show).
type Ranked struct {
	Candidate
	Score float64  `json:"score"`
	Why   []string `json:"why"`
}

type scorer struct {
	score float64
	why   []string
}

func (s *scorer) add(delta float64, why string) {
	s.score += delta
	if why != "" {
		s.why = append(s.why, why)
	}
}

var (
	// An ISO date may be followed by a time ("2026-10-28T09:00:00+00:00"): only a digit next to it makes it something else.
	isoDate    = regexp.MustCompile(`(?:^|\D)(\d{4}-\d{1,2}-\d{1,2})(?:\D|$)`)
	dmyDate    = regexp.MustCompile(`(?:^|\D)\d{1,2}[./-]\d{1,2}[./-]\d{2,4}(?:\D|$)`)
	monthWords = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec|januar|februar|märz|mai|juni|juli|oktober|dezember|janar|shkurt|mars|prill|qershor|korrik|gusht|shtator|tetor|nëntor|dhjetor|january|february|march|april|june|july|august|september|october|november|december)[a-z]*\b`)
)

// LooksLikeDate reports whether s reads like a date: an ISO date, a numeric day-month-year, or a day with a month name.
func LooksLikeDate(s string) bool {
	if isoDate.MatchString(s) || dmyDate.MatchString(s) {
		return true
	}
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
			break
		}
	}
	return hasDigit && monthWords.MatchString(s)
}

// referenceTitles are the texts the page itself uses for the job's title: JSON-LD, og:title, twitter:title and the document title.
func referenceTitles(p Page) []string {
	var out []string
	for _, j := range p.Structured.JSONLD {
		if strings.TrimSpace(j.Title) != "" {
			out = append(out, j.Title)
		}
	}
	for _, s := range []string{p.Structured.Meta.OgTitle, p.Structured.Meta.TwitterTitle, p.Title} {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func common(c Candidate, s *scorer) {
	if strings.TrimSpace(c.Value) == "" {
		s.add(-10, "empty")
	}
	switch c.PlacedIn {
	case "":
	case "consent":
		s.add(-8, "inside a consent or dialog box")
	default:
		s.add(-3, "inside the page's "+c.PlacedIn)
	}
	if c.InMain {
		s.add(1, "in the main part of the page")
	}
	if c.Unique {
		s.add(1, "its selector matches only this element")
	} else {
		s.add(-1, "its selector also matches other elements")
	}
}

func rankTitle(p Page, c Candidate, s *scorer) {
	switch strings.ToLower(c.Tag) {
	case "h1":
		s.add(3, "a first-level heading")
	case "h2":
		s.add(1, "a second-level heading")
	default:
		s.add(1, "named like a title")
	}
	refs := referenceTitles(p)
	best := 0.0
	agreed := false
	for _, r := range refs {
		if agrees(c.Value, r) {
			agreed = true
		}
		if o := overlap(c.Value, r); o > best {
			best = o
		}
	}
	switch {
	case agreed:
		s.add(5, "agrees with the page's own title")
	case best >= 0.6:
		s.add(3, "shares most words with the page's own title")
	case len(refs) > 0 && best < 0.3:
		s.add(-2, "does not look like the page's own title")
	}
	if site := p.Structured.Meta.OgSiteName; site != "" && norm(site) == norm(c.Value) {
		s.add(-5, "is the name of the site")
	}
	if n := runeLen(c.Value); n > 150 {
		s.add(-2, "very long for a title")
	} else if n < 3 {
		s.add(-3, "very short for a title")
	}
}

func rankCompany(p Page, c Candidate, s *scorer) {
	for _, j := range p.Structured.JSONLD {
		if j.Company == "" {
			continue
		}
		if norm(j.Company) == norm(c.Value) {
			s.add(6, "equals the company the page states")
			break
		}
		if agrees(c.Value, j.Company) {
			s.add(3, "contains the company the page states")
			break
		}
	}
	for _, r := range referenceTitles(p) {
		if runeLen(c.Value) > 12 && agrees(c.Value, r) && norm(c.Value) != norm(r) && strings.Contains(norm(c.Value), norm(firstWords(r, 3))) {
			s.add(-4, "also contains the job's title")
			break
		}
	}
	switch n := runeLen(c.Value); {
	case n > 120:
		s.add(-4, "too long for a company name")
	case n > 80:
		s.add(-2, "long for a company name")
	case n < 2:
		s.add(-3, "too short for a company name")
	}
}

func firstWords(s string, n int) string {
	f := strings.Fields(norm(s))
	if len(f) > n {
		f = f[:n]
	}
	return strings.Join(f, " ")
}

func rankLocation(p Page, c Candidate, s *scorer) {
	for _, j := range p.Structured.JSONLD {
		if j.Location != "" && (agrees(c.Value, j.Location) || overlap(c.Value, j.Location) >= 0.5) {
			s.add(4, "agrees with the location the page states")
			break
		}
	}
	switch n := runeLen(c.Value); {
	case n > 100:
		s.add(-2, "long for a location")
	case n < 2:
		s.add(-3, "too short for a location")
	}
}

func rankPostedAt(p Page, c Candidate, s *scorer) {
	if c.Attribute == "datetime" {
		s.add(3, "a machine date (the datetime attribute)")
	}
	if LooksLikeDate(c.Value) {
		s.add(3, "reads like a date")
	} else {
		s.add(-4, "does not read like a date")
	}
	for _, j := range p.Structured.JSONLD {
		if j.DatePosted == "" {
			continue
		}
		if m := isoDate.FindStringSubmatch(j.DatePosted); m != nil && strings.Contains(c.Value, m[1]) {
			s.add(4, "agrees with the date the page states")
			break
		}
	}
	if runeLen(c.Value) > 60 {
		s.add(-2, "long for a date")
	}
}

func rankDescription(p Page, c Candidate, s *scorer) {
	s.add(math.Log10(float64(maxInt(c.Chars, 1))), "")
	if c.ByAttr {
		s.add(1.5, "its id or class names it")
	}
	switch strings.ToLower(c.Tag) {
	case "main", "article", "body":
		s.add(-1, "a whole-page container")
	}
	if c.Headings >= 2 {
		s.add(0.5, "has headings")
	}
	if c.ListItems >= 3 {
		s.add(0.5, "has lists")
	}
	if c.Paragraphs >= 2 {
		s.add(0.3, "has paragraphs")
	}
	switch {
	case c.ShareOfPage > 0.9:
		s.add(-1, "is almost the whole page")
	case c.ShareOfPage >= 0.15:
		s.add(1, "a large part of the page")
	case c.ShareOfPage > 0 && c.ShareOfPage < 0.05:
		s.add(-1.5, "a small part of the page")
	}
	if c.Chars < 300 {
		s.add(-3, "very short for a description")
	}
	if c.Chars > 0 {
		furn := 0
		for _, f := range c.Furniture {
			furn += f.Chars
		}
		switch r := float64(furn) / float64(c.Chars); {
		case r > 0.5:
			s.add(-4, "mostly page furniture")
		case r > 0.3:
			s.add(-2, "a lot of page furniture inside")
		}
	}
	for _, j := range p.Structured.JSONLD {
		if j.DescriptionChars <= 0 || c.Chars <= 0 {
			continue
		}
		switch r := float64(c.Chars) / float64(j.DescriptionChars); {
		case r >= 0.7 && r <= 1.5:
			s.add(2, "about as long as the description the page states")
		case r < 0.5:
			s.add(-1, "much shorter than the description the page states (maybe only a part)")
		}
		break
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Rank orders the candidates of one field, best first (ties keep the page's order). The score is a guide, not a verdict: the person
// chooses, and the real extraction on sample jobs verifies.
func Rank(p Page, f Field) []Ranked {
	in := p.Candidates.For(f)
	out := make([]Ranked, 0, len(in))
	for _, c := range in {
		var s scorer
		if f == FieldDescription {
			rankDescription(p, c, &s)
			switch c.PlacedIn {
			case "consent":
				s.add(-10, "inside a consent or dialog box")
			case "":
			default:
				s.add(-5, "inside the page's "+c.PlacedIn)
			}
			if c.Unique {
				s.add(1, "its selector matches only this element")
			}
		} else {
			common(c, &s)
			switch f {
			case FieldTitle:
				rankTitle(p, c, &s)
			case FieldCompany:
				rankCompany(p, c, &s)
			case FieldLocation:
				rankLocation(p, c, &s)
			case FieldPostedAt:
				rankPostedAt(p, c, &s)
			}
		}
		if math.IsNaN(s.score) || math.IsInf(s.score, 0) {
			s.score = -100
		}
		out = append(out, Ranked{Candidate: c, Score: math.Round(s.score*100) / 100, Why: s.why})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// RankAll ranks every field.
func RankAll(p Page) map[Field][]Ranked {
	out := make(map[Field][]Ranked, len(Fields))
	for _, f := range Fields {
		out[f] = Rank(p, f)
	}
	return out
}
