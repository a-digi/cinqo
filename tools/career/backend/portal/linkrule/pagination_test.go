package linkrule

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

func TestLearnPaginationAsACounter(t *testing.T) {
	for _, c := range []struct {
		name         string
		cur          string
		picks        []string
		template     string
		second, step int
		needsConfirm bool
		next         []string
	}{
		{"page=2 on a first page without a number", "https://x.com/jobs?cat=1", []string{"https://x.com/jobs?cat=1&page=2"},
			"https://x.com/jobs?cat=1&page={n}", 2, 1, false, []string{"https://x.com/jobs?cat=1&page=2", "https://x.com/jobs?cat=1&page=3", "https://x.com/jobs?cat=1&page=4"}},
		{"the first page has page=1", "https://x.com/jobs?page=1", []string{"https://x.com/jobs?page=2"},
			"https://x.com/jobs?page={n}", 2, 1, false, nil},
		{"offset steps from the first page's own number", "https://x.com/jobs?start=0", []string{"https://x.com/jobs?start=20"},
			"https://x.com/jobs?start={n}", 20, 20, false, []string{"https://x.com/jobs?start=20", "https://x.com/jobs?start=40", "https://x.com/jobs?start=60"}},
		{"two picks give the step", "https://x.com/jobs", []string{"https://x.com/jobs?start=20", "https://x.com/jobs?start=40"},
			"https://x.com/jobs?start={n}", 20, 20, false, nil},
		{"picks in any order", "https://x.com/jobs", []string{"https://x.com/jobs?page=3", "https://x.com/jobs?page=2"},
			"https://x.com/jobs?page={n}", 2, 1, false, nil},
		{"one pick without a first number that is not 2 is assumed to step by 1 and needs a confirm", "https://x.com/jobs", []string{"https://x.com/jobs?start=20"},
			"https://x.com/jobs?start={n}", 20, 1, true, nil},
		{"the number sits in the path", "https://x.com/search/filter/latest/1", []string{"https://x.com/search/filter/latest/2"},
			"https://x.com/search/filter/latest/{n}", 2, 1, false, []string{"https://x.com/search/filter/latest/2", "https://x.com/search/filter/latest/3", "https://x.com/search/filter/latest/4"}},
		{"the number is part of a segment", "https://x.com/jobs/page-1", []string{"https://x.com/jobs/page-2"},
			"https://x.com/jobs/page-{n}", 2, 1, false, nil},
		{"the query of the first page is kept", "https://x.com/s/1?q=go", []string{"https://x.com/s/2?q=go"},
			"https://x.com/s/{n}?q=go", 2, 1, false, nil},
		{"the parameter keeps its place among the others", "https://x.com/jobs?a=1&page=1&b=2", []string{"https://x.com/jobs?a=1&page=2&b=2"},
			"https://x.com/jobs?a=1&page={n}&b=2", 2, 1, false, nil},
		{"picks that do not start where the first page says get a note and a confirm", "https://x.com/jobs?page=1", []string{"https://x.com/jobs?page=3", "https://x.com/jobs?page=4"},
			"https://x.com/jobs?page={n}", 3, 1, true, nil},
	} {
		got, err := LearnPagination(c.cur, c.picks, nil)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		pg := got.Pagination
		if pg.Kind != PagesTemplate || pg.Template != c.template || pg.Second != c.second || pg.Step != c.step || pg.MaxPages != DefaultMaxPages {
			t.Errorf("%s: %+v", c.name, pg)
		}
		if got.NeedsConfirm != c.needsConfirm {
			t.Errorf("%s: needsConfirm %v, notes %v", c.name, got.NeedsConfirm, got.Notes)
		}
		if c.needsConfirm && len(got.Notes) == 0 {
			t.Errorf("%s: a confirm needs a note", c.name)
		}
		if c.next != nil && !reflect.DeepEqual(got.NextURLs, c.next) {
			t.Errorf("%s: next urls %v, want %v", c.name, got.NextURLs, c.next)
		}
		if len(got.NextURLs) != 3 {
			t.Errorf("%s: next urls %v", c.name, got.NextURLs)
		}
		// the proposal is a valid pagination for a rule of that host
		host, _ := parseURL(c.cur)
		if err := ValidateRule(Rule{Host: host.host, Include: []string{host.host + "/**"}, Pagination: &pg}); err != nil {
			t.Errorf("%s: the learned pagination does not validate: %v", c.name, err)
		}
	}
}

