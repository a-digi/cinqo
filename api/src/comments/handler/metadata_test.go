package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const (
	prof1 = "11111111-1111-4111-8111-111111111111"
	prof2 = "22222222-2222-4222-8222-222222222222"
)

func (e *env) addMeta(t *testing.T, w identity, text, profile string) string {
	t.Helper()
	body := fmt.Sprintf(`{"body":%q,"metadata":{"profileId":%q}}`, text, profile)
	r := e.create(w, "career", "job", job1, body)
	if r.code != http.StatusCreated {
		t.Fatalf("create %q: %d %v", text, r.code, r.out)
	}
	return r.message()["id"].(string)
}

func meta(c map[string]any) map[string]any {
	m, _ := c["metadata"].(map[string]any)
	return m
}

func texts(r result) []string {
	var out []string
	list, _ := r.message()["comments"].([]any)
	for _, c := range list {
		out = append(out, c.(map[string]any)["body"].(string))
	}
	return out
}

func same(a []string, b ...string) bool { return strings.Join(a, "|") == strings.Join(b, "|") }

// ---- create ----

func TestCreateStoresMetadataAndEveryCommentReportsIt(t *testing.T) {
	e := newEnv(t)
	r := e.create(ada, "career", "job", job1, `{"body":"with","metadata":{"profileId":"`+prof1+`"}}`)
	if r.code != http.StatusCreated {
		t.Fatalf("%d %v", r.code, r.out)
	}
	if m := meta(r.message()); m["profileId"] != prof1 || len(m) != 1 {
		t.Errorf("created: %v", r.message())
	}
	e.add(t, ada, "without")
	list := e.list(ada, "career", "job", job1, "")
	comments := list.message()["comments"].([]any)
	if len(comments) != 2 {
		t.Fatalf("%v", list.out)
	}
	for _, c := range comments {
		cm := c.(map[string]any)
		m, ok := cm["metadata"].(map[string]any)
		if !ok {
			t.Fatalf("a comment has no metadata object: %v", cm)
		}
		switch cm["body"] {
		case "with":
			if m["profileId"] != prof1 {
				t.Errorf("with: %v", m)
			}
		case "without":
			if len(m) != 0 {
				t.Errorf("without: %v", m)
			}
		}
	}
}

func TestCreateWithNullOrEmptyMetadataIsNoMetadata(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`{"body":"a","metadata":null}`, `{"body":"b","metadata":{}}`, `{"body":"c"}`} {
		r := e.create(ada, "career", "job", job1, body)
		if r.code != http.StatusCreated || len(meta(r.message())) != 0 || meta(r.message()) == nil {
			t.Errorf("%s: %d %v", body, r.code, r.out)
		}
	}
}

func TestCreateRefusesBadMetadata(t *testing.T) {
	e := newEnv(t)
	nine := `{"a":"1","b":"1","c":"1","d":"1","e":"1","f":"1","g":"1","h":"1","i":"1"}`
	for name, tc := range map[string]struct{ body, mention string }{
		"a string":       {`{"body":"x","metadata":"profile"}`, "metadata must be an object of strings"},
		"an array":       {`{"body":"x","metadata":["a"]}`, "metadata must be an object of strings"},
		"a number value": {`{"body":"x","metadata":{"profileId":5}}`, "metadata must be an object of strings"},
		"a nested value": {`{"body":"x","metadata":{"profileId":{"a":"b"}}}`, "metadata must be an object of strings"},
		"a bad key":      {`{"body":"x","metadata":{"Profile":"v"}}`, "metadata"},
		"an empty value": {`{"body":"x","metadata":{"profileId":""}}`, "metadata.profileId"},
		"a long value":   {`{"body":"x","metadata":{"profileId":"` + strings.Repeat("v", 201) + `"}}`, "metadata.profileId"},
		"nine keys":      {`{"body":"x","metadata":` + nine + `}`, "metadata"},
	} {
		r := e.create(ada, "career", "job", job1, tc.body)
		if r.code != http.StatusBadRequest || !strings.Contains(r.errText(), tc.mention) {
			t.Errorf("%s: %d %q, want 400 mentioning %q", name, r.code, r.errText(), tc.mention)
		}
	}
	if list := e.list(ada, "career", "job", job1, ""); len(texts(list)) != 0 {
		t.Errorf("a refused comment was stored: %v", texts(list))
	}
}

