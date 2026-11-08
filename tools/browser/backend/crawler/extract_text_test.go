package crawler

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"
)

// loader shows html in the one browser page these tests share and returns its context. All the cases below run as subtests of
// TestExtractedText so that a single headless Chrome serves them all (starting one per case is slow and noisy).
type loader func(html string) context.Context

func readText(t *testing.T, load loader, html, selector string) string {
	t.Helper()
	return readField(t, load(html), selector, "")
}

func readField(t *testing.T, ctx context.Context, selector, attribute string) string {
	t.Helper()
	res, err := runExtractionOnCurrentPage(ctx, "", []extractField{{Label: "v", Selector: selector, Attribute: attribute}}, nil)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	v, ok := res.Results["v"]
	if !ok {
		t.Fatalf("selector %q found nothing (notFound %v)", selector, res.NotFound)
	}
	s, _ := v.(string)
	return s
}

func TestExtractedText(t *testing.T) {
	ctx := pageCtx(t, "<body></body>")
	load := func(html string) context.Context {
		if err := chromedp.Run(ctx, chromedp.Navigate("data:text/html;charset=utf-8,"+url.PathEscape(html))); err != nil {
			t.Fatalf("navigate: %v", err)
		}
		return ctx
	}
	cases := []struct {
		name string
		run  func(*testing.T, loader)
	}{
		{"ExtractedTextLeavesOutStyleAndScript", caseExtractedTextLeavesOutStyleAndScript},
		{"ExtractedTextLeavesOutNoscriptFallbacks", caseExtractedTextLeavesOutNoscriptFallbacks},
		{"ExtractedTextLeavesOutOtherNoTextElements", caseExtractedTextLeavesOutOtherNoTextElements},
		{"ExtractedTextHasLineBreaksWhereThePageHasBlocks", caseExtractedTextHasLineBreaksWhereThePageHasBlocks},
		{"ExtractedInlineTextStaysOnOneLine", caseExtractedInlineTextStaysOnOneLine},
		{"ExtractedTextTidiesWhitespace", caseExtractedTextTidiesWhitespace},
		{"PreformattedLinesAreKept", casePreformattedLinesAreKept},
		{"AnElementThatIsItselfNoTextGivesNothing", caseAnElementThatIsItselfNoTextGivesNothing},
		{"ReadingTheWholeBodyHasNoCSSOrCode", caseReadingTheWholeBodyHasNoCSSOrCode},
		{"AttributeReadsAreUnchanged", caseAttributeReadsAreUnchanged},
		{"ThePageIsLeftAlone", caseThePageIsLeftAlone},
		{"TheValueLimitCountsOnlyText", caseTheValueLimitCountsOnlyText},
		{"GroupedExtractionReadsCleanTextForEveryItem", caseGroupedExtractionReadsCleanTextForEveryItem},
		{"MultipleFieldReadsEachMatchCleanly", caseMultipleFieldReadsEachMatchCleanly},
		{"AnEmptyElementGivesAnEmptyString", caseAnEmptyElementGivesAnEmptyString},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { c.run(t, load) })
	}
}

func caseExtractedTextLeavesOutStyleAndScript(t *testing.T, load loader) {
	html := `<body><div id="d"><style>.job-details-sct { display: flex !important; }</style>Hello<script>var a = "</div>"; window.x = 1;</script> world</div></body>`
	if got := readText(t, load, html, "#d"); got != "Hello world" {
		t.Errorf("got %q", got)
	}
}

// The shape of a StepStone page: a no-js fallback pair that a browser keeps as plain text inside <noscript>.
func caseExtractedTextLeavesOutNoscriptFallbacks(t *testing.T, load loader) {
	html := `<body><article id="a"><noscript><style>#no-js-image-:R2j: { display: none !important; }</style><img src="https://x/logo.gif" alt="logo"/></noscript><h1>Software Engineer</h1><noscript>Please enable JavaScript</noscript><p>Real text</p></article></body>`
	got := readText(t, load, html, "#a")
	if got != "Software Engineer\n\nReal text" {
		t.Errorf("got %q", got)
	}
	for _, bad := range []string{"<style", "<img", "no-js-image", "enable JavaScript"} {
		if strings.Contains(got, bad) {
			t.Errorf("%q left in %q", bad, got)
		}
	}
}

func caseExtractedTextLeavesOutOtherNoTextElements(t *testing.T, load loader) {
	html := `<body><div id="d">A<template><p>hidden</p></template><svg viewBox="0 0 1 1"><title>Logo</title><text>Brand</text></svg><iframe src="about:blank">fallback</iframe>` +
		`<object data="a.swf">alt</object><canvas>fallback</canvas><picture><source srcset="a.webp"><img src="a.jpg" alt="pic"></picture>` +
		`<video src="a.mp4">no video</video><audio>no audio</audio><embed src="a.swf"><link rel="stylesheet" href="a.css"><meta name="x" content="y">B</div></body>`
	if got := readText(t, load, html, "#d"); got != "AB" {
		t.Errorf("got %q", got)
	}
}

