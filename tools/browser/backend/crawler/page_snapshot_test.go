package crawler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

const hostileDoc = `<!DOCTYPE html><html><head><title>Jobs</title>
<base href="https://evil.example/">
<meta http-equiv="refresh" content="0;url=https://evil.example/">
<link rel="stylesheet" href="/site.css"><style>.x{background:url(/bg.png)}</style>
<script>alert(1)</script></head>
<body onload="steal()">
<!-- a comment -->
<a href="/jobs/1" onclick="steal()" target="_blank" data-cinqo-a="1">One <b>bold</b></a>
<a href="javascript:alert(1)" data-cinqo-a="2">js</a>
<a href="mailto:a@b.c" data-cinqo-a="3">mail</a>
<a href="https://user:pw@example.com/x" data-cinqo-a="4">cred</a>
<div data-cinqo-a="9" data-cinqo-x="1" data-href="https://evil.example/">not an anchor</div>
<img src="/a.png" srcset="/a1.png 1x, javascript:evil 2x, //cdn.example.com/a2.png 2x" onerror="steal()">
<img data-src="/lazy.png"><a href="ftp://files.example.com/x" data-cinqo-a="5">ftp</a><img src="ftp://files.example.com/i.png"><img src="data:image/png;base64,AAAA"><video poster="data:text/html,x" src="javascript:x"></video>
<iframe src="https://evil.example/"></iframe><object data="x"></object><embed src="x"><noscript>n</noscript><template><b>t</b></template>
<form action="/search" method="post"><input name="q" formaction="/x"></form>
<svg><script>alert(2)</script><foreignObject><div>f</div></foreignObject><use href="https://evil.example/s.svg#a"></use><use xlink:href="#local"></use></svg>
<div style="background:url(/inline.png)">s</div>
</body></html>`

