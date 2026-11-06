package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// ---- validation (no browser) ----

func validRule() *descriptionRule {
	return &descriptionRule{Parts: []descriptionPart{{Include: "#box"}}}
}

func TestValidateDescriptionRuleFillsDefaults(t *testing.T) {
	r := validRule()
	if err := validateDescriptionRule(r); err != nil {
		t.Fatal(err)
	}
	if r.Label != "description" || r.Format != "plain" || r.Limit != DefaultDescriptionLimit || r.Parts[0].Heading != "none" {
		t.Fatalf("defaults not filled: %+v", r)
	}
	if err := validateDescriptionRule(nil); err != nil {
		t.Fatalf("a nil rule is fine (no rule), got %v", err)
	}
}

func TestValidateDescriptionRuleRefusals(t *testing.T) {
	long := strings.Repeat("a", maxRuleSelectorLen+1)
	manyParts := make([]descriptionPart, maxDescriptionParts+1)
	for i := range manyParts {
		manyParts[i] = descriptionPart{Include: "div"}
	}
	cases := []struct {
		name string
		mod  func(*descriptionRule)
	}{
		{"no parts", func(r *descriptionRule) { r.Parts = nil }},
		{"too many parts", func(r *descriptionRule) { r.Parts = manyParts }},
		{"empty include", func(r *descriptionRule) { r.Parts[0].Include = "  " }},
		{"long include", func(r *descriptionRule) { r.Parts[0].Include = long }},
		{"control character", func(r *descriptionRule) { r.Parts[0].Include = "div\x00" }},
		{"unknown format", func(r *descriptionRule) { r.Format = "markdown" }},
		{"limit too small", func(r *descriptionRule) { r.Limit = MinDescriptionLimit - 1 }},
		{"limit too large", func(r *descriptionRule) { r.Limit = MaxDescriptionLimit + 1 }},
		{"negative limit", func(r *descriptionRule) { r.Limit = -5 }},
		{"unknown heading", func(r *descriptionRule) { r.Parts[0].Heading = "first" }},
		{"heading selector without selector heading", func(r *descriptionRule) { r.Parts[0].HeadingSelector = "h2" }},
		{"selector heading without selector", func(r *descriptionRule) { r.Parts[0].Heading = "selector" }},
		{"too many excludes", func(r *descriptionRule) { r.Parts[0].Exclude = make([]string, maxRuleExcludes+1) }},
		{"empty exclude", func(r *descriptionRule) { r.Parts[0].Exclude = []string{""} }},
		{"too many cut-after", func(r *descriptionRule) { r.Parts[0].CutAfter = make([]string, maxRuleCutAfter+1) }},
		{"empty cut-after", func(r *descriptionRule) { r.Parts[0].CutAfter = []string{" "} }},
		{"long cut-after", func(r *descriptionRule) { r.Parts[0].CutAfter = []string{strings.Repeat("x", maxRuleCutAfterLen+1)} }},
		{"label with space", func(r *descriptionRule) { r.Label = "my text" }},
		{"long label", func(r *descriptionRule) { r.Label = strings.Repeat("l", maxDescriptionLabelLen+1) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := validRule()
			c.mod(r)
			if err := validateDescriptionRule(r); err == nil {
				t.Fatal("expected a refusal")
			}
		})
	}
}

func TestExtractHandlerRefusesABadDescriptionRule(t *testing.T) {
	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/extract", strings.NewReader(body))
		w := httptest.NewRecorder()
		ExtractHandler(w, req)
		return w
	}
	for name, body := range map[string]string{
		"invalid rule":    `{"fields":[{"label":"a","selector":"h1"}],"descriptionRule":{"parts":[]}}`,
		"with container":  `{"container":"li","fields":[{"label":"a","selector":"h1"}],"descriptionRule":{"parts":[{"include":"#b"}]}}`,
		"login window":    `{"requestId":"r","useLoginWindow":true,"fields":[{"label":"a","selector":"h1"}],"descriptionRule":{"parts":[{"include":"#b"}]}}`,
		"unknown formats": `{"fields":[{"label":"a","selector":"h1"}],"descriptionRule":{"format":"html","parts":[{"include":"#b"}]}}`,
	} {
		if w := post(body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, w.Code, w.Body.String())
		}
	}
}