func TestLearnPaginationFallsBackToALinkPattern(t *testing.T) {
	cands := []Link{
		{URL: "https://x.com/jobs/more/abc"}, {URL: "https://x.com/jobs/more/def"}, {URL: "https://x.com/jobs/1"}, {URL: "https://x.com/about"},
	}
	for name, c := range map[string]struct {
		cur   string
		picks []string
	}{
		"nothing to count":      {"https://x.com/jobs", []string{"https://x.com/jobs/more/abc"}},
		"two different changes": {"https://x.com/jobs?a=1", []string{"https://x.com/jobs?a=2&page=2"}},
		"uneven counter":        {"https://x.com/jobs", []string{"https://x.com/jobs?page=2", "https://x.com/jobs?page=3", "https://x.com/jobs?page=5"}},
		"not a number":          {"https://x.com/jobs", []string{"https://x.com/jobs?page=next"}},
		"different counters":    {"https://x.com/jobs", []string{"https://x.com/jobs?page=2", "https://x.com/jobs?p=3"}},
		"path depth differs":    {"https://x.com/jobs", []string{"https://x.com/jobs/page/2"}},
		"too many digits":       {"https://x.com/jobs?page=1", []string{"https://x.com/jobs?page=1234567"}},
		"duplicate number":      {"https://x.com/jobs", []string{"https://x.com/jobs?page=2", "https://x.com/jobs?page=2&x=1"}},
	} {
		got, err := LearnPagination(c.cur, c.picks, cands)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got.Pagination.Kind != PagesLink || got.Pagination.Pattern == "" || len(got.Notes) == 0 {
			t.Errorf("%s: %+v", name, got)
		}
	}
	got, _ := LearnPagination("https://x.com/jobs", []string{"https://x.com/jobs/more/abc"}, cands)
	if got.Pagination.Pattern != "x.com/jobs/more/*" || !got.NeedsConfirm {
		t.Errorf("single pick: %+v", got)
	}
	if !reflect.DeepEqual(got.NextURLs, []string{"https://x.com/jobs/more/abc", "https://x.com/jobs/more/def"}) {
		t.Errorf("the page's links the pattern matches: %v", got.NextURLs)
	}
	// a broad link pattern needs a confirm
	got, _ = LearnPagination("https://x.com/jobs", []string{"https://x.com/a/1", "https://x.com/b/c/2"}, nil)
	if got.Pagination.Kind != PagesLink || !got.NeedsConfirm {
		t.Errorf("broad: %+v", got)
	}
}

func TestLearnPaginationRefusesBadInput(t *testing.T) {
	if _, err := LearnPagination("not a url", []string{"https://x.com/a"}, nil); !errors.Is(err, ErrBadURL) {
		t.Errorf("bad current: %v", err)
	}
	if _, err := LearnPagination("https://x.com/jobs", nil, nil); err == nil {
		t.Error("no picks")
	}
	if _, err := LearnPagination("https://x.com/jobs", []string{"https://other.com/jobs?page=2"}, nil); !errors.Is(err, ErrMixedHosts) {
		t.Errorf("another site: %v", err)
	}
	if _, err := LearnPagination("https://x.com/jobs", []string{"https://www.x.com/jobs?page=2"}, nil); !errors.Is(err, ErrMixedHosts) {
		t.Errorf("www: %v", err)
	}
	if _, err := LearnPagination("https://x.com/jobs?page=1", []string{"https://x.com/jobs?page=1#top"}, nil); err == nil {
		t.Error("a pick equal to the first page")
	}
	if _, err := LearnPagination("https://x.com/jobs", []string{"javascript:void(0)"}, nil); !errors.Is(err, ErrBadURL) {
		t.Errorf("bad pick: %v", err)
	}
}

