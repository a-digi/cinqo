package linkrule

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeURL(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"https://Example.COM/Jobs?a=1#top", "https://example.com/Jobs?a=1", true},
		{"https://example.com", "https://example.com/", true},
		{"HTTP://example.com:8080/x", "http://example.com:8080/x", true},
		{"  https://example.com/a  ", "https://example.com/a", true},
		{"https://example.com/jobs/121-ü", "https://example.com/jobs/121-%C3%BC", true},
		{"javascript:void(0)", "", false}, {"mailto:a@b.c", "", false}, {"ftp://example.com/x", "", false},
		{"//example.com/x", "", false}, {"/relative", "", false}, {"", "", false}, {"   ", "", false},
		{"https://user:pw@example.com/x", "", false}, {"https:///nohost", "", false},
	} {
		got, ok := NormalizeURL(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("%q: got %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestCompileRefusesBadPatterns(t *testing.T) {
	for _, c := range []struct {
		in   string
		want error
	}{
		{"", ErrEmptyPattern}, {"   ", ErrEmptyPattern},
		{strings.Repeat("a", MaxPatternLen+1), ErrPatternTooLong},
		{"/jobs/*", ErrPatternNoHost}, {"https:///jobs", ErrPatternNoHost}, {"?id=*", ErrPatternNoHost},
		{"*.example.com/jobs/*", ErrPatternHostWildcard}, {"exam*ple.com/x", ErrPatternHostWildcard}, {"*/jobs", ErrPatternHostWildcard},
		{"example.com/jobs with space", ErrBadPatternChars}, {"example.com/a\nb", ErrBadPatternChars}, {"example.com/a\tb", ErrBadPatternChars},
		{"example.com/x?", ErrBadQueryPattern}, {"example.com/x?=1", ErrBadQueryPattern}, {"example.com/x?*=1", ErrBadQueryPattern}, {"example.com/x?a*=1", ErrBadQueryPattern}, {"example.com/x?&&", ErrBadQueryPattern},
	} {
		if _, err := Compile(c.in); !errors.Is(err, c.want) {
			t.Errorf("%q: got %v, want %v", truncate(c.in, 30), err, c.want)
		}
	}
	for _, ok := range []string{"example.com", "example.com/", "https://example.com/jobs/*", "HTTP://Example.com/Jobs/**", "example.com/a/b/c?x=1&y=*&z", "example.com:8080/x", strings.Repeat("a", MaxPatternLen-12) + ".com/x"} {
		if _, err := Compile(ok); err != nil {
			t.Errorf("%q: %v", truncate(ok, 30), err)
		}
	}
}

func TestMatchSemantics(t *testing.T) {
	type tc struct {
		pattern string
		match   []string
		not     []string
	}
	for _, c := range []tc{
		{"duapune.com/jobs/*",
			[]string{"https://duapune.com/jobs/121888-backend-developer", "http://duapune.com/jobs/1", "https://DUAPUNE.com/jobs/x/", "https://duapune.com/jobs/121888-backend-developer?utm=1#a"},
			[]string{"https://duapune.com/jobs/", "https://duapune.com/jobs", "https://duapune.com/jobs/1/apply", "https://duapune.com/job/1", "https://www.duapune.com/jobs/1", "https://duapune.com/employers/2rm-lab", "https://other.com/jobs/1", "https://duapune.com/"}},
		{"example.com/jobs/**",
			[]string{"https://example.com/jobs/1", "https://example.com/jobs/1/apply", "https://example.com/jobs/a/b/c/d"},
			[]string{"https://example.com/jobs", "https://example.com/jobs/", "https://example.com/other/1"}},
		{"example.com/job-*/apply",
			[]string{"https://example.com/job-12/apply", "https://example.com/job-/apply"},
			[]string{"https://example.com/job/12/apply", "https://example.com/job-12", "https://example.com/xjob-12/apply"}},
		{"example.com/*/jobs/*",
			[]string{"https://example.com/de/jobs/5", "https://example.com/en/jobs/x-y"},
			[]string{"https://example.com/jobs/5", "https://example.com/a/b/jobs/5"}},
		{"example.com/view?id=*",
			[]string{"https://example.com/view?id=5", "https://example.com/view?id=5&utm=1", "https://example.com/view?utm=1&id=abc", "https://example.com/view/?id=9"},
			[]string{"https://example.com/view", "https://example.com/view?ID=5", "https://example.com/other?id=5", "https://example.com/view?x=5"}},
		{"example.com/view?id=7&lang=de",
			[]string{"https://example.com/view?lang=de&id=7&x=1"},
			[]string{"https://example.com/view?id=7", "https://example.com/view?id=7&lang=en", "https://example.com/view?id=8&lang=de"}},
		{"example.com/view?flag",
			[]string{"https://example.com/view?flag", "https://example.com/view?flag=1&x=2"},
			[]string{"https://example.com/view", "https://example.com/view?other=1"}},
		{"example.com", []string{"https://example.com", "https://example.com/", "http://EXAMPLE.com"}, []string{"https://example.com/a", "https://x.example.com/"}},
		{"example.com/", []string{"https://example.com/"}, []string{"https://example.com/a"}},
		{"example.com/**", []string{"https://example.com/a", "https://example.com/a/b"}, []string{"https://example.com/", "https://example.com"}},
		{"example.com/*", []string{"https://example.com/a"}, []string{"https://example.com/a/b", "https://example.com/"}},
		{"example.com:8080/x/*", []string{"http://example.com:8080/x/1"}, []string{"http://example.com/x/1", "http://example.com:9090/x/1"}},
		{"example.com/jobs/*", []string{"https://example.com/jobs/121-%C3%BC", "https://example.com/jobs/121-ü"}, []string{}},
		{"example.com/jobs/121-ü", []string{"https://example.com/jobs/121-ü", "https://example.com/jobs/121-%C3%BC"}, []string{"https://example.com/jobs/121-u"}},
		{"example.com/Jobs/*", []string{"https://example.com/Jobs/1"}, []string{"https://example.com/jobs/1"}},
		{"https://example.com/jobs/*", []string{"http://example.com/jobs/1"}, nil},
		{"example.com/a/**/z", []string{"https://example.com/a/b/z", "https://example.com/a/b/c/z", "https://example.com/a//z"}, []string{"https://example.com/a/z", "https://example.com/a/b/c"}},
		{"example.com/***/x", []string{"https://example.com/a/b/x"}, nil}, // three stars = **
	} {
		p, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("%q: %v", c.pattern, err)
			continue
		}
		for _, u := range c.match {
			if !p.Match(u) {
				t.Errorf("%q should match %s", c.pattern, u)
			}
		}
		for _, u := range c.not {
			if p.Match(u) {
				t.Errorf("%q must not match %s", c.pattern, u)
			}
		}
	}
}

func TestMatchNeverMatchesWhatIsNotAWebAddress(t *testing.T) {
	p, _ := Compile("example.com/**")
	for _, u := range []string{"", "javascript:alert(1)", "mailto:a@example.com", "/relative", "ftp://example.com/x", "https://user:pw@example.com/x", "https://example.com/" + strings.Repeat("a", MaxURLLen)} {
		if p.Match(u) {
			t.Errorf("matched %q", truncate(u, 40))
		}
	}
	var zero *Pattern
	if zero.Match("https://example.com/x") || (&Pattern{}).Match("https://example.com/x") {
		t.Error("a nil or zero pattern must match nothing")
	}
}

// The matcher is a bounded table walk, not a backtracking engine: hostile
// patterns on hostile URLs finish at once.
func TestMatcherCostIsBounded(t *testing.T) {
	pat := "example.com/" + strings.Repeat("*a", 100) + "b"
	p, err := Compile(pat)
	if err != nil {
		t.Fatal(err)
	}
	long := "https://example.com/" + strings.Repeat("a", MaxURLLen-40)
	start := time.Now()
	for i := 0; i < 20; i++ {
		if p.Match(long) {
			t.Fatal("must not match")
		}
	}
	dbl, _ := Compile("example.com/" + strings.Repeat("**a", 90))
	for i := 0; i < 20; i++ {
		dbl.Match(long)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("hostile patterns took %s", d)
	}
}

func parsedAll(t *testing.T, urls ...string) []parsedURL {
	t.Helper()
	var out []parsedURL
	for _, u := range urls {
		p, ok := parseURL(u)
		if !ok {
			t.Fatalf("bad url %s", u)
		}
		out = append(out, p)
	}
	return out
}

func TestLearnFromPicks(t *testing.T) {
	for _, c := range []struct {
		name         string
		picks        []string
		pattern      string
		needsConfirm bool
	}{
		{"duapune jobs (the real case)", []string{"https://duapune.com/jobs/121888-backend-developer", "https://duapune.com/jobs/121726-devops-engineer"}, "duapune.com/jobs/#-*", false},
		{"three picks, ids differ", []string{"https://example.com/job/1", "https://example.com/job/22", "https://example.com/job/333"}, "example.com/job/#", false},
		{"shared word prefix kept at a boundary", []string{"https://example.com/jobs/job-12", "https://example.com/jobs/job-98"}, "example.com/jobs/job-*", false},
		{"accidental shared digits are not kept: the number is #", []string{"https://example.com/jobs/121888-a", "https://example.com/jobs/121726-b"}, "example.com/jobs/#-*", false},
		{"shared suffix kept at a boundary", []string{"https://example.com/jobs/12.html", "https://example.com/jobs/98.html"}, "example.com/jobs/#.html", false},
		{"varying middle segment", []string{"https://example.com/search/city/tirane/jobs/1", "https://example.com/search/city/durres/jobs/2"}, "example.com/search/city/*/jobs/#", false},
		{"tracking params ignored when the path varies", []string{"https://example.com/job/1?utm=a", "https://example.com/job/2?utm=b&x=1"}, "example.com/job/#", false},
		{"same path, id in the query", []string{"https://example.com/view?id=5&lang=de", "https://example.com/view?id=9&lang=de"}, "example.com/view?id=*&lang=de", false},
		{"same path, varying query key set keeps common keys only", []string{"https://example.com/view?id=5&x=1", "https://example.com/view?id=9"}, "example.com/view?id=*", false},
		{"different depths: shared start then anything below", []string{"https://example.com/careers/eng/1", "https://example.com/careers/eng/team/2"}, "example.com/careers/eng/**", false},
		{"different depths with nothing shared: broad, confirm", []string{"https://example.com/a/1", "https://example.com/b/c/2"}, "example.com/**", true},
		{"one pick: the id and the slug are generalised, confirm", []string{"https://duapune.com/jobs/121888-backend-developer"}, "duapune.com/jobs/#-*", true},
		{"one pick, a word: the last segment is *", []string{"https://example.com/jobs/backend"}, "example.com/jobs/*", true},
		{"one pick, a plain id", []string{"https://www.python.org/jobs/8195/"}, "www.python.org/jobs/#", true},
		{"one pick with digit segment", []string{"https://example.com/jobs/42/details"}, "example.com/jobs/#/*", true},
		{"one pick with a query keeps the path", []string{"https://example.com/view?id=5&utm=x"}, "example.com/view?id=*&utm=*", true},
		{"one pick at the root", []string{"https://example.com/"}, "example.com", true},
		{"picks given twice count once", []string{"https://example.com/job/1", "https://example.com/job/1/", "https://example.com/job/1#x", "https://example.com/job/2"}, "example.com/job/#", false},
		{"identical path and query apart from a value", []string{"https://example.com/p?a=1&b=x", "https://example.com/p?a=2&b=x"}, "example.com/p?a=*&b=x", false},
	} {
		got, err := LearnJobPattern(c.picks, nil, nil)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.Pattern != c.pattern {
			t.Errorf("%s: pattern %q, want %q (notes %v)", c.name, got.Pattern, c.pattern, got.Notes)
		}
		if got.NeedsConfirm != c.needsConfirm {
			t.Errorf("%s: needsConfirm %v, want %v", c.name, got.NeedsConfirm, c.needsConfirm)
		}
		if len(got.Unmatched) != 0 {
			t.Errorf("%s: the pattern misses picked links %v", c.name, got.Unmatched)
		}
		if len(got.Notes) == 0 && len(c.picks) == 1 {
			t.Errorf("%s: a single pick must come with a note", c.name)
		}
	}
}

func TestLearnRefusesBadInput(t *testing.T) {
	if _, err := LearnJobPattern(nil, nil, nil); !errors.Is(err, ErrNoPicks) {
		t.Errorf("no picks: %v", err)
	}
	if _, err := LearnJobPattern([]string{}, nil, nil); !errors.Is(err, ErrNoPicks) {
		t.Errorf("empty picks: %v", err)
	}
	if _, err := LearnJobPattern([]string{"https://a.com/x", "https://b.com/x"}, nil, nil); !errors.Is(err, ErrMixedHosts) {
		t.Errorf("mixed hosts: %v", err)
	}
	if _, err := LearnJobPattern([]string{"https://a.com/", "https://a.com/job/1"}, nil, nil); !errors.Is(err, ErrNoCommonPattern) {
		t.Errorf("start page with a job: %v", err)
	}
	if _, err := LearnJobPattern([]string{"https://a.com/x", "https://www.a.com/x"}, nil, nil); !errors.Is(err, ErrMixedHosts) {
		t.Errorf("www is another host: %v", err)
	}
	for _, bad := range []string{"", "javascript:alert(1)", "mailto:a@b.c", "/relative", "not a url", "https://u:p@a.com/x"} {
		if _, err := LearnJobPattern([]string{bad}, nil, nil); !errors.Is(err, ErrBadURL) {
			t.Errorf("pick %q: %v", bad, err)
		}
		if _, err := LearnJobPattern([]string{"https://a.com/x/1"}, []string{bad}, nil); !errors.Is(err, ErrBadURL) {
			t.Errorf("notJob %q: %v", bad, err)
		}
	}
}

func links(urls ...string) []Link {
	var out []Link
	for _, u := range urls {
		out = append(out, Link{URL: u})
	}
	return out
}

func TestProposalShowsWhatThePatternDoesOnThePage(t *testing.T) {
	page := links(
		"https://duapune.com/", "https://duapune.com/login", "https://duapune.com/employers/2rm-lab",
		"https://duapune.com/jobs/121888-backend-developer", "https://duapune.com/jobs/121726-devops-engineer",
		"https://duapune.com/jobs/121888-backend-developer", // repeated
		"https://duapune.com/jobs/777-qa", "https://duapune.com/jobs/121888-backend-developer#apply",
		"https://wa.me/355692031512", "https://duapune.com/search/city/tirane",
	)
	p, err := LearnJobPattern([]string{"https://duapune.com/jobs/121888-backend-developer", "https://duapune.com/jobs/121726-devops-engineer"}, nil, page)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://duapune.com/jobs/121888-backend-developer", "https://duapune.com/jobs/121726-devops-engineer", "https://duapune.com/jobs/777-qa"}
	if !reflect.DeepEqual(p.Matches, want) || p.MatchCount != 3 || p.Host != "duapune.com" {
		t.Errorf("matches %v count %d host %s", p.Matches, p.MatchCount, p.Host)
	}
	if p.NeedsConfirm || len(p.Unmatched) != 0 {
		t.Errorf("%+v", p)
	}
}

func TestNotJobLinksAreKeptOutWhenTheyCanBeSeparated(t *testing.T) {
	// slugs without numbers: a "featured" page looks exactly like a job to any pattern, so it cannot be kept out, and the tool says so
	page := links(
		"https://example.com/jobs/backend", "https://example.com/jobs/frontend",
		"https://example.com/jobs/featured", "https://example.com/jobs/latest",
	)
	p, err := LearnJobPattern([]string{"https://example.com/jobs/backend", "https://example.com/jobs/frontend"},
		[]string{"https://example.com/jobs/featured"}, page)
	if err != nil {
		t.Fatal(err)
	}
	// "featured" is matched by jobs/* ; its own exclusion pattern keeps it out unless it would also match a pick
	if p.Pattern != "example.com/jobs/*" {
		t.Fatalf("pattern %q", p.Pattern)
	}
	// exclusion learned from "featured" is example.com/jobs/* which matches the picks: cannot separate
	if len(p.Exclude) != 0 || !reflect.DeepEqual(p.ExcludedStillMatched, []string{"https://example.com/jobs/featured"}) {
		t.Errorf("exclude=%v still=%v", p.Exclude, p.ExcludedStillMatched)
	}
	found := false
	for _, n := range p.Notes {
		if strings.Contains(n, "not a job") {
			found = true
		}
	}
	if !found {
		t.Errorf("the notes must say a not-a-job link could not be kept out: %v", p.Notes)
	}

	// with ids the same page needs no help at all: # is a number, "featured" is a word
	idPage := links("https://example.com/jobs/1", "https://example.com/jobs/2", "https://example.com/jobs/featured", "https://example.com/jobs/latest")
	pn, err := LearnJobPattern([]string{"https://example.com/jobs/1", "https://example.com/jobs/2"}, []string{"https://example.com/jobs/featured"}, idPage)
	if err != nil {
		t.Fatal(err)
	}
	if pn.Pattern != "example.com/jobs/#" || pn.MatchCount != 2 || len(pn.ExcludedStillMatched) != 0 || len(pn.Exclude) != 0 {
		t.Errorf("number pattern: %+v", pn)
	}

	// separable: the not-a-job link has another shape (a deeper path) that no pick has
	page2 := links("https://example.com/jobs/1", "https://example.com/jobs/2", "https://example.com/jobs/tags/go")
	p2, err := LearnJobPattern([]string{"https://example.com/jobs/1", "https://example.com/jobs/2"}, []string{"https://example.com/jobs/tags/go"}, page2)
	if err != nil {
		t.Fatal(err)
	}
	// jobs/* does not match jobs/tags/go (a * never crosses /), so nothing to exclude and it is not matched
	if len(p2.Exclude) != 0 || len(p2.ExcludedStillMatched) != 0 || p2.MatchCount != 2 {
		t.Errorf("%+v", p2)
	}

	// a broad pattern from picks of different depths, and a not-a-job link it would take: an exclusion is proposed
	picks := []string{"https://example.com/careers/eng/1", "https://example.com/careers/eng/team/2"}
	page3 := links(picks[0], picks[1], "https://example.com/careers/eng/blog/hello")
	p3, err := LearnJobPattern(picks, []string{"https://example.com/careers/eng/blog/hello"}, page3)
	if err != nil {
		t.Fatal(err)
	}
	if p3.Pattern != "example.com/careers/eng/**" || len(p3.ExcludedStillMatched) != 0 && len(p3.Exclude) == 0 {
		t.Errorf("%+v", p3)
	}
	for _, m := range p3.Matches {
		if strings.Contains(m, "/blog/") && len(p3.Exclude) > 0 {
			t.Errorf("an excluded link is still in the matches: %v", p3.Matches)
		}
	}
}

func TestProposalNeedsConfirmWhenItMatchesTooMuch(t *testing.T) {
	var page []Link
	for i := 0; i < BroadMatchLimit+5; i++ {
		page = append(page, Link{URL: fmt.Sprintf("https://example.com/job/%d", i)})
	}
	p, err := LearnJobPattern([]string{"https://example.com/job/1", "https://example.com/job/2"}, nil, page)
	if err != nil {
		t.Fatal(err)
	}
	if !p.NeedsConfirm || p.MatchCount != BroadMatchLimit+5 {
		t.Errorf("needsConfirm=%v count=%d", p.NeedsConfirm, p.MatchCount)
	}
	// exactly at the limit is fine
	p, _ = LearnJobPattern([]string{"https://example.com/job/1", "https://example.com/job/2"}, nil, page[:BroadMatchLimit])
	if p.NeedsConfirm {
		t.Error("at the limit no confirm is needed")
	}
}

func TestProposalSaysWhenAPickIsNotInThePageList(t *testing.T) {
	p, _ := LearnJobPattern([]string{"https://example.com/job/1", "https://example.com/job/2"}, nil, links("https://example.com/job/1"))
	joined := strings.Join(p.Notes, " ")
	if !strings.Contains(joined, "not in this page's link list") {
		t.Errorf("notes: %v", p.Notes)
	}
	p, _ = LearnJobPattern([]string{"https://example.com/job/1", "https://example.com/job/2"}, nil, nil)
	if strings.Contains(strings.Join(p.Notes, " "), "link list") {
		t.Error("with no page list there is nothing to compare")
	}
}

// Whatever is picked, the proposal matches every pick; it never panics.
func TestLearnedPatternAlwaysMatchesItsPicks(t *testing.T) {
	rnd := rand.New(rand.NewSource(185))
	words := []string{"jobs", "job", "view", "careers", "eng", "de", "en", "x", "a-b", "1", "22", "job-7", "ü", "a.b", "q_r", "%41", "***", "a*b", "tags"}
	rndSeg := func() string { return words[rnd.Intn(len(words))] }
	for i := 0; i < 3000; i++ {
		n := 1 + rnd.Intn(4)
		var picks []string
		for k := 0; k < n; k++ {
			depth := rnd.Intn(4)
			var segs []string
			for d := 0; d < depth; d++ {
				segs = append(segs, rndSeg())
			}
			u := "https://example.com/" + strings.Join(segs, "/")
			if rnd.Intn(3) == 0 {
				u += fmt.Sprintf("?id=%d&k=%s", rnd.Intn(5), rndSeg())
			}
			if rnd.Intn(5) == 0 {
				u += "#frag"
			}
			picks = append(picks, u)
		}
		p, err := LearnJobPattern(picks, nil, nil)
		if err != nil {
			continue // an unusable pick (bad characters) is refused, not guessed
		}
		pat, err := Compile(p.Pattern)
		if err != nil {
			t.Fatalf("picks %v: proposed pattern %q does not compile: %v", picks, p.Pattern, err)
		}
		for _, u := range picks {
			if !pat.Match(u) {
				t.Fatalf("picks %v: pattern %q does not match %s", picks, p.Pattern, u)
			}
		}
		if len(p.Unmatched) != 0 {
			t.Fatalf("picks %v: unmatched %v", picks, p.Unmatched)
		}
	}
}

// ---- titles ----

func TestPickTitle(t *testing.T) {
	generic := map[string]bool{"më shumë": true, "apply": true}
	for _, c := range []struct {
		name string
		link Link
		want string
	}{
		{"the longest text wins over a short one", Link{Texts: []string{"Backend Developer", "Më shumë"}}, "Backend Developer"},
		{"order does not matter", Link{Texts: []string{"Më shumë", "Backend Developer"}}, "Backend Developer"},
		{"of two real texts the longer is the title", Link{Texts: []string{"Dev", "Senior Backend Developer"}}, "Senior Backend Developer"},
		{"of two real texts the longer is the title (other order)", Link{Texts: []string{"Senior Backend Developer", "Dev"}}, "Senior Backend Developer"},
		{"a tie keeps the first text", Link{Texts: []string{"Alpha", "Bravo"}}, "Alpha"},
		{"generic texts are never titles", Link{Texts: []string{"Apply"}}, ""},
		{"generic is judged folded and spaced", Link{Texts: []string{"  APPLY  "}}, ""},
		{"an icon is not a title", Link{Texts: []string{"→", "✓", "•"}}, ""},
		{"title attribute when no text", Link{Texts: nil, TitleAttr: "Job two"}, "Job two"},
		{"text beats the title attribute", Link{Texts: []string{"Short"}, TitleAttr: "A much longer title attribute"}, "Short"},
		{"generic title attribute is refused", Link{TitleAttr: "Apply"}, ""},
		{"nothing at all", Link{}, ""},
		{"whitespace is collapsed", Link{Texts: []string{"Senior \n  Go   Engineer"}}, "Senior Go Engineer"},
		{"a digit-only title counts", Link{Texts: []string{"2024"}}, "2024"},
		{"unicode", Link{Texts: []string{"Inxhinier Informatike"}}, "Inxhinier Informatike"},
	} {
		if got := PickTitle(c.link, generic); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGenericTexts(t *testing.T) {
	var page []Link
	for i := 0; i < 4; i++ {
		page = append(page, Link{URL: fmt.Sprintf("https://x.test/jobs/%d", i), Texts: []string{fmt.Sprintf("Job number %d title", i), "Read more"}})
	}
	// a title that really repeats (one text per URL) is not generic, and neither is a text on only two URLs
	for i := 10; i < 15; i++ {
		page = append(page, Link{URL: fmt.Sprintf("https://x.test/jobs/%d", i), Texts: []string{"Software Engineer"}})
	}
	page = append(page,
		Link{URL: "https://x.test/a", Texts: []string{"Only twice", "Hi"}}, Link{URL: "https://x.test/b", Texts: []string{"Other twice", "Hi"}},
		Link{URL: "javascript:void(0)", Texts: []string{"Read more"}},
	)
	g := GenericTexts(page)
	if !g["read more"] {
		t.Error("'Read more' repeats on 4 URLs and always loses: generic")
	}
	if g["software engineer"] {
		t.Error("a job title repeated with one text per URL must be kept")
	}
	if g["hi"] {
		t.Error("two URLs are not enough to call a text generic")
	}
	if len(g) != 1 {
		t.Errorf("generic = %v", g)
	}
}

func TestPickTitleWithGenericDetectionOnARealisticPage(t *testing.T) {
	page := []Link{
		{URL: "https://duapune.com/jobs/121888-backend-developer", Texts: []string{"Backend Developer", "Më shumë"}},
		{URL: "https://duapune.com/jobs/121726-devops-engineer", Texts: []string{"DevOps Engineer", "Më shumë"}},
		{URL: "https://duapune.com/jobs/1-qa", Texts: []string{"QA Engineer", "Më shumë"}},
		{URL: "https://duapune.com/jobs/2-pm", Texts: []string{"Më shumë"}}, // only the generic text
	}
	g := GenericTexts(page)
	want := []string{"Backend Developer", "DevOps Engineer", "QA Engineer", ""}
	for i, l := range page {
		if got := PickTitle(l, g); got != want[i] {
			t.Errorf("%s: %q, want %q", l.URL, got, want[i])
		}
	}
}

// ---- rules ----

func TestValidateRule(t *testing.T) {
	ok := Rule{Host: "duapune.com", Include: []string{"duapune.com/jobs/*"}}
	if err := ValidateRule(ok); err != nil {
		t.Fatal(err)
	}
	many := func(n int, host string) []string {
		var s []string
		for i := 0; i < n; i++ {
			s = append(s, fmt.Sprintf("%s/p%d/*", host, i))
		}
		return s
	}
	for _, c := range []struct {
		name string
		r    Rule
		want error
	}{
		{"no host", Rule{Include: []string{"a.com/*"}}, ErrNoHost},
		{"wildcard host", Rule{Host: "*.a.com", Include: []string{"a.com/*"}}, ErrNoHost},
		{"host with a path", Rule{Host: "a.com/jobs", Include: []string{"a.com/*"}}, ErrNoHost},
		{"no include", Rule{Host: "a.com"}, ErrNoInclude},
		{"pattern of another host", Rule{Host: "a.com", Include: []string{"b.com/jobs/*"}}, ErrHostMismatch},
		{"exclude of another host", Rule{Host: "a.com", Include: []string{"a.com/*"}, Exclude: []string{"b.com/x"}}, ErrHostMismatch},
		{"www is another host", Rule{Host: "a.com", Include: []string{"www.a.com/*"}}, ErrHostMismatch},
		{"too many includes", Rule{Host: "a.com", Include: many(MaxIncludes+1, "a.com")}, ErrTooManyRules},
		{"too many excludes", Rule{Host: "a.com", Include: []string{"a.com/*"}, Exclude: many(MaxExcludes+1, "a.com")}, ErrTooManyRules},
		{"bad pattern", Rule{Host: "a.com", Include: []string{"a.com/a b"}}, ErrBadPatternChars},
		{"empty pattern", Rule{Host: "a.com", Include: []string{""}}, ErrEmptyPattern},
		{"host is case-insensitive", Rule{Host: "A.com", Include: []string{"a.com/*"}}, nil},
		{"max includes ok", Rule{Host: "a.com", Include: many(MaxIncludes, "a.com")}, nil},
	} {
		err := ValidateRule(c.r)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, err, c.want)
		}
	}
}

func TestApplyFindsTheJobsOfAPage(t *testing.T) {
	page := []Link{
		{URL: "https://duapune.com/", Texts: []string{"duapune"}},
		{URL: "https://duapune.com/login", Texts: []string{"Identifikohu"}},
		{URL: "https://duapune.com/jobs/121888-backend-developer", Texts: []string{"Backend Developer", "Më shumë"}},
		{URL: "https://duapune.com/jobs/121726-devops-engineer", Texts: []string{"DevOps Engineer", "Më shumë"}},
		{URL: "https://duapune.com/jobs/3-qa", Texts: []string{"QA Engineer", "Më shumë"}},
		{URL: "https://duapune.com/jobs/121888-backend-developer#apply", Texts: []string{"Apply now"}}, // same URL again
		{URL: "https://duapune.com/jobs/4-icon-only", Texts: []string{"→"}},
		{URL: "https://duapune.com/jobs/5-no-text"},
		{URL: "https://duapune.com/jobs/featured", Texts: []string{"Featured jobs"}},
		{URL: "https://duapune.com/employers/2rm-lab", Texts: []string{"2RM Lab"}},
		{URL: "https://wa.me/355692031512", Texts: []string{"WhatsApp"}},
		{URL: "mailto:a@b.c", Texts: []string{"mail"}},
	}
	res, err := Apply(Rule{Host: "duapune.com", Include: []string{"duapune.com/jobs/*"}, Exclude: []string{"duapune.com/jobs/featured"}}, page)
	if err != nil {
		t.Fatal(err)
	}
	want := []Item{
		{"Backend Developer", "https://duapune.com/jobs/121888-backend-developer"},
		{"DevOps Engineer", "https://duapune.com/jobs/121726-devops-engineer"},
		{"QA Engineer", "https://duapune.com/jobs/3-qa"},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Errorf("items %+v", res.Items)
	}
	if res.Matched != 5 || res.SkippedNoTitle != 2 || res.OtherHost != 1 || len(res.SkippedSamples) != 2 {
		t.Errorf("matched=%d skipped=%d otherHost=%d samples=%v", res.Matched, res.SkippedNoTitle, res.OtherHost, res.SkippedSamples)
	}
}

func TestApplyWithAnInvalidRuleReturnsNothing(t *testing.T) {
	res, err := Apply(Rule{Host: "a.com"}, links("https://a.com/x"))
	if !errors.Is(err, ErrNoInclude) || len(res.Items) != 0 {
		t.Errorf("%v %+v", err, res)
	}
}

func TestApplyEdgeCases(t *testing.T) {
	r := Rule{Host: "a.com", Include: []string{"a.com/j/*"}}
	if res, _ := Apply(r, nil); res.Items == nil || len(res.Items) != 0 || res.Matched != 0 {
		t.Errorf("an empty page: %+v (Items must be an empty list, not null)", res)
	}
	// the exclusion wins over the inclusion
	res, _ := Apply(Rule{Host: "a.com", Include: []string{"a.com/j/*"}, Exclude: []string{"a.com/j/*"}}, []Link{{URL: "https://a.com/j/1", Texts: []string{"One"}}})
	if len(res.Items) != 0 || res.Matched != 0 {
		t.Errorf("exclusion must win: %+v", res)
	}
	// the host of the rule is compared case-insensitively
	res, _ = Apply(Rule{Host: "A.COM", Include: []string{"a.com/j/*"}}, []Link{{URL: "https://A.com/j/1", Texts: []string{"One"}}})
	if len(res.Items) != 1 {
		t.Errorf("%+v", res)
	}
	// more than 10 skipped samples are cut at 10
	var many []Link
	for i := 0; i < 30; i++ {
		many = append(many, Link{URL: fmt.Sprintf("https://a.com/j/%d", i)})
	}
	res, _ = Apply(r, many)
	if res.SkippedNoTitle != 30 || len(res.SkippedSamples) != 10 {
		t.Errorf("%d %d", res.SkippedNoTitle, len(res.SkippedSamples))
	}
	// Apply never fabricates a title from the URL
	for _, it := range res.Items {
		t.Errorf("an item without text was created: %+v", it)
	}
}

// What the proposal matches is exactly what Apply later finds (same matching code).
func TestProposalAndApplyAgree(t *testing.T) {
	page := []Link{
		{URL: "https://example.com/job/1", Texts: []string{"One"}}, {URL: "https://example.com/job/2", Texts: []string{"Two"}},
		{URL: "https://example.com/job/3?utm=x", Texts: []string{"Three"}}, {URL: "https://example.com/about", Texts: []string{"About"}},
	}
	p, err := LearnJobPattern([]string{"https://example.com/job/1", "https://example.com/job/2"}, nil, page)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Apply(Rule{Host: p.Host, Include: []string{p.Pattern}, Exclude: p.Exclude}, page)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, it := range res.Items {
		got = append(got, it.URL)
	}
	if !reflect.DeepEqual(got, p.Matches) {
		t.Errorf("apply %v vs proposal %v", got, p.Matches)
	}
}

// Nothing a page or a person can type may panic any of the entry points.
func FuzzLinkRule(f *testing.F) {
	for _, s := range []string{"duapune.com/jobs/*", "example.com/a/**/z?id=*&x", "https://example.com/view?id=5#f", "", "***", "a*b*c", "ü/%zz", "example.com/" + strings.Repeat("*a", 50)} {
		f.Add(s, "https://example.com/jobs/1", "https://example.com/jobs/2")
	}
	f.Fuzz(func(t *testing.T, pattern, a, b string) {
		if p, err := Compile(pattern); err == nil {
			p.Match(a)
			p.Match(b)
		}
		if prop, err := LearnJobPattern([]string{a, b}, []string{b}, []Link{{URL: a, Texts: []string{b}}, {URL: b}}); err == nil {
			p, cerr := Compile(prop.Pattern)
			if cerr != nil {
				t.Fatalf("proposed pattern %q does not compile: %v", prop.Pattern, cerr)
			}
			for _, u := range []string{a, b} {
				if _, ok := parseURL(u); ok && !p.Match(u) {
					t.Fatalf("pattern %q for picks %q %q misses %q", prop.Pattern, a, b, u)
				}
			}
		}
		_, _ = Apply(Rule{Host: "example.com", Include: []string{pattern}}, []Link{{URL: a, Texts: []string{b}}, {URL: b, TitleAttr: a}})
		_ = GenericTexts([]Link{{URL: a, Texts: []string{b, a}}})
	})
}

func TestBroad(t *testing.T) {
	for pattern, want := range map[string]bool{
		"example.com/**": true, "example.com/*": true, "example.com/*/*": true, "example.com/*/**": true, "example.com": true,
		"example.com/jobs/*": false, "example.com/job-*": false, "example.com/*/jobs": false, "example.com/*?id=*": false, "example.com/view?id=*": false, "example.com/a/**": false,
	} {
		p, err := Compile(pattern)
		if err != nil {
			t.Fatalf("%s: %v", pattern, err)
		}
		if got := p.Broad(); got != want {
			t.Errorf("%s: Broad() = %v, want %v", pattern, got, want)
		}
	}
	var nilPattern *Pattern
	if nilPattern.Broad() {
		t.Error("a nil pattern is not broad")
	}
}

func TestPreviewRule(t *testing.T) {
	page := []Link{{URL: "https://a.com/j/1", Texts: []string{"One"}}, {URL: "https://a.com/j/2"}, {URL: "https://a.com/about", Texts: []string{"About"}}}
	pv, err := PreviewRule(Rule{Host: "a.com", Include: []string{"a.com/j/*"}}, page)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Items) != 1 || pv.Matched != 2 || pv.SkippedNoTitle != 1 || pv.NeedsConfirm {
		t.Errorf("%+v", pv)
	}
	if len(pv.Notes) != 1 || !strings.Contains(pv.Notes[0], "no usable text") {
		t.Errorf("notes: %v", pv.Notes)
	}
	// broad
	pv, _ = PreviewRule(Rule{Host: "a.com", Include: []string{"a.com/**"}}, page)
	if !pv.NeedsConfirm {
		t.Errorf("a broad pattern needs a confirm: %+v", pv)
	}
	// too many matches
	var many []Link
	for i := 0; i < BroadMatchLimit+1; i++ {
		many = append(many, Link{URL: fmt.Sprintf("https://a.com/j/%d", i), Texts: []string{"T"}})
	}
	pv, _ = PreviewRule(Rule{Host: "a.com", Include: []string{"a.com/j/*"}}, many)
	if !pv.NeedsConfirm || pv.Matched != BroadMatchLimit+1 {
		t.Errorf("%d matches: %+v", pv.Matched, pv.NeedsConfirm)
	}
	pv, _ = PreviewRule(Rule{Host: "a.com", Include: []string{"a.com/j/*"}}, many[:BroadMatchLimit])
	if pv.NeedsConfirm {
		t.Error("exactly at the limit needs no confirm")
	}
	if _, err := PreviewRule(Rule{Host: "a.com"}, page); !errors.Is(err, ErrNoInclude) {
		t.Errorf("invalid rule: %v", err)
	}
	// Preview and Apply agree on the items
	res, _ := Apply(Rule{Host: "a.com", Include: []string{"a.com/j/*"}}, page)
	pv, _ = PreviewRule(Rule{Host: "a.com", Include: []string{"a.com/j/*"}}, page)
	if !reflect.DeepEqual(res, pv.Result) {
		t.Errorf("apply %+v vs preview %+v", res, pv.Result)
	}
}

