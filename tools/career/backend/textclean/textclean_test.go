package textclean

import (
	"math/rand"
	"strings"
	"testing"
)

func TestDescription(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"only spaces", "  \n\t ", ""},
		{"plain text is kept", "We build software.\nJoin us.", "We build software.\nJoin us."},
		{"style block goes with its content", "<style>.a { color: red; }</style>Hello", "Hello"},
		{"style block in the middle", "Before<style type=\"text/css\">p{x:y}</style>After", "BeforeAfter"},
		{"upper case style", "<STYLE>.a{}</STYLE>Hi", "Hi"},
		{"script block", "<script>var a = '<b>';</script>Text", "Text"},
		{"script with attributes", "<script src=\"x.js\" async>alert(1)</script>Text", "Text"},
		{"self-closing script drops only itself", "<script src=\"x.js\" />Text stays", "Text stays"},
		{"unclosed style takes the rest", "Intro<style>.a{}", "Intro"},
		{"unclosed script takes the rest", "Intro<script>var a", "Intro"},
		{"closing tag with a space", "<style>a{}</style >Hi", "Hi"},
		{"a longer tag name is not the closer", "<style>a{}</styled>b{}</style>Hi", "Hi"},
		{"noscript fallback", "<noscript><style>#x{}</style><img src=\"a.gif\"></noscript>Job text", "Job text"},
		{"noscript wrapping text goes too", "<noscript>Enable JavaScript</noscript>Real text", "Real text"},
		{"template", "<template><p>hidden</p></template>Shown", "Shown"},
		{"svg with content", "A<svg viewBox=\"0 0 1 1\"><path d=\"M0 0\"/></svg>B", "AB"},
		{"iframe", "A<iframe src=\"x\">fallback</iframe>B", "AB"},
		{"picture", "A<picture><source srcset=\"a.webp\"><img src=\"a.jpg\"></picture>B", "AB"},
		{"video with fallback text", "A<video src=\"a.mp4\">Your browser cannot play this</video>B", "AB"},
		{"audio", "A<audio controls>no audio</audio>B", "AB"},
		{"object", "A<object data=\"a.swf\">alt</object>B", "AB"},
		{"canvas", "A<canvas id=\"c\">fallback</canvas>B", "AB"},
		{"embed has no content to drop", "A<embed src=\"a.swf\">B", "AB"},
		{"divs are lines", "<div>One</div><div>Two</div>", "One\n\nTwo"},
		{"table rows are lines", "<table><tr><td>A</td></tr><tr><td>B</td></tr></table>", "A\n\nB"},
		{"a stray closing style tag", "A</style>B", "AB"},
		{"a stray closing script tag", "A</script>B", "AB"},
		{"xml declaration", "<?xml version=\"1.0\"?>Text", "Text"},
		{"unclosed doctype is text", "<!DOCTYPE html", "<!DOCTYPE html"},
		{"self-closing style drops only itself", "<style />Text stays", "Text stays"},
		{"soft hyphen and zero-width space are dropped", "Senior Software En\u00adgineer\u200b (m/w/d)", "Senior Software Engineer (m/w/d)"},
		{"byte order mark is dropped", "\ufeffText", "Text"},
		{"svg with text inside", "A<svg><title>Logo</title><text>Brand</text></svg>B", "AB"},
		{"picture with text inside", "A<picture>fallback words</picture>B", "AB"},
		{"comment with dashes inside", "A<!-- a -- b -->B", "AB"},
		{"a digit after the bracket is not a tag", "x <3 and y> z", "x <3 and y> z"},
		{"closing tag with a slash", "<style>a{}</style/>Hi", "Hi"},
		{"img with a slash", "<img src=\"a.gif\" alt=\"logo\"/>Title", "Title"},
		{"img without a slash", "<img src=\"a.gif\">Title", "Title"},
		{"img with a spaced slash", "<img src=\"a.gif\" />Title", "Title"},
		{"img upper case", "<IMG SRC=\"a.gif\">Title", "Title"},
		{"img with a greater-than in an attribute", "<img alt=\"a > b\" src=\"x\">Title", "Title"},
		{"img with a data uri", "<img src=\"data:image/svg+xml,%3Csvg%3E\" alt=\"\">Title", "Title"},
		{"link and meta", "<link rel=\"stylesheet\" href=\"a.css\"><meta charset=\"utf-8\">Text", "Text"},
		{"comment", "A<!-- hidden <b>x</b> -->B", "AB"},
		{"unclosed comment runs to the end", "A<!-- never closed B", "A"},
		{"doctype", "<!DOCTYPE html>Text", "Text"},
		{"inline tags are dropped, words kept", "We use <b>Go</b> and <a href=\"/x\">SQL</a>.", "We use Go and SQL."},
		{"br is a line break", "Line one<br>Line two<br/>Line three", "Line one\nLine two\nLine three"},
		{"paragraphs", "<p>First</p><p>Second</p>", "First\n\nSecond"},
		{"list items on their own lines", "<ul><li>Go</li><li>SQL</li></ul>", "Go\n\nSQL"},
		{"headings", "<h2>Tasks</h2>Build things", "Tasks\nBuild things"},
		{"table cells are separated", "<tr><td>A</td><td>B</td></tr>", "A B"},
		{"entities", "Tom &amp; Jerry &quot;quoted&quot; &#x27;x&#x27; &lt;3 caf&eacute;", "Tom & Jerry \"quoted\" 'x' <3 café"},
		{"nbsp becomes a space", "A&nbsp;&nbsp;B\u00a0C", "A B C"},
		{"a lone less-than stays", "5 < 6 and 7 > 3", "5 < 6 and 7 > 3"},
		{"less-than and a digit stays", "I <3 Go", "I <3 Go"},
		{"less-than at the end stays", "a <", "a <"},
		{"a tag-like start without a closing bracket is text", "see <b and more", "see <b and more"},
		{"email style brackets are not markup", "Contact <jobs@example.com> today", "Contact today"},
		{"whitespace runs", "A   B\t\tC", "A B C"},
		{"blank line runs", "A\n\n\n\n\nB", "A\n\nB"},
		{"windows line endings", "A\r\nB\r\n\r\n\r\nC", "A\nB\n\nC"},
		{"line padding", "  A  \n   B   ", "A\nB"},
		{"unicode is kept", "Software Engineer (m/w/d) – Düsseldorf, Zürich, Tiranë, 日本語, İstanbul", "Software Engineer (m/w/d) – Düsseldorf, Zürich, Tiranë, 日本語, İstanbul"},
		{"unicode before a block keeps offsets right", "İİİ<STYLE>x</STYLE>ok", "İİİok"},
		{"only markup gives nothing", "<style>a{}</style><img src=\"x\"><script>1</script>", ""},
		{"a closing tag alone", "A</div>B", "A\nB"},
		{"attribute with single quotes and a bracket", "<a title='a>b' href=x>Link</a>", "Link"},
		{"unbalanced quote falls back to the first bracket", "<a title=\"oops>Link</a> done", "Link done"},
	}
	for _, c := range cases {
		if got := Description(c.in); got != c.want {
			t.Errorf("%s:\n in:   %q\n got:  %q\n want: %q", c.name, c.in, got, c.want)
		}
	}
}