// Whatever is picked, a learned template produces every picked address.
func TestLearnedTemplateAlwaysProducesItsPicks(t *testing.T) {
	rnd := rand.New(rand.NewSource(186))
	for i := 0; i < 2000; i++ {
		key := []string{"page", "p", "start", "offset"}[rnd.Intn(4)]
		base := fmt.Sprintf("https://x.com/%s", []string{"jobs", "search/all", "a/b/c"}[rnd.Intn(3)])
		step := 1 + rnd.Intn(30)
		first := rnd.Intn(3)
		cur := base
		if first == 1 {
			cur += fmt.Sprintf("?%s=%d", key, 0)
		} else if first == 2 {
			cur += fmt.Sprintf("?%s=%d&q=go", key, 1)
		}
		n := 1 + rnd.Intn(3)
		var picks []string
		for k := 0; k < n; k++ {
			v := (k + 2) * step
			q := fmt.Sprintf("?%s=%d", key, v)
			if first == 2 {
				q = fmt.Sprintf("?%s=%d&q=go", key, v)
			}
			picks = append(picks, base+q)
		}
		got, err := LearnPagination(cur, picks, nil)
		if err != nil || got.Pagination.Kind != PagesTemplate {
			continue
		}
		produced := map[string]bool{}
		for k := 0; k < 8; k++ {
			u, _ := NormalizeURL(templateURL(got.Pagination.Template, got.Pagination.Second+k*got.Pagination.step()))
			produced[u] = true
		}
		for _, p := range picks {
			np, _ := NormalizeURL(p)
			if !produced[np] {
				t.Fatalf("cur %s picks %v: template %+v does not produce %s", cur, picks, got.Pagination, np)
			}
		}
	}
}

func TestValidatePagination(t *testing.T) {
	base := func(p *Pagination) Rule { return Rule{Host: "x.com", Include: []string{"x.com/j/*"}, Pagination: p} }
	ok := []*Pagination{
		nil, {}, {Kind: PagesNone}, {Kind: PagesNone, Template: "ignored"},
		{Kind: PagesTemplate, Template: "https://x.com/jobs?page={n}", Second: 2},
		{Kind: PagesTemplate, Template: "https://x.com/jobs/{n}", Second: 2, Step: 20, MaxPages: MaxMaxPages},
		{Kind: PagesTemplate, Template: "https://x.com/jobs/page-{n}/", Second: 2, MaxPages: 1},
		{Kind: PagesLink, Pattern: "x.com/jobs/more/*"},
		{Kind: PagesLink, Pattern: "x.com/jobs?page=*", MaxPages: 3},
	}
	for i, p := range ok {
		if err := ValidateRule(base(p)); err != nil {
			t.Errorf("valid #%d (%+v): %v", i, p, err)
		}
	}
	bad := map[string]*Pagination{
		"unknown kind":         {Kind: "scroll"},
		"max pages too big":    {Kind: PagesLink, Pattern: "x.com/m/*", MaxPages: MaxMaxPages + 1},
		"max pages negative":   {Kind: PagesLink, Pattern: "x.com/m/*", MaxPages: -1},
		"no {n}":               {Kind: PagesTemplate, Template: "https://x.com/jobs?page=2", Second: 2},
		"two {n}":              {Kind: PagesTemplate, Template: "https://x.com/{n}?page={n}", Second: 2},
		"other host":           {Kind: PagesTemplate, Template: "https://evil.com/jobs?page={n}", Second: 2},
		"www is another host":  {Kind: PagesTemplate, Template: "https://www.x.com/jobs?page={n}", Second: 2},
		"not a web address":    {Kind: PagesTemplate, Template: "javascript:{n}", Second: 2},
		"relative":             {Kind: PagesTemplate, Template: "/jobs?page={n}", Second: 2},
		"second is 0":          {Kind: PagesTemplate, Template: "https://x.com/jobs?page={n}", Second: 0},
		"second too big":       {Kind: PagesTemplate, Template: "https://x.com/jobs?page={n}", Second: MaxTemplateNumber + 1},
		"step too big":         {Kind: PagesTemplate, Template: "https://x.com/jobs?page={n}", Second: 2, Step: MaxTemplateNumber + 1},
		"step negative":        {Kind: PagesTemplate, Template: "https://x.com/jobs?page={n}", Second: 2, Step: -1},
		"link without pattern": {Kind: PagesLink},
		"link bad pattern":     {Kind: PagesLink, Pattern: "x.com/a b"},
		"link other host":      {Kind: PagesLink, Pattern: "other.com/m/*"},
		"user info":            {Kind: PagesTemplate, Template: "https://u:p@x.com/jobs?page={n}", Second: 2},
		"template too long":    {Kind: PagesTemplate, Template: "https://x.com/" + strings.Repeat("a", MaxURLLen) + "?page={n}", Second: 2},
	}
	for name, p := range bad {
		if err := ValidateRule(base(p)); !errors.Is(err, ErrBadPagination) {
			t.Errorf("%s: got %v, want ErrBadPagination", name, err)
		}
	}
}

