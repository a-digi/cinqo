package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// ---- validation (no browser) ----

func TestValidateStructuredSource(t *testing.T) {
	for _, c := range []struct {
		source, path string
		ok           bool
	}{
		{"jsonld", "title", true}, {"jsonld", "hiringOrganization.name", true}, {"jsonld", "jobLocation.0.address", true}, {"jsonld", "@type", true},
		{"jsonld", "a_b-c$d", true}, {"jsonld", "", false}, {"jsonld", "a..b", false}, {"jsonld", ".a", false}, {"jsonld", "a.", false}, {"jsonld", "a b", false},
		{"jsonld", "a[0]", false}, {"jsonld", `a"b`, false}, {"jsonld", "a:b", false}, {"jsonld", strings.Repeat("a", MaxStructuredPathLen+1), false},
		{"jsonld", strings.Repeat("a", MaxStructuredPathLen), true},
		{"meta", "og:title", true}, {"meta", "twitter:title", true}, {"meta", "article:published_time", true}, {"meta", "description", true},
		{"meta", "", false}, {"meta", "og title", false}, {"meta", `og"title`, false}, {"meta", "og]title", false},
		{"microdata", "title", false}, {"", "title", false}, {"JSONLD", "title", false},
	} {
		err := validateStructuredSource("field", c.source, c.path)
		if (err == nil) != c.ok {
			t.Errorf("%s %q: err=%v, want ok=%v", c.source, c.path, err, c.ok)
		}
	}
}