// The shape of what the crawl stored for a StepStone job: a no-js fallback pair in front of the title, then the text, with another
// fallback later on.
func TestStepStoneShapedText(t *testing.T) {
	in := `<style>#no-js-image-:R2j58qjada: { display: none !important; }</style><img src="https://www.stepstone.de/upload_DE/logo/1/logomsg.gif" alt="msg systems ag logo" style="display:block;max-width:100%"/>Software Engineer Public Sector (all genders)msg systems agBerlin, ChemnitzFeste Anstellung` +
		`<style>#no-js-image-:Rj2lada: { display: none !important; }</style><img src="data:image/svg+xml,%3Csvg xmlns=&#x27;http://www.w3.org/2000/svg&#x27; width=&#x27;852px&#x27;%3E%3Ctitle%3EHero%3C/title%3E%3C/svg%3E" alt="" style="display:block"/>Ihre Aufgaben: Entwicklung &amp; Betrieb`
	want := "Software Engineer Public Sector (all genders)msg systems agBerlin, ChemnitzFeste AnstellungIhre Aufgaben: Entwicklung & Betrieb"
	if got := Description(in); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

// What textContent gave for a page with a <style> element inside the container: raw CSS in front of the text.
func TestCSSOnlyBlockIsGone(t *testing.T) {
	in := "Inxhinier Ndërtimi\n\n<style>\n.job-details-sct {\n    display: flex !important;\n}\n@media (max-width: 700px) {\n .job-details-sct { gap: 5px; }\n}\n</style>\n\nPune me kohe te plote"
	want := "Inxhinier Ndërtimi\n\nPune me kohe te plote"
	if got := Description(in); got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestCleanTextIsLeftAlone(t *testing.T) {
	for _, s := range []string{
		"Short text",
		"Line one\nLine two\n\nParagraph two",
		"Requirements:\n- Go\n- SQL\n\nWe offer: 30 days & more.",
		"a < b and c > d",
		"Zürich, Düsseldorf – 100% remote",
	} {
		if got := Description(s); got != s {
			t.Errorf("clean text changed: %q -> %q", s, got)
		}
	}
}

func TestCleaningTwiceEqualsOnce(t *testing.T) {
	// Without entities in the result (an entity that decodes to markup text is the one case where a second pass could differ).
	fragments := []string{
		"Plain words ", "<style>.a{}</style>", "<script>x()</script>", "<noscript><img src=x></noscript>", "<img src=\"a\" />",
		"<p>", "</p>", "<br>", "<li>", "</li>", "<b>", "</b>", "<!-- c -->", "<td>", "a < b ", "<3 ", "Ünï ", "\n", "\n\n\n", "  ", "\t",
		"<a href=\"x>y\">", "</a>", "<", ">", "<style>", "</style>", "<svg><g/></svg>",
		".a { top: 0; }", "\n.b {\n gap: 1px;\n}\n", "@media (x: 1) { .c { top: 1px; } }", "/* c */", "{", "}", "a: b;", ".x,\n", "body ",
	}
	r := rand.New(rand.NewSource(147))
	for n := 0; n < 3000; n++ {
		var b strings.Builder
		for k := r.Intn(12); k >= 0; k-- {
			b.WriteString(fragments[r.Intn(len(fragments))])
		}
		in := b.String()
		once := Description(in)
		if twice := Description(once); twice != once {
			t.Fatalf("not idempotent for %q:\n once:  %q\n twice: %q", in, once, twice)
		}
		lowered := strings.ToLower(once)
		for _, bad := range []string{"<style", "<script", "<img", "<noscript", "<svg", "<!--", "!important"} {
			if strings.Contains(lowered, bad) {
				t.Fatalf("%q left in the output of %q: %q", bad, in, once)
			}
		}
	}
}

func TestNeverPanicsOnBrokenInput(t *testing.T) {
	for _, s := range []string{"<", "<<", "<<<>>>", "</", "<!", "<!-", "<!--", "<?", "<a", "<a ", "<a \"", "<a '>", "</a", "<style", "<style>", "</style>", "<script/", "&", "&#", "&#x", "&#xZZ;", "\x00<\xff>", "<\u0130>"} {
		_ = Description(s)
	}
}

func TestCSSWithoutTags(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"one rule", "Title\n.job-details-sct {\n  display: flex !important;\n  gap: 0px;\n}\nText", "Title\n\nText"},
		{"rule on one line", "Title\n.a { color: red; }\nText", "Title\n\nText"},
		{"last declaration without a semicolon", "A\n.a { color: red }\nB", "A\n\nB"},
		{"minified rules in a row", "A\n.a{color:red;}.b{margin:0}\nB", "A\n\nB"},
		{"media query with nested rules", "A\n@media (max-width: 700px) {\n  .x {\n    gap: 5px !important;\n  }\n}\nB", "A\n\nB"},
		{"font face", "A\n@font-face { font-family: X; src: url(a.woff); }\nB", "A\n\nB"},
		{"keyframes", "A\n@keyframes spin { from { transform: rotate(0); } to { transform: rotate(1turn); } }\nB", "A\n\nB"},
		{"selector list over lines", "A\n.inner:before,\n.inner:after {\n  height: 14px;\n}\nB", "A\n\nB"},
		{"brace on its own line", "A\n.card\n{\n  color: red;\n}\nB", "A\n\nB"},
		{"element selector", "A\nbody { margin: 0; }\nB", "A\n\nB"},
		{"descendant and child selectors", "A\n.a > .b + .c { top: 0; }\nB", "A\n\nB"},
		{"data url holds a semicolon", "A\n.a { background: url(data:image/png;base64,AAAA); color: red; }\nB", "A\n\nB"},
		{"braces inside a string", "A\n.a { content: '}'; color: red; }\nB", "A\n\nB"},
		{"comments around and inside rules", "A\n/* Modal */\n.a { color: red; /* note */ }\n/* next */\n.b { top: 0; }\n/* end */\nB", "A\n\nB"},
		{"at-import and charset lines", "@charset \"utf-8\";\n@import url(a.css);\nText", "Text"},
		{"prose before the selector on the same line stays", "Welcome to the team .a { color: red; }", "Welcome to the team"},
		{"css after tags removed", "<style>.a { color: red; }</style>Text", "Text"},
		{"text between two rules stays", ".a { top: 0; }\nReal words\n.b { top: 1; }", "Real words"},
	}
	for _, c := range cases {
		if got := Description(c.in); got != c.want {
			t.Errorf("%s:\n in:   %q\n got:  %q\n want: %q", c.name, c.in, got, c.want)
		}
	}
}