func TestNextPage(t *testing.T) {
	tmpl := Rule{Host: "x.com", Include: []string{"x.com/j/*"}, Pagination: &Pagination{Kind: PagesTemplate, Template: "https://x.com/jobs?start={n}", Second: 20, Step: 20}}
	for index, want := range map[int]string{1: "https://x.com/jobs?start=20", 2: "https://x.com/jobs?start=40", 5: "https://x.com/jobs?start=100"} {
		if got, ok := NextPage(tmpl, index, nil, nil); !ok || got != want {
			t.Errorf("template after page %d: %q %v, want %q", index, got, ok, want)
		}
	}
	link := Rule{Host: "x.com", Include: []string{"x.com/j/*"}, Exclude: []string{"x.com/more/skip"},
		Pagination: &Pagination{Kind: PagesLink, Pattern: "x.com/more/*"}}
	links := []Link{
		{URL: "https://x.com/more/skip"},                // excluded
		{URL: "https://x.com/j/1"},                      // a job
		{URL: "https://other.com/more/x"},               // another site
		{URL: "https://x.com/more/seen"},                // visited
		{URL: "https://x.com/more/first"},               // the first match
		{URL: "https://x.com/more/second", Rel: "next"}, // rel=next wins
		{URL: "https://x.com/about"},
	}
	visited := map[string]bool{"https://x.com/more/seen": true}
	if got, ok := NextPage(link, 1, links, visited); !ok || got != "https://x.com/more/second" {
		t.Errorf("rel=next is preferred: %q %v", got, ok)
	}
	links[5].Rel = ""
	if got, ok := NextPage(link, 1, links, visited); !ok || got != "https://x.com/more/first" {
		t.Errorf("first unvisited match: %q %v", got, ok)
	}
	visited["https://x.com/more/first"] = true
	visited["https://x.com/more/second"] = true
	if got, ok := NextPage(link, 1, links, visited); ok {
		t.Errorf("everything visited: %q", got)
	}
	// a pattern that also matches the job links never follows a job link
	jobby := Rule{Host: "x.com", Include: []string{"x.com/j/*"}, Pagination: &Pagination{Kind: PagesLink, Pattern: "x.com/**"}}
	if got, ok := NextPage(jobby, 1, []Link{{URL: "https://x.com/j/1"}, {URL: "https://x.com/j/2"}}, nil); ok {
		t.Errorf("a job link was taken as the next page: %q", got)
	}
	// rel=next that the pattern does not allow is not followed
	if got, ok := NextPage(link, 1, []Link{{URL: "https://x.com/elsewhere", Rel: "next"}}, nil); ok {
		t.Errorf("rel=next outside the pattern: %q", got)
	}
	for name, r := range map[string]Rule{
		"none":    {Host: "x.com", Include: []string{"x.com/j/*"}},
		"invalid": {Host: "x.com"},
	} {
		if got, ok := NextPage(r, 1, links, nil); ok {
			t.Errorf("%s: %q", name, got)
		}
	}
}

// ---- RunPages ----

type pageFixture struct {
	pages map[string]Page
	calls []string
	fail  map[string]error
}

