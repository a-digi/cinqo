package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
	"career-tool-backend/profilerules"
)

type rulesAnswer struct {
	Rules []profilerules.Rule `json:"rules"`
}

func listRules(t *testing.T, profileID string) []profilerules.Rule {
	t.Helper()
	rec := boardCall(t, profileMatchRulesHandler, "GET", "/profile-match-rules?profileId="+profileID, "")
	wantStatus(t, rec, 200)
	var a rulesAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return a.Rules
}

func createRule(t *testing.T, profileID string, percent int, statusID string) profilerules.Rule {
	t.Helper()
	rec := boardCall(t, profileMatchRulesHandler, "POST", "/profile-match-rules", fmt.Sprintf(`{"profileId":%q,"minPercent":%d,"statusId":%q}`, profileID, percent, statusID))
	wantStatus(t, rec, 201)
	var r profilerules.Rule
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProfileMatchRulesRoutesEndToEnd(t *testing.T) {
	p1, _ := boardFixture(t)
	st, err := jobboard.CreateStatus(p1, "To check")
	if err != nil {
		t.Fatal(err)
	}

	if rules := listRules(t, p1); rules == nil || len(rules) != 0 {
		t.Fatalf("a new profile must list an empty array, got %v", rules)
	}
	rec := boardCall(t, profileMatchRulesHandler, "GET", "/profile-match-rules?profileId="+p1, "")
	if !strings.Contains(rec.Body.String(), `"rules":[]`) {
		t.Errorf("body = %s, want an empty array, not null", rec.Body.String())
	}

	low := createRule(t, p1, 60, st.ID)
	high := createRule(t, p1, 85, jobboard.NotReviewedID)
	if low.ID == "" || low.ProfileID != p1 || low.MinPercent != 60 || low.StatusID != st.ID || low.StatusName != "To check" {
		t.Errorf("created = %+v", low)
	}
	rules := listRules(t, p1)
	if len(rules) != 2 || rules[0].ID != high.ID || rules[1].ID != low.ID || rules[0].StatusName != "Not Reviewed" {
		t.Errorf("list (highest first) = %+v", rules)
	}

	rec = boardCall(t, profileMatchRulesHandler, "PUT", "/profile-match-rules", fmt.Sprintf(`{"id":%q,"minPercent":70,"statusId":%q}`, low.ID, jobboard.AppliedID))
	wantStatus(t, rec, 200)
	var changed profilerules.Rule
	_ = json.Unmarshal(rec.Body.Bytes(), &changed)
	if changed.ID != low.ID || changed.MinPercent != 70 || changed.StatusID != jobboard.AppliedID || changed.StatusName != "Applied" || changed.ProfileID != p1 {
		t.Errorf("changed = %+v", changed)
	}

	rec = boardCall(t, profileMatchRulesHandler, "DELETE", "/profile-match-rules?id="+low.ID, "")
	wantStatus(t, rec, 204)
	if rules := listRules(t, p1); len(rules) != 1 || rules[0].ID != high.ID {
		t.Errorf("after delete = %+v", rules)
	}
	// deleting again, or an unknown id, is a no-op
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "DELETE", "/profile-match-rules?id="+low.ID, ""), 204)
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "DELETE", "/profile-match-rules?id=nope", ""), 204)
}

func TestProfileMatchRulesGetRefusals(t *testing.T) {
	boardFixture(t)
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "GET", "/profile-match-rules", ""), 400)
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "GET", "/profile-match-rules?profileId=", ""), 400)
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "GET", "/profile-match-rules?profileId=nope", ""), 404)
}

