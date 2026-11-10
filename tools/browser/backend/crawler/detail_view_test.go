package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/go-rod/rod"
	"golang.org/x/net/html"

	"browser-tool-backend/shared"
)

func TestTheAnalysisScriptNumbersTheElementsOnlyWhenTheViewIsAskedFor(t *testing.T) {
	if detailCandidatesScript(false) != detailCandidatesJS {
		t.Error("without the view the script is the one that was always run")
	}
	on := detailCandidatesScript(true)
	if !strings.Contains(on, "var VIEW = true;") || strings.Contains(on, "/*__VIEW__*/false") || !strings.Contains(detailCandidatesJS, "var VIEW = /*__VIEW__*/false;") {
		t.Errorf("the switch must turn the view on and only that: %q", on[:200])
	}
	if strings.Count(on, "true;")-strings.Count(detailCandidatesJS, "true;") != 1 {
		t.Error("exactly one literal is swapped")
	}
}

func TestViewElementsAreBoundedAndOnlyAKnownDateAttributeSurvives(t *testing.T) {
	long := strings.Repeat("s", maxDetailSelector+1)
	in := []detailViewElement{
		{N: 1, Selector: "h1", Unique: true, Tag: "h1", Chars: 5, Text: "Title", Attribute: ""},
		{N: 2, Selector: "time", Tag: "time", Chars: 9, Text: strings.Repeat("t", 200), Attribute: "datetime"},
		{N: 3, Selector: "a.b", Tag: "a", Chars: 1, Text: "x", Attribute: "href"}, // not a date attribute: dropped from the element
		{N: 0, Selector: "x"}, {N: -4, Selector: "x"}, {N: maxDetailViewNumber + 1, Selector: "x"}, // numbers the copy cannot carry
		{N: 2, Selector: "dup"},                      // a number used twice: the first stays
		{N: 5, Selector: ""}, {N: 6, Selector: long}, // no selector, a selector that cannot be stored
		{N: 7, Selector: "p\x00q\n r", Tag: "p", Chars: -3, Text: "a\tb\x01c"},
	}
	trunc := false
	got := clampViewElements(in, &trunc)
	var ns []int
	for _, e := range got {
		ns = append(ns, e.N)
	}
	if len(got) != 4 || ns[0] != 1 || ns[1] != 2 || ns[2] != 3 || ns[3] != 7 {
		t.Fatalf("%v %+v", ns, got)
	}
	if utf8Runes(got[1].Text) != maxDetailViewText || got[1].Attribute != "datetime" || got[2].Attribute != "" || got[1].Selector != "time" {
		t.Errorf("%+v %+v", got[1], got[2])
	}
	if got[3].Chars != 0 || strings.ContainsAny(got[3].Selector, "\x00\n") || got[3].Text != "a bc" || trunc {
		t.Errorf("%+v trunc=%v", got[3], trunc)
	}
	many := make([]detailViewElement, maxDetailViewElements+50)
	for i := range many {
		many[i] = detailViewElement{N: i + 1, Selector: "x" + strconv.Itoa(i)}
	}
	trunc = false
	if got := clampViewElements(many, &trunc); len(got) != maxDetailViewElements || !trunc {
		t.Errorf("%d trunc=%v", len(got), trunc)
	}
	if got := clampViewElements(nil, &trunc); got != nil {
		t.Errorf("%v", got)
	}
}

func utf8Runes(s string) int { return len([]rune(s)) }