func buildHostile(t *testing.T, sheets []snapshotSheet) string {
	t.Helper()
	out, _, err := buildSnapshotHTML(snapshotCapture{HTML: hostileDoc, Sheets: sheets, Base: "https://jobs.example.com/list/"}, "https://jobs.example.com/list/")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSnapshotDropsEverythingThatRunsOrNavigates(t *testing.T) {
	css := ".x{background:url(/bg.png)}"
	out := buildHostile(t, []snapshotSheet{{CSS: &css}})
	low := strings.ToLower(out)
	for _, bad := range []string{"<script", "alert(", "onclick", "onload", "onerror", "steal", "<iframe", "<object", "<embed", "<noscript", "<template", "<base", "refresh", "<foreignobject", "evil.example", "javascript:", "formaction", "action=", "target=", "<!--", "data:text/html", "user:pw", "mailto:"} {
		if strings.Contains(low, bad) {
			t.Errorf("%q survived in:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, `http-equiv="Content-Security-Policy"`) || !strings.Contains(out, "default-src &#39;none&#39;") {
		t.Errorf("no CSP meta:\n%s", out)
	}
	if strings.Count(low, "<meta") != 2 { // charset and the CSP: nothing the page brought
		t.Errorf("meta elements: %d", strings.Count(low, "<meta"))
	}
}

func TestSnapshotAnchorsKeepTheirNumberAndLeaveTheViewSafely(t *testing.T) {
	out := buildHostile(t, nil)
	if tag := regexpAnchorTag(out, 1); !strings.Contains(tag, `href="#"`) || !strings.Contains(tag, `data-href="https://jobs.example.com/jobs/1"`) || !strings.Contains(out, ">One <b>bold</b></a>") {
		t.Errorf("anchor 1 is %q:\n%s", tag, out)
	}
	for _, n := range []string{`data-cinqo-a="2"`, `data-cinqo-a="3"`, `data-cinqo-a="4"`} { // kept (they are numbered), but with no way out
		if !strings.Contains(out, n) {
			t.Errorf("%s missing", n)
		}
	}
	if strings.Contains(out, `data-cinqo-a="9"`) || strings.Contains(out, "data-cinqo-x") {
		t.Error("a page-made data-cinqo-* attribute on a non-anchor survived")
	}
	if strings.Contains(out, `data-href="https://evil.example/"`) {
		t.Error("a page-made data-href survived")
	}
	for _, a := range []string{`data-cinqo-a="2"`, `data-cinqo-a="3"`, `data-cinqo-a="4"`} {
		i := strings.Index(out, a)
		tag := out[strings.LastIndex(out[:i], "<a"):i]
		if strings.Contains(tag, "href=") || strings.Contains(tag, "data-href") {
			t.Errorf("anchor with an unusable address kept an address: %s", tag)
		}
	}
}

func TestSnapshotAddressesAreAbsoluteAndWebOnly(t *testing.T) {
	out := buildHostile(t, nil)
	for _, want := range []string{`src="https://jobs.example.com/a.png"`, `https://jobs.example.com/a1.png 1x`, `https://cdn.example.com/a2.png 2x`, ` src="https://jobs.example.com/lazy.png"`, `src="data:image/png;base64,AAAA"`, `url(&#34;https://jobs.example.com/inline.png&#34;)`, `<use xlink:href="#local">`} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing:\n%s", want, out)
		}
	}
	if strings.Contains(out, "evil") || strings.Contains(out, "s.svg") {
		t.Error("an external svg reference survived")
	}
	if strings.Contains(out, "ftp:") {
		t.Errorf("a non-web address survived:\n%s", out)
	}
}

func TestSnapshotStylesAreInlinedWithTheirOwnBase(t *testing.T) {
	a := `.x{background:url(img/bg.png)} @import url("other.css"); .y{background:url(data:image/gif;base64,R0)} .z{font-family:"F";src:url(//cdn.example.com/f.woff2)}`
	b := `.w{color:red} </style><script>x</script>`
	out, trunc, err := buildSnapshotHTML(snapshotCapture{HTML: hostileDoc, Base: "https://jobs.example.com/list/", Sheets: []snapshotSheet{
		{Href: "https://static.example.com/css/site.css", CSS: &a},
		{Href: "", CSS: &b, Media: "print"},
		{Href: "https://cross.example.com/theme.css"}, // not readable: stays a remote link
		{Href: "javascript:x"},                        // never a link
	}}, "https://jobs.example.com/list/")
	if err != nil || trunc {
		t.Fatal(err, trunc)
	}
	for _, want := range []string{`url("https://static.example.com/css/img/bg.png")`, `data:image/gif;base64,R0`, `url("https://cdn.example.com/f.woff2")`, "@media print {", `<link rel="stylesheet" href="https://cross.example.com/theme.css"/>`} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing:\n%s", want, out)
		}
	}
	if strings.Contains(out, "@import") || strings.Contains(out, "other.css") {
		t.Error("@import survived")
	}
	if strings.Contains(strings.ToLower(out), "</style><script") || strings.Contains(out, "javascript:x") {
		t.Errorf("a style sheet broke out of its element:\n%s", out)
	}
	if strings.Index(out, "Content-Security-Policy") > strings.Index(out, "data-cinqo-sheets") {
		t.Error("the CSP must come before the styles")
	}
}

func TestSnapshotIsCutAtTheBoundAndSaysSo(t *testing.T) {
	big := "<!DOCTYPE html><html><head></head><body>" + strings.Repeat(`<p class="a">0123456789</p>`, maxSnapshotHTML/20) + "</body></html>"
	out, trunc, err := buildSnapshotHTML(snapshotCapture{HTML: big, Base: "https://x.example/"}, "https://x.example/")
	if err != nil || !trunc || len(out) > maxSnapshotHTML+8 || !strings.HasSuffix(out, ">") {
		t.Errorf("err=%v trunc=%v len=%d", err, trunc, len(out))
	}
	huge := strings.Repeat("a{b:c}", maxSnapshotCSS/3)
	_, trunc, _ = buildSnapshotHTML(snapshotCapture{HTML: hostileDoc, Sheets: []snapshotSheet{{Href: "https://x.example/big.css", CSS: &huge}}}, "https://x.example/")
	if !trunc {
		t.Error("an oversized sheet must be reported")
	}
	if _, err := parseSnapshotCapture(strings.Repeat("x", maxSnapshotCapture+1)); err == nil {
		t.Error("an oversized capture must be refused")
	}
}