func (f *pageFixture) fetch(ctx context.Context, rawURL string) (Page, error) {
	f.calls = append(f.calls, rawURL)
	if err := f.fail[rawURL]; err != nil {
		return Page{}, err
	}
	p, ok := f.pages[rawURL]
	if !ok {
		return Page{}, fmt.Errorf("no such page %s", rawURL)
	}
	if p.FinalURL == "" {
		p.FinalURL = rawURL
	}
	return p, nil
}

func jobLinks(from, to int) []Link {
	var out []Link
	for i := from; i <= to; i++ {
		out = append(out, Link{URL: fmt.Sprintf("https://x.com/j/%d", i), Texts: []string{fmt.Sprintf("Job number %d", i)}})
	}
	return out
}

var counterRule = Rule{Host: "x.com", Include: []string{"x.com/j/*"}, Pagination: &Pagination{Kind: PagesTemplate, Template: "https://x.com/jobs?page={n}", Second: 2, MaxPages: 10}}

func counterPages(n int) *pageFixture {
	f := &pageFixture{pages: map[string]Page{}}
	f.pages["https://x.com/jobs"] = Page{Links: jobLinks(1, 3)}
	for i := 2; i <= n; i++ {
		f.pages[fmt.Sprintf("https://x.com/jobs?page=%d", i)] = Page{Links: jobLinks((i-1)*3+1, i*3)}
	}
	return f
}

func TestRunPagesOnePage(t *testing.T) {
	f := counterPages(1)
	r := Rule{Host: "x.com", Include: []string{"x.com/j/*"}}
	res, err := RunPages(context.Background(), r, "https://x.com/jobs", f.fetch, 0)
	if err != nil || res.StopReason != StopMaxPages && res.StopReason != StopNoNextPage {
		t.Fatalf("%+v %v", res, err)
	}
	// one page: the limit of a rule without pagination is 1
	if len(res.Pages) != 1 || len(res.Items) != 3 || res.Matched != 3 || len(f.calls) != 1 {
		t.Errorf("%+v calls=%v", res, f.calls)
	}
	if res.StopReason != StopMaxPages {
		t.Errorf("stop reason %q: with no pagination the one page is the page limit", res.StopReason)
	}
}

func TestRunPagesFollowsACounterAndStopsAtTheLimit(t *testing.T) {
	f := counterPages(10)
	r := counterRule
	pg := *r.Pagination
	pg.MaxPages = 4
	r.Pagination = &pg
	res, err := RunPages(context.Background(), r, "https://x.com/jobs", f.fetch, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://x.com/jobs", "https://x.com/jobs?page=2", "https://x.com/jobs?page=3", "https://x.com/jobs?page=4"}
	if !reflect.DeepEqual(f.calls, want) || res.StopReason != StopMaxPages || len(res.Items) != 12 || len(res.Pages) != 4 {
		t.Errorf("calls %v stop %s items %d", f.calls, res.StopReason, len(res.Items))
	}
	for i, p := range res.Pages {
		if p.NewJobs != 3 || p.Items != 3 || p.Matched != 3 || p.URL != want[i] {
			t.Errorf("page %d: %+v", i+1, p)
		}
	}
	// a test caps the pages below the rule's limit
	f2 := counterPages(10)
	res, _ = RunPages(context.Background(), r, "https://x.com/jobs", f2.fetch, 2)
	if len(res.Pages) != 2 || res.StopReason != StopMaxPages || len(f2.calls) != 2 {
		t.Errorf("limit 2: %v %+v", f2.calls, res.StopReason)
	}
	// a limit above the rule's own does not raise it
	f3 := counterPages(10)
	res, _ = RunPages(context.Background(), r, "https://x.com/jobs", f3.fetch, 99)
	if len(res.Pages) != 4 {
		t.Errorf("limit above the rule's: %d pages", len(res.Pages))
	}
	// no maxPages = the default
	r2 := counterRule
	pg2 := *r2.Pagination
	pg2.MaxPages = 0
	r2.Pagination = &pg2
	f4 := counterPages(10)
	res, _ = RunPages(context.Background(), r2, "https://x.com/jobs", f4.fetch, 0)
	if len(res.Pages) != DefaultMaxPages {
		t.Errorf("default limit: %d pages", len(res.Pages))
	}
}

func TestRunPagesStopsWhenAPageBringsNothingNew(t *testing.T) {
	f := counterPages(3)
	f.pages["https://x.com/jobs?page=3"] = Page{Links: jobLinks(4, 6)} // the same jobs as page 2
	res, err := RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != StopNoNewJobs || len(res.Pages) != 3 || len(res.Items) != 6 || res.Pages[2].NewJobs != 0 || len(f.calls) != 3 {
		t.Errorf("%+v calls=%v", res, f.calls)
	}
	// an empty first page: nothing found, nothing followed
	f = &pageFixture{pages: map[string]Page{"https://x.com/jobs": {Links: []Link{{URL: "https://x.com/about"}}}}}
	res, _ = RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0)
	if res.StopReason != StopNoNewJobs || len(res.Pages) != 1 || len(f.calls) != 1 || len(res.Items) != 0 {
		t.Errorf("empty first page: %+v calls=%v", res, f.calls)
	}
}