func TestTheCopyKeepsOurElementNumbersAndNoOthers(t *testing.T) {
	doc := `<!DOCTYPE html><html><body>
<h1 data-cinqo-e="1">a</h1><div data-cinqo-e="12345">b</div><span data-cinqo-e="7" class="x">c</span>
<p data-cinqo-e="07">leading zero</p><p data-cinqo-e="123456">six digits</p><p data-cinqo-e="-1">sign</p><p data-cinqo-e="">empty</p><p data-cinqo-e="1 2">space</p><p data-cinqo-e="x">word</p><p data-cinqo-e="1e3">float</p>
<a href="/j" data-cinqo-a="3" data-cinqo-e="4">link</a><div data-cinqo-a="9" data-cinqo-x="1">not a link</div></body></html>`
	out, _, err := buildSnapshotHTML(snapshotCapture{HTML: doc, Base: "https://x.example/"}, "https://x.example/")
	if err != nil {
		t.Fatal(err)
	}
	root, _ := html.Parse(strings.NewReader(out))
	got := map[string]string{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "data-cinqo-e" {
					got[n.Data+":"+a.Val] = "yes"
				}
				if a.Key == "data-cinqo-a" {
					got["a:"+n.Data] = a.Val
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	for _, want := range []string{"h1:1", "div:12345", "span:7", "a:4"} {
		if got[want] != "yes" {
			t.Errorf("%s must be kept: %v", want, got)
		}
	}
	for k := range got {
		if strings.HasPrefix(k, "p:") {
			t.Errorf("a number that is not 1 to 5 plain digits survived: %s", k)
		}
	}
	if got["a:a"] != "3" || got["a:div"] != "" || strings.Contains(out, "data-cinqo-x") {
		t.Errorf("the link number stays on links only: %v", got)
	}
}

func TestFetchDetailCandidatesAsksForTheCopyOnlyWhenToldTo(t *testing.T) {
	var asked []bool
	stubDetail(t, func(ctx context.Context, rawURL, requestID string) (fetchDetailCandidatesResponse, error) {
		asked = append(asked, wantsSnapshot(ctx))
		r := fetchDetailCandidatesResponse{FinalURL: rawURL, Title: "T"}
		if wantsSnapshot(ctx) {
			r.Snapshot = &snapshotInfo{ID: strings.Repeat("a", 32), Bytes: 9}
			r.ViewElements = []detailViewElement{{N: 1, Selector: "h1", Unique: true, Tag: "h1", Chars: 3, Text: "abc"}}
		}
		return r, nil
	})
	with := postDetail(`{"url":"` + publicTestURL + `","snapshot":true}`)
	without := postDetail(`{"url":"` + publicTestURL + `"}`)
	if len(asked) != 2 || !asked[0] || asked[1] {
		t.Fatalf("asked %v", asked)
	}
	var a, b fetchDetailCandidatesResponse
	_ = json.Unmarshal(with.Body.Bytes(), &a)
	_ = json.Unmarshal(without.Body.Bytes(), &b)
	if a.Snapshot == nil || a.Snapshot.ID != strings.Repeat("a", 32) || len(a.ViewElements) != 1 || a.ViewElements[0].Selector != "h1" {
		t.Errorf("with: %s", with.Body.String())
	}
	if b.Snapshot != nil || b.ViewElements != nil || strings.Contains(without.Body.String(), "snapshot") || strings.Contains(without.Body.String(), "viewElements") {
		t.Errorf("without: %s", without.Body.String())
	}
	if rec := postDetail(`{"url":"` + publicTestURL + `","snapshot":true,"cookies":"a=b"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("other fields stay refused: %d", rec.Code)
	}
}

// ---- a live page ----

const viewFixture = `<!doctype html><html><head><title>Backend Engineer at Acme</title><link rel="stylesheet" href="/site.css"></head><body>
<header><nav>Menu entry</nav></header>
<main><h1 class="job-title">Backend Engineer</h1>
<span class="company">Acme GmbH</span> <span class="loc">Hamburg</span> <time datetime="2026-11-01">1 Nov 2026</time>
<div id="desc"><p>We build things in Go.</p><p>You will own services.</p></div>
<div class="hostile" data-cinqo-e="99">a page that numbers itself</div>
<div class="spacer"></div><div class="spoof" data-cinqo-e="3"></div><p class="spoof2" data-cinqo-e="4"><img src="/x.png" alt=""></p>
<div role="dialog">Cookie consent text</div>
<div class="huge">%HUGE%</div>
<script>var late=document.createElement("p");late.className="late";late.textContent="Added after load";document.querySelector("main").appendChild(late)</script>
</main></body></html>`

// viewCopyOf reads the sanitised copy a fetch made.
func viewCopyOf(t *testing.T, res fetchDetailCandidatesResponse) string {
	t.Helper()
	if res.Snapshot == nil || res.SnapshotError != "" {
		t.Fatalf("no copy: %+v %q", res.Snapshot, res.SnapshotError)
	}
	b, err := os.ReadFile(filepath.Join(shared.PageSnapshotsDir, res.Snapshot.ID+".html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func numberedElements(t *testing.T, page string) map[int]*html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		t.Fatal(err)
	}
	out := map[int]*html.Node{}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "data-cinqo-e" {
					if v, err := strconv.Atoi(a.Val); err == nil {
						if _, dup := out[v]; dup {
							t.Errorf("number %d is on two elements", v)
						}
						out[v] = n
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(x *html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data + " ")
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

func TestKatanaDetailViewNumbersTheElementsAndMakesTheCopy(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	page := strings.Replace(viewFixture, "%HUGE%", strings.Repeat("word ", 10001), 1) // more than 50,000 characters
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/job": htmlPage(page),
		"/site.css": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/css")
			_, _ = w.Write([]byte(".job-title{font-weight:700}"))
		},
	})
	res, err := fetchDetailCandidatesWithKatana(withSnapshot(context.Background(), true), srv.URL+"/job", "")
	if err != nil {
		t.Fatal(err)
	}
	copyHTML := viewCopyOf(t, res)
	if len(res.ViewElements) < 8 {
		t.Fatalf("%d view elements: %+v", len(res.ViewElements), res.ViewElements)
	}
	numbered := numberedElements(t, copyHTML)
	byText := map[string]detailViewElement{}
	for i, e := range res.ViewElements {
		if e.N != i+1 {
			t.Errorf("the numbers must run 1, 2, 3...: element %d has %d", i, e.N)
		}
		if numbered[e.N] == nil {
			t.Errorf("number %d is in the answer but not in the copy", e.N)
		}
		byText[e.Text] = e
	}
	if len(numbered) != len(res.ViewElements) {
		t.Errorf("%d numbered elements in the copy, %d in the answer", len(numbered), len(res.ViewElements))
	}
	title, ok := byText["Backend Engineer"]
	if !ok || title.Tag != "h1" || !title.Unique || nodeText(numbered[title.N]) != "Backend Engineer" || !strings.Contains(title.Selector, "job-title") {
		t.Errorf("title: %+v", title)
	}
	if tm := byText["1 Nov 2026"]; tm.Tag != "time" || tm.Attribute != "datetime" {
		t.Errorf("a date that holds a machine date says which attribute: %+v", tm)
	}
	if el, ok := byText["Hamburg"]; !ok || el.Attribute != "" {
		t.Errorf("an ordinary element has no attribute: %+v", el)
	}
	desc, ok := byText["We build things in Go. You will own services."]
	if !ok || desc.Selector != "#desc" || !desc.Unique || desc.Chars < 30 {
		t.Errorf("a box that holds the paragraphs can be chosen: %+v", desc)
	}
	for _, e := range res.ViewElements {
		if e.Chars == 0 || strings.TrimSpace(e.Text) == "" {
			t.Errorf("an element without text is not offered: %+v", e)
		}
	}
	if n3 := numbered[3]; n3 == nil || nodeText(n3) == "" {
		t.Errorf("a number the page put on an element of its own, that is not offered, must not survive: %v", n3)
	}
	if _, ok := byText["Added after load"]; !ok {
		t.Error("an element the page added after load is numbered too")
	}
	for text := range byText {
		if strings.Contains(text, "Cookie consent") {
			t.Error("a consent dialog is not offered")
		}
		if strings.HasPrefix(text, "word word") {
			t.Error("an element with more than 50,000 characters is not offered")
		}
	}
	hostile, ok := byText["a page that numbers itself"]
	if !ok || nodeText(numbered[hostile.N]) != "a page that numbers itself" {
		t.Errorf("a number the page made itself is replaced by ours: %+v", hostile)
	}
	if strings.Contains(copyHTML, "<script") {
		t.Error("no script in the copy")
	}
	// every selector the Browser verified on the live page finds exactly its element in the sanitised copy
	srv2 := rodTestServer(t, map[string]http.HandlerFunc{"/copy.html": htmlPage(copyHTML)})
	type probe struct {
		N        int    `json:"n"`
		Selector string `json:"selector"`
	}
	var probes []probe
	for _, e := range res.ViewElements {
		if e.Unique {
			probes = append(probes, probe{e.N, e.Selector})
		}
	}
	pj, _ := json.Marshal(probes)
	js := `(function(){var ps=` + string(pj) + `;var bad=[];ps.forEach(function(p){var m;try{m=document.querySelectorAll(p.selector)}catch(e){bad.push([p.n,"error"]);return}if(m.length!==1||m[0].getAttribute("data-cinqo-e")!==String(p.n))bad.push([p.n,m.length,p.selector])});return JSON.stringify(bad)})()`
	bad, err := runKatanaPage(context.Background(), srv2.URL+"/copy.html", "", func(fctx context.Context, p *rod.Page, _ *mainDocumentResponse) (string, error) {
		return rodEvalString(p.Context(fctx), js)
	})
	if err != nil {
		t.Fatal(err)
	}
	if bad != "[]" {
		t.Errorf("selectors that do not find their numbered element in the copy: %s (of %d)", bad, len(probes))
	}
	if len(probes) < 6 {
		t.Errorf("only %d verified selectors were probed", len(probes))
	}

	// without the view nothing is numbered and no copy is made
	plain, err := fetchDetailCandidatesWithKatana(context.Background(), srv.URL+"/job", "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.ViewElements != nil || plain.Snapshot != nil || plain.SnapshotError != "" {
		t.Errorf("%+v", plain)
	}
	if n, _, _ := snapshotStats(); n != 1 {
		t.Errorf("one copy in the folder, got %d", n)
	}
}

func TestKatanaDetailViewKeepsTheAnalysisWhenTheCopyCannotBeMade(t *testing.T) {
	requireKatanaChrome(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	notAFolder := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notAFolder, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	orig := shared.PageSnapshotsDir
	shared.PageSnapshotsDir = filepath.Join(notAFolder, "page_snapshots") // a folder below a file cannot be made
	t.Cleanup(func() { shared.PageSnapshotsDir = orig })
	srv := rodTestServer(t, map[string]http.HandlerFunc{"/job": htmlPage(strings.Replace(viewFixture, "%HUGE%", "x", 1))})
	res, err := fetchDetailCandidatesWithKatana(withSnapshot(context.Background(), true), srv.URL+"/job", "")
	if err != nil {
		t.Fatalf("a copy that cannot be made must not fail the analysis: %v", err)
	}
	if res.Snapshot != nil || res.SnapshotError == "" {
		t.Errorf("the reason must be given: %+v %q", res.Snapshot, res.SnapshotError)
	}
	if res.ViewElements != nil {
		t.Errorf("elements that point into a copy that does not exist must not be offered: %d", len(res.ViewElements))
	}
	if len(res.Candidates.Title) == 0 || res.Candidates.Title[0].Value != "Backend Engineer" || len(res.Elements) == 0 {
		t.Errorf("the suggestions are good without the picture: %+v", res.Candidates.Title)
	}
}