func TestBracesInRealTextAreKept(t *testing.T) {
	for _, s := range []string{
		"Use {name} as a placeholder",
		"Config looks like {\"a\": 1}",
		"Set {role: admin} for new users",
		"Benefits {health, dental}: yes",
		"if (x) { return y }",
		"Salary: {negotiable}",
		"Unbalanced { brace stays",
		"Closing } alone stays",
		"We love Go: it is fast; it is simple {really}.",
	} {
		if got := Description(s); got != s {
			t.Errorf("text with braces changed: %q -> %q", s, got)
		}
	}
}

// The text of the job that showed the problem: the page's <style> elements came out as bare CSS between the real lines.
func TestRealJobWithBareCSS(t *testing.T) {
	in := "Inxhinier Ndërtimi Projekte Energjetike\n                \n            \n         Inxhinier Ndërtimi         \n        \n    .job-details-sct {\n        display: flex !important;\n        flex-wrap: wrap !important;\n        gap: 0px !important;\n        margin-top: 0.6em !important;\n    }\n\n    @media (max-width: 700px) {\n        .job-details-sct {\n            flex-direction: unset !important;\n            gap: 5px !important;\n            margin: 1em 0 !important;\n        }\n    }\n\n\n\n            \n             Tirane, Shqipëri\n        \n                         25-11-2026\n   /* Default modal content styling */\n.inner-qr-code {\n    display: inline-block;\n    position: relative;\n    padding: 10px;\n}\n\n.inner-qr-code:before,\n.inner-qr-code:after {\n    height: 14px;\n    width: 14px;\n    content: '';\n}\n\n/* Button styling */\n.btn-group .btn {\n    font-size: 14px; /* Adjust font size */\n    padding: 10px 15px; /* Padding adjustment */\n}\n\n                            edhe 25 ditë\n  Kodi Punës:\n122247"
	want := "Inxhinier Ndërtimi Projekte Energjetike\n\nInxhinier Ndërtimi\n\nTirane, Shqipëri\n\n25-11-2026\n\nedhe 25 ditë\nKodi Punës:\n122247"
	if got := Description(in); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
