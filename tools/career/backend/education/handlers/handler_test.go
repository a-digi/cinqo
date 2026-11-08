package educationhandlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/education"
	"career-tool-backend/handler/handlertest"
	"career-tool-backend/profile"
)

func edCall(method, target, body string) (int, map[string]any, string) {
	rec := handlertest.PiCall(EducationHandler, method, target, body)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func edList(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	list := []map[string]any{}
	for _, x := range out["education"].([]any) {
		list = append(list, x.(map[string]any))
	}
	return list
}

func TestEducationRoute(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, err := profile.CreateProfile("Ada", "L")
	if err != nil {
		t.Fatal(err)
	}
	code, out, body := edCall("POST", "/education", `{"profileId":"`+p+`","institution":"MIT","degree":"B.Sc.","fieldOfStudy":"Physics","startDate":"2010-09","endDate":"2014","description":"thesis"}`)
	if code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	list := edList(t, out)
	if len(list) != 1 || list[0]["institution"] != "MIT" || list[0]["fieldOfStudy"] != "Physics" || list[0]["endDate"] != "2014" {
		t.Fatalf("%v", list)
	}
	id := list[0]["id"].(string)
	_, out, _ = edCall("POST", "/education", `{"profileId":"`+p+`","institution":"Evening school","startDate":"2020"}`)
	if len(edList(t, out)) != 2 {
		t.Errorf("%v", out)
	}
	// a partial update answers with the profile's list
	code, out, _ = edCall("PUT", "/education", `{"id":"`+id+`","degree":"M.Sc."}`)
	if code != 200 {
		t.Fatal(code)
	}
	var found map[string]any
	for _, e := range edList(t, out) {
		if e["id"] == id {
			found = e
		}
	}
	if found["degree"] != "M.Sc." || found["institution"] != "MIT" {
		t.Errorf("%v", found)
	}
	// GET
	code, out, _ = edCall("GET", "/education?profileId="+p, "")
	if code != 200 || len(edList(t, out)) != 2 {
		t.Errorf("%d %v", code, out)
	}
	// refusals
	for name, c := range map[string]struct {
		method, target, body string
		want                 int
		msg                  string
	}{
		"no institution":   {"POST", "/education", `{"profileId":"` + p + `","institution":" "}`, 400, "institution"},
		"bad date":         {"POST", "/education", `{"profileId":"` + p + `","institution":"X","startDate":"soon"}`, 400, "YYYY"},
		"end before start": {"POST", "/education", `{"profileId":"` + p + `","institution":"X","startDate":"2020","endDate":"2019"}`, 400, "before"},
		"unknown profile":  {"POST", "/education", `{"profileId":"nope","institution":"X"}`, 400, "unknown profile"},
		"no profile":       {"POST", "/education", `{"institution":"X"}`, 400, ""},
		"not json":         {"POST", "/education", `{`, 400, ""},
		"put unknown":      {"PUT", "/education", `{"id":"nope","degree":"x"}`, 400, "unknown education"},
		"put bad date":     {"PUT", "/education", `{"id":"` + id + `","endDate":"2009"}`, 400, "before"},
		"put no id":        {"PUT", "/education", `{"degree":"x"}`, 400, ""},
		"get no profile":   {"GET", "/education", "", 400, ""},
		"delete no id":     {"DELETE", "/education", "", 400, ""},
		"delete unknown":   {"DELETE", "/education?id=nope", "", 400, "unknown education"},
		"patch":            {"PATCH", "/education", "", 405, ""},
	} {
		code, _, body := edCall(c.method, c.target, c.body)
		if code != c.want || !strings.Contains(body, c.msg) {
			t.Errorf("%s: %d %q", name, code, body)
		}
	}
	// the refusals changed nothing
	_, out, _ = edCall("GET", "/education?profileId="+p, "")
	if len(edList(t, out)) != 2 {
		t.Errorf("a refusal changed the list: %v", out)
	}
	if code, _, _ := edCall("DELETE", "/education?id="+id, ""); code != 204 {
		t.Errorf("delete: %d", code)
	}
	_, out, _ = edCall("GET", "/education?profileId="+p, "")
	if len(edList(t, out)) != 1 {
		t.Errorf("%v", out)
	}
}

func TestEducationTools(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, _ := profile.CreateProfile("Ada", "L")
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	education.RegisterListEducation(server)
	education.RegisterAddEducation(server)
	education.RegisterUpdateEducation(server)
	education.RegisterRemoveEducation(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) (map[string]any, string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		var out map[string]any
		_ = json.Unmarshal([]byte(text), &out)
		return out, text, res.IsError
	}
	added, text, isErr := call("add_education", map[string]any{"profileId": p, "institution": "MIT", "degree": "B.Sc.", "startDate": "2010", "endDate": "2014"})
	if isErr || added["id"] == nil {
		t.Fatalf("%s", text)
	}
	if _, text, isErr := call("add_education", map[string]any{"profileId": p, "institution": "X", "startDate": "2020", "endDate": "2019"}); !isErr || !strings.Contains(text, "before the start") {
		t.Errorf("end before start: %v %s", isErr, text)
	}
	if _, text, isErr := call("add_education", map[string]any{"profileId": "nope", "institution": "X"}); !isErr || !strings.Contains(text, "unknown profile") {
		t.Errorf("unknown profile: %v %s", isErr, text)
	}
	if _, text, isErr := call("add_education", map[string]any{"profileId": p, "institution": " "}); !isErr || !strings.Contains(text, "institution") {
		t.Errorf("no institution: %v %s", isErr, text)
	}
	list, _, _ := call("list_education", map[string]any{"profileId": p})
	if len(list["education"].([]any)) != 1 {
		t.Errorf("%v", list)
	}
	if _, text, isErr := call("update_education", map[string]any{"id": added["id"], "degree": "M.Sc.", "endDate": ""}); isErr {
		t.Fatalf("%s", text)
	}
	list, _, _ = call("list_education", map[string]any{"profileId": p})
	e := list["education"].([]any)[0].(map[string]any)
	if e["degree"] != "M.Sc." || e["endDate"] != "" || e["institution"] != "MIT" {
		t.Errorf("%v", e)
	}
	if _, text, isErr := call("update_education", map[string]any{"id": "nope", "degree": "x"}); !isErr || !strings.Contains(text, "unknown education") {
		t.Errorf("unknown: %v %s", isErr, text)
	}
	if _, text, isErr := call("remove_education", map[string]any{"id": added["id"]}); isErr {
		t.Fatalf("%s", text)
	}
	if _, text, isErr := call("remove_education", map[string]any{"id": added["id"]}); !isErr || !strings.Contains(text, "unknown education") {
		t.Errorf("twice: %v %s", isErr, text)
	}
}
