package languageshandlers

import (
	"encoding/json"
	"strings"
	"testing"

	"career-tool-backend/handler/handlertest"
	"career-tool-backend/profile"
)

func call(method, target, body string) (int, map[string]any, string) {
	rec := handlertest.PiCall(LanguagesHandler, method, target, body)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func list(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	res := []map[string]any{}
	for _, x := range out["languages"].([]any) {
		res = append(res, x.(map[string]any))
	}
	return res
}

func TestLanguagesRoute(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, err := profile.CreateProfile("Ada", "L")
	if err != nil {
		t.Fatal(err)
	}
	code, out, body := call("POST", "/languages", `{"profileId":"`+p+`","language":"German","level":"c1","canSpeak":true,"canWrite":false}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	l := list(t, out)
	if len(l) != 1 || l[0]["language"] != "German" || l[0]["level"] != "c1" || l[0]["canSpeak"] != true || l[0]["canWrite"] != false || l[0]["profileId"] != p {
		t.Fatalf("%v", l)
	}
	id := l[0]["id"].(string)
	_, out, _ = call("POST", "/languages", `{"profileId":"`+p+`","language":"Greek","level":"native","canSpeak":true,"canWrite":true}`)
	if got := list(t, out); len(got) != 2 || got[0]["language"] != "Greek" {
		t.Errorf("native first: %v", got)
	}
	code, out, _ = call("PUT", "/languages", `{"id":"`+id+`","canWrite":true}`)
	if code != 200 {
		t.Fatal(code)
	}
	for _, e := range list(t, out) {
		if e["id"] == id && (e["canWrite"] != true || e["canSpeak"] != true || e["level"] != "c1") {
			t.Errorf("a partial update: %v", e)
		}
	}
	if code, out, _ = call("GET", "/languages?profileId="+p, ""); code != 200 || len(list(t, out)) != 2 {
		t.Errorf("%d %v", code, out)
	}
	for name, c := range map[string]struct {
		method, target, body string
		want                 int
		msg                  string
	}{
		"no name":         {"POST", "/languages", `{"profileId":"` + p + `","language":" ","canSpeak":true}`, 400, "name"},
		"bad level":       {"POST", "/languages", `{"profileId":"` + p + `","language":"X","level":"fluent","canSpeak":true}`, 400, "level"},
		"neither":         {"POST", "/languages", `{"profileId":"` + p + `","language":"X"}`, 400, "spoken"},
		"duplicate":       {"POST", "/languages", `{"profileId":"` + p + `","language":"german","canSpeak":true}`, 400, "already listed"},
		"unknown profile": {"POST", "/languages", `{"profileId":"nope","language":"X","canSpeak":true}`, 400, "unknown profile"},
		"no profile":      {"POST", "/languages", `{"language":"X"}`, 400, ""},
		"not json":        {"POST", "/languages", `{`, 400, ""},
		"put unknown":     {"PUT", "/languages", `{"id":"nope","level":"a1"}`, 400, "unknown language"},
		"put neither":     {"PUT", "/languages", `{"id":"` + id + `","canSpeak":false,"canWrite":false}`, 400, "spoken"},
		"put no id":       {"PUT", "/languages", `{"level":"a1"}`, 400, ""},
		"get no profile":  {"GET", "/languages", "", 400, ""},
		"delete no id":    {"DELETE", "/languages", "", 400, ""},
		"delete unknown":  {"DELETE", "/languages?id=nope", "", 400, "unknown language"},
		"patch":           {"PATCH", "/languages", "", 405, ""},
	} {
		code, _, body := call(c.method, c.target, c.body)
		if code != c.want || !strings.Contains(body, c.msg) {
			t.Errorf("%s: %d %q", name, code, body)
		}
	}
	_, out, _ = call("GET", "/languages?profileId="+p, "")
	if len(list(t, out)) != 2 {
		t.Errorf("a refusal changed the list: %v", out)
	}
	if code, _, _ := call("DELETE", "/languages?id="+id, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	_, out, _ = call("GET", "/languages?profileId="+p, "")
	if len(list(t, out)) != 1 {
		t.Errorf("%v", out)
	}
}