func TestBuildLinkListReportsAnchorNumbers(t *testing.T) {
	links, _, _ := buildLinkList([]rawPageLink{
		{Href: "https://e.com/a", Kind: "a", N: 1}, {Href: "https://e.com/b", Kind: "a", N: 2}, {Href: "https://e.com/a", Kind: "a", N: 3},
		{Href: "https://e.com/n", Kind: "link", Rel: "next"}, {Href: "https://e.com/z", Kind: "a"},
	}, "e.com")
	if len(links) != 4 || strings.Join(strings.Fields(strings.Trim(strings.ReplaceAll(strings.ReplaceAll(toJSON(links[0].Anchors), ",", " "), "null", ""), "[]")), ",") != "1,3" || len(links[1].Anchors) != 1 || links[2].Anchors != nil || links[3].Anchors != nil {
		t.Errorf("%+v", links)
	}
}

func toJSON(v any) string { b, _ := json.Marshal(v); return string(b) }

// ---- the store ----

func newStore(t *testing.T) string {
	t.Helper()
	useBrowserSettings(t, shared.FetchEngineKatana)
	return shared.PageSnapshotsDir
}

func TestSnapshotStoreWritesReadsAndDeletes(t *testing.T) {
	dir := newStore(t)
	info, err := writeSnapshot("<html>hi</html>", snapshotMeta{URL: "https://a.example/", Title: "T"})
	if err != nil || !snapshotIDRe.MatchString(info.ID) || info.Bytes != 15 {
		t.Fatal(info, err)
	}
	for _, f := range []string{info.ID + ".html", info.ID + ".json"} {
		st, err := os.Stat(filepath.Join(dir, f))
		if err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v %v", f, st, err)
		}
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("folder mode %v", st.Mode().Perm())
	}
	if n, b, _ := snapshotStats(); n != 1 || b != 15 {
		t.Errorf("stats %d %d", n, b)
	}
	if err := deleteSnapshot(info.ID); err != nil {
		t.Fatal(err)
	}
	if err := deleteSnapshot(info.ID); err != nil { // already gone is fine
		t.Fatal(err)
	}
	if n, _, _ := snapshotStats(); n != 0 {
		t.Errorf("count after delete: %d", n)
	}
	for _, bad := range []string{"", "../x", "..%2f", strings.Repeat("a", 31), strings.Repeat("A", 32), strings.Repeat("g", 32), info.ID + "/../x"} {
		if deleteSnapshot(bad) == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestTheFolderIsRecreatedPrivate(t *testing.T) {
	dir := newStore(t)
	os.RemoveAll(dir)
	if _, err := writeSnapshot("<p>", snapshotMeta{}); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("%v %v", st, err)
	}
}

func TestDeleteAllTouchesOnlyItsOwnFiles(t *testing.T) {
	dir := newStore(t)
	a, _ := writeSnapshot("<a>", snapshotMeta{})
	b, _ := writeSnapshot("<b>", snapshotMeta{})
	foreign := filepath.Join(dir, "notes.txt")
	notOurs := filepath.Join(dir, strings.Repeat("a", 32)+".txt")
	sub := filepath.Join(dir, strings.Repeat("b", 32)+".html")
	for _, f := range []string{foreign, notOurs} {
		os.WriteFile(f, []byte("keep"), 0o600)
	}
	os.Mkdir(sub, 0o700) // a folder named like a snapshot is not one
	n, err := deleteAllSnapshots()
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	for _, id := range []string{a.ID, b.ID} {
		if _, err := os.Stat(filepath.Join(dir, id+".html")); err == nil {
			t.Error("a snapshot survived")
		}
	}
	for _, f := range []string{foreign, notOurs, sub} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s was deleted: %v", f, err)
		}
	}
}

