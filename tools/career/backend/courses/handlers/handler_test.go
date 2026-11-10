package courseshandlers

import (
	"encoding/json"
	"strings"
	"testing"

	"career-tool-backend/handler/handlertest"
	"career-tool-backend/profile"
)

func call(method, target, body string) (int, map[string]any, string) {
	rec := handlertest.PiCall(CoursesHandler, method, target, body)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func list(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	res := []map[string]any{}
	for _, x := range out["courses"].([]any) {
		res = append(res, x.(map[string]any))
	}
	return res
}

func TestCoursesRoute(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, err := profile.CreateProfile("Ada", "L")
	if err != nil {
		t.Fatal(err)
	}
	code, out, body := call("POST", "/courses", `{"profileId":"`+p+`","title":"Go","provider":"Udemy","completedDate":"2024-05","credentialUrl":"https://example.com/c","description":"adv"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	l := list(t, out)
	if len(l) != 1 || l[0]["title"] != "Go" || l[0]["provider"] != "Udemy" || l[0]["completedDate"] != "2024-05" || l[0]["credentialUrl"] != "https://example.com/c" || l[0]["description"] != "adv" {
		t.Fatalf("%v", l)
	}
	id := l[0]["id"].(string)
	_, out, _ = call("POST", "/courses", `{"profileId":"`+p+`","title":"Newer","completedDate":"2025"}`)
	if got := list(t, out); len(got) != 2 || got[0]["title"] != "Newer" {
		t.Errorf("newest first: %v", got)
	}
	code, out, _ = call("PUT", "/courses", `{"id":"`+id+`","provider":"Coursera"}`)
	if code != 200 {
		t.Fatal(code)
	}
	for _, e := range list(t, out) {
		if e["id"] == id && (e["provider"] != "Coursera" || e["title"] != "Go" || e["completedDate"] != "2024-05") {
			t.Errorf("a partial update: %v", e)
		}
	}
	if code, out, _ = call("GET", "/courses?profileId="+p, ""); code != 200 || len(list(t, out)) != 2 {
		t.Errorf("%d %v", code, out)
	}
	for name, c := range map[string]struct {
		method, target, body string
		want                 int
		msg                  string
	}{
		"no title":        {"POST", "/courses", `{"profileId":"` + p + `","title":" "}`, 400, "title"},
		"bad date":        {"POST", "/courses", `{"profileId":"` + p + `","title":"X","completedDate":"soon"}`, 400, "YYYY"},
		"bad link":        {"POST", "/courses", `{"profileId":"` + p + `","title":"X","credentialUrl":"javascript:alert(1)"}`, 400, "http"},
		"too long":        {"POST", "/courses", `{"profileId":"` + p + `","title":"` + strings.Repeat("x", 201) + `"}`, 400, "too long"},
		"unknown profile": {"POST", "/courses", `{"profileId":"nope","title":"X"}`, 400, "unknown profile"},
		"no profile":      {"POST", "/courses", `{"title":"X"}`, 400, ""},
		"not json":        {"POST", "/courses", `{`, 400, ""},
		"put unknown":     {"PUT", "/courses", `{"id":"nope","title":"x"}`, 400, "unknown course"},
		"put bad link":    {"PUT", "/courses", `{"id":"` + id + `","credentialUrl":"ftp://x/y"}`, 400, "http"},
		"put no id":       {"PUT", "/courses", `{"title":"x"}`, 400, ""},
		"get no profile":  {"GET", "/courses", "", 400, ""},
		"delete no id":    {"DELETE", "/courses", "", 400, ""},
		"delete unknown":  {"DELETE", "/courses?id=nope", "", 400, "unknown course"},
		"patch":           {"PATCH", "/courses", "", 405, ""},
	} {
		code, _, body := call(c.method, c.target, c.body)
		if code != c.want || !strings.Contains(body, c.msg) {
			t.Errorf("%s: %d %q", name, code, body)
		}
	}
	_, out, _ = call("GET", "/courses?profileId="+p, "")
	if len(list(t, out)) != 2 {
		t.Errorf("a refusal changed the list: %v", out)
	}
	if code, _, _ := call("DELETE", "/courses?id="+id, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	_, out, _ = call("GET", "/courses?profileId="+p, "")
	if len(list(t, out)) != 1 {
		t.Errorf("%v", out)
	}
}
