package hobbieshandlers

import (
	"encoding/json"
	"strings"
	"testing"

	"career-tool-backend/handler/handlertest"
	"career-tool-backend/profile"
)

func call(method, target, body string) (int, map[string]any, string) {
	rec := handlertest.PiCall(HobbiesHandler, method, target, body)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func list(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	res := []map[string]any{}
	for _, x := range out["hobbies"].([]any) {
		res = append(res, x.(map[string]any))
	}
	return res
}

func TestHobbiesRoute(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, err := profile.CreateProfile("Ada", "L")
	if err != nil {
		t.Fatal(err)
	}
	code, out, body := call("POST", "/hobbies", `{"profileId":"`+p+`","name":"Chess","description":"club"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	l := list(t, out)
	if len(l) != 1 || l[0]["name"] != "Chess" || l[0]["description"] != "club" || l[0]["profileId"] != p {
		t.Fatalf("%v", l)
	}
	id := l[0]["id"].(string)
	_, out, _ = call("POST", "/hobbies", `{"profileId":"`+p+`","name":"Archery"}`)
	if got := list(t, out); len(got) != 2 || got[0]["name"] != "Archery" {
		t.Errorf("by name: %v", got)
	}
	code, out, _ = call("PUT", "/hobbies", `{"id":"`+id+`","description":"weekly"}`)
	if code != 200 {
		t.Fatal(code)
	}
	for _, e := range list(t, out) {
		if e["id"] == id && (e["description"] != "weekly" || e["name"] != "Chess") {
			t.Errorf("a partial update: %v", e)
		}
	}
	if code, out, _ = call("GET", "/hobbies?profileId="+p, ""); code != 200 || len(list(t, out)) != 2 {
		t.Errorf("%d %v", code, out)
	}
	for name, c := range map[string]struct {
		method, target, body string
		want                 int
		msg                  string
	}{
		"no name":         {"POST", "/hobbies", `{"profileId":"` + p + `","name":" "}`, 400, "name"},
		"duplicate":       {"POST", "/hobbies", `{"profileId":"` + p + `","name":"chess"}`, 400, "already listed"},
		"too long":        {"POST", "/hobbies", `{"profileId":"` + p + `","name":"` + strings.Repeat("x", 201) + `"}`, 400, "too long"},
		"unknown profile": {"POST", "/hobbies", `{"profileId":"nope","name":"X"}`, 400, "unknown profile"},
		"no profile":      {"POST", "/hobbies", `{"name":"X"}`, 400, ""},
		"not json":        {"POST", "/hobbies", `{`, 400, ""},
		"put unknown":     {"PUT", "/hobbies", `{"id":"nope","name":"x"}`, 400, "unknown hobby"},
		"put no id":       {"PUT", "/hobbies", `{"name":"x"}`, 400, ""},
		"get no profile":  {"GET", "/hobbies", "", 400, ""},
		"delete no id":    {"DELETE", "/hobbies", "", 400, ""},
		"delete unknown":  {"DELETE", "/hobbies?id=nope", "", 400, "unknown hobby"},
		"patch":           {"PATCH", "/hobbies", "", 405, ""},
	} {
		code, _, body := call(c.method, c.target, c.body)
		if code != c.want || !strings.Contains(body, c.msg) {
			t.Errorf("%s: %d %q", name, code, body)
		}
	}
	if code, _, _ := call("DELETE", "/hobbies?id="+id, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	_, out, _ = call("GET", "/hobbies?profileId="+p, "")
	if len(list(t, out)) != 1 {
		t.Errorf("%v", out)
	}
}
