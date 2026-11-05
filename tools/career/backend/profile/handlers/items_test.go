package profilehandlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/handlertest"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
	"career-tool-backend/profile/profileitems"
)

// Plan 173, step 2: the routes of the profile library and of what a persona uses.
type piWorld struct{ p1, p2, pa, pb, pc string }

func piFixture(t *testing.T) piWorld {
	t.Helper()
	handlertest.SetupHandlerDB(t)
	var w piWorld
	var err error
	if w.p1, err = profile.CreateProfile("Ada", "L"); err != nil {
		t.Fatal(err)
	}
	if w.p2, err = profile.CreateProfile("Bob", "M"); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		dst     *string
		profile string
	}{{&w.pa, w.p1}, {&w.pb, w.p1}, {&w.pc, w.p2}} {
		if *x.dst, err = persona.CreatePersona(x.profile, "P", ""); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func piIDs(t *testing.T, list any, key string) []string {
	t.Helper()
	out := []string{}
	for _, x := range list.([]any) {
		out = append(out, x.(map[string]any)[key].(string))
	}
	return out
}

func TestProfileSkillsRoute(t *testing.T) {
	w := piFixture(t)
	// create (a repeat in another case is not added again), read, rename, refuse a duplicate, delete
	res := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "POST", "/profile-skills", `{"profileId":"`+w.p1+`","skill":" Go "}`), 200)
	if skills := res["skills"].([]any); len(skills) != 1 || skills[0].(map[string]any)["skill"] != "Go" {
		t.Fatalf("%v", res)
	}
	handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "POST", "/profile-skills", `{"profileId":"`+w.p1+`","skill":"GO"}`), 200)
	res = handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "POST", "/profile-skills", `{"profileId":"`+w.p1+`","skill":"Rust"}`), 200)
	if len(res["skills"].([]any)) != 2 {
		t.Fatalf("the repeat must not be added: %v", res)
	}
	ids := piIDs(t, res["skills"], "id")
	got := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "GET", "/profile-skills?profileId="+w.p1, ""), 200)
	if len(got["skills"].([]any)) != 2 {
		t.Errorf("%v", got)
	}
	if other := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "GET", "/profile-skills?profileId="+w.p2, ""), 200); len(other["skills"].([]any)) != 0 {
		t.Errorf("another profile has none: %v", other)
	}
	rename := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "PUT", "/profile-skills", `{"id":"`+ids[0]+`","skill":"Golang"}`), 200)
	if rename["skills"].([]any)[0].(map[string]any)["skill"] != "Golang" {
		t.Errorf("%v", rename)
	}
	if rec := handlertest.PiCall(ProfileSkillsHandler, "PUT", "/profile-skills", `{"id":"`+ids[0]+`","skill":"rust"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate: %d", rec.Code)
	}
	for name, c := range map[string]struct {
		method, target, body string
		want                 int
	}{
		"post without profile":   {"POST", "/profile-skills", `{"skill":"X"}`, 400},
		"post unknown profile":   {"POST", "/profile-skills", `{"profileId":"nope","skill":"X"}`, 400},
		"post blank":             {"POST", "/profile-skills", `{"profileId":"` + w.p1 + `","skill":" "}`, 400},
		"put unknown":            {"PUT", "/profile-skills", `{"id":"nope","skill":"X"}`, 400},
		"put blank":              {"PUT", "/profile-skills", `{"id":"` + ids[0] + `","skill":""}`, 400},
		"get without profile id": {"GET", "/profile-skills", "", 400},
		"delete without id":      {"DELETE", "/profile-skills", "", 400},
		"delete unknown":         {"DELETE", "/profile-skills?id=nope", "", 400},
		"not json":               {"POST", "/profile-skills", "{", 400},
		"patch":                  {"PATCH", "/profile-skills", "", 405},
	} {
		if rec := handlertest.PiCall(ProfileSkillsHandler, c.method, c.target, c.body); rec.Code != c.want {
			t.Errorf("%s: %d (want %d) %s", name, rec.Code, c.want, rec.Body.String())
		}
	}
	if rec := handlertest.PiCall(ProfileSkillsHandler, "DELETE", "/profile-skills?id="+ids[0], ""); rec.Code != 204 {
		t.Errorf("delete: %d", rec.Code)
	}
	if left := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "GET", "/profile-skills?profileId="+w.p1, ""), 200); len(left["skills"].([]any)) != 1 {
		t.Errorf("%v", left)
	}
}

func TestProfileExperienceRoute(t *testing.T) {
	w := piFixture(t)
	res := handlertest.PiJSON(t, handlertest.PiCall(ProfileExperienceHandler, "POST", "/profile-experience",
		`{"profileId":"`+w.p1+`","company":"Acme","title":"Dev","startDate":"2020","endDate":"2022","description":"built X"}`), 200)
	list := res["experience"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["company"] != "Acme" || list[0].(map[string]any)["description"] != "built X" {
		t.Fatalf("%v", res)
	}
	id := list[0].(map[string]any)["id"].(string)
	// a partial update changes only what is given; the answer is the profile's list
	up := handlertest.PiJSON(t, handlertest.PiCall(ProfileExperienceHandler, "PUT", "/profile-experience", `{"id":"`+id+`","title":"Lead"}`), 200)
	e := up["experience"].([]any)[0].(map[string]any)
	if e["title"] != "Lead" || e["company"] != "Acme" || e["startDate"] != "2020" {
		t.Errorf("%v", e)
	}
	for name, c := range map[string]struct {
		method, target, body string
		want                 int
	}{
		"no company":      {"POST", "/profile-experience", `{"profileId":"` + w.p1 + `","title":"T"}`, 400},
		"no title":        {"POST", "/profile-experience", `{"profileId":"` + w.p1 + `","company":"C"}`, 400},
		"unknown profile": {"POST", "/profile-experience", `{"profileId":"nope","company":"C","title":"T"}`, 400},
		"blank title put": {"PUT", "/profile-experience", `{"id":"` + id + `","title":" "}`, 400},
		"unknown put":     {"PUT", "/profile-experience", `{"id":"nope","title":"T"}`, 400},
		"get no profile":  {"GET", "/profile-experience", "", 400},
		"delete unknown":  {"DELETE", "/profile-experience?id=nope", "", 400},
		"patch":           {"PATCH", "/profile-experience", "", 405},
	} {
		if rec := handlertest.PiCall(ProfileExperienceHandler, c.method, c.target, c.body); rec.Code != c.want {
			t.Errorf("%s: %d (want %d) %s", name, rec.Code, c.want, rec.Body.String())
		}
	}
	if rec := handlertest.PiCall(ProfileExperienceHandler, "DELETE", "/profile-experience?id="+id, ""); rec.Code != 204 {
		t.Errorf("delete: %d", rec.Code)
	}
}

func TestPersonaSkillsRouteChoosesFromTheLibraryAndShowsTheOwn(t *testing.T) {
	w := piFixture(t)
	var ids []string
	for _, s := range []string{"Go", "Python", "SQL"} {
		res := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "POST", "/profile-skills", `{"profileId":"`+w.p1+`","skill":"`+s+`"}`), 200)
		ids = piIDs(t, res["skills"], "id")
	}
	other := handlertest.PiJSON(t, handlertest.PiCall(ProfileSkillsHandler, "POST", "/profile-skills", `{"profileId":"`+w.p2+`","skill":"Other"}`), 200)
	otherID := piIDs(t, other["skills"], "id")[0]
	// an own custom skill through the old route
	if rec := handlertest.PiCall(SkillsHandler, "POST", "/skills", `{"personaId":"`+w.pa+`","skill":"Rust"}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	view := handlertest.PiJSON(t, handlertest.PiCall(PersonaSkillsHandler, "PUT", "/persona-skills", `{"personaId":"`+w.pa+`","skillIds":["`+ids[0]+`","`+ids[1]+`"]}`), 200)
	if used := piIDs2(view["used"]); len(used) != 2 {
		t.Errorf("used %v", view["used"])
	}
	if len(view["library"].([]any)) != 3 {
		t.Errorf("the whole library of the persona's own profile is offered: %v", view["library"])
	}
	if own := view["own"].([]any); len(own) != 1 || own[0].(map[string]any)["skill"] != "Rust" {
		t.Errorf("own %v", view["own"])
	}
	// the effective list (the older route) shows used + own, with the sources
	eff := handlertest.PiJSON(t, handlertest.PiCall(SkillsHandler, "GET", "/skills?personaId="+w.pa, ""), 200)
	if !reflect.DeepEqual(eff["skills"], []any{"Go", "Python", "Rust"}) {
		t.Errorf("effective skills %v", eff["skills"])
	}
	if items := eff["skillItems"].([]any); len(items) != 3 || items[2].(map[string]any)["source"] != "persona" || items[0].(map[string]any)["source"] != "profile" {
		t.Errorf("skillItems %v", eff["skillItems"])
	}
	// refusals change nothing
	for name, body := range map[string]string{
		"other profile":   `{"personaId":"` + w.pa + `","skillIds":["` + otherID + `"]}`,
		"unknown skill":   `{"personaId":"` + w.pa + `","skillIds":["nope"]}`,
		"unknown persona": `{"personaId":"ghost","skillIds":[]}`,
		"no skillIds":     `{"personaId":"` + w.pa + `"}`,
		"no persona":      `{"skillIds":[]}`,
	} {
		if rec := handlertest.PiCall(PersonaSkillsHandler, "PUT", "/persona-skills", body); rec.Code != 400 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	after := handlertest.PiJSON(t, handlertest.PiCall(PersonaSkillsHandler, "GET", "/persona-skills?personaId="+w.pa, ""), 200)
	if len(piIDs2(after["used"])) != 2 {
		t.Errorf("a refusal must change nothing: %v", after["used"])
	}
	// none is allowed; another persona is untouched
	handlertest.PiJSON(t, handlertest.PiCall(PersonaSkillsHandler, "PUT", "/persona-skills", `{"personaId":"`+w.pb+`","skillIds":["`+ids[2]+`"]}`), 200)
	cleared := handlertest.PiJSON(t, handlertest.PiCall(PersonaSkillsHandler, "PUT", "/persona-skills", `{"personaId":"`+w.pa+`","skillIds":[]}`), 200)
	if len(piIDs2(cleared["used"])) != 0 {
		t.Errorf("%v", cleared["used"])
	}
	if b := handlertest.PiJSON(t, handlertest.PiCall(PersonaSkillsHandler, "GET", "/persona-skills?personaId="+w.pb, ""), 200); len(piIDs2(b["used"])) != 1 {
		t.Errorf("B: %v", b["used"])
	}
	if rec := handlertest.PiCall(PersonaSkillsHandler, "GET", "/persona-skills", ""); rec.Code != 400 {
		t.Errorf("no persona: %d", rec.Code)
	}
	if rec := handlertest.PiCall(PersonaSkillsHandler, "GET", "/persona-skills?personaId=ghost", ""); rec.Code != 400 {
		t.Errorf("unknown persona: %d", rec.Code)
	}
}

func piIDs2(v any) []string {
	out := []string{}
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestPersonaExperienceRouteOverridesAndTheOldRoutesRefuseLibraryItems(t *testing.T) {
	w := piFixture(t)
	add := func(company, title string) string {
		res := handlertest.PiJSON(t, handlertest.PiCall(ProfileExperienceHandler, "POST", "/profile-experience", `{"profileId":"`+w.p1+`","company":"`+company+`","title":"`+title+`","description":"base"}`), 200)
		for _, x := range res["experience"].([]any) {
			if x.(map[string]any)["company"] == company {
				return x.(map[string]any)["id"].(string)
			}
		}
		t.Fatal("not added")
		return ""
	}
	e1, e2 := add("Acme", "Dev"), add("Beta", "Lead")
	view := handlertest.PiJSON(t, handlertest.PiCall(PersonaExperienceHandler, "PUT", "/persona-experience",
		`{"personaId":"`+w.pa+`","experience":[{"id":"`+e1+`","titleOverride":"Backend Dev","descriptionOverride":""},{"id":"`+e2+`"}]}`), 200)
	if len(view["used"].([]any)) != 2 || len(view["library"].([]any)) != 2 || len(view["own"].([]any)) != 0 {
		t.Fatalf("%v", view)
	}
	// the effective list through the old route carries the overrides and the sources
	eff := handlertest.PiJSON(t, handlertest.PiCall(ExperienceHandler, "GET", "/experience?personaId="+w.pa, ""), 200)
	by := map[string]map[string]any{}
	for _, x := range eff["experience"].([]any) {
		m := x.(map[string]any)
		by[m["company"].(string)] = m
	}
	if m := by["Acme"]; m["title"] != "Backend Dev" || m["titleOverridden"] != true || m["source"] != "profile" || m["description"] != nil {
		t.Errorf("Acme %v (a blank description is an override to empty)", m)
	}
	if m := by["Beta"]; m["title"] != "Lead" || m["source"] != "profile" || m["description"] != "base" {
		t.Errorf("Beta %v", m)
	}
	// the persona's own routes refuse a library experience, with a clear message, and change nothing
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"put":    handlertest.PiCall(ExperienceHandler, "PUT", "/experience", `{"personaId":"`+w.pa+`","id":"`+e1+`","title":"Changed"}`),
		"delete": handlertest.PiCall(ExperienceHandler, "DELETE", "/experience?personaId="+w.pa+"&id="+e1, ""),
	} {
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "comes from the profile") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	// own items still work and show as own
	handlertest.PiJSON(t, handlertest.PiCall(ExperienceHandler, "POST", "/experience", `{"personaId":"`+w.pa+`","company":"Own","title":"Freelance"}`), 200)
	own := handlertest.PiJSON(t, handlertest.PiCall(PersonaExperienceHandler, "GET", "/persona-experience?personaId="+w.pa, ""), 200)
	if o := own["own"].([]any); len(o) != 1 || o[0].(map[string]any)["company"] != "Own" {
		t.Errorf("own %v", own["own"])
	}
	// refusals
	for name, body := range map[string]string{
		"blank title override": `{"personaId":"` + w.pa + `","experience":[{"id":"` + e1 + `","titleOverride":" "}]}`,
		"unknown":              `{"personaId":"` + w.pa + `","experience":[{"id":"nope"}]}`,
		"unknown persona":      `{"personaId":"ghost","experience":[]}`,
		"no experience":        `{"personaId":"` + w.pa + `"}`,
	} {
		if rec := handlertest.PiCall(PersonaExperienceHandler, "PUT", "/persona-experience", body); rec.Code != 400 {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	// the library edit reaches the persona except what it overrides
	handlertest.PiJSON(t, handlertest.PiCall(ProfileExperienceHandler, "PUT", "/profile-experience", `{"id":"`+e1+`","title":"Principal","description":"new base"}`), 200)
	eff = handlertest.PiJSON(t, handlertest.PiCall(ExperienceHandler, "GET", "/experience?personaId="+w.pa, ""), 200)
	for _, x := range eff["experience"].([]any) {
		if m := x.(map[string]any); m["company"] == "Acme" && (m["title"] != "Backend Dev") {
			t.Errorf("the override must win over the library edit: %v", m)
		}
	}
	// deleting the library item takes it off the persona
	if rec := handlertest.PiCall(ProfileExperienceHandler, "DELETE", "/profile-experience?id="+e1, ""); rec.Code != 204 {
		t.Fatal(rec.Code)
	}
	eff = handlertest.PiJSON(t, handlertest.PiCall(ExperienceHandler, "GET", "/experience?personaId="+w.pa, ""), 200)
	if len(eff["experience"].([]any)) != 2 { // Beta (library) and the own one
		t.Errorf("%v", eff["experience"])
	}
}

// The AI tools of the library, through a real in-memory MCP session.
func TestProfileLibraryTools(t *testing.T) {
	w := piFixture(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	profileitems.RegisterListProfileSkills(server)
	profileitems.RegisterAddProfileSkill(server)
	profileitems.RegisterListProfileExperience(server)
	profileitems.RegisterAddProfileExperience(server)
	profileitems.RegisterSetPersonaSkills(server)
	profileitems.RegisterSetPersonaExperience(server)
	persona.RegisterAddCareerSkill(server)
	persona.RegisterUpdateCareerExperience(server)
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

	add, text, isErr := call("add_profile_skill", map[string]any{"profileId": w.p1, "skill": "Go"})
	if isErr || add["created"] != true {
		t.Fatalf("add: %s", text)
	}
	again, _, _ := call("add_profile_skill", map[string]any{"profileId": w.p1, "skill": "go"})
	if again["created"] != false || again["id"] != add["id"] {
		t.Errorf("a repeat must return the existing skill: %v", again)
	}
	if _, text, isErr := call("add_profile_skill", map[string]any{"profileId": "nope", "skill": "X"}); !isErr || !strings.Contains(text, "unknown profile") {
		t.Errorf("unknown profile: %v %s", isErr, text)
	}
	list, _, _ := call("list_profile_skills", map[string]any{"profileId": w.p1})
	if len(list["skills"].([]any)) != 1 {
		t.Errorf("%v", list)
	}
	exp, text, isErr := call("add_profile_experience", map[string]any{"profileId": w.p1, "company": "Acme", "title": "Dev", "description": "built X"})
	if isErr {
		t.Fatalf("%s", text)
	}
	if _, text, isErr := call("add_profile_experience", map[string]any{"profileId": w.p1, "company": "Acme", "title": " "}); !isErr || !strings.Contains(text, "company and a title") {
		t.Errorf("missing title: %v %s", isErr, text)
	}
	// choose for a persona, with an override
	if _, text, isErr := call("set_persona_skills", map[string]any{"personaId": w.pa, "skillIds": []string{add["id"].(string)}}); isErr {
		t.Fatalf("%s", text)
	}
	if _, text, isErr := call("set_persona_experience", map[string]any{"personaId": w.pa, "experience": []map[string]any{{"id": exp["id"], "titleOverride": "Backend Dev"}}}); isErr {
		t.Fatalf("%s", text)
	}
	d, err := persona.FetchPersonaDetails(w.pa)
	if err != nil || !reflect.DeepEqual(d.Skills, []string{"Go"}) || len(d.Experience) != 1 || d.Experience[0].Title != "Backend Dev" {
		t.Fatalf("%+v %v", d, err)
	}
	// refusals
	if _, text, isErr := call("set_persona_skills", map[string]any{"personaId": w.pc, "skillIds": []string{add["id"].(string)}}); !isErr || !strings.Contains(text, "another profile") {
		t.Errorf("another profile's skill: %v %s", isErr, text)
	}
	if _, text, isErr := call("set_persona_experience", map[string]any{"personaId": w.pa, "experience": []map[string]any{{"id": exp["id"], "titleOverride": " "}}}); !isErr || !strings.Contains(text, "title override") {
		t.Errorf("blank override: %v %s", isErr, text)
	}
	// the persona's own tool refuses a library experience, saying what to do
	if _, text, isErr := call("update_career_experience", map[string]any{"personaId": w.pa, "id": exp["id"], "title": "X"}); !isErr || !strings.Contains(text, "comes from the profile") {
		t.Errorf("update of a library experience: %v %s", isErr, text)
	}
	// the older tool adds an own skill
	if _, text, isErr := call("add_career_skill", map[string]any{"personaId": w.pa, "skill": "Rust"}); isErr {
		t.Fatalf("%s", text)
	}
	d, _ = persona.FetchPersonaDetails(w.pa)
	if !reflect.DeepEqual(d.Skills, []string{"Go", "Rust"}) {
		t.Errorf("%v", d.Skills)
	}
	if lib, _, _ := call("list_profile_skills", map[string]any{"profileId": w.p1}); len(lib["skills"].([]any)) != 1 {
		t.Errorf("the own skill must not enter the library: %v", lib)
	}
}
