package crawler

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func snapshotOf(t *testing.T, body string) string {
	t.Helper()
	out, _, err := buildSnapshotHTML(snapshotCapture{HTML: "<!DOCTYPE html><html><head></head><body>" + body + "</body></html>", Base: "https://site.example/list/"}, "https://site.example/list/")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// elementsOf returns every element of the output whose data-cinqo-tag is set, as attribute maps.
func standIns(t *testing.T, out string) []map[string]string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	var found []map[string]string
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			m := map[string]string{"": n.Data}
			has := false
			for _, a := range n.Attr {
				m[a.Key] = a.Val
				if a.Key == "data-cinqo-tag" {
					has = true
				}
			}
			if has {
				found = append(found, m)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

func TestAnIframeBecomesAnInertDivThatKeepsWhatASelectorCanName(t *testing.T) {
	out := snapshotOf(t, `<div class="wrap"><iframe id="player" class="player big" role="application" title="The match" data-quality="hd" src="https://evil.example/e" srcdoc="<script>x()</script>" sandbox="allow-scripts" allow="autoplay" allowfullscreen name="f" onload="steal()" scrolling="no">fallback <b>text</b></iframe></div>`)
	got := standIns(t, out)
	if len(got) != 1 {
		t.Fatalf("%d stand-ins in:\n%s", len(got), out)
	}
	s := got[0]
	if s[""] != "div" || s["data-cinqo-tag"] != "iframe" || s["id"] != "player" || s["class"] != "player big" || s["role"] != "application" || s["title"] != "The match" || s["data-quality"] != "hd" {
		t.Errorf("%v", s)
	}
	for _, left := range []string{"src", "srcdoc", "sandbox", "allow", "allowfullscreen", "name", "onload", "scrolling"} {
		if _, ok := s[left]; ok {
			t.Errorf("%s was carried over: %v", left, s)
		}
	}
	low := strings.ToLower(out)
	for _, bad := range []string{"<iframe", "evil.example", "steal", "script", "fallback", "<b>text"} {
		if strings.Contains(low, bad) {
			t.Errorf("%q survived:\n%s", bad, out)
		}
	}
}

func TestObjectAndEmbedGetTheSameStandInWithTheirOwnName(t *testing.T) {
	out := snapshotOf(t, `<object data="x.swf" class="o"><param name="a" value="b"><embed src="y.swf" class="e"></object><embed src="z" id="solo">`)
	got := standIns(t, out)
	names := map[string]string{}
	for _, s := range got {
		names[s["data-cinqo-tag"]] += s["class"] + s["id"] + ","
	}
	if len(got) != 1 && len(got) != 2 {
		t.Fatalf("%v in\n%s", got, out)
	}
	if !strings.Contains(names["object"], "o,") || !strings.Contains(names["embed"], "solo,") {
		t.Errorf("%v", names)
	}
	low := strings.ToLower(out)
	for _, bad := range []string{"<object", "<embed", "<param", "x.swf", "y.swf"} {
		if strings.Contains(low, bad) {
			t.Errorf("%q survived:\n%s", bad, out)
		}
	}
}

func TestWidthAndHeightBecomeAStyleOnlyWhenTheyAreSizes(t *testing.T) {
	cases := map[string]string{
		`width="640" height="360"`:             "width:640px;height:360px;",
		`width="100%" height="50px"`:           "width:100%;height:50px;",
		`width="640"`:                          "width:640px;",
		`width="abc" height="url(x)"`:          "",
		`width="1e3" height="-5"`:              "",
		`width="640; background:red"`:          "",
		`width="123456"`:                       "",
		`width=" 12 "`:                         "width:12px;",
		`height="9" style="height:20px"`:       "height:9px;height:20px",
		`style="background:url(/p.png)"`:       `background:url("https://site.example/p.png")`,
		`width="5" style="background:url(/p)"`: `width:5px;background:url("https://site.example/p")`,
	}
	for attrs, want := range cases {
		got := standIns(t, snapshotOf(t, `<iframe `+attrs+`></iframe>`))
		if len(got) != 1 {
			t.Fatalf("%s: %v", attrs, got)
		}
		if want == "" {
			if _, ok := got[0]["style"]; ok {
				t.Errorf("%s: a style appeared: %q", attrs, got[0]["style"])
			}
			continue
		}
		if got[0]["style"] != want {
			t.Errorf("%s: style %q, want %q", attrs, got[0]["style"], want)
		}
	}
}

func TestOnlyOurOwnCodeCanSetTheTagAndTheNumbers(t *testing.T) {
	out := snapshotOf(t, `<div id="fake" data-cinqo-tag="iframe">x</div><iframe id="real" data-cinqo-tag="evil" data-cinqo-a="3" data-cinqo-e="12" data-cinqo-x="1"></iframe>`)
	got := standIns(t, out)
	if len(got) != 1 || got[0]["id"] != "real" || got[0]["data-cinqo-tag"] != "iframe" {
		t.Fatalf("the page set the tag on something else, or the tag is not ours: %v\n%s", got, out)
	}
	if got[0]["data-cinqo-e"] != "12" {
		t.Errorf("a valid element number is kept: %v", got[0])
	}
	if _, ok := got[0]["data-cinqo-a"]; ok {
		t.Errorf("an anchor number belongs to anchors only: %v", got[0])
	}
	if _, ok := got[0]["data-cinqo-x"]; ok {
		t.Errorf("an unknown data-cinqo attribute survived: %v", got[0])
	}
}

func TestDataAttributesWithOddNamesOrAddressesDoNotGetThrough(t *testing.T) {
	out := snapshotOf(t, `<iframe data-ok="1" data-src="javascript:x()" data-original="https://site.example/a.png" data-href="https://evil.example/" data-a"b="1" data-x<y="1"></iframe>`)
	got := standIns(t, out)
	if len(got) != 1 {
		t.Fatalf("%v\n%s", got, out)
	}
	s := got[0]
	if s["data-ok"] != "1" || s["data-original"] != "https://site.example/a.png" {
		t.Errorf("%v", s)
	}
	for _, bad := range []string{"data-src", "data-href", `data-a"b`, "data-x<y"} {
		if _, ok := s[bad]; ok {
			t.Errorf("%s survived: %v", bad, s)
		}
	}
	if strings.Contains(out, "evil.example") || strings.Contains(out, "javascript:") {
		t.Errorf("%s", out)
	}
}

func TestAnIframeInsideAnIframeOrAnAnchorIsHandledToo(t *testing.T) {
	out := snapshotOf(t, `<a href="/x" data-cinqo-a="1"><iframe id="in-a"></iframe></a><iframe id="outer"><iframe id="inner"></iframe></iframe><div><div><iframe id="deep"></iframe></div></div>`)
	var ids []string
	for _, s := range standIns(t, out) {
		ids = append(ids, s["id"])
	}
	if strings.Join(ids, ",") != "in-a,outer,deep" {
		t.Errorf("%v in\n%s", ids, out)
	}
	if strings.Contains(out, "<iframe") {
		t.Errorf("%s", out)
	}
}

func TestThePlaceholderLookIsOnlyAddedWhenThereIsAStandIn(t *testing.T) {
	with := snapshotOf(t, `<iframe class="p"></iframe>`)
	without := snapshotOf(t, `<div class="p">no player</div><video class="v" src="/v.mp4"></video>`)
	if strings.Count(with, `data-cinqo-standin="1"`) != 1 || !strings.Contains(with, ":where([data-cinqo-tag])") || !strings.Contains(with, "min-height:96px") {
		t.Errorf("the placeholder style is missing:\n%s", with)
	}
	if strings.Contains(without, "data-cinqo-standin") || strings.Contains(without, "data-cinqo-tag") {
		t.Errorf("a placeholder with nothing to show:\n%s", without)
	}
	if strings.Contains(standInCSS, "url(") || strings.Contains(standInCSS, "@import") || strings.Contains(standInCSS, "</") {
		t.Error("the placeholder style must load nothing")
	}
}

func TestTwoStandInsMakeOnePlaceholderStyleAndBothCountedInTheWholeTree(t *testing.T) {
	out := snapshotOf(t, `<iframe id="a"></iframe><section><iframe id="b"></iframe><embed id="c"></section>`)
	if strings.Count(out, `data-cinqo-standin="1"`) != 1 || len(standIns(t, out)) != 3 {
		t.Errorf("%s", out)
	}
}

func TestVideoAndAudioStayRealElementsWithTheirNameAndClass(t *testing.T) {
	out := snapshotOf(t, `<video id="v" class="vid" src="/v.mp4" poster="/p.jpg" onplay="x()"></video><audio class="aud" src="/a.mp3"></audio>`)
	low := strings.ToLower(out)
	if !strings.Contains(low, `<video id="v" class="vid" src="https://site.example/v.mp4" poster="https://site.example/p.jpg">`) || !strings.Contains(low, `<audio class="aud" src="https://site.example/a.mp3">`) {
		t.Errorf("%s", out)
	}
	if strings.Contains(low, "onplay") || strings.Contains(out, "data-cinqo-tag") {
		t.Errorf("%s", out)
	}
}

func TestFramesetsAndFramesAreStillDropped(t *testing.T) {
	out := snapshotOf(t, `<frameset><frame src="/a"></frameset><applet code="x"></applet><p id="kept">k</p>`)
	low := strings.ToLower(out)
	if strings.Contains(low, "<frame") || strings.Contains(low, "<applet") || strings.Contains(out, "data-cinqo-tag") || !strings.Contains(out, `id="kept"`) {
		t.Errorf("%s", out)
	}
}

func TestSanitizeTreeCountsStandInsAndNothingElse(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`<html><body><iframe></iframe><script>x</script><object></object><p><embed></p><a href="/x">a</a></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if n := sanitizeTree(doc, nil); n != 3 {
		t.Errorf("%d stand-ins counted", n)
	}
}

func TestThePlaceholderIsABlockBoxWithALabelAndNeverBeatsThePagesOwnRules(t *testing.T) {
	for _, want := range []string{":where([data-cinqo-tag]){", "display:block", "box-sizing:border-box", "min-height:96px", `content:"embedded " attr(data-cinqo-tag)`} {
		if !strings.Contains(standInCSS, want) {
			t.Errorf("the placeholder style lacks %q: %s", want, standInCSS)
		}
	}
	if strings.Contains(standInCSS, "!important") || strings.Contains(strings.ReplaceAll(standInCSS, ":where([data-cinqo-tag])", ""), "[data-cinqo-tag]") {
		t.Error("every selector of the placeholder must sit inside :where() and nothing may be !important, or the page's own rules could not win")
	}
}

func TestAFramesetDocumentLeavesNoFrames(t *testing.T) {
	out, _, err := buildSnapshotHTML(snapshotCapture{HTML: `<!DOCTYPE html><html><head></head><frameset><frame src="/a"><frame src="/b"></frameset></html>`}, "https://site.example/")
	if err != nil {
		t.Fatal(err)
	}
	if low := strings.ToLower(out); strings.Contains(low, "<frame") || strings.Contains(low, "/a") || strings.Contains(out, "data-cinqo-tag") {
		t.Errorf("%s", out)
	}
}