func TestSweepUsesTheAgeLimitsAndTheLoginLimit(t *testing.T) {
	dir := newStore(t)
	plain, _ := writeSnapshot("<p>", snapshotMeta{})
	login, _ := writeSnapshot("<l>", snapshotMeta{Login: true})
	fresh, _ := writeSnapshot("<f>", snapshotMeta{})
	age := func(id string, d time.Duration) {
		for _, ext := range []string{".html", ".json"} {
			past := time.Now().Add(-d)
			os.Chtimes(filepath.Join(dir, id+ext), past, past)
		}
	}
	age(plain.ID, 61*time.Minute)
	age(login.ID, 16*time.Minute)
	age(fresh.ID, 14*time.Minute)
	n, err := sweepSnapshots(time.Now())
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(filepath.Join(dir, fresh.ID+".html")); err != nil {
		t.Error("a young snapshot was swept")
	}
	// 30 minutes old: the login one would go, a plain one stays
	p2, _ := writeSnapshot("<p>", snapshotMeta{})
	l2, _ := writeSnapshot("<l>", snapshotMeta{Login: true})
	age(p2.ID, 30*time.Minute)
	age(l2.ID, 30*time.Minute)
	sweepSnapshots(time.Now())
	if _, err := os.Stat(filepath.Join(dir, p2.ID+".html")); err != nil {
		t.Error("a 30 minute plain snapshot must stay")
	}
	if _, err := os.Stat(filepath.Join(dir, l2.ID+".html")); err == nil {
		t.Error("a 30 minute login snapshot must go")
	}
}

func TestSnapshotRoutes(t *testing.T) {
	newStore(t)
	info, _ := writeSnapshot("<html><body>page</body></html>", snapshotMeta{})
	call := func(h http.HandlerFunc, method, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(method, target, nil))
		return rec
	}
	rec := call(PageSnapshotHandler, http.MethodGet, "/page-snapshot?id="+info.ID)
	if rec.Code != 200 || rec.Body.String() != "<html><body>page</body></html>" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" || !strings.HasPrefix(rec.Header().Get("Content-Security-Policy"), "sandbox;") {
		t.Errorf("%d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	for _, c := range []struct {
		method, target string
		want           int
	}{
		{"GET", "/page-snapshot", 400}, {"GET", "/page-snapshot?id=../../etc/passwd", 400}, {"GET", "/page-snapshot?id=" + strings.Repeat("a", 32), 404},
		{"POST", "/page-snapshot?id=" + info.ID, 405}, {"DELETE", "/page-snapshot?id=" + strings.Repeat("a", 32), 204},
	} {
		if got := call(PageSnapshotHandler, c.method, c.target).Code; got != c.want {
			t.Errorf("%s %s: %d, want %d", c.method, c.target, got, c.want)
		}
	}
	var stats struct{ Count, Bytes int }
	json.Unmarshal(call(PageSnapshotsHandler, "GET", "/page-snapshots").Body.Bytes(), &stats)
	if stats.Count != 1 || stats.Bytes != 30 {
		t.Errorf("stats %+v", stats)
	}
	if call(PageSnapshotHandler, "DELETE", "/page-snapshot?id="+info.ID).Code != 204 {
		t.Error("delete")
	}
	if call(PageSnapshotHandler, "GET", "/page-snapshot?id="+info.ID).Code != 404 {
		t.Error("a deleted snapshot must be gone")
	}
	info2, _ := writeSnapshot("<x>", snapshotMeta{})
	rec = call(PageSnapshotsHandler, "DELETE", "/page-snapshots")
	var del struct{ Deleted int }
	json.Unmarshal(rec.Body.Bytes(), &del)
	if rec.Code != 200 || del.Deleted != 1 {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	if call(PageSnapshotHandler, "GET", "/page-snapshot?id="+info2.ID).Code != 404 || call(PageSnapshotsHandler, "PUT", "/page-snapshots").Code != 405 {
		t.Error("delete-all / method check")
	}
}

// ---- /fetch-links wiring ----

func TestFetchLinksPassesTheSnapshotRequestAndAnswersIt(t *testing.T) {
	resetEngineTracking(t)
	recordStops(t)
	useBrowserSettings(t, shared.FetchEngineKatana)
	var asked []bool
	stubFetchLinks(t, func(ctx context.Context, rawURL, requestID string) (fetchLinksResponse, error) {
		asked = append(asked, wantsSnapshot(ctx))
		r := fetchLinksResponse{FinalURL: rawURL, Links: []pageLink{{URL: "http://93.184.216.34/j/1", Anchors: []int{1, 4}}}}
		if wantsSnapshot(ctx) {
			r.Snapshot = &snapshotInfo{ID: strings.Repeat("a", 32), Bytes: 5}
		}
		return r, nil
	})
	with := postFetchLinks(`{"url":"` + publicTestURL + `","snapshot":true}`)
	without := postFetchLinks(`{"url":"` + publicTestURL + `"}`)
	if len(asked) != 2 || !asked[0] || asked[1] {
		t.Fatalf("asked %v", asked)
	}
	var a, b fetchLinksResponse
	json.Unmarshal(with.Body.Bytes(), &a)
	json.Unmarshal(without.Body.Bytes(), &b)
	if a.Snapshot == nil || a.Snapshot.ID != strings.Repeat("a", 32) || len(a.Links[0].Anchors) != 2 || b.Snapshot != nil || strings.Contains(without.Body.String(), "snapshot") {
		t.Errorf("with=%s without=%s", with.Body.String(), without.Body.String())
	}
}

