package linkrule

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var (
	ErrNoPicks    = errors.New("pick at least one job link")
	ErrBadURL     = errors.New("not a usable web address")
	ErrMixedHosts = errors.New("the picked links belong to different sites: pick links of one site")
	// ErrNoCommonPattern: no pattern fits all the picks (for example the site's start page picked together with jobs).
	ErrNoCommonPattern = errors.New("no single pattern fits all the picked links: pick links of one kind (a start page is not a job link)")
)

// BroadMatchLimit: a pattern that matches more links than this on one page needs an explicit confirm.
const BroadMatchLimit = 200

// Link is one distinct URL of a page with what the page says about it: the
// shape of the Browser tool's /fetch-links list.
type Link struct {
	URL       string   `json:"url"`
	Texts     []string `json:"texts,omitempty"`
	TitleAttr string   `json:"titleAttr,omitempty"`
	Rel       string   `json:"rel,omitempty"`
	// Anchors (plan 247): the numbers of this link's <a> elements in the page copy the picker shows; only present when a copy was asked for. Never read by the
	// rule code, only carried to the screen and back.
	Anchors []int `json:"anchors,omitempty"`
}

// Proposal is what the tool shows the person after they picked links: the
// partial URL it found, and what that pattern would do on this page.
type Proposal struct {
	// Pattern is the proposed partial URL (see the package comment), Host its host.
	Pattern string `json:"pattern"`
	Host    string `json:"host"`
	// Exclude holds patterns that keep links the person marked "not a job" out,
	// only for those the include pattern would otherwise still match.
	Exclude []string `json:"exclude,omitempty"`
	// Matches are the page's links (normalised, in page order) the pattern
	// matches and Exclude does not; MatchCount is their number.
	Matches    []string `json:"matches"`
	MatchCount int      `json:"matchCount"`
	// Unmatched are picked links the pattern does not match (should be none: said, never hidden).
	Unmatched []string `json:"unmatched,omitempty"`
	// ExcludedStillMatched are "not a job" links the pattern still matches (it could not be separated from the picks).
	ExcludedStillMatched []string `json:"excludedStillMatched,omitempty"`
	// NeedsConfirm: the person must confirm before this is saved (one pick,
	// a very broad pattern, or more than BroadMatchLimit matches).
	NeedsConfirm bool     `json:"needsConfirm"`
	Notes        []string `json:"notes,omitempty"`
}

// boundary characters inside a segment where a literal prefix/suffix may be cut.
func isBoundary(r rune) bool { return r == '-' || r == '_' || r == '.' }

