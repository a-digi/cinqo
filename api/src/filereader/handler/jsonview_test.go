package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/a-digi/cinqo/src/filereader/jsonview"
	"github.com/a-digi/cinqo/src/filereader/source"
)

// newJSONEnv is an environment whose one trace file holds the given text and whose handlers have a JSON cache.
func newJSONEnv(t *testing.T, content string) *env {
	t.Helper()
	e := newEnv(t, 1)
	if err := os.WriteFile(filepath.Join(e.root, conv, turn+".txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cache, err := jsonview.NewCache(filepath.Join(t.TempDir(), "jsoncache"), jsonview.CacheLimits{})
	if err != nil {
		t.Fatal(err)
	}
	e.d.jsonView = cache
	return e
}

func (e *env) searchQ(who identity, src, ref, query string) result {
	return call(func(w http.ResponseWriter, r *http.Request) { e.d.search(w, r, who, src, ref) }, query)
}

func (e *env) segmentQ(who identity, src, ref, query string) result {
	return call(func(w http.ResponseWriter, r *http.Request) { e.d.segment(w, r, who, src, ref) }, query)
}

func (e *env) infoQ(who identity, src, ref, query string) result {
	return call(func(w http.ResponseWriter, r *http.Request) { e.d.info(w, r, who, src, ref) }, query)
}

const oneLine = `{"id":"x","fields":[{"label":"a","selector":"b"}],"html":"<p>hi</p>"}`

func lineTexts(res result) []string {
	var out []string
	for _, l := range res.message()["lines"].([]any) {
		out = append(out, l.(map[string]any)["text"].(string))
	}
	return out
}

func TestViewJSONReadsTheIndentedCopyThroughTheSameRoutes(t *testing.T) {
	e := newJSONEnv(t, oneLine)

	raw := e.lines(admin, source.AITraceID, ref, "")
	if got := lineTexts(raw); len(got) != 1 || got[0] != oneLine {
		t.Fatalf("the raw view must stay one line: %v", got)
	}

	pretty := e.lines(admin, source.AITraceID, ref, "view=json")
	if pretty.code != 200 {
		t.Fatalf("code %d: %v", pretty.code, pretty.out)
	}
	got := lineTexts(pretty)
	want := []string{`{`, `  "id": "x",`, `  "fields": [`, `    {`, `      "label": "a",`, `      "selector": "b"`, `    }`, `  ],`, `  "html": "<p>hi</p>"`, `}`}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("indented view:\n%s", strings.Join(got, "\n"))
	}

	info := e.infoQ(admin, source.AITraceID, ref, "view=json").message()
	if info["view"] != "json" || info["sourceBytes"].(float64) != float64(len(oneLine)) || info["totalLines"].(float64) != 10 || info["complete"] != true {
		t.Fatalf("info: %v", info)
	}
	if rawInfo := e.infoQ(admin, source.AITraceID, ref, "").message(); rawInfo["view"] != nil || rawInfo["totalLines"].(float64) != 1 {
		t.Fatalf("the raw info must not change: %v", rawInfo)
	}
}

func TestViewJSONSearchAndSegmentWorkOnTheIndentedView(t *testing.T) {
	e := newJSONEnv(t, oneLine)
	res := e.searchQ(admin, source.AITraceID, ref, "view=json&q=selector")
	m := res.message()
	matches, _ := m["matches"].([]any)
	if res.code != 200 || len(matches) != 1 || matches[0].(map[string]any)["n"].(float64) != 6 {
		t.Fatalf("search in the indented view: %d %v", res.code, res.out)
	}
	seg := e.segmentQ(admin, source.AITraceID, ref, "view=json&line=2&offset=0&length=100").message()
	if seg == nil || seg["text"] != `  "id": "x",` {
		t.Fatalf("segment of an indented line: %v", seg)
	}
}

func TestViewIsCheckedAfterTheScopeAndOnlyKnownValuesPass(t *testing.T) {
	e := newJSONEnv(t, oneLine)
	// no scope: 403 for every view and for a file that does not exist, never a hint about the JSON
	for _, q := range []string{"", "view=json", "view=bogus"} {
		for _, r := range []string{ref, conv + "/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "../../etc/passwd"} {
			if res := e.infoQ(user, source.AITraceID, r, q); res.code != http.StatusForbidden {
				t.Errorf("view=%q ref=%q for a user without the scope: %d", q, r, res.code)
			}
		}
	}
	if n, _ := e.d.jsonView.Stats(); n != 0 {
		t.Fatalf("a request without the scope made %d copies", n)
	}
	for _, q := range []string{"view=bogus", "view=JSON", "view=json,raw", "view=%00"} {
		if res := e.infoQ(admin, source.AITraceID, ref, q); res.code != http.StatusBadRequest {
			t.Errorf("%q: %d", q, res.code)
		}
	}
	for _, q := range []string{"view=raw", "view="} {
		if res := e.infoQ(admin, source.AITraceID, ref, q); res.code != 200 {
			t.Errorf("%q: %d", q, res.code)
		}
	}
	// a file that does not exist: 404 under both views, and no copy
	missing := conv + "/bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	for _, q := range []string{"", "view=json"} {
		if res := e.infoQ(admin, source.AITraceID, missing, q); res.code != http.StatusNotFound {
			t.Errorf("a missing file with view=%q: %d", q, res.code)
		}
	}
}

