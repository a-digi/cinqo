package profilehandlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/handlertest"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
	"career-tool-backend/profile/profiledetails"
)

func pdCall(method, target, body string) (int, map[string]any, string) {
	rec := handlertest.PiCall(ProfileDetailsHandler, method, target, body)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func TestProfileDetailsRoute(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, _ := profile.CreateProfile("Ada", "L")

	code, out, body := pdCall("GET", "/profile-details?profileId="+p, "")
	if code != 200 || out["summary"] != "" || out["minSalary"].(float64) != 0 || out["remotePreference"] != "" || out["profileId"] != p {
		t.Fatalf("empty profile: %d %s", code, body)
	}
	code, out, body = pdCall("PUT", "/profile-details", `{"profileId":"`+p+`","summary":"About","desiredTitles":"CTO","minSalary":80000,"remotePreference":"open"}`)
	if code != 200 || out["summary"] != "About" || out["desiredTitles"] != "CTO" || out["minSalary"].(float64) != 80000 || out["remotePreference"] != "open" {
		t.Fatalf("put: %d %s", code, body)
	}
	_, out, _ = pdCall("GET", "/profile-details?profileId="+p, "")
	if out["summary"] != "About" {
		t.Errorf("read back: %v", out)
	}
	// a partial update keeps the rest
	_, out, _ = pdCall("PUT", "/profile-details", `{"profileId":"`+p+`","desiredLocations":"Berlin"}`)
	if out["summary"] != "About" || out["desiredLocations"] != "Berlin" {
		t.Errorf("partial: %v", out)
	}

	for name, c := range map[string]struct{ method, target, body, want string }{
		"no profileId":    {"GET", "/profile-details", "", "profileId"},
		"unknown profile": {"GET", "/profile-details?profileId=nope", "", "unknown profile"},
		"put no id":       {"PUT", "/profile-details", `{"summary":"x"}`, "profileId"},
		"put bad json":    {"PUT", "/profile-details", `{`, "profileId"},
		"put unknown":     {"PUT", "/profile-details", `{"profileId":"nope","summary":"x"}`, "unknown profile"},
		"bad remote":      {"PUT", "/profile-details", `{"profileId":"` + p + `","summary":"Lost","remotePreference":"sometimes"}`, "remotePreference"},
		"negative salary": {"PUT", "/profile-details", `{"profileId":"` + p + `","summary":"Lost","minSalary":-5}`, "negative"},
	} {
		if code, _, body := pdCall(c.method, c.target, c.body); code != 400 || !strings.Contains(body, c.want) {
			t.Errorf("%s: %d %q", name, code, body)
		}
	}
	if code, _, _ := pdCall("DELETE", "/profile-details?profileId="+p, ""); code != 405 {
		t.Errorf("delete: %d", code)
	}
	if _, out, _ = pdCall("GET", "/profile-details?profileId="+p, ""); out["summary"] != "About" {
		t.Errorf("a refused request changed something: %v", out)
	}
}

func TestProfileDetailsToolsAndWhatAPersonaShows(t *testing.T) {
	handlertest.SetupHandlerDB(t)
	p, _ := profile.CreateProfile("Ada", "L")
	pers, err := persona.CreatePersona(p, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	profiledetails.RegisterGetProfileDetails(server)
	profiledetails.RegisterUpdateProfileDetails(server)
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
	if out, text, isErr := call("update_profile_details", map[string]any{"profileId": p, "summary": "Shared", "desiredTitles": "CTO", "minSalary": 90000}); isErr || out["summary"] != "Shared" {
		t.Fatalf("update: %s", text)
	}
	if out, text, isErr := call("get_profile_details", map[string]any{"profileId": p}); isErr || out["desiredTitles"] != "CTO" || out["minSalary"].(float64) != 90000 {
		t.Fatalf("get: %s", text)
	}
	// the persona shows what the profile holds
	shown, _ := persona.FetchPersonaDetails(pers)
	if shown.PersonaDetails == nil || shown.PersonaDetails.Summary != "Shared" || shown.PersonaDetails.DesiredTitles != "CTO" {
		t.Errorf("persona shows %+v", shown.PersonaDetails)
	}
	for name, c := range map[string]struct {
		args map[string]any
		want string
	}{
		"unknown profile": {map[string]any{"profileId": "nope", "summary": "x"}, "unknown profile"},
		"bad remote":      {map[string]any{"profileId": p, "summary": "Lost", "remotePreference": "sometimes"}, "remotePreference"},
		"negative":        {map[string]any{"profileId": p, "summary": "Lost", "minSalary": -1}, "negative"},
		"no id":           {map[string]any{"summary": "x"}, "profileId"},
	} {
		if _, text, isErr := call("update_profile_details", c.args); !isErr || !strings.Contains(text, c.want) {
			t.Errorf("%s: %v %s", name, isErr, text)
		}
	}
	if _, text, isErr := call("get_profile_details", map[string]any{"profileId": "nope"}); !isErr || !strings.Contains(text, "unknown profile") {
		t.Errorf("get unknown: %v %s", isErr, text)
	}
	if out, _, _ := call("get_profile_details", map[string]any{"profileId": p}); out["summary"] != "Shared" {
		t.Errorf("a refused tool call changed something: %v", out)
	}
}
