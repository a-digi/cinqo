package linkrule

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// How a rule finds the next page of a listing.
const (
	PagesNone     = "none"     // one page
	PagesTemplate = "template" // the next pages' addresses follow a counter: ...?page={n} or .../page/{n}
	PagesLink     = "link"     // the next page is a link of the current page that matches a pattern
)

// The bounds of pagination.
const (
	DefaultMaxPages   = 5
	MaxMaxPages       = 50
	MaxTemplateNumber = 100000
)

var ErrBadPagination = errors.New("the pagination is not valid")

// Pagination says where the pages after the first are.
//
// Template: Template is an address with exactly one {n}; page 2 is the template with n = Second, page k >= 2 has
// n = Second + (k-2)*Step. The first page is the listing's own address.
// Link: Pattern (a pattern as in the package comment) selects the link of the current page that leads to the next one; a link the
// crawl already visited, and a link that is a job link of the rule, is never followed; rel="next" is preferred.
// MaxPages (0 = DefaultMaxPages, at most MaxMaxPages) is the most pages one crawl loads.
type Pagination struct {
	Kind     string `json:"kind"`
	Template string `json:"template,omitempty"`
	Second   int    `json:"second,omitempty"`
	Step     int    `json:"step,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
	MaxPages int    `json:"maxPages,omitempty"`
}

func (p *Pagination) active() bool { return p != nil && p.Kind != "" && p.Kind != PagesNone }

// pageLimit is the most pages a crawl with this pagination loads.
func (p *Pagination) pageLimit() int {
	if !p.active() {
		return 1
	}
	if p.MaxPages <= 0 {
		return DefaultMaxPages
	}
	return p.MaxPages
}

func (p *Pagination) step() int {
	if p.Step <= 0 {
		return 1
	}
	return p.Step
}

// templateURL fills the template for counter value n.
func templateURL(template string, n int) string {
	return strings.Replace(template, "{n}", strconv.Itoa(n), 1)
}

// compiledPagination is a validated pagination; link is its compiled pattern (link kind).
type compiledPagination struct {
	p    *Pagination
	link *Pattern
}

// validatePagination checks a pagination for a rule of host. nil and "none" are valid (one page).
func validatePagination(p *Pagination, host string) (compiledPagination, error) {
	if !p.active() {
		return compiledPagination{}, nil
	}
	if p.MaxPages < 0 || p.MaxPages > MaxMaxPages {
		return compiledPagination{}, fmt.Errorf("%w: maxPages must be 1 to %d", ErrBadPagination, MaxMaxPages)
	}
	switch p.Kind {
	case PagesTemplate:
		if strings.Count(p.Template, "{n}") != 1 {
			return compiledPagination{}, fmt.Errorf("%w: the template needs exactly one {n}", ErrBadPagination)
		}
		if p.Second < 1 || p.Second > MaxTemplateNumber || p.Step < 0 || p.Step > MaxTemplateNumber {
			return compiledPagination{}, fmt.Errorf("%w: second must be 1 to %d and step at most %d", ErrBadPagination, MaxTemplateNumber, MaxTemplateNumber)
		}
		// the last page the crawl can ask for must still be a valid address on the rule's host
		last := p.Second + (MaxMaxPages-2)*p.step()
		for _, n := range []int{p.Second, last} {
			u, ok := parseURL(templateURL(p.Template, n))
			if !ok {
				return compiledPagination{}, fmt.Errorf("%w: the template does not make a web address", ErrBadPagination)
			}
			if u.host != host {
				return compiledPagination{}, fmt.Errorf("%w: the template must be on %s", ErrBadPagination, host)
			}
		}
		return compiledPagination{p: p}, nil
	case PagesLink:
		pat, err := Compile(p.Pattern)
		if err != nil {
			return compiledPagination{}, fmt.Errorf("%w: the next-page pattern: %v", ErrBadPagination, err)
		}
		if pat.host != host {
			return compiledPagination{}, fmt.Errorf("%w: the next-page pattern must be for %s", ErrBadPagination, host)
		}
		return compiledPagination{p: p, link: pat}, nil
	}
	return compiledPagination{}, fmt.Errorf("%w: kind must be none, template or link", ErrBadPagination)
}

func hasRelToken(rel, token string) bool {
	for _, t := range strings.Fields(strings.ToLower(rel)) {
		if t == token {
			return true
		}
	}
	return false
}

// nextPage returns the address of the page after page number pageIndex (1-based) of the crawl, or false when there is none. links are
// the links of the page just loaded; visited holds the normalised addresses already loaded.
func (c compiledRule) nextPage(pageIndex int, links []Link, visited map[string]bool) (string, bool) {
	if !c.paging.p.active() {
		return "", false
	}
	switch c.paging.p.Kind {
	case PagesTemplate:
		n := c.paging.p.Second + (pageIndex-1)*c.paging.p.step()
		norm, ok := NormalizeURL(templateURL(c.paging.p.Template, n))
		return norm, ok
	case PagesLink:
		best := ""
		for _, l := range links {
			u, ok := parseURL(l.URL)
			if !ok || u.host != c.host || visited[u.norm] || !c.paging.link.matchParsed(u) {
				continue
			}
			// a job link, or one the person excluded, is not a next page
			if anyMatch(c.include, u) || anyMatch(c.exclude, u) {
				continue
			}
			if hasRelToken(l.Rel, "next") {
				return u.norm, true
			}
			if best == "" {
				best = u.norm
			}
		}
		return best, best != ""
	}
	return "", false
}

// NextPage is nextPage for a rule that is known to be valid (false for an invalid rule).
func NextPage(r Rule, pageIndex int, links []Link, visited map[string]bool) (string, bool) {
	c, err := compileRule(r)
	if err != nil {
		return "", false
	}
	return c.nextPage(pageIndex, links, visited)
}

// ---- learning ----

// PaginationProposal is what the tool shows after the person picked the link(s) of the next page(s).
type PaginationProposal struct {
	Pagination Pagination `json:"pagination"`
	// NextURLs: for a template the addresses of pages 2, 3 and 4; for a link pattern the page's links it matches (at most 10).
	NextURLs     []string `json:"nextUrls"`
	NeedsConfirm bool     `json:"needsConfirm"`
	Notes        []string `json:"notes,omitempty"`
}

var digitRun = regexp.MustCompile(`\d+`)

// templateFrom derives, from the first page's address (cur) and one picked next-page address (pick), a template where exactly ONE
// thing differs: a numeric query value or one path segment's number. n is the pick's number; a is the first page's number there
// (hasA false when the first page has none).
func templateFrom(cur, pick parsedURL) (template string, n, a int, hasA, ok bool) {
	pu, err := url.Parse(pick.norm)
	if err != nil {
		return "", 0, 0, false, false
	}
	// A) the same path, one numeric query value that differs
	if cur.path == pick.path {
		var diff []string
		keys := map[string]bool{}
		for k := range cur.query {
			keys[k] = true
		}
		for k := range pick.query {
			keys[k] = true
		}
		for k := range keys {
			if !reflect.DeepEqual(cur.query[k], pick.query[k]) {
				diff = append(diff, k)
			}
		}
		if len(diff) != 1 {
			return "", 0, 0, false, false
		}
		k := diff[0]
		pv := pick.query[k]
		if len(pv) != 1 || !isNumber(pv[0]) {
			return "", 0, 0, false, false
		}
		n, _ = strconv.Atoi(pv[0])
		if cv := cur.query[k]; len(cv) == 1 && isNumber(cv[0]) {
			a, _ = strconv.Atoi(cv[0])
			hasA = true
		} else if len(cv) != 0 {
			return "", 0, 0, false, false
		}
		var toks []string
		replaced := false
		for _, tok := range strings.Split(pu.RawQuery, "&") {
			key, _, _ := strings.Cut(tok, "=")
			if dk, err := url.QueryUnescape(key); err == nil && dk == k && !replaced {
				toks = append(toks, key+"={n}")
				replaced = true
			} else {
				toks = append(toks, tok)
			}
		}
		if !replaced {
			return "", 0, 0, false, false
		}
		pu.RawQuery = strings.Join(toks, "&")
		// url.URL.String would escape the braces; build the template by hand from the parts
		return pu.Scheme + "://" + pu.Host + pu.EscapedPath() + "?" + pu.RawQuery, n, a, hasA, true
	}
	// B) the same query, the same number of path segments, one segment with a different number
	if reflect.DeepEqual(cur.query, pick.query) {
		cs, ps := cur.segments(), pick.segments()
		if len(cs) != len(ps) || len(ps) == 0 {
			return "", 0, 0, false, false
		}
		j := -1
		for i := range cs {
			if cs[i] != ps[i] {
				if j != -1 {
					return "", 0, 0, false, false
				}
				j = i
			}
		}
		if j == -1 {
			return "", 0, 0, false, false
		}
		pLoc := digitRun.FindAllStringIndex(ps[j], -1)
		cLoc := digitRun.FindAllStringIndex(cs[j], -1)
		if len(pLoc) == 0 || len(cLoc) != len(pLoc) {
			return "", 0, 0, false, false
		}
		// the number that differs must be the last digit run, and the text around the numbers must be equal
		last := pLoc[len(pLoc)-1]
		cLast := cLoc[len(cLoc)-1]
		if ps[j][:last[0]] != cs[j][:cLast[0]] || ps[j][last[1]:] != cs[j][cLast[1]:] {
			return "", 0, 0, false, false
		}
		var err error
		if n, err = strconv.Atoi(ps[j][last[0]:last[1]]); err != nil || len(ps[j][last[0]:last[1]]) > 6 {
			return "", 0, 0, false, false
		}
		if a, err = strconv.Atoi(cs[j][cLast[0]:cLast[1]]); err != nil {
			return "", 0, 0, false, false
		}
		hasA = true
		segs := append([]string(nil), ps...)
		segs[j] = ps[j][:last[0]] + "{n}" + ps[j][last[1]:]
		q := ""
		if pu.RawQuery != "" {
			q = "?" + pu.RawQuery
		}
		return pu.Scheme + "://" + pu.Host + "/" + strings.Join(segs, "/") + q, n, a, hasA, true
	}
	return "", 0, 0, false, false
}

func isNumber(s string) bool { return allDigits(s) && len(s) <= 6 }

// LearnPagination finds how the pages follow each other from the first page's address and the link(s) the person picked for the next
// page(s): first as a counter in the address (a numeric query value or path number that differs, with the step taken from two picks,
// from the first page's own number, or assumed to be 1), else as a pattern for a link of the current page. candidates are the page's links.
func LearnPagination(currentURL string, picked []string, candidates []Link) (PaginationProposal, error) {
	cur, ok := parseURL(currentURL)
	if !ok {
		return PaginationProposal{}, fmt.Errorf("%w: %q", ErrBadURL, truncate(currentURL, 120))
	}
	if len(picked) == 0 {
		return PaginationProposal{}, errors.New("pick the link of the next page")
	}
	var picks []parsedURL
	seen := map[string]bool{}
	for _, raw := range picked {
		p, ok := parseURL(raw)
		if !ok {
			return PaginationProposal{}, fmt.Errorf("%w: %q", ErrBadURL, truncate(raw, 120))
		}
		if p.host != cur.host {
			return PaginationProposal{}, ErrMixedHosts
		}
		if p.norm == cur.norm {
			return PaginationProposal{}, errors.New("a picked link is the first page itself: pick the link to the NEXT page")
		}
		if !seen[p.norm] {
			seen[p.norm] = true
			picks = append(picks, p)
		}
	}

	if prop, ok := learnTemplate(cur, picks); ok {
		return prop, nil
	}
	// A link pattern: the next page is a link of the current page that looks like the picked ones.
	lp, err := LearnJobPattern(picked, nil, candidates)
	if err != nil {
		return PaginationProposal{}, err
	}
	prop := PaginationProposal{
		Pagination:   Pagination{Kind: PagesLink, Pattern: lp.Pattern, MaxPages: DefaultMaxPages},
		NeedsConfirm: lp.NeedsConfirm,
		Notes:        []string{"The next page cannot be built from a counter, so the crawl follows the link of each page that matches this pattern (a link already visited is never followed again)."},
	}
	prop.Notes = append(prop.Notes, lp.Notes...)
	if len(prop.Pagination.Pattern) > 0 {
		if c, err := Compile(lp.Pattern); err == nil && c.Broad() {
			prop.NeedsConfirm = true
		}
	}
	pat, _ := Compile(lp.Pattern)
	for _, c := range candidates {
		if u, ok := parseURL(c.URL); ok && pat.matchParsed(u) && u.norm != cur.norm {
			prop.NextURLs = append(prop.NextURLs, u.norm)
			if len(prop.NextURLs) == 10 {
				break
			}
		}
	}
	if prop.NextURLs == nil {
		prop.NextURLs = []string{}
	}
	return prop, nil
}

// learnTemplate tries the counter-in-the-address reading for all picks.
func learnTemplate(cur parsedURL, picks []parsedURL) (PaginationProposal, bool) {
	type derived struct {
		template string
		n        int
	}
	var all []derived
	var a int
	var hasA bool
	for _, p := range picks {
		t, n, pa, pHasA, ok := templateFrom(cur, p)
		if !ok {
			return PaginationProposal{}, false
		}
		if len(all) > 0 && t != all[0].template {
			return PaginationProposal{}, false
		}
		a, hasA = pa, pHasA
		all = append(all, derived{t, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n < all[j].n })
	for i := 1; i < len(all); i++ {
		if all[i].n == all[i-1].n {
			return PaginationProposal{}, false
		}
	}
	var notes []string
	needsConfirm := false
	step := 1
	second := all[0].n
	switch {
	case len(all) >= 2:
		step = all[1].n - all[0].n
		for i := 2; i < len(all); i++ {
			if all[i].n-all[i-1].n != step {
				return PaginationProposal{}, false // not an even counter
			}
		}
		if hasA {
			// the first page's own number says where page 2 is
			if all[0].n != a+step {
				notes = append(notes, fmt.Sprintf("The first page has the number %d and the picked pages count by %d, so page 2 should be %d; you picked %d as the first. Check the addresses below.", a, step, a+step, all[0].n))
				needsConfirm = true
			}
		}
	case hasA && all[0].n > a:
		step = all[0].n - a
		second = a + step
	case !hasA && all[0].n == 2:
		// "page=2" on a first page that has no number: plainly the second page, counting up by 1
	default:
		// one pick and the first page has no number: the next pages are assumed to count up by 1
		notes = append(notes, "Only one next page was picked, so the following pages are assumed to count up by 1. Check the addresses below, or pick two pages to be sure.")
		needsConfirm = true
	}
	if second < 1 || second > MaxTemplateNumber || step < 1 || step > MaxTemplateNumber {
		return PaginationProposal{}, false
	}
	pg := Pagination{Kind: PagesTemplate, Template: all[0].template, Second: second, Step: step, MaxPages: DefaultMaxPages}
	prop := PaginationProposal{Pagination: pg, NeedsConfirm: needsConfirm, Notes: notes}
	for i := 0; i < 3; i++ {
		if u, ok := NormalizeURL(templateURL(pg.Template, second+i*step)); ok {
			prop.NextURLs = append(prop.NextURLs, u)
		}
	}
	// every picked address must be one the template produces
	produced := map[string]bool{}
	for i := 0; i < len(all)+3; i++ {
		if u, ok := NormalizeURL(templateURL(pg.Template, second+i*step)); ok {
			produced[u] = true
		}
	}
	for _, p := range picks {
		if !produced[p.norm] {
			return PaginationProposal{}, false
		}
	}
	return prop, true
}

// ---- running the pages ----

// Page is one loaded page: where it ended up, its title and its links.
type Page struct {
	FinalURL string
	Title    string
	Links    []Link
}

// FetchPage loads one page. The caller (the crawl, the test) decides how: with the Browser tool, with a stub.
type FetchPage func(ctx context.Context, rawURL string) (Page, error)

// Why a run of pages ended.
const (
	StopMaxPages     = "max_pages"                 // the page limit was reached
	StopNoNextPage   = "no_next_page"              // the last page has no link / address to a next one
	StopNoNewJobs    = "no_new_jobs"               // a page brought no job address that an earlier page had not
	StopRepeatedPage = "next_page_already_visited" // the next page is one already loaded
	StopFetchFailed  = "fetch_failed"              // a page after the first could not be loaded (the earlier pages are kept)
	StopCancelled    = "cancelled"                 // the caller stopped
	StopLeftTheSite  = "left_the_site"             // a page ended up on another site
)

// PageResult is what one loaded page gave.
type PageResult struct {
	URL      string `json:"url"`
	FinalURL string `json:"finalUrl"`
	Title    string `json:"title"`
	Links    int    `json:"links"`
	// Matched counts the rule's job addresses on the page; NewJobs those no earlier page had; Items the jobs created from them.
	Matched int `json:"matched"`
	NewJobs int `json:"newJobs"`
	Items   int `json:"items"`
}

// PagesResult is what a rule found over the pages it loaded.
type PagesResult struct {
	Pages          []PageResult `json:"pages"`
	Items          []Item       `json:"items"`
	Matched        int          `json:"matched"`
	SkippedNoTitle int          `json:"skippedNoTitle"`
	SkippedSamples []string     `json:"skippedSamples,omitempty"`
	OtherHost      int          `json:"otherHost"`
	StopReason     string       `json:"stopReason"`
	StopDetail     string       `json:"stopDetail,omitempty"`
}

// RunPages loads the first page and, following the rule's pagination, the pages after it, and collects the jobs the rule finds:
// one item per distinct address over all pages. limit (0 = none) caps the pages below the rule's own limit (a test uses 2).
// The loop stops for the reasons above; a failure of the FIRST page is returned as an error (nothing was found), a failure of a later
// one ends the run with the earlier pages kept. An invalid rule or a first address that is not on the rule's host is an error.
func RunPages(ctx context.Context, r Rule, firstURL string, fetch FetchPage, limit int) (PagesResult, error) {
	c, err := compileRule(r)
	if err != nil {
		return PagesResult{}, err
	}
	first, ok := parseURL(firstURL)
	if !ok {
		return PagesResult{}, fmt.Errorf("%w: %q", ErrBadURL, truncate(firstURL, 120))
	}
	if first.host != c.host {
		return PagesResult{}, fmt.Errorf("%w: the first page is on %s", ErrHostMismatch, first.host)
	}
	maxPages := c.paging.p.pageLimit()
	if limit > 0 && limit < maxPages {
		maxPages = limit
	}

	res := PagesResult{Pages: []PageResult{}, Items: []Item{}}
	visited := map[string]bool{}
	seenURL := map[string]bool{}
	pageURL := first.norm
	for index := 1; ; index++ {
		if err := ctx.Err(); err != nil {
			res.StopReason, res.StopDetail = StopCancelled, err.Error()
			return res, nil
		}
		visited[pageURL] = true
		page, err := fetch(ctx, pageURL)
		if err != nil {
			if ctx.Err() != nil {
				res.StopReason, res.StopDetail = StopCancelled, ctx.Err().Error()
				return res, nil
			}
			if index == 1 {
				return PagesResult{}, err
			}
			res.StopReason, res.StopDetail = StopFetchFailed, err.Error()
			return res, nil
		}
		if fu, ok := parseURL(page.FinalURL); ok {
			if fu.host != c.host {
				res.StopReason, res.StopDetail = StopLeftTheSite, fu.host
				return res, nil
			}
			visited[fu.norm] = true
		}
		var pageRes Result
		newMatched := c.applyLinks(page.Links, seenURL, &pageRes)
		res.Items = append(res.Items, pageRes.Items...)
		res.Matched += pageRes.Matched
		res.SkippedNoTitle += pageRes.SkippedNoTitle
		res.OtherHost += pageRes.OtherHost
		for _, s := range pageRes.SkippedSamples {
			if len(res.SkippedSamples) < 10 {
				res.SkippedSamples = append(res.SkippedSamples, s)
			}
		}
		res.Pages = append(res.Pages, PageResult{URL: pageURL, FinalURL: page.FinalURL, Title: page.Title, Links: len(page.Links),
			Matched: pageRes.Matched, NewJobs: newMatched, Items: len(pageRes.Items)})
		if newMatched == 0 {
			res.StopReason = StopNoNewJobs
			return res, nil
		}
		if index >= maxPages {
			res.StopReason = StopMaxPages
			return res, nil
		}
		next, ok := c.nextPage(index, page.Links, visited)
		if !ok {
			res.StopReason = StopNoNextPage
			return res, nil
		}
		if visited[next] {
			res.StopReason, res.StopDetail = StopRepeatedPage, next
			return res, nil
		}
		pageURL = next
	}
}
