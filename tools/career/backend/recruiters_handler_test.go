package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/recruiters"
)

func rcCall(method, target, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	recruitersHandler(rec, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rec
}

func rcFixture(t *testing.T) {
	t.Helper()
	setupHandlerDB(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO companies (id, name) VALUES ('c1', 'Acme'), ('c2', 'Beta'), ('c3', 'Gamma')`); err != nil {
		t.Fatal(err)
	}
}

type rcRecruiter struct {
	ID         string   `json:"id"`
	CompanyID  string   `json:"companyId"`
	CompanyIDs []string `json:"companyIds"`
	Companies  []struct{ ID, Name string }
	FirstName  string `json:"firstName"`
	Freelancer bool   `json:"freelancer"`
}

func rcList(t *testing.T, target string) []rcRecruiter {
	t.Helper()
	rec := rcCall("GET", target, "")
	if rec.Code != 200 {
		t.Fatalf("GET %s: %d %s", target, rec.Code, rec.Body.String())
	}
	var out struct{ Recruiters []rcRecruiter }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Recruiters
}

func rcCreate(t *testing.T, body string) string {
	t.Helper()
	rec := rcCall("POST", "/recruiters", body)
	if rec.Code != 200 {
		t.Fatalf("POST %s: %d %s", body, rec.Code, rec.Body.String())
	}
	var out struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out.ID
}

func rcOne(t *testing.T, id string) rcRecruiter {
	t.Helper()
	for _, r := range rcList(t, "/recruiters") {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("%s not listed", id)
	return rcRecruiter{}
}

func TestRecruitersPostAcceptsTheOldCompanyIdAListBothOrNone(t *testing.T) {
	rcFixture(t)
	old := rcOne(t, rcCreate(t, `{"companyId":"c1","firstName":"Old"}`))
	if !reflect.DeepEqual(old.CompanyIDs, []string{"c1"}) || old.CompanyID != "c1" {
		t.Errorf("old shape: %+v", old)
	}
	list := rcOne(t, rcCreate(t, `{"companyIds":["c2","c3"],"firstName":"List"}`))
	if !reflect.DeepEqual(list.CompanyIDs, []string{"c2", "c3"}) || list.CompanyID != "c2" || len(list.Companies) != 2 || list.Companies[1].Name != "Gamma" {
		t.Errorf("list: %+v", list)
	}
	both := rcOne(t, rcCreate(t, `{"companyId":"c1","companyIds":["c2","c1"],"firstName":"Both"}`))
	if !reflect.DeepEqual(both.CompanyIDs, []string{"c1", "c2"}) {
		t.Errorf("both merged without repeats: %+v", both)
	}
	none := rcOne(t, rcCreate(t, `{"firstName":"None"}`))
	if none.CompanyID != "" || none.CompanyIDs == nil || len(none.CompanyIDs) != 0 {
		t.Errorf("none: %+v", none)
	}
}

func TestRecruitersPostRejectsBadInputAndWritesNothing(t *testing.T) {
	rcFixture(t)
	for name, body := range map[string]string{
		"unknown single":     `{"companyId":"nope","firstName":"x"}`,
		"one unknown of two": `{"companyIds":["c1","nope"],"firstName":"x"}`,
		"not json":           `{`,
	} {
		if rec := rcCall("POST", "/recruiters", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if n := len(rcList(t, "/recruiters")); n != 0 {
		t.Errorf("a rejected create left %d recruiters", n)
	}
}

func TestRecruitersPutReplacesTheCompaniesOnly(t *testing.T) {
	rcFixture(t)
	id := rcCreate(t, `{"companyIds":["c1","c2"],"firstName":"Ada"}`)
	put := func(body string) int { return rcCall("PUT", "/recruiters", body).Code }

	if c := put(`{"id":"` + id + `","firstName":"Ada2"}`); c != 200 {
		t.Fatal(c)
	}
	if r := rcOne(t, id); r.FirstName != "Ada2" || !reflect.DeepEqual(r.CompanyIDs, []string{"c1", "c2"}) {
		t.Errorf("a name-only PUT must keep the companies: %+v", r)
	}
	if c := put(`{"id":"` + id + `","companyIds":null}`); c != 200 || len(rcOne(t, id).CompanyIDs) != 2 {
		t.Errorf("null companyIds must leave them: %d", c)
	}
	if c := put(`{"id":"` + id + `","companyIds":["c3","c2"]}`); c != 200 {
		t.Fatal(c)
	}
	if r := rcOne(t, id); !reflect.DeepEqual(r.CompanyIDs, []string{"c2", "c3"}) || r.FirstName != "Ada2" {
		t.Errorf("replaced: %+v", r)
	}
	if c := put(`{"id":"` + id + `","firstName":"Never","companyIds":["c1","nope"]}`); c != http.StatusBadRequest {
		t.Errorf("unknown company: %d", c)
	}
	if r := rcOne(t, id); r.FirstName != "Ada2" || !reflect.DeepEqual(r.CompanyIDs, []string{"c2", "c3"}) {
		t.Errorf("a bad id must change nothing, not even the name: %+v", r)
	}
	if c := put(`{"id":"ghost","companyIds":["c1"]}`); c != http.StatusBadRequest {
		t.Errorf("unknown recruiter: %d", c)
	}
	if c := put(`{"id":"` + id + `","companyIds":[]}`); c != 200 || len(rcOne(t, id).CompanyIDs) != 0 {
		t.Errorf("an empty list clears: %d", c)
	}
}

func TestRecruitersGetFiltersByCompanyAndASurvivesItsCompany(t *testing.T) {
	rcFixture(t)
	a := rcCreate(t, `{"companyIds":["c1","c2"],"firstName":"A"}`)
	b := rcCreate(t, `{"companyIds":["c2"],"firstName":"B"}`)
	if got := rcList(t, "/recruiters?companyId=c1"); len(got) != 1 || got[0].ID != a {
		t.Errorf("c1: %+v", got)
	}
	if got := rcList(t, "/recruiters?companyId=c2"); len(got) != 2 {
		t.Errorf("c2: %+v", got)
	}
	if _, err := db.JobsDB.Exec(`DELETE FROM companies WHERE id IN ('c1','c2')`); err != nil {
		t.Fatal(err)
	}
	all := rcList(t, "/recruiters")
	if len(all) != 2 || len(all[0].CompanyIDs) != 0 || len(all[1].CompanyIDs) != 0 || all[0].ID+all[1].ID == "" {
		t.Errorf("both recruiters must stay, with no company: %+v", all)
	}
	_ = b
}

// The AI tools, called through a real in-memory MCP session.
func TestRecruiterToolsTakeAndReplaceCompanies(t *testing.T) {
	rcFixture(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	recruiters.RegisterCreateRecruiter(server)
	recruiters.RegisterListRecruiters(server)
	recruiters.RegisterUpdateRecruiter(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(name string, args map[string]any) (string, bool) {
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
		return text, res.IsError
	}

	text, isErr := call("create_recruiter", map[string]any{"companyIds": []string{"c1", "c2"}, "firstName": "Tool", "lastName": "L", "email": ""})
	if isErr {
		t.Fatalf("create: %s", text)
	}
	var created struct{ ID string }
	_ = json.Unmarshal([]byte(text), &created)
	if got := rcOne(t, created.ID).CompanyIDs; !reflect.DeepEqual(got, []string{"c1", "c2"}) {
		t.Errorf("created with %v", got)
	}
	if text, isErr := call("create_recruiter", map[string]any{"firstName": "Nobody", "lastName": "L", "email": ""}); isErr {
		t.Errorf("no company is allowed: %s", text)
	}
	if _, isErr := call("create_recruiter", map[string]any{"companyId": "nope", "firstName": "x", "lastName": "L", "email": ""}); !isErr {
		t.Error("an unknown company must be an error")
	}
	if text, isErr := call("update_recruiter", map[string]any{"id": created.ID, "companyIds": []string{"c3"}}); isErr {
		t.Fatalf("update: %s", text)
	}
	if got := rcOne(t, created.ID).CompanyIDs; !reflect.DeepEqual(got, []string{"c3"}) {
		t.Errorf("replaced with %v", got)
	}
	if _, isErr := call("update_recruiter", map[string]any{"id": created.ID, "companyIds": []string{"nope"}}); !isErr {
		t.Error("an unknown company on update must be an error")
	}
	if _, isErr := call("update_recruiter", map[string]any{"id": created.ID, "firstName": "Renamed"}); isErr {
		t.Error("a name-only update must work")
	}
	if r := rcOne(t, created.ID); r.FirstName != "Renamed" || !reflect.DeepEqual(r.CompanyIDs, []string{"c3"}) {
		t.Errorf("name-only update must keep the companies: %+v", r)
	}
	if text, isErr := call("update_recruiter", map[string]any{"id": created.ID, "companyIds": []string{}}); isErr || len(rcOne(t, created.ID).CompanyIDs) != 0 {
		t.Errorf("an empty list clears: %v %s", isErr, text)
	}
	list, isErr := call("list_recruiters", map[string]any{"companyId": "c1"})
	if isErr || !strings.Contains(list, `"recruiters"`) {
		t.Errorf("list: %v %s", isErr, list)
	}
}
