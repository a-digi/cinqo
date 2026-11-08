package linkrule

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// hosts.go (plan 203, step 1): what a page's links say about the SITES they point to. A rule belongs to one site (the link's own), so when the
// posting-like links of a page are on another host the person and the AI must be told, not shown a list without jobs in it.

var (
	uuidSegment   = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	digitsOnly    = regexp.MustCompile(`^\d+$`)
	hasDigit      = regexp.MustCompile(`\d`)
	longHexSegmnt = regexp.MustCompile(`(?i)^[0-9a-f]{16,}$`)
)

// SegmentShape turns one path segment into its shape: ids become #, slugs with a digit or several words become *, other words stay.
func SegmentShape(seg string) string {
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

// ShapeOf is the shape of an address: host, the segments' shapes, and the names (not the values) of the query. varies is true when the last
// segment differs from link to link (a job id or a slug), the mark of a list of postings.
func ShapeOf(raw string) (shape string, varies bool) {
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
		out = append(out, SegmentShape(s))
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

// HostSummary says how many of a page's links are on one host and how many of them look like postings: the size of the biggest FAMILY of
// links whose last address part varies (a job id or slug), a family being the links of one host that start with the same first address part
// (/jobanzeige/...), whatever follows, with at least MinJobLike links. A family and not one exact shape, because real boards mix several
// shapes for their postings (an alias part such as step5, talet or sprit: 10 + 2 + 2 links of one list).
type HostSummary struct {
	Host    string `json:"host"`
	Links   int    `json:"links"`
	JobLike int    `json:"jobLike"`
	// Shape is the biggest exact address shape inside that family, without the host (for example /jobanzeige/job_search/*/*/*), Sample one
	// address of it without the scheme.
	Shape  string `json:"shape,omitempty"`
	Sample string `json:"sample,omitempty"`
	// Allowed: this is the job host the person confirmed for the link (plan 203), so its links may be picked and are not "another site".
	Allowed bool `json:"allowed,omitempty"`
}

const (
	// MinJobLike is how many links of one shape make them look like a list of postings.
	MinJobLike = 3
	// MaxOtherHosts bounds how many other hosts are reported.
	MaxOtherHosts = 5
	maxSampleLen  = 90
)

// SummarizeHosts splits a page's links by host: the summary of siteHost (the link's own host, always present even with no links) and the
// other hosts, the one with the most posting-like links first, at most MaxOtherHosts of them. A link whose address cannot be read is left
// out. Pure: it only looks at the addresses.
func SummarizeHosts(links []Link, siteHost string) (own HostSummary, others []HostSummary) {
	siteHost = strings.ToLower(strings.TrimSpace(siteHost))
	type group struct {
		count  int
		varies bool
		family string
		sample string
	}
	type hostAcc struct {
		links    int
		groups   map[string]*group
		order    []string
		families map[string]int // the links whose last part varies, by the shape of their first path part
	}
	byHost := map[string]*hostAcc{}
	var hostOrder []string
	seen := map[string]bool{}
	for _, l := range links {
		u, err := url.Parse(l.URL)
		if err != nil || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		// the same address is one link, whatever the case of its host (the path and query are case-sensitive)
		key := strings.ToLower(u.Scheme) + "://" + host + u.EscapedPath() + "?" + u.RawQuery
		if seen[key] {
			continue
		}
		seen[key] = true
		acc := byHost[host]
		if acc == nil {
			acc = &hostAcc{groups: map[string]*group{}, families: map[string]int{}}
			byHost[host] = acc
			hostOrder = append(hostOrder, host)
		}
		acc.links++
		full, varies := ShapeOf(l.URL)
		shape := strings.TrimPrefix(full, host)
		family := firstPartShape(u)
		g := acc.groups[shape]
		if g == nil {
			g = &group{varies: varies, family: family, sample: strings.TrimPrefix(strings.TrimPrefix(l.URL, "https://"), "http://")}
			acc.groups[shape] = g
			acc.order = append(acc.order, shape)
		}
		g.count++
		if varies {
			acc.families[family]++
		}
	}
	summarize := func(host string) HostSummary {
		s := HostSummary{Host: host}
		acc := byHost[host]
		if acc == nil {
			return s
		}
		s.Links = acc.links
		// the biggest family of posting-like links (ties go to the one that came first on the page)
		bestFamily := ""
		for _, shape := range acc.order {
			g := acc.groups[shape]
			if g.varies && acc.families[g.family] >= MinJobLike && acc.families[g.family] > s.JobLike {
				s.JobLike, bestFamily = acc.families[g.family], g.family
			}
		}
		if s.JobLike > 0 {
			biggest := 0
			for _, shape := range acc.order {
				g := acc.groups[shape]
				if g.varies && g.family == bestFamily && g.count > biggest {
					biggest, s.Shape, s.Sample = g.count, shape, clipTo(g.sample, maxSampleLen)
				}
			}
		}
		return s
	}
	own = summarize(siteHost)
	own.Host = siteHost
	for _, host := range hostOrder {
		if host != siteHost {
			others = append(others, summarize(host))
		}
	}
	sort.SliceStable(others, func(i, j int) bool {
		if others[i].JobLike != others[j].JobLike {
			return others[i].JobLike > others[j].JobLike
		}
		return others[i].Links > others[j].Links
	})
	if len(others) > MaxOtherHosts {
		others = others[:MaxOtherHosts]
	}
	return own, others
}

// MarkAllowed sets Allowed on the entry of the job host the person confirmed for the link (jobHost "" marks none).
func MarkAllowed(others []HostSummary, jobHost string) []HostSummary {
	jobHost = strings.ToLower(strings.TrimSpace(jobHost))
	if jobHost == "" {
		return others
	}
	out := make([]HostSummary, len(others))
	copy(out, others)
	for i := range out {
		out[i].Allowed = out[i].Host == jobHost
	}
	return out
}

// PostingsOnAnotherSite reports the other host whose links look like the page's postings better than the link's own site's do: the biggest
// posting-like group on it is bigger than the biggest on the own site and than the one on the confirmed job host (Allowed), and the host is
// not itself the confirmed one. false when none does. This is the case in which a rule for the link cannot see the jobs at all.
func PostingsOnAnotherSite(own HostSummary, others []HostSummary) (HostSummary, bool) {
	best := own.JobLike
	for _, o := range others {
		if o.Allowed && o.JobLike > best {
			best = o.JobLike
		}
	}
	for _, o := range others { // ordered: the one with the most posting-like links first
		if !o.Allowed && o.JobLike >= MinJobLike && o.JobLike > best {
			return o, true
		}
	}
	return HostSummary{}, false
}

// firstPartShape is the shape of an address's first path part ("" for the root): the family of a posting-like link.
func firstPartShape(u *url.URL) string {
	first := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 2)[0]
	return SegmentShape(first)
}

func clipTo(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