func TestProfileMatchRulesPostRefusals(t *testing.T) {
	p1, p2 := boardFixture(t)
	mine, _ := jobboard.CreateStatus(p1, "Mine")
	theirs, _ := jobboard.CreateStatus(p2, "Theirs")
	createRule(t, p1, 60, mine.ID)

	post := func(body string) int {
		return boardCall(t, profileMatchRulesHandler, "POST", "/profile-match-rules", body).Code
	}
	cases := []struct {
		name, body string
		want       int
	}{
		{"not json", `nope`, 400},
		{"empty body", ``, 400},
		{"percentage as text", fmt.Sprintf(`{"profileId":%q,"minPercent":"60","statusId":%q}`, p1, mine.ID), 400},
		{"percentage with a decimal", fmt.Sprintf(`{"profileId":%q,"minPercent":60.5,"statusId":%q}`, p1, mine.ID), 400},
		{"no profile id", fmt.Sprintf(`{"minPercent":70,"statusId":%q}`, mine.ID), 400},
		{"unknown profile", fmt.Sprintf(`{"profileId":"nope","minPercent":70,"statusId":%q}`, mine.ID), 400},
		{"percentage missing", fmt.Sprintf(`{"profileId":%q,"statusId":%q}`, p1, mine.ID), 400},
		{"percentage 0", fmt.Sprintf(`{"profileId":%q,"minPercent":0,"statusId":%q}`, p1, mine.ID), 400},
		{"percentage 101", fmt.Sprintf(`{"profileId":%q,"minPercent":101,"statusId":%q}`, p1, mine.ID), 400},
		{"percentage negative", fmt.Sprintf(`{"profileId":%q,"minPercent":-5,"statusId":%q}`, p1, mine.ID), 400},
		{"status missing", fmt.Sprintf(`{"profileId":%q,"minPercent":70}`, p1), 400},
		{"unknown status", fmt.Sprintf(`{"profileId":%q,"minPercent":70,"statusId":"nope"}`, p1), 400},
		{"another profile's status", fmt.Sprintf(`{"profileId":%q,"minPercent":70,"statusId":%q}`, p1, theirs.ID), 400},
		{"the same percentage again", fmt.Sprintf(`{"profileId":%q,"minPercent":60,"statusId":%q}`, p1, mine.ID), 409},
	}
	for _, c := range cases {
		if got := post(c.body); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
	if n := len(listRules(t, p1)); n != 1 {
		t.Errorf("refused posts changed the rules: %d", n)
	}
}

func TestProfileMatchRulesAtMostTenPerProfile(t *testing.T) {
	p1, p2 := boardFixture(t)
	for i := 1; i <= 10; i++ {
		createRule(t, p1, i*5, jobboard.NotReviewedID)
	}
	rec := boardCall(t, profileMatchRulesHandler, "POST", "/profile-match-rules", fmt.Sprintf(`{"profileId":%q,"minPercent":99,"statusId":%q}`, p1, jobboard.NotReviewedID))
	wantStatus(t, rec, 400)
	createRule(t, p2, 99, jobboard.NotReviewedID) // the limit is per profile
}

func TestProfileMatchRulesPutRefusals(t *testing.T) {
	p1, p2 := boardFixture(t)
	mine, _ := jobboard.CreateStatus(p1, "Mine")
	theirs, _ := jobboard.CreateStatus(p2, "Theirs")
	r1 := createRule(t, p1, 60, mine.ID)
	createRule(t, p1, 80, mine.ID)

	put := func(body string) int {
		return boardCall(t, profileMatchRulesHandler, "PUT", "/profile-match-rules", body).Code
	}
	cases := []struct {
		name, body string
		want       int
	}{
		{"not json", `nope`, 400},
		{"no id", fmt.Sprintf(`{"minPercent":70,"statusId":%q}`, mine.ID), 400},
		{"unknown rule", fmt.Sprintf(`{"id":"nope","minPercent":70,"statusId":%q}`, mine.ID), 404},
		{"percentage 0", fmt.Sprintf(`{"id":%q,"minPercent":0,"statusId":%q}`, r1.ID, mine.ID), 400},
		{"percentage 101", fmt.Sprintf(`{"id":%q,"minPercent":101,"statusId":%q}`, r1.ID, mine.ID), 400},
		{"unknown status", fmt.Sprintf(`{"id":%q,"minPercent":70,"statusId":"nope"}`, r1.ID), 400},
		{"another profile's status", fmt.Sprintf(`{"id":%q,"minPercent":70,"statusId":%q}`, r1.ID, theirs.ID), 400},
		{"another rule's percentage", fmt.Sprintf(`{"id":%q,"minPercent":80,"statusId":%q}`, r1.ID, mine.ID), 409},
		{"its own percentage is fine", fmt.Sprintf(`{"id":%q,"minPercent":60,"statusId":%q}`, r1.ID, jobboard.AppliedID), 200},
	}
	for _, c := range cases {
		if got := put(c.body); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
	rules := listRules(t, p1)
	if len(rules) != 2 || rules[0].MinPercent != 80 || rules[1].MinPercent != 60 || rules[1].StatusID != jobboard.AppliedID {
		t.Errorf("rules = %+v (only the last, valid, change may have applied)", rules)
	}
}

func TestProfileMatchRulesDeleteWithoutIdAndWrongMethods(t *testing.T) {
	boardFixture(t)
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "DELETE", "/profile-match-rules", ""), 400)
	for _, m := range []string{"PATCH", "HEAD", "OPTIONS"} {
		wantStatus(t, boardCall(t, profileMatchRulesHandler, m, "/profile-match-rules", ""), 405)
	}
}

// A rule goes with its status, and a profile's rules go with the profile, as seen through the routes.
func TestProfileMatchRulesFollowTheirStatusAndProfile(t *testing.T) {
	p1, p2 := boardFixture(t)
	gone, _ := jobboard.CreateStatus(p1, "Gone")
	createRule(t, p1, 60, gone.ID)
	createRule(t, p1, 70, jobboard.AppliedID)
	createRule(t, p2, 60, jobboard.NotReviewedID)

	wantStatus(t, boardCall(t, jobStatusesHandler, "DELETE", "/job-statuses?id="+gone.ID, ""), 204)
	if rules := listRules(t, p1); len(rules) != 1 || rules[0].MinPercent != 70 {
		t.Errorf("after the status was deleted: %+v", rules)
	}
	if rules := listRules(t, p2); len(rules) != 1 {
		t.Errorf("the other profile's rules = %+v", rules)
	}
	if err := profile.DeleteProfile(p2); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, boardCall(t, profileMatchRulesHandler, "GET", "/profile-match-rules?profileId="+p2, ""), 404)
	var left int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, p2).Scan(&left); err != nil || left != 0 {
		t.Errorf("rules left for the deleted profile: %d (%v)", left, err)
	}
	if rules := listRules(t, p1); len(rules) != 1 {
		t.Errorf("the first profile's rules = %+v", rules)
	}
}

// The routes are in the manifest with the jobs scope, one per method, and routed in main.go.
func TestProfileMatchRulesRoutesAreDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Routes []struct {
			Method        string `json:"method"`
			PathSuffix    string `json:"path_suffix"`
			RequiredScope string `json:"required_scope"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, r := range m.Routes {
		if r.PathSuffix == "profile-match-rules" {
			seen[r.Method] = r.RequiredScope
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete} {
		if seen[method] != "tool:career:jobs" {
			t.Errorf("%s profile-match-rules: scope %q, want tool:career:jobs", method, seen[method])
		}
	}
	if len(seen) != 4 {
		t.Errorf("methods declared = %v, want exactly GET, POST, PUT and DELETE", seen)
	}
	mainSrc, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/profile-match-rules", profileMatchRulesHandler`) {
		t.Error("main.go does not route /profile-match-rules")
	}
}