func TestRunPagesFollowsLinksAndNeverTheSamePageTwice(t *testing.T) {
	r := Rule{Host: "x.com", Include: []string{"x.com/j/*"}, Pagination: &Pagination{Kind: PagesLink, Pattern: "x.com/p/*", MaxPages: 10}}
	f := &pageFixture{pages: map[string]Page{
		"https://x.com/jobs": {Links: append(jobLinks(1, 2), Link{URL: "https://x.com/p/2", Rel: "next"}, Link{URL: "https://x.com/p/other"})},
		"https://x.com/p/2":  {Links: append(jobLinks(3, 4), Link{URL: "https://x.com/jobs"}, Link{URL: "https://x.com/p/2"}, Link{URL: "https://x.com/p/3", Rel: "next"})},
		"https://x.com/p/3":  {Links: append(jobLinks(5, 5), Link{URL: "https://x.com/p/2"})}, // only back links: the end
	}}
	res, err := RunPages(context.Background(), r, "https://x.com/jobs", f.fetch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.calls, []string{"https://x.com/jobs", "https://x.com/p/2", "https://x.com/p/3"}) || res.StopReason != StopNoNextPage || len(res.Items) != 5 {
		t.Errorf("calls %v stop %s items %d", f.calls, res.StopReason, len(res.Items))
	}
}

func TestRunPagesStopsOnARepeatedPage(t *testing.T) {
	// page 2 redirects to the address page 3 would have: the next page is one already loaded
	f := counterPages(3)
	f.pages["https://x.com/jobs?page=2"] = Page{FinalURL: "https://x.com/jobs?page=3", Links: jobLinks(4, 6)}
	res, err := RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != StopRepeatedPage || res.StopDetail != "https://x.com/jobs?page=3" || len(res.Pages) != 2 || len(f.calls) != 2 {
		t.Errorf("%+v calls=%v", res, f.calls)
	}
}

func TestRunPagesAFailureOfTheFirstPageIsAnErrorAndOfALaterOneKeepsWhatWasFound(t *testing.T) {
	boom := errors.New("the site stopped the page")
	f := counterPages(3)
	f.fail = map[string]error{"https://x.com/jobs": boom}
	if res, err := RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0); !errors.Is(err, boom) || len(res.Items) != 0 {
		t.Errorf("first page: %v %+v", err, res)
	}
	f = counterPages(3)
	f.fail = map[string]error{"https://x.com/jobs?page=3": boom}
	res, err := RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0)
	if err != nil {
		t.Fatalf("a later page must not turn the run into an error: %v", err)
	}
	if res.StopReason != StopFetchFailed || !strings.Contains(res.StopDetail, "stopped the page") || len(res.Items) != 6 || len(res.Pages) != 2 {
		t.Errorf("%+v", res)
	}
}

func TestRunPagesStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	f := counterPages(5)
	calls := 0
	fetch := func(c context.Context, u string) (Page, error) {
		calls++
		if calls == 2 {
			cancel() // the person stops the crawl while page 2 loads
			return Page{}, c.Err()
		}
		return f.fetch(c, u)
	}
	res, err := RunPages(ctx, counterRule, "https://x.com/jobs", fetch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.StopReason != StopCancelled || len(res.Items) != 3 || calls != 2 {
		t.Errorf("%+v calls=%d", res, calls)
	}
	// already cancelled before the first page
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	f2 := counterPages(2)
	res, err = RunPages(ctx2, counterRule, "https://x.com/jobs", f2.fetch, 0)
	if err != nil || res.StopReason != StopCancelled || len(f2.calls) != 0 {
		t.Errorf("%+v %v calls=%v", res, err, f2.calls)
	}
}