// A reply takes its thread's metadata whatever it says.
func TestAReplyOverTheAPIKeepsTheThreadsMetadata(t *testing.T) {
	e := newEnv(t)
	top := e.addMeta(t, ada, "top", prof1)
	r := e.create(grace, "career", "job", job1, `{"body":"reply","parentId":"`+top+`","metadata":{"profileId":"`+prof2+`"}}`)
	if r.code != http.StatusCreated || meta(r.message())["profileId"] != prof1 {
		t.Fatalf("%d %v", r.code, r.out)
	}
	r = e.reply(t, grace, top, "plain reply")
	if r.code != http.StatusCreated || meta(r.message())["profileId"] != prof1 {
		t.Errorf("%d %v", r.code, r.out)
	}
	if got := e.list(ada, "career", "job", job1, "metadata.profileId="+prof2); len(texts(got)) != 0 {
		t.Errorf("a reply escaped to the other profile: %v", texts(got))
	}
}

// ---- edit ----

func TestEditNeverChangesTheMetadataAndIgnoresWhatItSays(t *testing.T) {
	e := newEnv(t)
	id := e.addMeta(t, ada, "before", prof1)
	for _, body := range []string{
		`{"body":"after","metadata":{"profileId":"` + prof2 + `"}}`,
		`{"body":"after2","metadata":{}}`,
		`{"body":"after3","metadata":"not even an object"}`,
		`{"body":"after4","metadata":{"Bad Key":5}}`,
	} {
		r := e.edit(ada, "career", "job", job1, id, body)
		if r.code != http.StatusOK || meta(r.message())["profileId"] != prof1 {
			t.Errorf("%s: %d %v", body, r.code, r.out)
		}
	}
	got := e.list(ada, "career", "job", job1, "metadata.profileId="+prof1)
	if !same(texts(got), "after4") {
		t.Errorf("still in its profile: %v", texts(got))
	}
}

// ---- list ----

func TestListScopeShowsOnlyThatProfilesComments(t *testing.T) {
	e := newEnv(t)
	e.addMeta(t, ada, "mine", prof1)
	e.addMeta(t, grace, "theirs", prof2)
	e.add(t, ada, "nobodys")

	r := e.list(ada, "career", "job", job1, "metadata.profileId="+prof1)
	if r.code != http.StatusOK || !same(texts(r), "mine") || r.message()["total"] != float64(1) {
		t.Errorf("p1: %d %v", r.code, r.out)
	}
	r = e.list(ada, "career", "job", job1, "metadata.profileId="+prof2)
	if !same(texts(r), "theirs") || r.message()["total"] != float64(1) {
		t.Errorf("p2: %v", r.out)
	}
	r = e.list(ada, "career", "job", job1, "metadata.profileId=33333333-3333-4333-8333-333333333333")
	if len(texts(r)) != 0 || r.message()["total"] != float64(0) {
		t.Errorf("unknown profile: %v", r.out)
	}
	r = e.list(ada, "career", "job", job1, "")
	if len(texts(r)) != 3 || r.message()["total"] != float64(3) {
		t.Errorf("unscoped: %v", r.out)
	}
	// a parameter that is not a metadata one changes nothing
	r = e.list(ada, "career", "job", job1, "metadataX=1&other=2")
	if len(texts(r)) != 3 {
		t.Errorf("unrelated parameters: %v", r.out)
	}
}

func TestListScopeCombinesWithTheOtherFiltersAndPaging(t *testing.T) {
	e := newEnv(t)
	for i := 1; i <= 3; i++ {
		e.addMeta(t, ada, fmt.Sprintf("ada-p1-%d", i), prof1)
		e.addMeta(t, grace, fmt.Sprintf("grace-p1-%d", i), prof1)
		e.addMeta(t, ada, fmt.Sprintf("ada-p2-%d", i), prof2)
	}
	r := e.list(ada, "career", "job", job1, "metadata.profileId="+prof1+"&authorId=ada-id")
	if len(texts(r)) != 3 || r.message()["total"] != float64(3) {
		t.Errorf("author: %v", texts(r))
	}
	for _, b := range texts(r) {
		if !strings.HasPrefix(b, "ada-p1-") {
			t.Errorf("unexpected %q", b)
		}
	}
	var seen []string
	before := ""
	for i := 0; i < 10; i++ {
		q := "limit=2&metadata.profileId=" + prof1
		if before != "" {
			q += "&before=" + before
		}
		page := e.list(ada, "career", "job", job1, q)
		if page.code != http.StatusOK {
			t.Fatalf("%v", page.out)
		}
		seen = append(seen, texts(page)...)
		next, _ := page.message()["nextBefore"].(string)
		if next == "" {
			break
		}
		before = next
	}
	if len(seen) != 6 {
		t.Errorf("walked %v", seen)
	}
}

func TestListScopeWithTwoKeysNeedsBoth(t *testing.T) {
	e := newEnv(t)
	e.create(ada, "career", "job", job1, `{"body":"both","metadata":{"profileId":"`+prof1+`","view":"board"}}`)
	e.create(ada, "career", "job", job1, `{"body":"one","metadata":{"profileId":"`+prof1+`"}}`)
	r := e.list(ada, "career", "job", job1, "metadata.profileId="+prof1+"&metadata.view=board")
	if !same(texts(r), "both") {
		t.Errorf("%v", texts(r))
	}
}