// ---- the number wildcard # ----

func TestNumberWildcardMatching(t *testing.T) {
	type tc struct {
		pattern string
		match   []string
		not     []string
	}
	for _, c := range []tc{
		{"www.python.org/jobs/#",
			[]string{"https://www.python.org/jobs/8195/", "https://www.python.org/jobs/8195", "https://www.python.org/jobs/1"},
			[]string{"https://www.python.org/jobs/types/", "https://www.python.org/jobs/categories/", "https://www.python.org/jobs/create/", "https://www.python.org/jobs/", "https://www.python.org/jobs/8195/apply", "https://www.python.org/jobs/81x95", "https://www.python.org/jobs/location/remote-remote/"}},
		{"duapune.com/jobs/#-*",
			[]string{"https://duapune.com/jobs/121888-backend-developer", "https://duapune.com/jobs/3-qa", "https://duapune.com/jobs/7-"},
			[]string{"https://duapune.com/jobs/backend-developer", "https://duapune.com/jobs/121888", "https://duapune.com/jobs/-x"}},
		{"example.com/jobs/#.html", []string{"https://example.com/jobs/12.html"}, []string{"https://example.com/jobs/a.html", "https://example.com/jobs/12.htm", "https://example.com/jobs/.html"}},
		{"example.com/#/jobs/#", []string{"https://example.com/12/jobs/34"}, []string{"https://example.com/de/jobs/34", "https://example.com/12/jobs/x"}},
		{"example.com/view?id=#", []string{"https://example.com/view?id=5", "https://example.com/view?id=123&x=1"}, []string{"https://example.com/view?id=abc", "https://example.com/view?id=", "https://example.com/view?id=5x", "https://example.com/view"}},
		{"example.com/p#", []string{"https://example.com/p7"}, []string{"https://example.com/p", "https://example.com/px"}},
		{"example.com/##", []string{"https://example.com/12"}, nil}, // two number wildcards in a row still match a number
	} {
		p, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("%q: %v", c.pattern, err)
			continue
		}
		for _, u := range c.match {
			if !p.Match(u) {
				t.Errorf("%q should match %s", c.pattern, u)
			}
		}
		for _, u := range c.not {
			if p.Match(u) {
				t.Errorf("%q must not match %s", c.pattern, u)
			}
		}
	}
}

