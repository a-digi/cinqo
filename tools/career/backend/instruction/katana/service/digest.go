package service

import (
	"fmt"
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
	// Site is the link's own host and OtherHosts the other hosts the page's links point to, the one with the most posting-like links first
	// (plan 203). Note is set when the page's postings are on another site than the link's: a rule cannot be made for them.
	Site       *linkrule.HostSummary  `json:"site,omitempty"`
	OtherHosts []linkrule.HostSummary `json:"otherHosts,omitempty"`
	Note       string                 `json:"note,omitempty"`
}

// maxDigestHosts bounds the other hosts a digest lists (the digest is read by a small model: a few lines, not a table).
const maxDigestHosts = 3

// WithHosts adds what the page's links say about sites to a digest: the link's own host, the other hosts, and the note that a rule cannot be
// made when the postings are on another host that no person confirmed for the link (jobHost). siteHost "" leaves the digest as it was.
func (d Digest) WithHosts(links []linkrule.Link, siteHost, jobHost string) Digest {
	if siteHost == "" {
		return d
	}
	own, others := linkrule.SummarizeHosts(links, siteHost)
	others = linkrule.MarkAllowed(others, jobHost)
	d.Site = &own
	if len(others) > maxDigestHosts {
		others = others[:maxDigestHosts]
	}
	d.OtherHosts = others
	if top, ok := linkrule.PostingsOnAnotherSite(own, others); ok {
		d.Note = fmt.Sprintf("the posting-like links of this page (%d, shaped %s) are on %s, another site than this link's own (%s). A rule can only be made for the link's own site, so these cannot be a rule: STOP and tell the person (they can set this link's address to %s if it shows the same list); do not guess a rule", top.JobLike, top.Shape, top.Host, siteHost, top.Host)
	}
	return d
}

var nextText = regexp.MustCompile(`(?i)^\s*(next( page)?|weiter|n[äa]chste( seite)?|older|more|[›»>]+|siguiente|suivant|avanti|volgende)\s*$`)

// shapeOf is the shape of an address (host, the segments' shapes, the names of the query): shared with the link crawler's host summary.
func shapeOf(raw string) (shape string, varies bool) { return linkrule.ShapeOf(raw) }

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