func TestListRefusesABadScope(t *testing.T) {
	e := newEnv(t)
	e.add(t, ada, "x")
	for name, tc := range map[string]struct{ query, mention string }{
		"a bare metadata parameter": {"metadata=" + prof1, "metadata.<key>=<value>"},
		"a key given twice":         {"metadata.profileId=a&metadata.profileId=b", "metadata.profileId: must be given once"},
		"an empty key":              {"metadata.=x", "metadata"},
		"a bad key":                 {"metadata.Profile=x", "metadata"},
		"a key with a quote":        {"metadata." + url.QueryEscape(`a"] OR 1=1 --`) + "=x", "metadata"},
		"an empty value":            {"metadata.profileId=", "metadata.profileId"},
		"a long value":              {"metadata.profileId=" + strings.Repeat("v", 201), "metadata.profileId"},
	} {
		r := e.list(ada, "career", "job", job1, tc.query)
		if r.code != http.StatusBadRequest || !strings.Contains(r.errText(), tc.mention) {
			t.Errorf("%s: %d %q, want 400 mentioning %q", name, r.code, r.errText(), tc.mention)
		}
	}
}

// A value that looks like SQL only matches itself.
func TestListScopeValueIsNeverSQL(t *testing.T) {
	e := newEnv(t)
	evil := `x' OR '1'='1`
	e.addMeta(t, ada, "evil", evil)
	e.addMeta(t, ada, "plain", prof1)
	r := e.list(ada, "career", "job", job1, "metadata.profileId="+url.QueryEscape(evil))
	if !same(texts(r), "evil") {
		t.Errorf("%v", texts(r))
	}
}

// ---- permissions are what they were ----

func TestMetadataChangesNoPermission(t *testing.T) {
	e := newEnv(t)
	e.addMeta(t, ada, "mine", prof1)
	// a reader may read a scoped list, and is told they cannot add
	r := e.list(reader, "career", "job", job1, "metadata.profileId="+prof1)
	if r.code != http.StatusOK || !same(texts(r), "mine") || r.message()["canCreate"] != false {
		t.Errorf("reader: %d %v", r.code, r.out)
	}
	// a reader may not create, with or without metadata
	body := `{"body":"x","metadata":{"profileId":"` + prof1 + `"}}`
	if r := e.create(reader, "career", "job", job1, body); r.code != http.StatusForbidden {
		t.Errorf("reader create: %d %v", r.code, r.out)
	}
	// someone without the scope gets the same "not found" as before, scope or not
	if r := e.list(nobody, "career", "job", job1, "metadata.profileId="+prof1); r.code != http.StatusNotFound {
		t.Errorf("nobody list: %d", r.code)
	}
	if r := e.create(nobody, "career", "job", job1, body); r.code != http.StatusNotFound {
		t.Errorf("nobody create: %d", r.code)
	}
	// and a writer sees canCreate under a scope
	if r := e.list(ada, "career", "job", job1, "metadata.profileId="+prof1); r.message()["canCreate"] != true {
		t.Errorf("writer: %v", r.out)
	}
}

// Another owner's comments are never reached, scope or not.
func TestTheScopeStaysInsideTheOwnersFile(t *testing.T) {
	e := newEnv(t)
	e.addMeta(t, ada, "career's", prof1)
	if r := e.list(admin, "cinqo", "note", job1, "metadata.profileId="+prof1); r.code != http.StatusOK || len(texts(r)) != 0 {
		t.Errorf("%d %v", r.code, r.out)
	}
}

// Delete still takes the whole thread, whatever it carries.
func TestDeleteStillTakesAThreadWithMetadata(t *testing.T) {
	e := newEnv(t)
	top := e.addMeta(t, ada, "top", prof1)
	e.reply(t, grace, top, "reply")
	if r := e.remove(ada, "career", "job", job1, top); r.code != http.StatusNoContent {
		t.Fatalf("%d %v", r.code, r.out)
	}
	if r := e.list(ada, "career", "job", job1, "metadata.profileId="+prof1); len(texts(r)) != 0 || r.message()["total"] != float64(0) {
		t.Errorf("%v", r.out)
	}
}

// A type error that is not about metadata keeps its old message.
func TestOtherBodyTypeErrorsKeepTheirMessage(t *testing.T) {
	e := newEnv(t)
	for _, body := range []string{`{"body":5}`, `{"body":"x","parentId":7}`, `not json`} {
		r := e.create(ada, "career", "job", job1, body)
		if r.code != http.StatusBadRequest || !strings.Contains(r.errText(), `body must be {"body"`) {
			t.Errorf("%s: %d %q", body, r.code, r.errText())
		}
	}
}