func caseExtractedTextHasLineBreaksWhereThePageHasBlocks(t *testing.T, load loader) {
	html := `<body><div id="d"><h2>Tasks</h2><p>First paragraph</p><p>Second<br>line two</p><ul><li>Go</li><li>SQL</li></ul>` +
		`<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>Closing words</div></body>`
	want := "Tasks\n\nFirst paragraph\n\nSecond\nline two\n\nGo\n\nSQL\n\nA B\n\nC D\n\nClosing words"
	if got := readText(t, load, html, "#d"); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func caseExtractedInlineTextStaysOnOneLine(t *testing.T, load loader) {
	html := `<body><p id="p">We use <b>Go</b>, <a href="/x">SQL</a> and <span>React</span>.</p></body>`
	if got := readText(t, load, html, "#p"); got != "We use Go, SQL and React." {
		t.Errorf("got %q", got)
	}
}

func caseExtractedTextTidiesWhitespace(t *testing.T, load loader) {
	html := "<body><div id=\"d\">\n   Line   one  \n\n\n\n   <p>   Line\ttwo   </p>\n\n  &nbsp;  three­​ </div></body>"
	got := readText(t, load, html, "#d")
	if got != "Line one\nLine two\nthree" {
		t.Errorf("got %q", got)
	}
	if strings.Contains(got, "­") || strings.Contains(got, "​") || strings.Contains(got, "  ") || strings.Contains(got, "\n\n\n") {
		t.Errorf("untidy: %q", got)
	}
}

func casePreformattedLinesAreKept(t *testing.T, load loader) {
	html := "<body><pre id=\"p\">first line\n  second line\n\n\n\nthird</pre></body>"
	if got := readText(t, load, html, "#p"); got != "first line\nsecond line\n\nthird" {
		t.Errorf("got %q", got)
	}
}

func caseAnElementThatIsItselfNoTextGivesNothing(t *testing.T, load loader) {
	html := `<body><style id="s">.a { top: 0; }</style><script id="c">var a = 1;</script><p id="p">text</p></body>`
	ctx := load(html)
	if got := readField(t, ctx, "#s", ""); got != "" {
		t.Errorf("style: %q", got)
	}
	if got := readField(t, ctx, "#c", ""); got != "" {
		t.Errorf("script: %q", got)
	}
	if got := readField(t, ctx, "#p", ""); got != "text" {
		t.Errorf("p: %q", got)
	}
}

func caseReadingTheWholeBodyHasNoCSSOrCode(t *testing.T, load loader) {
	html := `<html><head><style>body { margin: 0; }</style></head><body><style>.x { top: 0; }</style><script>run()</script><div>Visible text</div></body></html>`
	got := readText(t, load, html, "body")
	if got != "Visible text" {
		t.Errorf("got %q", got)
	}
}

// Attributes are read as before: an <img> still gives its src, a link its href; only the text read changed.
func caseAttributeReadsAreUnchanged(t *testing.T, load loader) {
	html := `<body><a id="a" href="/jobs/1"><img id="i" src="/logo.png" alt="Acme">Open job</a></body>`
	ctx := load(html)
	if got := readField(t, ctx, "#a", "href"); got != "/jobs/1" {
		t.Errorf("href %q", got)
	}
	if got := readField(t, ctx, "#i", "src"); got != "/logo.png" {
		t.Errorf("src %q", got)
	}
	if got := readField(t, ctx, "#a", "text"); got != "Open job" {
		t.Errorf("text %q", got)
	}
	if got := readField(t, ctx, "#i", ""); got != "" {
		t.Errorf("an image has no text: %q", got)
	}
}

// The page is read, never changed: the elements that were left out of the text are still in it.
func caseThePageIsLeftAlone(t *testing.T, load loader) {
	html := `<body><div id="d"><style>.a{top:0}</style><script>1</script>text<img src="x"></div></body>`
	ctx := load(html)
	_ = readField(t, ctx, "#d", "")
	var n int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelectorAll('#d style, #d script, #d img').length`, &n)); err != nil || n != 3 {
		t.Errorf("elements left: %d (%v)", n, err)
	}
}

// The limit on a value now counts real text only: junk in front of the text no longer uses it up.
func caseTheValueLimitCountsOnlyText(t *testing.T, load loader) {
	junk := strings.Repeat(".rule { color: red; } ", 400) // about 9000 characters
	html := `<body><div id="d"><style>` + junk + `</style>` + strings.Repeat("word ", 20) + `</div></body>`
	got := readText(t, load, html, "#d")
	if got != strings.TrimSpace(strings.Repeat("word ", 20)) {
		t.Errorf("got %q", got)
	}
	long := `<body><div id="d">` + strings.Repeat("x", maxExtractedValueLength+500) + `</div></body>`
	if got := readText(t, load, long, "#d"); len(got) != maxExtractedValueLength {
		t.Errorf("a long text is still cut at the limit: %d", len(got))
	}
}

func caseGroupedExtractionReadsCleanTextForEveryItem(t *testing.T, load loader) {
	html := `<body>` +
		`<div class="job"><h2>One</h2><div class="d"><style>.a{top:0}</style>First<br>text</div></div>` +
		`<div class="job"><h2>Two</h2><div class="d"><noscript><img src="x"></noscript>Second text</div></div></body>`
	ctx := load(html)
	res, err := runExtractionOnCurrentPage(ctx, ".job", []extractField{
		{Label: "title", Selector: "h2"}, {Label: "description", Selector: ".d"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 2 || res.Items[0]["title"] != "One" || res.Items[0]["description"] != "First\ntext" || res.Items[1]["description"] != "Second text" {
		t.Errorf("items %v", res.Items)
	}
}

func caseMultipleFieldReadsEachMatchCleanly(t *testing.T, load loader) {
	html := `<body><li class="s">Go<script>1</script></li><li class="s"><style>.a{}</style>SQL</li></body>`
	ctx := load(html)
	res, err := runExtractionOnCurrentPage(ctx, "", []extractField{{Label: "skills", Selector: ".s", Multiple: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := res.Results["skills"].([]any)
	if len(got) != 2 || got[0] != "Go" || got[1] != "SQL" {
		t.Errorf("skills %v", res.Results["skills"])
	}
}

func caseAnEmptyElementGivesAnEmptyString(t *testing.T, load loader) {
	html := `<body><div id="d">  <style>.a{}</style>  </div></body>`
	if got := readText(t, load, html, "#d"); got != "" {
		t.Errorf("got %q", got)
	}
}