func TestRunPagesStaysOnTheRulesSite(t *testing.T) {
	f := counterPages(3)
	f.pages["https://x.com/jobs?page=2"] = Page{FinalURL: "https://elsewhere.example/landing", Links: jobLinks(4, 6)}
	res, _ := RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0)
	if res.StopReason != StopLeftTheSite || len(res.Items) != 3 {
		t.Errorf("%+v", res)
	}
	if _, err := RunPages(context.Background(), counterRule, "https://other.example/jobs", f.fetch, 0); !errors.Is(err, ErrHostMismatch) {
		t.Errorf("first page of another site: %v", err)
	}
	if _, err := RunPages(context.Background(), counterRule, "javascript:alert(1)", f.fetch, 0); !errors.Is(err, ErrBadURL) {
		t.Errorf("bad first page: %v", err)
	}
	if _, err := RunPages(context.Background(), Rule{Host: "x.com"}, "https://x.com/jobs", f.fetch, 0); !errors.Is(err, ErrNoInclude) {
		t.Errorf("invalid rule: %v", err)
	}
	if len(f.calls) != 2 {
		t.Errorf("a refused run reached the fetch: %v", f.calls)
	}
}

func TestRunPagesCollectsOneItemPerAddressAndCountsWhatIsSkipped(t *testing.T) {
	f := counterPages(2)
	// page 2 repeats one job of page 1 and has a link without text
	f.pages["https://x.com/jobs?page=2"] = Page{Links: []Link{
		{URL: "https://x.com/j/1", Texts: []string{"Job number 1"}}, // already found
		{URL: "https://x.com/j/10", Texts: []string{"Job number 10"}},
		{URL: "https://x.com/j/11"}, // no text
	}}
	res, err := RunPages(context.Background(), counterRule, "https://x.com/jobs", f.fetch, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 4 || res.Matched != 5 || res.SkippedNoTitle != 1 || !reflect.DeepEqual(res.SkippedSamples, []string{"https://x.com/j/11"}) {
		t.Errorf("%+v", res)
	}
	if res.Pages[1].NewJobs != 2 || res.Pages[1].Matched != 2 || res.Pages[1].Items != 1 {
		t.Errorf("page 2: %+v", res.Pages[1])
	}
	seen := map[string]bool{}
	for _, it := range res.Items {
		if seen[it.URL] {
			t.Errorf("duplicate item %s", it.URL)
		}
		seen[it.URL] = true
	}
}

// Nothing a page or a person can type may panic the pagination entry points, and a proposal is always a valid pagination.
func FuzzPagination(f *testing.F) {
	for _, s := range [][3]string{
		{"https://x.com/jobs", "https://x.com/jobs?page=2", "https://x.com/jobs?page=3"},
		{"https://x.com/s/1", "https://x.com/s/2", ""},
		{"https://x.com/jobs/page-1", "https://x.com/jobs/page-2", "x"},
		{"x", "y", "z"},
	} {
		f.Add(s[0], s[1], s[2])
	}
	f.Fuzz(func(t *testing.T, cur, a, b string) {
		prop, err := LearnPagination(cur, []string{a, b}, []Link{{URL: a}, {URL: b, Rel: "next"}})
		if err != nil {
			return
		}
		host := ""
		if u, ok := parseURL(cur); ok {
			host = u.host
		}
		rule := Rule{Host: host, Include: []string{host + "/**"}, Pagination: &prop.Pagination}
		if err := ValidateRule(rule); err != nil {
			t.Fatalf("cur %q picks %q %q: the learned pagination %+v does not validate: %v", cur, a, b, prop.Pagination, err)
		}
		_, _ = NextPage(rule, 1, []Link{{URL: a}, {URL: b}}, nil)
	})
}