func TestLeadingNumber(t *testing.T) {
	for in, want := range map[string]struct {
		digits, rest string
		ok           bool
	}{
		"8195":                     {"8195", "", true},
		"121888-backend-developer": {"121888", "-backend-developer", true},
		"12.html":                  {"12", ".html", true},
		"7_x":                      {"7", "_x", true},
		"12abc":                    {"", "", false},
		"types":                    {"", "", false},
		"":                         {"", "", false},
		"-12":                      {"", "", false},
		"1234567890123":            {"", "", false}, // more than 12 digits is not an id
		"123456789012":             {"123456789012", "", true},
	} {
		d, r, ok := leadingNumber(in)
		if d != want.digits || r != want.rest || ok != want.ok {
			t.Errorf("%q: %q %q %v, want %+v", in, d, r, ok, want)
		}
	}
}

func TestTightenWithNumbers(t *testing.T) {
	for _, c := range []struct {
		values []string
		want   string
	}{
		{[]string{"1", "22", "333"}, "#"},
		{[]string{"121888-backend-developer", "121726-devops-engineer"}, "#-*"},
		{[]string{"12.html", "98.html"}, "#.html"},
		{[]string{"12-a.html", "98-b.html"}, "#-*.html"},
		{[]string{"12", "13-x"}, "#*"},                    // an id with and without a slug
		{[]string{"12", "types"}, "*"},                    // a number and a word: no number pattern
		{[]string{"12abc", "34abc"}, "*"},                 // not number-led (no boundary): the old rules
		{[]string{"job-12", "job-98"}, "job-*"},           // a word first: the old rules
		{[]string{"1234567890123", "1234567890124"}, "*"}, // more than 12 digits is not an id
		{[]string{"8195", "8195"}, "8195"},                // equal values stay literal
	} {
		got := tighten(c.values)
		if got != c.want {
			t.Errorf("%v: %q, want %q", c.values, got, c.want)
		}
	}
}

