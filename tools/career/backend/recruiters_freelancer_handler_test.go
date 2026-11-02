package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/recruiters"
)

// Plan step-171, step 2: the freelancer flag through the HTTP route.
func TestRecruitersPostCreatesAFreelancerAndRefusesOneWithCompanies(t *testing.T) {
	rcFixture(t)
	f := rcOne(t, rcCreate(t, `{"freelancer":true,"firstName":"Free"}`))
	if !f.Freelancer || len(f.CompanyIDs) != 0 || f.CompanyID != "" {
		t.Errorf("freelancer: %+v", f)
	}
	plain := rcOne(t, rcCreate(t, `{"companyId":"c1","firstName":"Ada"}`))
	if plain.Freelancer {
		t.Errorf("a company recruiter is not a freelancer: %+v", plain)
	}
	none := rcOne(t, rcCreate(t, `{"firstName":"None"}`))
	if none.Freelancer {
		t.Errorf("neither given is not a freelancer: %+v", none)
	}
	before := len(rcList(t, "/recruiters"))
	for name, body := range map[string]string{
		"companyId":  `{"freelancer":true,"companyId":"c1","firstName":"x"}`,
		"companyIds": `{"freelancer":true,"companyIds":["c1","c2"],"firstName":"x"}`,
		"unknown":    `{"freelancer":true,"companyIds":["nope"],"firstName":"x"}`,
	} {
		rec := rcCall("POST", "/recruiters", body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "freelancer has no companies") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if rec := rcCall("POST", "/recruiters", `{"freelancer":"yes","firstName":"x"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a non-boolean freelancer: %d", rec.Code)
	}
	if n := len(rcList(t, "/recruiters")); n != before {
		t.Errorf("a refused create left %d recruiter(s)", n-before)
	}
}

func TestRecruitersPutMakesAFreelancerAndBack(t *testing.T) {
	rcFixture(t)
	id := rcCreate(t, `{"companyIds":["c1","c2"],"firstName":"Ada"}`)
	put := func(body string) int { return rcCall("PUT", "/recruiters", body).Code }

	if c := put(`{"id":"` + id + `","freelancer":true}`); c != 200 {
		t.Fatal(c)
	}
	if r := rcOne(t, id); !r.Freelancer || len(r.CompanyIDs) != 0 {
		t.Fatalf("freelancer:true clears the companies: %+v", r)
	}
	// a freelancer cannot get companies, and a refusal changes nothing, not even the name
	if c := put(`{"id":"` + id + `","firstName":"Never","companyIds":["c1"]}`); c != http.StatusBadRequest {
		t.Errorf("companies for a freelancer: %d", c)
	}
	if c := put(`{"id":"` + id + `","freelancer":true,"firstName":"Never","companyIds":["c1"]}`); c != http.StatusBadRequest {
		t.Errorf("freelancer true with companies: %d", c)
	}
	if r := rcOne(t, id); !r.Freelancer || r.FirstName != "Ada" || len(r.CompanyIDs) != 0 {
		t.Errorf("a refusal must change nothing: %+v", r)
	}
	// an empty list on a freelancer is fine (nothing to clear)
	if c := put(`{"id":"` + id + `","companyIds":[]}`); c != 200 || !rcOne(t, id).Freelancer {
		t.Errorf("empty list on a freelancer: %d", c)
	}
	// back to a company recruiter in one request
	if c := put(`{"id":"` + id + `","freelancer":false,"companyIds":["c3"]}`); c != 200 {
		t.Fatal(c)
	}
	if r := rcOne(t, id); r.Freelancer || !reflect.DeepEqual(r.CompanyIDs, []string{"c3"}) {
		t.Errorf("back: %+v", r)
	}
	// freelancer:false alone on a company recruiter changes nothing else
	if c := put(`{"id":"` + id + `","freelancer":false}`); c != 200 || !reflect.DeepEqual(rcOne(t, id).CompanyIDs, []string{"c3"}) {
		t.Errorf("freelancer:false alone: %d", c)
	}
	// unknown ids
	if c := put(`{"id":"ghost","freelancer":true}`); c != http.StatusBadRequest {
		t.Errorf("unknown recruiter: %d", c)
	}
	if c := put(`{"id":"` + id + `","freelancer":false,"companyIds":["nope"]}`); c != http.StatusBadRequest {
		t.Errorf("unknown company: %d", c)
	}
	if c := put(`{"id":"` + id + `","freelancer":null,"firstName":"Plain"}`); c != 200 || rcOne(t, id).FirstName != "Plain" {
		t.Errorf("null freelancer leaves it: %d", c)
	}
}

func TestRecruitersGetFiltersFreelancersAndAFreelancerSurvivesCompanyDeletes(t *testing.T) {
	rcFixture(t)
	f := rcCreate(t, `{"freelancer":true,"firstName":"F"}`)
	a := rcCreate(t, `{"companyIds":["c1"],"firstName":"A"}`)
	got := rcList(t, "/recruiters?freelancer=true")
	if len(got) != 1 || got[0].ID != f || !got[0].Freelancer {
		t.Errorf("freelancer=true: %+v", got)
	}
	if got := rcList(t, "/recruiters?freelancer=false"); len(got) != 2 {
		t.Errorf("freelancer=false is no filter: %+v", got)
	}
	if got := rcList(t, "/recruiters?freelancer=true&companyId=c1"); len(got) != 0 {
		t.Errorf("a freelancer works for no company: %+v", got)
	}
	if _, err := db.JobsDB.Exec(`DELETE FROM companies`); err != nil {
		t.Fatal(err)
	}
	all := rcList(t, "/recruiters")
	if len(all) != 2 {
		t.Fatalf("both stay: %+v", all)
	}
	for _, r := range all {
		if r.ID == f && !r.Freelancer {
			t.Error("the freelancer changed")
		}
		if r.ID == a && r.Freelancer {
			t.Error("the company recruiter must not become a freelancer when its company goes")
		}
	}
}

// The job route lists the recruiters of a job; a freelancer is marked in it.
func TestJobRecruitersRouteMarksAFreelancer(t *testing.T) {
	rcFixture(t)
	f := rcCreate(t, `{"freelancer":true,"firstName":"F"}`)
	if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title) VALUES ('j1', 'https://e/1', 'Job')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO job_recruiters (job_id, recruiter_id) VALUES ('j1', ?)`, f); err != nil {
		t.Fatal(err)
	}
	rec := jrCall("GET", "/job-recruiters?jobId=j1", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"freelancer": true`) && !strings.Contains(rec.Body.String(), `"freelancer":true`) {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
}

// The AI tools, through a real in-memory MCP session.
func TestRecruiterToolsCreateAndSwitchAFreelancer(t *testing.T) {
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
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
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

	text, isErr := call("create_recruiter", map[string]any{"freelancer": true, "firstName": "Free", "lastName": "L", "email": ""})
	if isErr {
		t.Fatalf("create freelancer: %s", text)
	}
	if text, isErr := call("create_recruiter", map[string]any{"freelancer": true, "companyId": "c1", "firstName": "x", "lastName": "L", "email": ""}); !isErr || !strings.Contains(text, "freelancer has no companies") {
		t.Errorf("freelancer with a company must be an error: %v %s", isErr, text)
	}
	list, _ := call("list_recruiters", map[string]any{"freelancer": true})
	var listed struct{ Recruiters []rcRecruiter }
	if err := json.Unmarshal([]byte(list), &listed); err != nil || len(listed.Recruiters) != 1 || !listed.Recruiters[0].Freelancer {
		t.Fatalf("list freelancers: %v %s", err, list)
	}
	id := listed.Recruiters[0].ID
	if text, isErr := call("update_recruiter", map[string]any{"id": id, "companyIds": []string{"c1"}}); !isErr || !strings.Contains(text, "freelancer has no companies") {
		t.Errorf("companies for a freelancer: %v %s", isErr, text)
	}
	if text, isErr := call("update_recruiter", map[string]any{"id": id, "freelancer": false, "companyIds": []string{"c1"}}); isErr {
		t.Fatalf("switch: %s", text)
	}
	if r := rcOne(t, id); r.Freelancer || !reflect.DeepEqual(r.CompanyIDs, []string{"c1"}) {
		t.Errorf("after the switch: %+v", r)
	}
	if text, isErr := call("update_recruiter", map[string]any{"id": id, "freelancer": true}); isErr {
		t.Fatalf("back: %s", text)
	}
	if r := rcOne(t, id); !r.Freelancer || len(r.CompanyIDs) != 0 {
		t.Errorf("after switching back: %+v", r)
	}
	// a name-only update keeps everything
	if text, isErr := call("update_recruiter", map[string]any{"id": id, "firstName": "Renamed"}); isErr {
		t.Fatal(text)
	}
	if r := rcOne(t, id); !r.Freelancer || r.FirstName != "Renamed" {
		t.Errorf("rename: %+v", r)
	}
}