func TestAFileThatIsNotJSONAnswers422WithTheByteAndNothingOfItsContent(t *testing.T) {
	secret := "TOPSECRETVALUE"
	e := newJSONEnv(t, `{"k":"`+secret+`" "oops"}`)
	res := e.lines(admin, source.AITraceID, ref, "view=json")
	if res.code != http.StatusUnprocessableEntity {
		t.Fatalf("code %d: %v", res.code, res.out)
	}
	msg, _ := res.out["message"].(string)
	if !strings.Contains(msg, "not valid JSON") || !strings.Contains(msg, "at byte") || strings.Contains(msg, secret) {
		t.Fatalf("message %q", msg)
	}
	if n, _ := e.d.jsonView.Stats(); n != 0 {
		t.Fatalf("an invalid file was kept: %d", n)
	}
	// the same file is still readable as text
	if raw := e.lines(admin, source.AITraceID, ref, ""); raw.code != 200 {
		t.Fatalf("raw: %d", raw.code)
	}
}

func TestOtherFormattingFailuresHaveTheirOwnStatus(t *testing.T) {
	tooBig := newJSONEnv(t, `["`+strings.Repeat("x", 300)+`"]`)
	cache, _ := jsonview.NewCache(filepath.Join(t.TempDir(), "c"), jsonview.CacheLimits{Format: jsonview.Limits{MaxBytes: 100}})
	tooBig.d.jsonView = cache
	if res := tooBig.lines(admin, source.AITraceID, ref, "view=json"); res.code != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: %d", res.code)
	}

	deep := newJSONEnv(t, strings.Repeat("[", 10)+strings.Repeat("]", 10))
	cache, _ = jsonview.NewCache(filepath.Join(t.TempDir(), "c"), jsonview.CacheLimits{Format: jsonview.Limits{MaxDepth: 5}})
	deep.d.jsonView = cache
	if res := deep.lines(admin, source.AITraceID, ref, "view=json"); res.code != http.StatusUnprocessableEntity {
		t.Errorf("too deep: %d", res.code)
	}

	none := newJSONEnv(t, oneLine)
	none.d.jsonView = nil // the boot could not set the cache up
	if res := none.lines(admin, source.AITraceID, ref, "view=json"); res.code != http.StatusServiceUnavailable {
		t.Errorf("not available: %d", res.code)
	}
	if res := none.lines(admin, source.AITraceID, ref, ""); res.code != 200 {
		t.Errorf("raw without the cache: %d", res.code)
	}
}

