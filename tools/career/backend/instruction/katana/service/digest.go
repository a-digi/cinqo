package service

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"career-tool-backend/portal/linkrule"
)

// The digest of a page's links is what the AI reads instead of the raw list (plan 192, token economy): the links grouped by the SHAPE of
// their address, a few samples of each group, and the links that look like a way to the next page. About 750 tokens for a page of 44 links.

const (
	maxGroups        = 12
	samplesPerGroup  = 3
	maxSampleURL     = 90
	maxSampleText    = 60
	maxNextCandidate = 3
)

// Group is the links of one address shape.
type Group struct {
	Shape   string   `json:"shape"`
	Count   int      `json:"count"`
	Samples []Sample `json:"samples"`
	// Varies: the last part of the address differs from link to link (a job id or a slug), the mark of a list of postings.
	Varies bool `json:"varies,omitempty"`
}

// Sample is one example link of a group.
type Sample struct {
	URL  string `json:"url"`
	Text string `json:"text,omitempty"`
}

// Digest is the compact picture of a page's links.
type Digest struct {
	Page       string   `json:"page"`
	Title      string   `json:"title,omitempty"`
	Links      int      `json:"links"`
	Truncated  bool     `json:"truncated,omitempty"`
	Groups     []Group  `json:"groups"`
	MoreGroups int      `json:"moreGroups,omitempty"`
	NextPage   []string `json:"nextPageCandidates,omitempty"`
}

var (
	uuidSegment   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	nextText      = regexp.MustCompile(`(?i)^\s*(next( page)?|weiter|n[äa]chste( seite)?|older|more|[›»>]+|siguiente|suivant|avanti|volgende)\s*$`)
	digitsOnly    = regexp.MustCompile(`^\d+$`)
	hasDigit      = regexp.MustCompile(`\d`)
	longHexSegmnt = regexp.MustCompile(`(?i)^[0-9a-f]{16,}$`)
)

// segmentShape turns one path segment into its shape: ids become #, slugs with a digit or several words become *, other words stay.
func segmentShape(seg string) string {
	switch {
	case seg == "":
		return ""
	case uuidSegment.MatchString(seg), digitsOnly.MatchString(seg), longHexSegmnt.MatchString(seg):
		return "#"
	case hasDigit.MatchString(seg), strings.Count(seg, "-") >= 2, utf8.RuneCountInString(seg) > 28:
		return "*"
	}
	return seg
}

// shapeOf is the shape of an address: host, the segments' shapes, and the names (not the values) of the query.
func shapeOf(raw string) (shape string, varies bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", false
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		if s == "" {
			continue
		}
		out = append(out, segmentShape(s))
	}
	shape = strings.ToLower(u.Host)
	if len(out) > 0 {
		shape += "/" + strings.Join(out, "/")
		last := out[len(out)-1]
		varies = last == "#" || last == "*"
	}
	if q := u.Query(); len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		shape += "?" + strings.Join(keys, "&")
	}
	return shape, varies
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func firstText(l linkrule.Link) string {
	for _, t := range l.Texts {
		if t = strings.TrimSpace(t); t != "" {
			return t
		}
	}
	return strings.TrimSpace(l.TitleAttr)
}

// shortURL drops the scheme and clips: the host is in the shape already, the path is what tells links apart.
func shortURL(raw string) string {
	s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
	return clip(s, maxSampleURL)
}

// BuildDigest groups the links by shape (largest group first), keeps a few samples of each and lists the next-page candidates.
func BuildDigest(page, title string, links []linkrule.Link, truncated bool) Digest {
	d := Digest{Page: page, Title: clip(title, 80), Links: len(links), Truncated: truncated, Groups: []Group{}}
	byShape := map[string]*Group{}
	var order []string
	seenURL := map[string]bool{}
	for _, l := range links {
		if next := isNextCandidate(l); next && len(d.NextPage) < maxNextCandidate && !seenURL[l.URL] {
			d.NextPage = append(d.NextPage, shortURL(l.URL))
		}
		seenURL[l.URL] = true
		shape, varies := shapeOf(l.URL)
		if shape == "" {
			continue
		}
		g, ok := byShape[shape]
		if !ok {
			g = &Group{Shape: shape, Varies: varies}
			byShape[shape] = g
			order = append(order, shape)
		}
		g.Count++
		if len(g.Samples) < samplesPerGroup {
			g.Samples = append(g.Samples, Sample{URL: shortURL(l.URL), Text: clip(firstText(l), maxSampleText)})
		}
	}
	groups := make([]Group, 0, len(order))
	for _, s := range order {
		groups = append(groups, *byShape[s])
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	if len(groups) > maxGroups {
		d.MoreGroups = len(groups) - maxGroups
		groups = groups[:maxGroups]
	}
	d.Groups = groups
	return d
}

func isNextCandidate(l linkrule.Link) bool {
	if strings.EqualFold(strings.TrimSpace(l.Rel), "next") {
		return true
	}
	return nextText.MatchString(firstText(l))
}