func TestPaginatedHandlerRefusesABadDescriptionRule(t *testing.T) {
	for name, body := range map[string]string{
		"invalid rule":   `{"url":"https://example.com/","fields":[{"label":"a","selector":"h1"}],"requestedMaxPages":1,"effectiveMaxPages":1,"descriptionRule":{"parts":[]}}`,
		"with container": `{"url":"https://example.com/","container":"li","fields":[{"label":"a","selector":"h1"}],"requestedMaxPages":1,"effectiveMaxPages":1,"descriptionRule":{"parts":[{"include":"#b"}]}}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/crawl-paginated", strings.NewReader(body))
		w := httptest.NewRecorder()
		PaginatedCrawlHandler(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, w.Code, w.Body.String())
		}
	}
}

// ---- reading pages (one real browser for all cases) ----

func readRule(t *testing.T, ctx context.Context, html string, rule *descriptionRule) *descriptionResult {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	if err := validateDescriptionRule(rule); err != nil {
		t.Fatalf("rule: %v", err)
	}
	res, err := runDescriptionRule(ctx, rule)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return res
}

func oneBox(sel string) *descriptionRule {
	return &descriptionRule{Parts: []descriptionPart{{Include: sel}}}
}

func TestDescriptionRule(t *testing.T) {
	ctx := pageCtx(t, "<body></body>")
	cases := []struct {
		name string
		run  func(*testing.T, context.Context)
	}{
		{"OneBoxPlainEqualsTheFieldRead", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><h2>Tasks</h2><p>Build <b>APIs</b>.</p><ul><li>Go</li><li>SQL</li></ul><script>var x=1</script><style>p{color:red}</style></div></body>`
			res := readRule(t, ctx, html, oneBox("#d"))
			want, err := runExtractionOnCurrentPage(ctx, "", []extractField{{Label: "v", Selector: "#d"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := want.Results["v"].(string); res.Text != got {
				t.Fatalf("plain text differs from the field read:\n rule: %q\nfield: %q", res.Text, got)
			}
			if res.Truncated || res.Parts[0].Matches != 1 || res.Parts[0].NotFound {
				t.Fatalf("report wrong: %+v", res)
			}
		}},
		{"StructuredFormatHasHeadingsAndListMarkers", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><h2>Tasks</h2><p>First</p><p>Second</p><h2>Profile</h2><ul><li>Go</li><li></li><li>SQL</li></ul></div></body>`
			r := oneBox("#d")
			r.Format = "structured"
			res := readRule(t, ctx, html, r)
			want := "Tasks\n\nFirst\n\nSecond\n\nProfile\n\n- Go\n- SQL"
			if res.Text != want {
				t.Fatalf("got %q want %q", res.Text, want)
			}
			if len(res.Outline) != 2 || res.Outline[0] != "Tasks" || res.Outline[1] != "Profile" {
				t.Fatalf("outline %v", res.Outline)
			}
		}},
		{"OutlineEntriesAreOneLine", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><h2>Founding <span>New</span><br>Company GmbH</h2><p>text</p></div></body>`
			res := readRule(t, ctx, html, oneBox("#d"))
			if len(res.Outline) != 1 || strings.ContainsAny(res.Outline[0], "\n\t") {
				t.Fatalf("outline %q", res.Outline)
			}
		}},
		{"SeveralBoxesInTheOrderOfTheParts", func(t *testing.T, ctx context.Context) {
			html := `<body><div class="b">Second box</div><div class="a">First box</div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: ".a"}, {Include: ".b"}}}
			res := readRule(t, ctx, html, r)
			if res.Text != "First box\n\nSecond box" {
				t.Fatalf("got %q", res.Text)
			}
			if res.Parts[0].Chars != len("First box") || res.Parts[1].Chars != len("Second box") {
				t.Fatalf("per-part sizes: %+v", res.Parts)
			}
		}},
		{"OnePartTakesEveryMatchInPageOrder", func(t *testing.T, ctx context.Context) {
			html := `<body><section class="s">One</section><p>x</p><section class="s">Two</section></body>`
			res := readRule(t, ctx, html, oneBox(".s"))
			if res.Text != "One\n\nTwo" || res.Parts[0].Matches != 2 {
				t.Fatalf("got %q %+v", res.Text, res.Parts)
			}
		}},
		{"ExcludeTakesFurnitureOutAndReportsIt", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><p>Real text of the job.</p><div class="share">Share this job</div><a class="apply">Apply now</a></div></body>`
			r := oneBox("#d")
			r.Parts[0].Exclude = []string{".share", ".apply"}
			res := readRule(t, ctx, html, r)
			if res.Text != "Real text of the job." {
				t.Fatalf("got %q", res.Text)
			}
			if res.RemovedTotal != 2 || len(res.Removed) != 2 || res.Removed[0].Kind != "exclude" || res.Removed[0].Text != "Share this job" {
				t.Fatalf("removed report: %+v", res.Removed)
			}
			if res.Parts[0].Removed != len("Share this job")+len("Apply now") {
				t.Fatalf("removed chars: %+v", res.Parts[0])
			}
		}},
		{"ConsentAndDialogBoxesInsideTheBoxAreAlwaysLeftOut", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><p>Job text.</p><div class="cookie-banner">We use cookies</div><div role="dialog">Privacy settings</div><div id="consent-x">Manage consent</div></div></body>`
			res := readRule(t, ctx, html, oneBox("#d"))
			if res.Text != "Job text." {
				t.Fatalf("got %q", res.Text)
			}
			for _, r := range res.Removed {
				if r.Kind != "consent" {
					t.Fatalf("kind %q", r.Kind)
				}
			}
			if res.RemovedTotal != 3 {
				t.Fatalf("removed %d", res.RemovedTotal)
			}
		}},
		{"CutAfterStopsAtTheFirstLineThatStartsWithTheText", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><p>Real part one.</p><p>Real part two.</p><p>Ähnliche Jobs</p><p>Other job A</p></div></body>`
			r := oneBox("#d")
			r.Parts[0].CutAfter = []string{"ähnliche"}
			res := readRule(t, ctx, html, r)
			if res.Text != "Real part one.\n\nReal part two." {
				t.Fatalf("got %q", res.Text)
			}
			if res.Parts[0].Cut == 0 || res.Removed[0].Kind != "cutAfter" {
				t.Fatalf("report: %+v %+v", res.Parts[0], res.Removed)
			}
		}},
		{"CutAfterDoesNotCutAMentionInTheMiddleOfALine", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><p>We hire. Similar jobs exist elsewhere.</p><p>More text.</p></div></body>`
			r := oneBox("#d")
			r.Parts[0].CutAfter = []string{"similar jobs"}
			res := readRule(t, ctx, html, r)
			if !strings.Contains(res.Text, "More text.") || res.Parts[0].Cut != 0 {
				t.Fatalf("cut a line that only mentions the text: %q", res.Text)
			}
		}},
		{"HeadingAutoWritesTheNearestHeadingBeforeTheBox", func(t *testing.T, ctx context.Context) {
			html := `<body><h3>Aufgaben</h3><div class="b">Backend bauen</div><h3>Profil</h3><div class="c">Go können</div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: ".b", Heading: "auto"}, {Include: ".c", Heading: "auto"}}}
			res := readRule(t, ctx, html, r)
			if res.Text != "Aufgaben\n\nBackend bauen\n\nProfil\n\nGo können" {
				t.Fatalf("got %q", res.Text)
			}
		}},
		{"HeadingAutoDoesNotRepeatAHeadingTheBoxStartsWith", func(t *testing.T, ctx context.Context) {
			html := `<body><h3>Aufgaben</h3><div class="b"><h3>Aufgaben</h3><p>Backend bauen</p></div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: ".b", Heading: "auto"}}}
			res := readRule(t, ctx, html, r)
			if strings.Count(res.Text, "Aufgaben") != 1 {
				t.Fatalf("heading repeated: %q", res.Text)
			}
		}},
		{"HeadingSelectorUsesTheGivenElement", func(t *testing.T, ctx context.Context) {
			html := `<body><span id="h">About the role</span><div class="b">Text</div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: ".b", Heading: "selector", HeadingSelector: "#h"}}}
			res := readRule(t, ctx, html, r)
			if res.Text != "About the role\n\nText" {
				t.Fatalf("got %q", res.Text)
			}
		}},
		{"TheLimitCutsAndSaysSo", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d">` + strings.Repeat("<p>"+strings.Repeat("w", 99)+"</p>", 30) + `</div></body>`
			r := oneBox("#d")
			r.Limit = MinDescriptionLimit
			res := readRule(t, ctx, html, r)
			if !res.Truncated || res.Chars != MinDescriptionLimit || res.FullLength <= MinDescriptionLimit {
				t.Fatalf("truncated=%v chars=%d full=%d", res.Truncated, res.Chars, res.FullLength)
			}
		}},
		{"ADescriptionLongerThanTheOldSilentCutIsKeptWhole", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d">` + strings.Repeat("<p>"+strings.Repeat("w", 99)+"</p>", 120) + `</div></body>` // about 12,000 characters
			res := readRule(t, ctx, html, oneBox("#d"))
			if res.Truncated || res.Chars < 5000 {
				t.Fatalf("a long description was cut: truncated=%v chars=%d", res.Truncated, res.Chars)
			}
		}},
		{"TheCutNeverSplitsASurrogatePair", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d">` + strings.Repeat("😀", 1500) + `</div></body>`
			r := oneBox("#d")
			r.Limit = MinDescriptionLimit + 1 // an odd number of UTF-16 units falls inside a pair
			res := readRule(t, ctx, html, r)
			if !res.Truncated {
				t.Fatal("expected a cut")
			}
			for _, c := range res.Text {
				if c == 0xFFFD {
					t.Fatal("a half emoji survived the cut")
				}
			}
		}},
		{"AnInvalidSelectorIsReportedNotACrash", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d">Text</div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: "#d", Exclude: []string{"div[["}}, {Include: "[[["}}}
			res := readRule(t, ctx, html, r)
			if res.Text != "Text" || len(res.Errors) != 2 || !res.Parts[1].NotFound {
				t.Fatalf("got %q errors=%v parts=%+v", res.Text, res.Errors, res.Parts)
			}
		}},
		{"APartThatMatchesNothingIsNotFoundAndTheRuleStillWorks", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d">Text</div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: "#missing"}, {Include: "#d"}}}
			res := readRule(t, ctx, html, r)
			if res.Text != "Text" || !res.Parts[0].NotFound || res.Parts[1].NotFound || descriptionNotFound(res) {
				t.Fatalf("got %q %+v", res.Text, res.Parts)
			}
			none := readRule(t, ctx, html, oneBox("#missing"))
			if !descriptionNotFound(none) || none.Text != "" {
				t.Fatalf("a rule that matches nothing must say so: %+v", none)
			}
		}},
		{"OverlappingPartsAreFlaggedAndNotRepeated", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="outer">A<div id="inner">B</div></div></body>`
			r := &descriptionRule{Parts: []descriptionPart{{Include: "#outer"}, {Include: "#inner"}}}
			res := readRule(t, ctx, html, r)
			if !res.Parts[1].Overlap || strings.Count(res.Text, "B") != 1 {
				t.Fatalf("overlap not handled: %q %+v", res.Text, res.Parts)
			}
		}},
		{"HostilePageScriptsAndHugeNestingAreHarmless", func(t *testing.T, ctx context.Context) {
			deep := strings.Repeat("<div>", 400) + "deep text" + strings.Repeat("</div>", 400)
			html := `<body><div id="d"><script>var harmless = 1</script>` + deep + `<p id="p" onclick="window.__clicked = true">click me</p></div></body>`
			res := readRule(t, ctx, html, oneBox("#d"))
			if !strings.Contains(res.Text, "deep text") || !strings.Contains(res.Text, "click me") {
				t.Fatalf("got %q", res.Text)
			}
			var clicked bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__clicked === true`, &clicked)); err != nil {
				t.Fatal(err)
			}
			if clicked {
				t.Fatal("reading ran a page handler")
			}
		}},
		{"ThePageIsLeftAlone", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="d"><p>Text</p><div class="share">Share</div></div></body>`
			r := oneBox("#d")
			r.Parts[0].Exclude = []string{".share"}
			readRule(t, ctx, html, r)
			var n int
			if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('.share').length`, &n)); err != nil || n != 1 {
				t.Fatalf("the page was changed (share elements: %d, err %v)", n, err)
			}
		}},
		{"ExtractReturnsTheTextUnderTheLabelAndAReport", func(t *testing.T, ctx context.Context) {
			html := `<body><h1>Engineer</h1><div id="d"><p>The text</p></div></body>`
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
				t.Fatal(err)
			}
			rule := oneBox("#d")
			rule.Label = "desc"
			if err := validateDescriptionRule(rule); err != nil {
				t.Fatal(err)
			}
			base, err := runExtractionOnCurrentPage(ctx, "", []extractField{{Label: "title", Selector: "h1"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			got, err := applyDescriptionRule(ctx, base, rule)
			if err != nil {
				t.Fatal(err)
			}
			if got.Results["title"] != "Engineer" || got.Results["desc"] != "The text" || got.Description == nil {
				t.Fatalf("got %+v", got)
			}
			miss := oneBox("#nothing")
			_ = validateDescriptionRule(miss)
			got2, _ := applyDescriptionRule(ctx, base, miss)
			if _, ok := got2.Results["description"]; ok || len(got2.NotFound) != 1 || got2.NotFound[0] != "description" {
				t.Fatalf("a missing box must be notFound: %+v", got2)
			}
		}},
		{"AFieldValueCutAtTheLimitIsReportedNotSilent", func(t *testing.T, ctx context.Context) {
			html := `<body><div id="long">` + strings.Repeat("x", maxExtractedValueLength+50) + `</div><div id="short">ok</div></body>`
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
				t.Fatal(err)
			}
			res, err := runExtractionOnCurrentPage(ctx, "", []extractField{{Label: "long", Selector: "#long"}, {Label: "short", Selector: "#short"}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Truncated) != 1 || res.Truncated[0] != "long" {
				t.Fatalf("truncated labels: %v", res.Truncated)
			}
			if len(res.Results["long"].(string)) != maxExtractedValueLength {
				t.Fatal("the value should still be cut at the field limit")
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, ctx) })
	}
}