func TestAChangedFileGetsAFreshIndentedView(t *testing.T) {
	e := newJSONEnv(t, `{"a":1}`)
	first := lineTexts(e.lines(admin, source.AITraceID, ref, "view=json"))
	if err := os.WriteFile(filepath.Join(e.root, conv, turn+".txt"), []byte(`{"a":1,"b":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	second := lineTexts(e.lines(admin, source.AITraceID, ref, "view=json"))
	if len(first) != 3 || len(second) != 4 || second[2] != `  "b": 2` {
		t.Fatalf("first %v second %v", first, second)
	}
}

func TestTheAuditLineSaysViewJSONAndNeverTheContent(t *testing.T) {
	e := newJSONEnv(t, oneLine)
	e.lines(admin, source.AITraceID, ref, "")
	e.lines(admin, source.AITraceID, ref, "view=json")
	e.lines(admin, source.AITraceID, ref, "view=json") // held back: one line per person, file and view
	var raw, json int
	for _, l := range e.audit {
		switch {
		case strings.Contains(l, "view=json"):
			json++
		case strings.Contains(l, "opened source="):
			raw++
		}
		if strings.Contains(l, "selector") || strings.Contains(l, "<p>") {
			t.Errorf("content in the audit line: %q", l)
		}
	}
	if raw != 1 || json != 1 {
		t.Fatalf("raw lines %d, json lines %d: %v", raw, json, e.audit)
	}
	for _, l := range e.audit {
		if strings.HasSuffix(l, "view=raw") {
			t.Errorf("the raw line changed its shape: %q", l)
		}
	}
}

func TestManyViewersOfOneFileMakeOneCopy(t *testing.T) {
	e := newJSONEnv(t, `[`+strings.Repeat(`{"k":"vvvvvvvv"},`, 5000)+`1]`)
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = e.lines(admin, source.AITraceID, ref, "view=json&from=1&count=5").code
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != 200 {
			t.Fatalf("viewer %d: %d", i, c)
		}
	}
	if n, _ := e.d.jsonView.Stats(); n != 1 {
		t.Fatalf("%d copies for one file", n)
	}
}

func TestNoServerPathLeaksFromTheJSONView(t *testing.T) {
	e := newJSONEnv(t, oneLine)
	for _, q := range []string{"view=json", "view=json&from=1&count=3"} {
		res := e.lines(admin, source.AITraceID, ref, q)
		raw := res.out
		blob := ""
		for k, v := range raw {
			blob += k + "=" + toString(v) + ";"
		}
		if strings.Contains(blob, e.root) || strings.Contains(blob, "jsoncache") || strings.Contains(blob, ".txt") {
			t.Errorf("a path leaked: %s", blob)
		}
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		s := ""
		for k, vv := range x {
			s += k + ":" + toString(vv) + ","
		}
		return s
	case []any:
		s := ""
		for _, vv := range x {
			s += toString(vv) + ","
		}
		return s
	}
	return ""
}

func TestTwoFilesOfTheSameSizeAndTimeNeverShareACopy(t *testing.T) {
	e := newJSONEnv(t, `{"a":1}`)
	other := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	otherPath := filepath.Join(e.root, conv, other+".txt")
	if err := os.WriteFile(otherPath, []byte(`{"a":2}`), 0o600); err != nil { // the same length, different content
		t.Fatal(err)
	}
	firstPath := filepath.Join(e.root, conv, turn+".txt")
	st, _ := os.Stat(firstPath)
	if err := os.Chtimes(otherPath, st.ModTime(), st.ModTime()); err != nil { // and the same modification time
		t.Fatal(err)
	}
	one := lineTexts(e.lines(admin, source.AITraceID, ref, "view=json"))
	two := lineTexts(e.lines(admin, source.AITraceID, conv+"/"+other, "view=json"))
	if len(one) != 3 || len(two) != 3 || one[1] != `  "a": 1` || two[1] != `  "a": 2` {
		t.Fatalf("one %v two %v: a copy was shared between two files", one, two)
	}
	if n, _ := e.d.jsonView.Stats(); n != 2 {
		t.Fatalf("%d copies for two files", n)
	}
}

// A 409 means "not indexed yet" to the viewer (it waits and asks again for up to a minute), so a file that changed while it was
// formatted must be a plain try-again, not a 409.
func TestAFileThatChangedWhileFormattedIsATryAgainNotAConflict(t *testing.T) {
	e := newJSONEnv(t, oneLine)
	rec := httptest.NewRecorder()
	e.d.fail(rec, httptest.NewRequest(http.MethodGet, "/x", nil), jsonview.ErrChanged)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", rec.Code)
	}
	if rec.Code == http.StatusConflict {
		t.Fatal("a 409 would make the viewer wait for an index")
	}
}