func TestValidateExtractField(t *testing.T) {
	good := []extractField{
		{Label: "title", Selector: "h1"},
		{Label: "title", Selector: "h1", Attribute: "href", Multiple: true},
		{Label: "company", Source: "jsonld", Path: "hiringOrganization"},
		{Label: "og", Source: "meta", Path: "og:title"},
	}
	for _, f := range good {
		if err := validateExtractField(f); err != nil {
			t.Errorf("%+v: %v", f, err)
		}
	}
	bad := map[string]extractField{
		"no label":                 {Selector: "h1"},
		"no selector":              {Label: "a"},
		"path without source":      {Label: "a", Selector: "h1", Path: "title"},
		"source with selector":     {Label: "a", Source: "jsonld", Path: "title", Selector: "h1"},
		"source with attribute":    {Label: "a", Source: "jsonld", Path: "title", Attribute: "href"},
		"source with multiple":     {Label: "a", Source: "jsonld", Path: "title", Multiple: true},
		"source without path":      {Label: "a", Source: "jsonld"},
		"an unknown source":        {Label: "a", Source: "microdata", Path: "title"},
		"a path of the wrong kind": {Label: "a", Source: "jsonld", Path: "og:title"},
	}
	for name, f := range bad {
		if err := validateExtractField(f); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := validateExtractFields("li.job", []extractField{{Label: "a", Source: "jsonld", Path: "title"}}); err == nil {
		t.Error("a structured field per container was accepted")
	}
	if err := validateExtractFields("li.job", []extractField{{Label: "a", Selector: ".t"}}); err != nil {
		t.Errorf("a selector field per container: %v", err)
	}
}

func TestDescriptionPartMayBeStructuredButNotBoth(t *testing.T) {
	ok := &descriptionRule{Parts: []descriptionPart{{Source: "jsonld", Path: "description"}, {Include: "#a"}}}
	if err := validateDescriptionRule(ok); err != nil {
		t.Fatalf("a structured part and a box part: %v", err)
	}
	for name, p := range map[string]descriptionPart{
		"both":                {Include: "#a", Source: "jsonld", Path: "description"},
		"source only":         {Source: "jsonld"},
		"path only":           {Path: "description"},
		"unknown source":      {Source: "microdata", Path: "description"},
		"a bad path":          {Source: "jsonld", Path: "a b"},
		"neither":             {},
		"include with a path": {Include: "#a", Path: "x"},
	} {
		r := &descriptionRule{Parts: []descriptionPart{p}}
		if err := validateDescriptionRule(r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestHandlersAcceptStructuredFieldsAndRefuseBadOnes(t *testing.T) {
	post := func(h http.HandlerFunc, path, body string) (int, string) {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
		return w.Code, w.Body.String()
	}
	// /extract: a structured field gets past the field check (it is then stopped by the login window rule, no browser is started)
	code, msg := post(ExtractHandler, "/extract", `{"useLoginWindow":true,"fields":[{"label":"a","source":"jsonld","path":"title"}]}`)
	if code != 400 || !strings.Contains(msg, "requestId is required") {
		t.Errorf("accepted field: %d %s", code, msg)
	}
	for name, body := range map[string]string{
		"per container":    `{"container":"li","fields":[{"label":"a","source":"jsonld","path":"title"}]}`,
		"bad source":       `{"fields":[{"label":"a","source":"microdata","path":"title"}]}`,
		"selector as well": `{"fields":[{"label":"a","source":"jsonld","path":"title","selector":"h1"}]}`,
	} {
		if code, msg := post(ExtractHandler, "/extract", body); code != 400 || strings.Contains(msg, "requestId") {
			t.Errorf("extract %s: %d %s", name, code, msg)
		}
	}
	// /crawl-paginated validates every field now: a private address is the next thing that would stop a good body
	good := `{"url":"http://127.0.0.1/x","requestedMaxPages":1,"effectiveMaxPages":1,"fields":[{"label":"a","source":"jsonld","path":"title"}]}`
	if code, msg := post(PaginatedCrawlHandler, "/crawl-paginated", good); code != 400 || strings.Contains(msg, "structured") || strings.Contains(msg, "path") {
		t.Errorf("a good structured field was refused for its shape: %d %s", code, msg)
	}
	bad := `{"url":"https://example.com/x","requestedMaxPages":1,"effectiveMaxPages":1,"fields":[{"label":"a","source":"jsonld","path":"a b"}]}`
	if code, msg := post(PaginatedCrawlHandler, "/crawl-paginated", bad); code != 400 || !strings.Contains(msg, "path") {
		t.Errorf("a bad path: %d %s", code, msg)
	}
}

func TestCapabilitiesAnswersWhatThisBuildCanDo(t *testing.T) {
	w := httptest.NewRecorder()
	CapabilitiesHandler(w, httptest.NewRequest(http.MethodGet, "/capabilities", nil))
	var got map[string]bool
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || !got["structuredFields"] || !got["descriptionRule"] {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	CapabilitiesHandler(w, httptest.NewRequest(http.MethodPost, "/capabilities", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", w.Code)
	}
	raw, err := os.ReadFile("../../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"path_suffix": "capabilities"`) {
		t.Error("capabilities is not a route in manifest.json: the core would not forward it")
	}
}

// ---- reading pages (one real browser) ----

const structuredPage = `<!doctype html><html><head><title>T</title>
<meta property="og:title" content="  Engineer at Acme  "><meta name="twitter:title" content="Tw"><meta property="og:empty" content="  ">
<script type="application/ld+json">{ not json</script>
<script type="application/ld+json">{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"Board"},
{"@type":["Thing","JobPosting"],"title":"Senior Engineer &amp; Lead","hiringOrganization":{"@type":"Organization","name":"Acme GmbH"},
 "jobLocation":[{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Berlin","addressRegion":"BE","addressCountry":{"@type":"Country","name":"Germany"}}},{"@type":"Place","address":{"addressLocality":"Hamburg"}}],
 "datePosted":"2026-10-28T09:00:00+00:00","validThrough":"","employmentType":["FULL_TIME","PART_TIME"],"baseSalary":{"value":{"minValue":50000}},"openings":3,"remote":true,
 "description":"<p>Build <b>APIs</b> &amp; services.</p><ul><li>Go</li><li>SQL</li></ul><p>Similar jobs</p><p>Other job</p><img src=x onerror=\"window.__ran=1\"><script>window.__ran=1<\/script>"}]}</script>
</head><body><h1 id="h">Visible title</h1></body></html>`

func structuredFields(t *testing.T, ctx context.Context, html string, fields []extractField) extractResponse {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
		t.Fatal(err)
	}
	res, err := runExtractionOnCurrentPage(ctx, "", fields, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStructuredFields(t *testing.T) {
	ctx := pageCtx(t, "<body></body>")
	cases := []struct {
		name string
		run  func(*testing.T, context.Context)
	}{
		{"ReadsTheJobPostingByPath", func(t *testing.T, ctx context.Context) {
			res := structuredFields(t, ctx, structuredPage, []extractField{
				{Label: "title", Source: "jsonld", Path: "title"},
				{Label: "company", Source: "jsonld", Path: "hiringOrganization"},
				{Label: "companyName", Source: "jsonld", Path: "hiringOrganization.name"},
				{Label: "location", Source: "jsonld", Path: "jobLocation"},
				{Label: "second", Source: "jsonld", Path: "jobLocation.1"},
				{Label: "firstAddress", Source: "jsonld", Path: "jobLocation.0.address.addressLocality"},
				{Label: "noIndex", Source: "jsonld", Path: "jobLocation.address.addressLocality"},
				{Label: "date", Source: "jsonld", Path: "datePosted"},
				{Label: "type", Source: "jsonld", Path: "employmentType"},
				{Label: "number", Source: "jsonld", Path: "openings"},
				{Label: "flag", Source: "jsonld", Path: "remote"},
				{Label: "visible", Selector: "h1"},
			})
			want := map[string]string{
				"title": "Senior Engineer & Lead", "company": "Acme GmbH", "companyName": "Acme GmbH", "location": "Berlin, BE, Germany", "second": "Hamburg",
				"firstAddress": "Berlin", "noIndex": "Berlin", "date": "2026-10-28T09:00:00+00:00", "type": "FULL_TIME", "number": "3", "flag": "true", "visible": "Visible title",
			}
			for k, v := range want {
				if res.Results[k] != v {
					t.Errorf("%s = %q, want %q", k, res.Results[k], v)
				}
			}
			if len(res.NotFound) != 0 {
				t.Errorf("notFound %v", res.NotFound)
			}
		}},
		{"ReadsMetaTags", func(t *testing.T, ctx context.Context) {
			res := structuredFields(t, ctx, structuredPage, []extractField{
				{Label: "og", Source: "meta", Path: "og:title"}, {Label: "tw", Source: "meta", Path: "twitter:title"},
				{Label: "empty", Source: "meta", Path: "og:empty"}, {Label: "missing", Source: "meta", Path: "og:none"},
			})
			if res.Results["og"] != "Engineer at Acme" || res.Results["tw"] != "Tw" {
				t.Errorf("results %v", res.Results)
			}
			if len(res.NotFound) != 2 {
				t.Errorf("an empty or missing tag is not found: %v", res.NotFound)
			}
		}},
		{"AnythingTheJobPostingDoesNotSayIsNotFound", func(t *testing.T, ctx context.Context) {
			res := structuredFields(t, ctx, structuredPage, []extractField{
				{Label: "nope", Source: "jsonld", Path: "salary"}, {Label: "empty", Source: "jsonld", Path: "validThrough"}, {Label: "deep", Source: "jsonld", Path: "hiringOrganization.name.x"},
				{Label: "object", Source: "jsonld", Path: "baseSalary"}, {Label: "badindex", Source: "jsonld", Path: "jobLocation.9"},
			})
			if len(res.Results) != 0 || len(res.NotFound) != 5 {
				t.Errorf("results %v notFound %v", res.Results, res.NotFound)
			}
		}},
		{"PrototypeNamesAreNotPathsIntoTheObject", func(t *testing.T, ctx context.Context) {
			res := structuredFields(t, ctx, structuredPage, []extractField{
				{Label: "a", Source: "jsonld", Path: "constructor"}, {Label: "b", Source: "jsonld", Path: "__proto__"}, {Label: "c", Source: "jsonld", Path: "toString"},
				{Label: "d", Source: "jsonld", Path: "hiringOrganization.constructor.name"},
			})
			if len(res.Results) != 0 || len(res.NotFound) != 4 {
				t.Errorf("results %v notFound %v", res.Results, res.NotFound)
			}
		}},
		{"APageWithoutStructuredDataGivesNotFound", func(t *testing.T, ctx context.Context) {
			res := structuredFields(t, ctx, `<body><h1>x</h1></body>`, []extractField{{Label: "t", Source: "jsonld", Path: "title"}, {Label: "h", Selector: "h1"}})
			if res.Results["h"] != "x" || len(res.NotFound) != 1 || res.NotFound[0] != "t" {
				t.Errorf("%v %v", res.Results, res.NotFound)
			}
		}},
		{"HTMLInAValueIsReadAsTextAndRunsNothing", func(t *testing.T, ctx context.Context) {
			res := structuredFields(t, ctx, structuredPage, []extractField{{Label: "d", Source: "jsonld", Path: "description"}})
			d, _ := res.Results["d"].(string)
			if !strings.Contains(d, "Build APIs & services.") || !strings.Contains(d, "Go") || strings.Contains(d, "<") || strings.Contains(d, "onerror") || strings.Contains(d, "__ran") {
				t.Errorf("description text: %q", d)
			}
			var ran bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__ran === 1`, &ran)); err != nil || ran {
				t.Errorf("reading ran page code (ran=%v err=%v)", ran, err)
			}
		}},
		{"ALongStructuredValueIsCutAndFlagged", func(t *testing.T, ctx context.Context) {
			long := `<script type="application/ld+json">{"@type":"JobPosting","title":"T","description":"` + strings.Repeat("word ", 2000) + `"}</script>`
			res := structuredFields(t, ctx, `<html><head>`+long+`</head><body></body></html>`, []extractField{{Label: "description", Source: "jsonld", Path: "description"}, {Label: "t", Source: "jsonld", Path: "title"}})
			if d, _ := res.Results["description"].(string); len(d) != maxExtractedValueLength {
				t.Errorf("length %d", len(d))
			}
			if len(res.Truncated) != 1 || res.Truncated[0] != "description" {
				t.Errorf("truncated %v", res.Truncated)
			}
		}},
		{"ThePageIsLeftAlone", func(t *testing.T, ctx context.Context) {
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(structuredPage))); err != nil {
				t.Fatal(err)
			}
			var before, after string
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &before))
			if _, err := runExtractionOnCurrentPage(ctx, "", []extractField{{Label: "d", Source: "jsonld", Path: "description"}}, nil); err != nil {
				t.Fatal(err)
			}
			_ = chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.outerHTML`, &after))
			if before != after {
				t.Error("the page was changed")
			}
		}},
		{"ADescriptionRulePartCanBeTheStructuredDescription", func(t *testing.T, ctx context.Context) {
			rule := &descriptionRule{Format: "structured", Parts: []descriptionPart{{Source: "jsonld", Path: "description", CutAfter: []string{"Similar jobs"}}}}
			if err := validateDescriptionRule(rule); err != nil {
				t.Fatal(err)
			}
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(structuredPage))); err != nil {
				t.Fatal(err)
			}
			res, err := runDescriptionRule(ctx, rule)
			if err != nil {
				t.Fatal(err)
			}
			if res.Text != "Build APIs & services.\n\n- Go\n- SQL" {
				t.Errorf("text %q", res.Text)
			}
			if len(res.Parts) != 1 || res.Parts[0].Include != "jsonld:description" || res.Parts[0].Matches != 1 || res.Parts[0].Cut == 0 {
				t.Errorf("report %+v", res.Parts)
			}
		}},
		{"AStructuredPartHonoursExcludeLimitAndMixesWithABox", func(t *testing.T, ctx context.Context) {
			html := strings.Replace(structuredPage, `<body><h1 id="h">Visible title</h1></body>`, `<body><div id="box">Box text.</div></body>`, 1)
			rule := &descriptionRule{Limit: MinDescriptionLimit, Parts: []descriptionPart{
				{Include: "#box"}, {Source: "jsonld", Path: "description", Exclude: []string{"ul"}},
			}}
			if err := validateDescriptionRule(rule); err != nil {
				t.Fatal(err)
			}
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
				t.Fatal(err)
			}
			res, err := runDescriptionRule(ctx, rule)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(res.Text, "Box text.") || !strings.Contains(res.Text, "Build APIs") || strings.Contains(res.Text, "SQL") || res.RemovedTotal != 1 {
				t.Errorf("text %q removed %d", res.Text, res.RemovedTotal)
			}
		}},
		{"AnEmptyStructuredValueIsNotAPartThatFoundSomething", func(t *testing.T, ctx context.Context) {
			for _, part := range []descriptionPart{{Source: "jsonld", Path: "validThrough"}, {Source: "meta", Path: "og:empty"}, {Source: "meta", Path: "og:none"}, {Source: "jsonld", Path: "salary"}} {
				rule := &descriptionRule{Parts: []descriptionPart{part}}
				if err := validateDescriptionRule(rule); err != nil {
					t.Fatal(err)
				}
				if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(structuredPage))); err != nil {
					t.Fatal(err)
				}
				res, err := runDescriptionRule(ctx, rule)
				if err != nil || !descriptionNotFound(res) || res.Parts[0].Matches != 0 {
					t.Errorf("%+v: %v %+v", part, err, res.Parts)
				}
			}
			// a meta tag can be a description part too
			rule := &descriptionRule{Parts: []descriptionPart{{Source: "meta", Path: "og:title"}}}
			_ = validateDescriptionRule(rule)
			_ = chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(structuredPage)))
			if res, err := runDescriptionRule(ctx, rule); err != nil || res.Text != "Engineer at Acme" {
				t.Errorf("meta part: %v %+v", err, res)
			}
		}},
		{"AStructuredPartWithoutStructuredDataIsNotFound", func(t *testing.T, ctx context.Context) {
			rule := &descriptionRule{Parts: []descriptionPart{{Source: "jsonld", Path: "description"}}}
			_ = validateDescriptionRule(rule)
			if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(`<body>no data</body>`))); err != nil {
				t.Fatal(err)
			}
			res, err := runDescriptionRule(ctx, rule)
			if err != nil || !descriptionNotFound(res) || res.Text != "" {
				t.Errorf("%v %+v", err, res)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, ctx) })
	}
}