// The real case from the live pass on python.org: ids and words under one path.
func TestPythonOrgJobsAreFoundByNumberNotByWord(t *testing.T) {
	page := links(
		"https://www.python.org/jobs/", "https://www.python.org/jobs/types/", "https://www.python.org/jobs/categories/", "https://www.python.org/jobs/locations/",
		"https://www.python.org/jobs/create/", "https://www.python.org/jobs/type/back-end/", "https://www.python.org/jobs/location/remote-worldwide/",
		"https://www.python.org/jobs/8195/", "https://www.python.org/jobs/8189/", "https://www.python.org/jobs/8183/", "https://www.python.org/jobs/8180/",
		"https://www.python.org/community/jobs/howto/",
	)
	p, err := LearnJobPattern([]string{"https://www.python.org/jobs/8195/", "https://www.python.org/jobs/8189/", "https://www.python.org/jobs/8183/"}, nil, page)
	if err != nil {
		t.Fatal(err)
	}
	if p.Pattern != "www.python.org/jobs/#" || p.MatchCount != 4 || p.NeedsConfirm {
		t.Errorf("%+v", p)
	}
	for _, m := range p.Matches {
		if !strings.Contains(m, "/jobs/81") {
			t.Errorf("a non-job link matched: %s", m)
		}
	}
}

// Patterns with # compile into a valid rule, are not broad, and a learned one always matches its picks (the randomised test above
// already runs with numbers in its words).
func TestNumberPatternsInRulesAndBroad(t *testing.T) {
	if err := ValidateRule(Rule{Host: "x.com", Include: []string{"x.com/jobs/#", "x.com/v?id=#"}}); err != nil {
		t.Fatal(err)
	}
	for pattern, want := range map[string]bool{"x.com/#": false, "x.com/jobs/#": false, "x.com/*": true, "x.com/#/*": false, "x.com/**": true} {
		p, err := Compile(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if p.Broad() != want {
			t.Errorf("%s: Broad() = %v", pattern, p.Broad())
		}
	}
}