// ---- a live page ----

const snapshotFixture = `<!doctype html><html><head><title>Listing</title><style>.job{color:rgb(1,2,3)}</style><link rel="stylesheet" href="/site.css"></head><body onload="x()">
	<a class="job" href="/job/1">Backend Developer</a> <a href="/job/2">Frontend Developer</a>
	<div id="late"></div><script>setTimeout(function(){document.getElementById("late").innerHTML='<a href="/job/3">Late job</a>'},150)</script>
	<div id="host"></div><script>document.getElementById("host").attachShadow({mode:"open"}).innerHTML='<a href="/job/6">Shadow job</a>'</script>
	<img src="/img/a.png"><iframe src="/frame"></iframe></body></html>`

func TestKatanaSnapshotOfALivePage(t *testing.T) {
	requireKatanaChrome(t)
	dir := newStore(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/list": htmlPage(snapshotFixture),
		"/site.css": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte(".job{font-weight:700;background:url(/bg.png)}"))
		},
	})
	resp, err := fetchLinksWithKatana(withSnapshot(context.Background(), true), srv.URL+"/list", "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Snapshot == nil || resp.SnapshotError != "" {
		t.Fatalf("snapshot %+v %q", resp.Snapshot, resp.SnapshotError)
	}
	b, err := os.ReadFile(filepath.Join(dir, resp.Snapshot.ID+".html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	byURL := map[string]pageLink{}
	for _, l := range resp.Links {
		byURL[strings.TrimPrefix(l.URL, srv.URL)] = l
	}
	for _, p := range []string{"/job/1", "/job/2", "/job/3"} {
		l := byURL[p]
		if len(l.Anchors) != 1 {
			t.Fatalf("%s anchors %v", p, l.Anchors)
		}
		tag := regexpAnchorTag(page, l.Anchors[0])
		if !strings.Contains(tag, `data-href="`+srv.URL+p+`"`) || !strings.Contains(tag, `href="#"`) {
			t.Errorf("%s: anchor %d is %q", p, l.Anchors[0], tag)
		}
	}
	if sh := byURL["/job/6"]; len(sh.Anchors) != 1 || strings.Contains(page, `data-cinqo-a="`+toJSON(sh.Anchors[0])+`"`) {
		t.Errorf("a link in a shadow root is listed with its number but is not in the picture: %+v", sh)
	}
	for _, want := range []string{"rgb(1, 2, 3)", "font-weight: 700", `url("` + srv.URL + `/bg.png")`, srv.URL + "/img/a.png", "Content-Security-Policy"} {
		if !strings.Contains(page, want) {
			t.Errorf("%q missing:\n%s", want, page)
		}
	}
	for _, bad := range []string{"<script", "onload", "<iframe", "x()"} {
		if strings.Contains(page, bad) {
			t.Errorf("%q survived", bad)
		}
	}
	// without the request nothing is stored and no number is set
	plain, err := fetchLinksWithKatana(context.Background(), srv.URL+"/list", "")
	if err != nil || plain.Snapshot != nil || len(plain.Links[0].Anchors) != 0 {
		t.Errorf("%+v %v", plain, err)
	}
	if n, _, _ := snapshotStats(); n != 1 {
		t.Errorf("%d snapshots in the folder, want 1", n)
	}
}

// regexpAnchorTag returns the opening tag of the anchor numbered n in a snapshot ("" when it is not there).
func regexpAnchorTag(page string, n int) string {
	marker := `data-cinqo-a="` + toJSON(n) + `"`
	i := strings.Index(page, marker)
	if i < 0 {
		return ""
	}
	start := strings.LastIndex(page[:i], "<a")
	end := strings.Index(page[i:], ">")
	return page[start : i+end+1]
}