func safeLiteral(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r == '*' || r == '?' || r == '&' || r == '=' || r == '#' || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// leadingNumber splits a segment value into its leading digits and the rest. ok only when there is at least one digit and the rest is
// empty or starts at a word boundary (- _ .): "8195" and "121888-backend-developer" are number-led, "12abc" and "types" are not.
func leadingNumber(v string) (digits, rest string, ok bool) {
	i := 0
	for i < len(v) && v[i] >= '0' && v[i] <= '9' {
		i++
	}
	if i == 0 || i > 12 {
		return "", "", false
	}
	rest = v[i:]
	if rest != "" && !isBoundary([]rune(rest)[0]) {
		return "", "", false
	}
	return v[:i], rest, true
}

// tightenNumberLed handles values that all start with a number (an id): "#" for the number, then what follows it, tightened like any
// segment ("#" alone for plain ids, "#-*" for id-slug, "#.html" for id.html). It is what tells /jobs/8195 from /jobs/types.
func tightenNumberLed(values []string) (string, bool) {
	rests := make([]string, len(values))
	for i, v := range values {
		_, rest, ok := leadingNumber(v)
		if !ok {
			return "", false
		}
		rests[i] = rest
	}
	same := true
	for _, r := range rests[1:] {
		if r != rests[0] {
			same = false
		}
	}
	switch {
	case same && rests[0] == "":
		return "#", true
	case same && safeLiteral(rests[0]):
		return "#" + rests[0], true
	}
	// differing tails: the same tightening as for any segment (it never returns a digit-led pattern for a tail that starts at a boundary)
	return "#" + tightenTail(rests), true
}

// tightenTail tightens the tails that follow the number: "" when they are all empty-or-unsafe is "*".
func tightenTail(rests []string) string {
	for _, r := range rests {
		if r == "" || !safeLiteral(r) {
			return "*"
		}
	}
	return tighten(rests)
}

// tighten turns the values one segment has in the picked links into a pattern
// segment: the value itself when all are equal; "#" for a number (and "#-*" for
// a number followed by a slug); else "prefix*suffix" where the prefix/suffix
// are the parts the values share up to a word boundary (- _ .), so an
// accidental shared "121" is not kept but "job-" is.
func tighten(values []string) string {
	same := true
	for _, v := range values[1:] {
		if v != values[0] {
			same = false
			break
		}
	}
	if same {
		if safeLiteral(values[0]) {
			return values[0]
		}
		return "*"
	}
	for _, v := range values {
		if !safeLiteral(v) {
			return "*"
		}
	}
	if p, ok := tightenNumberLed(values); ok {
		return p
	}
	rs := make([][]rune, len(values))
	shortest := len(values[0])
	for i, v := range values {
		rs[i] = []rune(v)
		if len(rs[i]) < shortest || i == 0 {
			shortest = len(rs[i])
		}
	}
	// common prefix, cut after its last boundary character
	lcp := 0
	for lcp < shortest {
		c := rs[0][lcp]
		ok := true
		for _, r := range rs[1:] {
			if r[lcp] != c {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		lcp++
	}
	prefix := ""
	for i := lcp; i > 0; i-- {
		if isBoundary(rs[0][i-1]) {
			prefix = string(rs[0][:i])
			break
		}
	}
	// common suffix, kept from its first boundary character
	lcs := 0
	for lcs < shortest-len([]rune(prefix)) {
		c := rs[0][len(rs[0])-1-lcs]
		ok := true
		for _, r := range rs[1:] {
			if r[len(r)-1-lcs] != c {
				ok = false
				break
			}
		}
		if !ok {
			break
		}
		lcs++
	}
	suffix := ""
	tail := rs[0][len(rs[0])-lcs:]
	for i, r := range tail {
		if isBoundary(r) {
			suffix = string(tail[i:])
			break
		}
	}
	return prefix + "*" + suffix
}

// learnedPattern is the result of learning one pattern from parsed picks.
type learnedPattern struct {
	pattern string
	notes   []string
	broad   bool // the pattern is very broad (only stars after the host)
}

// learn derives one pattern from the picked links of one host (len(ps) >= 1,
// already de-duplicated).
func learn(ps []parsedURL) learnedPattern {
	host := ps[0].host
	if len(ps) == 1 {
		return learnSingle(ps[0])
	}
	segs := make([][]string, len(ps))
	sameLen := true
	for i, p := range ps {
		segs[i] = p.segments()
		if len(segs[i]) != len(segs[0]) {
			sameLen = false
		}
	}
	if !sameLen {
		// different depths: keep the literal start the picks share, then anything below it
		var common []string
		for i := 0; i < len(segs[0]); i++ {
			ok := true
			for _, s := range segs[1:] {
				if i >= len(s) || s[i] != segs[0][i] || !safeLiteral(s[i]) {
					ok = false
					break
				}
			}
			if !ok {
				break
			}
			common = append(common, segs[0][i])
		}
		pat := host + "/" + strings.Join(append(common, "**"), "/")
		return learnedPattern{pattern: pat, broad: len(common) == 0,
			notes: []string{"The picked links have different depths, so the pattern matches everything below the part they share. Check the matches."}}
	}
	out := make([]string, len(segs[0]))
	varies := false
	for pos := range out {
		vals := make([]string, len(ps))
		for i := range ps {
			vals[i] = segs[i][pos]
		}
		out[pos] = tighten(vals)
		if out[pos] != vals[0] {
			varies = true
		}
	}
	pat := host
	if len(out) > 0 {
		pat += "/" + strings.Join(out, "/")
	}
	if varies {
		// the path tells the jobs apart: the query (tracking parameters) is ignored
		return learnedPattern{pattern: pat, broad: allStars(out)}
	}
	// identical paths: the jobs differ in the query, so the query is the pattern
	q := learnQuery(ps)
	if q == "" {
		return learnedPattern{pattern: pat, broad: allStars(out),
			notes: []string{"The picked links differ only in a way no pattern can express; the pattern matches that page for every link."}}
	}
	return learnedPattern{pattern: pat + "?" + q, broad: false}
}

func allStars(segs []string) bool {
	for _, s := range segs {
		if s != "*" && s != "**" {
			return false
		}
	}
	return true
}

// learnQuery builds the query part for picks whose paths are identical:
// parameters every pick has, constant ones literally, varying ones as *.
func learnQuery(ps []parsedURL) string {
	var keys []string
	for k := range ps[0].query {
		inAll := true
		for _, p := range ps[1:] {
			if _, ok := p.query[k]; !ok {
				inAll = false
				break
			}
		}
		if inAll && safeLiteral(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := make([]string, len(ps))
		for i, p := range ps {
			vals[i] = p.query.Get(k)
		}
		same := true
		for _, v := range vals[1:] {
			if v != vals[0] {
				same = false
			}
		}
		if same && safeLiteral(vals[0]) {
			parts = append(parts, k+"="+vals[0])
		} else {
			parts = append(parts, k+"=*")
		}
	}
	return strings.Join(parts, "&")
}

// learnSingle generalises ONE picked link: with a query the path stays and the
// parameter values become *; without one, digit-only segments and the last
// segment become *.
func learnSingle(p parsedURL) learnedPattern {
	notes := []string{"Only one link was picked, so the pattern is a guess: the last part of the address is replaced by *. Pick two or three links to get a safer pattern."}
	segs := p.segments()
	if len(p.query) > 0 {
		var keys []string
		for k := range p.query {
			if safeLiteral(k) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		pat := p.host
		if len(segs) > 0 {
			pat += "/" + strings.Join(segs, "/")
		}
		if len(keys) > 0 {
			parts := make([]string, len(keys))
			for i, k := range keys {
				parts[i] = k + "=*"
			}
			pat += "?" + strings.Join(parts, "&")
		}
		return learnedPattern{pattern: pat, notes: notes}
	}
	out := make([]string, len(segs))
	for i, s := range segs {
		switch {
		case !safeLiteral(s):
			out[i] = "*"
		case allDigits(s):
			out[i] = "#"
		case i == len(segs)-1:
			if _, rest, ok := leadingNumber(s); ok && rest != "" {
				out[i] = "#" + string([]rune(rest)[0]) + "*" // an id and a slug: 121888-backend-developer
			} else {
				out[i] = "*"
			}
		default:
			out[i] = s
		}
	}
	pat := p.host
	if len(out) > 0 {
		pat += "/" + strings.Join(out, "/")
	}
	return learnedPattern{pattern: pat, notes: notes, broad: allStars(out) && len(out) > 0}
}

// LearnJobPattern finds the partial URL of the jobs from the links a person
// picked (picked) and, optionally, links they marked "not a job" (notJob),
// and shows what it would do on this page (candidates, the page's link list).
func LearnJobPattern(picked, notJob []string, candidates []Link) (Proposal, error) {
	if len(picked) == 0 {
		return Proposal{}, ErrNoPicks
	}
	parse := func(urls []string) ([]parsedURL, error) {
		seen := map[string]bool{}
		var out []parsedURL
		for _, raw := range urls {
			p, ok := parseURL(raw)
			if !ok {
				return nil, fmt.Errorf("%w: %q", ErrBadURL, truncate(raw, 120))
			}
			if !seen[p.norm] {
				seen[p.norm] = true
				out = append(out, p)
			}
		}
		return out, nil
	}
	picks, err := parse(picked)
	if err != nil {
		return Proposal{}, err
	}
	nots, err := parse(notJob)
	if err != nil {
		return Proposal{}, err
	}
	for _, p := range picks[1:] {
		if p.host != picks[0].host {
			return Proposal{}, ErrMixedHosts
		}
	}

	l := learn(picks)
	pat, cerr := Compile(l.pattern)
	// The pattern must match every pick. If it does not (it should always), fall back to the whole host's
	// path below the shared start, and say so; never return a pattern that misses a link the person picked.
	if cerr != nil || !matchesAll(pat, picks) {
		l = learnedPattern{pattern: picks[0].host + "/**", broad: true,
			notes: append(l.notes, "No narrower pattern fits all picked links, so this one matches the whole site. Check the matches.")}
		pat, cerr = Compile(l.pattern)
		// host/** matches every page below the start page but not the start page itself: a set of picks that
		// still is not matched has nothing in common that a pattern can say.
		if cerr != nil || !matchesAll(pat, picks) {
			return Proposal{}, ErrNoCommonPattern
		}
	}

	prop := Proposal{Pattern: l.pattern, Host: pat.host, Notes: l.notes}
	prop.NeedsConfirm = len(picks) == 1 || l.broad

	// "not a job" links the pattern would still take: a pattern of their own keeps them out when it does not
	// also match a pick; otherwise they are said, not hidden.
	var excludes []*Pattern
	for _, n := range nots {
		if !pat.matchParsed(n) {
			continue
		}
		ex := learnSingle(n)
		exPat, err := Compile(ex.pattern)
		if err == nil && !matchesAny(exPat, picks) {
			dup := false
			for _, have := range prop.Exclude {
				if have == ex.pattern {
					dup = true
				}
			}
			if !dup {
				prop.Exclude = append(prop.Exclude, ex.pattern)
				excludes = append(excludes, exPat)
			}
		}
	}
	for _, n := range nots {
		if pat.matchParsed(n) && !anyMatch(excludes, n) {
			prop.ExcludedStillMatched = append(prop.ExcludedStillMatched, n.norm)
		}
	}
	if len(prop.ExcludedStillMatched) > 0 {
		prop.Notes = append(prop.Notes, "Some links you marked as not a job look like the picked jobs, so the pattern cannot keep them out.")
	}

	// what the pattern does on this page
	prop.Matches = []string{}
	seen := map[string]bool{}
	for _, c := range candidates {
		u, ok := parseURL(c.URL)
		if !ok || seen[u.norm] {
			continue
		}
		seen[u.norm] = true
		if pat.matchParsed(u) && !anyMatch(excludes, u) {
			prop.Matches = append(prop.Matches, u.norm)
		}
	}
	prop.MatchCount = len(prop.Matches)
	if len(candidates) > 0 {
		for _, p := range picks {
			if !seen[p.norm] {
				prop.Notes = append(prop.Notes, "A picked link is not in this page's link list: the list may be out of date. Load the links again.")
				break
			}
		}
	}
	for _, p := range picks {
		if !pat.matchParsed(p) {
			prop.Unmatched = append(prop.Unmatched, p.norm)
		}
	}
	if prop.MatchCount > BroadMatchLimit {
		prop.NeedsConfirm = true
		prop.Notes = append(prop.Notes, fmt.Sprintf("The pattern matches %d links on this page: confirm that they are all jobs.", prop.MatchCount))
	}
	if l.broad {
		prop.Notes = append(prop.Notes, "This pattern is very broad: it matches almost every link of the site.")
	}
	return prop, nil
}

func matchesAll(p *Pattern, us []parsedURL) bool {
	for _, u := range us {
		if !p.matchParsed(u) {
			return false
		}
	}
	return true
}

func matchesAny(p *Pattern, us []parsedURL) bool {
	for _, u := range us {
		if p.matchParsed(u) {
			return true
		}
	}
	return false
}

func anyMatch(ps []*Pattern, u parsedURL) bool {
	for _, p := range ps {
		if p.matchParsed(u) {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
