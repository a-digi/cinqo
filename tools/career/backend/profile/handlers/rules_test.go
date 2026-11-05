package profilehandlers

import (
	jobshandlers "career-tool-backend/jobs/handlers"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/handlertest"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/profile"
	"career-tool-backend/profile/profilejobmatchconsumer"
	"career-tool-backend/profile/profilerules"
)

type rulesAnswer struct {
	Rules []profilerules.Rule `json:"rules"`
}

func listRules(t *testing.T, profileID string) []profilerules.Rule {
	t.Helper()
	rec := handlertest.BoardCall(t, ProfileMatchRulesHandler, "GET", "/profile-match-rules?profileId="+profileID, "")
	handlertest.WantStatus(t, rec, 200)
	var a rulesAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return a.Rules
}

func createRule(t *testing.T, profileID string, percent int, statusID string) profilerules.Rule {
	t.Helper()
	rec := handlertest.BoardCall(t, ProfileMatchRulesHandler, "POST", "/profile-match-rules", fmt.Sprintf(`{"profileId":%q,"minPercent":%d,"statusId":%q}`, profileID, percent, statusID))
	handlertest.WantStatus(t, rec, 201)
	var r profilerules.Rule
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProfileMatchRulesRoutesEndToEnd(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	st, err := jobboard.CreateStatus(p1, "To check")
	if err != nil {
		t.Fatal(err)
	}

	if rules := listRules(t, p1); rules == nil || len(rules) != 0 {
		t.Fatalf("a new profile must list an empty array, got %v", rules)
	}
	rec := handlertest.BoardCall(t, ProfileMatchRulesHandler, "GET", "/profile-match-rules?profileId="+p1, "")
	if !strings.Contains(rec.Body.String(), `"rules":[]`) {
		t.Errorf("body = %s, want an empty array, not null", rec.Body.String())
	}

	low := createRule(t, p1, 60, st.ID)
	high := createRule(t, p1, 85, jobboard.NotReviewedID)
	if low.ID == "" || low.ProfileID != p1 || low.MinPercent != 60 || low.StatusID != st.ID || low.StatusName != "To check" {
		t.Errorf("created = %+v", low)
	}
	rules := listRules(t, p1)
	if len(rules) != 2 || rules[0].ID != low.ID || rules[1].ID != high.ID || rules[1].StatusName != "Not Reviewed" || rules[0].Position != 0 || rules[1].Position != 1 {
		t.Errorf("list (in the order they were made in) = %+v", rules)
	}

	rec = handlertest.BoardCall(t, ProfileMatchRulesHandler, "PUT", "/profile-match-rules", fmt.Sprintf(`{"id":%q,"minPercent":70,"statusId":%q}`, low.ID, jobboard.AppliedID))
	handlertest.WantStatus(t, rec, 200)
	var changed profilerules.Rule
	_ = json.Unmarshal(rec.Body.Bytes(), &changed)
	if changed.ID != low.ID || changed.MinPercent != 70 || changed.StatusID != jobboard.AppliedID || changed.StatusName != "Applied" || changed.ProfileID != p1 {
		t.Errorf("changed = %+v", changed)
	}

	rec = handlertest.BoardCall(t, ProfileMatchRulesHandler, "DELETE", "/profile-match-rules?id="+low.ID, "")
	handlertest.WantStatus(t, rec, 204)
	if rules := listRules(t, p1); len(rules) != 1 || rules[0].ID != high.ID {
		t.Errorf("after delete = %+v", rules)
	}
	// deleting again, or an unknown id, is a no-op
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "DELETE", "/profile-match-rules?id="+low.ID, ""), 204)
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "DELETE", "/profile-match-rules?id=nope", ""), 204)
}

func TestProfileMatchRulesGetRefusals(t *testing.T) {
	handlertest.BoardFixture(t)
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "GET", "/profile-match-rules", ""), 400)
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "GET", "/profile-match-rules?profileId=", ""), 400)
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "GET", "/profile-match-rules?profileId=nope", ""), 404)
}

func TestProfileMatchRulesPostRefusals(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	mine, _ := jobboard.CreateStatus(p1, "Mine")
	theirs, _ := jobboard.CreateStatus(p2, "Theirs")
	createRule(t, p1, 60, mine.ID)

	post := func(body string) int {
		return handlertest.BoardCall(t, ProfileMatchRulesHandler, "POST", "/profile-match-rules", body).Code
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
	p1, p2 := handlertest.BoardFixture(t)
	for i := 1; i <= 10; i++ {
		createRule(t, p1, i*5, jobboard.NotReviewedID)
	}
	rec := handlertest.BoardCall(t, ProfileMatchRulesHandler, "POST", "/profile-match-rules", fmt.Sprintf(`{"profileId":%q,"minPercent":99,"statusId":%q}`, p1, jobboard.NotReviewedID))
	handlertest.WantStatus(t, rec, 400)
	createRule(t, p2, 99, jobboard.NotReviewedID) // the limit is per profile
}

func TestProfileMatchRulesPutRefusals(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	mine, _ := jobboard.CreateStatus(p1, "Mine")
	theirs, _ := jobboard.CreateStatus(p2, "Theirs")
	r1 := createRule(t, p1, 60, mine.ID)
	createRule(t, p1, 80, mine.ID)

	put := func(body string) int {
		return handlertest.BoardCall(t, ProfileMatchRulesHandler, "PUT", "/profile-match-rules", body).Code
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
	if len(rules) != 2 || rules[0].MinPercent != 60 || rules[1].MinPercent != 80 || rules[0].StatusID != jobboard.AppliedID {
		t.Errorf("rules = %+v (only the last, valid, change may have applied)", rules)
	}
}

func TestProfileMatchRulesDeleteWithoutIdAndWrongMethods(t *testing.T) {
	handlertest.BoardFixture(t)
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "DELETE", "/profile-match-rules", ""), 400)
	for _, m := range []string{"PATCH", "HEAD", "OPTIONS"} {
		handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, m, "/profile-match-rules", ""), 405)
	}
}

// A rule goes with its status, and a profile's rules go with the profile, as seen through the routes.
func TestProfileMatchRulesFollowTheirStatusAndProfile(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	gone, _ := jobboard.CreateStatus(p1, "Gone")
	createRule(t, p1, 60, gone.ID)
	createRule(t, p1, 70, jobboard.AppliedID)
	createRule(t, p2, 60, jobboard.NotReviewedID)

	handlertest.WantStatus(t, handlertest.BoardCall(t, jobshandlers.JobStatusesHandler, "DELETE", "/job-statuses?id="+gone.ID, ""), 204)
	if rules := listRules(t, p1); len(rules) != 1 || rules[0].MinPercent != 70 {
		t.Errorf("after the status was deleted: %+v", rules)
	}
	if rules := listRules(t, p2); len(rules) != 1 {
		t.Errorf("the other profile's rules = %+v", rules)
	}
	if err := profile.DeleteProfile(p2); err != nil {
		t.Fatal(err)
	}
	handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesHandler, "GET", "/profile-match-rules?profileId="+p2, ""), 404)
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
	raw, err := os.ReadFile("../../../manifest.json")
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
	mainSrc, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/profile-match-rules", profilehandlers.ProfileMatchRulesHandler`) {
		t.Error("main.go does not route /profile-match-rules")
	}
}

// ---- the order (step 143) ----

func putOrder(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return handlertest.BoardCall(t, ProfileMatchRulesOrderHandler, "PUT", "/profile-match-rules/order", body)
}

func orderBody(profileID string, ids ...string) string {
	raw, _ := json.Marshal(map[string]any{"profileId": profileID, "ruleIds": ids})
	return string(raw)
}

func TestTheOrderRouteSetsTheOrderAndAnswersWithIt(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	a := createRule(t, p1, 60, jobboard.NotReviewedID)
	b := createRule(t, p1, 85, jobboard.NotReviewedID)
	c := createRule(t, p1, 70, jobboard.NotReviewedID)
	other := createRule(t, p2, 50, jobboard.NotReviewedID)
	if got := listRules(t, p1); got[0].ID != a.ID || got[1].ID != b.ID || got[2].ID != c.ID || got[2].Position != 2 {
		t.Fatalf("created order = %+v", got)
	}

	rec := putOrder(t, orderBody(p1, c.ID, a.ID, b.ID))
	handlertest.WantStatus(t, rec, 200)
	var answer rulesAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	gotIDs := []string{}
	for i, r := range answer.Rules {
		gotIDs = append(gotIDs, r.ID)
		if r.Position != i {
			t.Errorf("position of %d%% = %d, want %d", r.MinPercent, r.Position, i)
		}
	}
	if !reflect.DeepEqual(gotIDs, []string{c.ID, a.ID, b.ID}) {
		t.Errorf("answer order = %v", gotIDs)
	}
	// the list route shows the same order, and another profile's rules are untouched
	again := listRules(t, p1)
	if again[0].ID != c.ID || again[1].ID != a.ID || again[2].ID != b.ID {
		t.Errorf("list after the reorder = %+v", again)
	}
	if o := listRules(t, p2); len(o) != 1 || o[0].ID != other.ID || o[0].Position != 0 {
		t.Errorf("another profile's rules changed: %+v", o)
	}
	// a rule made afterwards goes last
	last := createRule(t, p1, 40, jobboard.NotReviewedID)
	if l := listRules(t, p1); l[len(l)-1].ID != last.ID {
		t.Error("a new rule must go to the end")
	}
}

func TestTheOrderRouteRefusals(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	a := createRule(t, p1, 60, jobboard.NotReviewedID)
	b := createRule(t, p1, 85, jobboard.NotReviewedID)
	foreign := createRule(t, p2, 50, jobboard.NotReviewedID)

	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"not json":            {`nope`, 400},
		"empty body":          {``, 400},
		"no profile id":       {`{"ruleIds":["x"]}`, 400},
		"empty profile id":    {`{"profileId":"","ruleIds":[]}`, 400},
		"ruleIds as text":     {fmt.Sprintf(`{"profileId":%q,"ruleIds":"abc"}`, p1), 400},
		"a rule missing":      {orderBody(p1, a.ID), 400},
		"no rules listed":     {orderBody(p1), 400},
		"ruleIds missing":     {fmt.Sprintf(`{"profileId":%q}`, p1), 400},
		"ruleIds null":        {fmt.Sprintf(`{"profileId":%q,"ruleIds":null}`, p1), 400},
		"an unknown rule":     {orderBody(p1, a.ID, "nope"), 400},
		"another profile's":   {orderBody(p1, a.ID, foreign.ID), 400},
		"a rule twice":        {orderBody(p1, a.ID, a.ID), 400},
		"too many":            {orderBody(p1, a.ID, b.ID, foreign.ID), 400},
		"an unknown profile":  {orderBody("nope", a.ID), 404},
		"a valid order works": {orderBody(p1, b.ID, a.ID), 200},
	} {
		if rec := putOrder(t, tc.body); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, tc.want, rec.Body.String())
		}
	}
	// only the last, valid call changed anything
	if l := listRules(t, p1); len(l) != 2 || l[0].ID != b.ID || l[1].ID != a.ID {
		t.Errorf("order = %+v, want b then a", l)
	}
	for _, m := range []string{"GET", "POST", "DELETE", "PATCH"} {
		handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesOrderHandler, m, "/profile-match-rules/order", orderBody(p1, a.ID, b.ID)), 405)
	}
}

func TestTheOrderRouteOfAProfileWithoutRulesAnswersAnEmptyList(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	rec := putOrder(t, orderBody(p1))
	handlertest.WantStatus(t, rec, 200)
	if !strings.Contains(rec.Body.String(), `"rules":[]`) {
		t.Errorf("body = %s, want an empty array", rec.Body.String())
	}
}

// The order route is declared with the jobs scope next to the others and routed in main.go.
func TestTheOrderRouteIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
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
		if r.PathSuffix == "profile-match-rules/order" {
			seen[r.Method] = r.RequiredScope
		}
	}
	if !reflect.DeepEqual(seen, map[string]string{"PUT": "tool:career:jobs"}) {
		t.Errorf("declared = %v, want exactly PUT with the jobs scope", seen)
	}
	mainSrc, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/profile-match-rules/order", profilehandlers.ProfileMatchRulesOrderHandler`) {
		t.Error("main.go does not route /profile-match-rules/order")
	}
}

// ---- apply to existing jobs (step 143) ----

func postApply(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	return handlertest.BoardCall(t, ProfileMatchRulesApplyHandler, "POST", "/profile-match-rules/apply", body)
}

func applyBody(profileID string, dryRun bool) string {
	raw, _ := json.Marshal(map[string]any{"profileId": profileID, "dryRun": dryRun})
	return string(raw)
}

// withApply replaces the pass the route runs for one test.
func withApply(t *testing.T, fn func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error)) {
	t.Helper()
	old := applyRules
	applyRules = fn
	t.Cleanup(func() { applyRules = old })
}

type applyAnswer struct {
	Jobs      int `json:"jobs"`
	NotJudged int `json:"notJudged"`
	Below     int `json:"below"`
	OnBoard   int `json:"onBoard"`
	Removed   int `json:"removed"`
	Total     int `json:"total"`
	Statuses  []struct {
		StatusID   string `json:"statusId"`
		StatusName string `json:"statusName"`
		Count      int    `json:"count"`
	} `json:"statuses"`
}

// The whole route with the real scorer: a preview, then the real thing, then a repeat that adds nothing.
func TestTheApplyRouteEndToEnd(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	fit := handlertest.SeedScoringPersona(t, p1, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	_ = fit
	handlertest.SeedScoringPersona(t, p2, "Other", []string{"Cobol", "Fortran"})
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`UPDATE jobs SET title = 'Backend Engineer (m/w/d)', location = 'Hamburg', description = ? WHERE id = ?`, handlertest.RecalcDesc, id); err != nil {
			t.Fatal(err)
		}
	}
	st, err := jobboard.CreateStatus(p1, "To check")
	if err != nil {
		t.Fatal(err)
	}
	createRule(t, p1, 60, st.ID)
	handlertest.MustSet(t, p1, "j2", jobboard.AppliedID) // a card the person put there: never moved

	rec := postApply(t, applyBody(p1, true))
	handlertest.WantStatus(t, rec, 200)
	var preview applyAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if preview.Jobs != 2 || preview.Total != 1 || preview.OnBoard != 1 || len(preview.Statuses) != 1 || preview.Statuses[0].StatusName != "To check" || preview.Statuses[0].Count != 1 {
		t.Fatalf("preview = %+v", preview)
	}
	if n := countBoard(t, p1); n != 1 {
		t.Fatalf("a preview must write nothing: %d entries", n)
	}
	rec = postApply(t, applyBody(p1, false))
	handlertest.WantStatus(t, rec, 200)
	var done applyAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &done)
	if !reflect.DeepEqual(preview, done) {
		t.Errorf("the real run must report what the preview said:\n preview %+v\n real    %+v", preview, done)
	}
	if n := countBoard(t, p1); n != 2 {
		t.Errorf("entries = %d, want 2", n)
	}
	var j2 string
	_ = db.JobsDB.QueryRow(`SELECT status_id FROM job_board_entries WHERE profile_id = ? AND job_id = 'j2'`, p1).Scan(&j2)
	if j2 != jobboard.AppliedID {
		t.Errorf("a card already on the board moved to %q", j2)
	}
	rec = postApply(t, applyBody(p1, false))
	var again applyAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &again)
	if again.Total != 0 || again.OnBoard != 2 {
		t.Errorf("a repeat must add nothing: %+v", again)
	}
	if n := countBoard(t, p2); n != 0 {
		t.Errorf("another profile's board changed: %d", n)
	}
}

func countBoard(t *testing.T, profileID string) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_board_entries WHERE profile_id = ?`, profileID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTheApplyRouteRefusals(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	for name, tc := range map[string]struct {
		body string
		want int
	}{
		"not json":                          {`nope`, 400},
		"empty body":                        {``, 400},
		"no profile id":                     {`{"dryRun":true}`, 400},
		"empty profile id":                  {`{"profileId":""}`, 400},
		"dryRun as text":                    {fmt.Sprintf(`{"profileId":%q,"dryRun":"yes"}`, p1), 400},
		"profile id as number":              {`{"profileId":5}`, 400},
		"an unknown profile":                {applyBody("nope", true), 404},
		"a profile with no rules (preview)": {applyBody(p1, true), 400},
		"a profile with no rules (apply)":   {applyBody(p1, false), 400},
	} {
		if rec := postApply(t, tc.body); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, tc.want, rec.Body.String())
		}
	}
	rec := postApply(t, applyBody(p1, false))
	if !strings.Contains(rec.Body.String(), "add a rule first") {
		t.Errorf("the no-rules answer must say what to do: %s", rec.Body.String())
	}
	for _, m := range []string{"GET", "PUT", "DELETE", "PATCH"} {
		handlertest.WantStatus(t, handlertest.BoardCall(t, ProfileMatchRulesApplyHandler, m, "/profile-match-rules/apply", applyBody(p1, true)), 405)
	}
	if countBoard(t, p1) != 0 {
		t.Error("a refused request must write nothing")
	}
}

func TestTheApplyRoutePassesTheProfileAndTheDryRunFlagOn(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	var gotProfile string
	var gotDry []bool
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		gotProfile = profileID
		gotDry = append(gotDry, dryRun)
		return profilejobmatchconsumer.ApplySummary{Jobs: 3, Total: 2, Statuses: []profilejobmatchconsumer.StatusCount{{StatusID: "s", StatusName: "S", Count: 2}}}, nil
	})
	handlertest.WantStatus(t, postApply(t, applyBody(p1, true)), 200)
	handlertest.WantStatus(t, postApply(t, applyBody(p1, false)), 200)
	handlertest.WantStatus(t, postApply(t, fmt.Sprintf(`{"profileId":%q}`, p1)), 200) // dryRun left out means a real run
	if gotProfile != p1 || !reflect.DeepEqual(gotDry, []bool{true, false, false}) {
		t.Errorf("profile %q, dry flags %v", gotProfile, gotDry)
	}
	rec := postApply(t, applyBody(p1, true))
	var a applyAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a.Jobs != 3 || a.Total != 2 || len(a.Statuses) != 1 || a.Statuses[0].Count != 2 {
		t.Errorf("the summary is answered as it is: %s", rec.Body.String())
	}
}

// The request's own context reaches the pass, so a caller that goes away stops it between chunks.
func TestTheApplyRoutePassesTheRequestContext(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	var sawCancel bool
	withApply(t, func(c context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		cancel()
		sawCancel = c.Err() != nil
		return profilejobmatchconsumer.ApplySummary{}, c.Err()
	})
	req := httptest.NewRequest("POST", "/profile-match-rules/apply", strings.NewReader(applyBody(p1, false))).WithContext(ctx)
	ProfileMatchRulesApplyHandler(httptest.NewRecorder(), req)
	if !sawCancel {
		t.Error("the pass did not get the request's context")
	}
}

// One pass per profile at a time: a second request for the same profile is a 409 while the first runs; another profile is
// free; the profile is free again afterwards, also after a failure.
func TestOnlyOnePassPerProfileRunsAtATime(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	release := make(chan struct{})
	started := make(chan string, 4)
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		started <- profileID
		<-release
		return profilejobmatchconsumer.ApplySummary{Jobs: 1}, nil
	})
	first := make(chan int, 1)
	go func() { first <- postApply(t, applyBody(p1, false)).Code }()
	if got := <-started; got != p1 {
		t.Fatalf("started %q", got)
	}
	if rec := postApply(t, applyBody(p1, false)); rec.Code != http.StatusConflict {
		t.Errorf("a second pass for the same profile while one runs: status %d, want 409", rec.Code)
	}
	if rec := postApply(t, applyBody(p1, true)); rec.Code != http.StatusConflict {
		t.Errorf("a preview for the same profile while a pass runs: status %d, want 409", rec.Code)
	}
	other := make(chan int, 1)
	go func() { other <- postApply(t, applyBody(p2, false)).Code }()
	if got := <-started; got != p2 {
		t.Fatalf("the other profile must be free to start, started %q", got)
	}
	close(release)
	if code := <-first; code != 200 {
		t.Errorf("the first pass answered %d", code)
	}
	if code := <-other; code != 200 {
		t.Errorf("the other profile's pass answered %d", code)
	}
	if rec := postApply(t, applyBody(p1, false)); rec.Code != 200 {
		t.Errorf("the profile must be free again afterwards, got %d", rec.Code)
	}
}

func TestTheProfileIsFreeAgainAfterAFailureAndAfterNoRules(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	calls := 0
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		calls++
		switch calls {
		case 1:
			return profilejobmatchconsumer.ApplySummary{}, errors.New("boom")
		case 2:
			return profilejobmatchconsumer.ApplySummary{}, profilejobmatchconsumer.ErrNoRules
		}
		return profilejobmatchconsumer.ApplySummary{}, nil
	})
	for i, want := range []int{500, 400, 200} {
		if rec := postApply(t, applyBody(p1, false)); rec.Code != want {
			t.Errorf("call %d: status %d, want %d", i+1, rec.Code, want)
		}
	}
}

// A failure answers 500 with the reason and what was done before it (what was added stays).
func TestAFailedPassAnswers500WithThePartialSummary(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		return profilejobmatchconsumer.ApplySummary{Jobs: 4, Total: 3, Statuses: []profilejobmatchconsumer.StatusCount{{StatusID: "s", StatusName: "S", Count: 3}}}, errors.New("could not score job x")
	})
	rec := postApply(t, applyBody(p1, false))
	handlertest.WantStatus(t, rec, 500)
	var body struct {
		Error   string      `json:"error"`
		Summary applyAnswer `json:"summary"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	if !strings.Contains(body.Error, "could not score job x") || body.Summary.Total != 3 || body.Summary.Jobs != 4 {
		t.Errorf("body = %+v", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("content type %q", ct)
	}
}

func TestAnUnknownProfileFromThePassIsA404(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		return profilejobmatchconsumer.ApplySummary{}, profile.ErrUnknownProfile // the profile vanished between the two checks
	})
	handlertest.WantStatus(t, postApply(t, applyBody(p1, false)), 404)
}

// The route is declared with the jobs scope and routed in main.go.
func TestTheApplyRouteIsDeclaredAndRouted(t *testing.T) {
	raw, err := os.ReadFile("../../../manifest.json")
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
		if r.PathSuffix == "profile-match-rules/apply" {
			seen[r.Method] = r.RequiredScope
		}
	}
	if !reflect.DeepEqual(seen, map[string]string{"POST": "tool:career:jobs"}) {
		t.Errorf("declared = %v, want exactly POST with the jobs scope", seen)
	}
	mainSrc, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSrc), `"/profile-match-rules/apply", profilehandlers.ProfileMatchRulesApplyHandler`) {
		t.Error("main.go does not route /profile-match-rules/apply")
	}
}

// The route checks the profile itself, before it takes the guard or runs the pass (a fake pass would accept anything).
func TestTheApplyRouteChecksTheProfileBeforeRunningThePass(t *testing.T) {
	handlertest.BoardFixture(t)
	ran := false
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		ran = true
		return profilejobmatchconsumer.ApplySummary{}, nil
	})
	handlertest.WantStatus(t, postApply(t, applyBody("no-such-profile", false)), 404)
	if ran {
		t.Error("the pass must not run for an unknown profile")
	}
	applying.Lock()
	n := len(applying.profiles)
	applying.Unlock()
	if n != 0 {
		t.Errorf("the guard holds %d profile(s) after a refused request", n)
	}
}

// Freeing one profile must not free another that is still running.
func TestFinishingOneProfilesPassKeepsTheOthersGuard(t *testing.T) {
	p1, p2 := handlertest.BoardFixture(t)
	releaseP1, releaseP2 := make(chan struct{}), make(chan struct{})
	started := make(chan string, 4)
	var secondForP2 atomic.Bool
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		started <- profileID
		switch profileID {
		case p1:
			<-releaseP1
		case p2:
			if secondForP2.Swap(true) { // a second pass for p2 can only happen with a broken guard: do not block the test on it
				return profilejobmatchconsumer.ApplySummary{}, nil
			}
			<-releaseP2
		}
		return profilejobmatchconsumer.ApplySummary{}, nil
	})
	done1, done2 := make(chan int, 1), make(chan int, 1)
	go func() { done1 <- postApply(t, applyBody(p1, false)).Code }()
	<-started
	go func() { done2 <- postApply(t, applyBody(p2, false)).Code }()
	<-started
	close(releaseP1)
	if code := <-done1; code != 200 {
		t.Fatalf("the first pass answered %d", code)
	}
	// asked with a time limit: with a broken guard the request would run instead of being refused
	probe := make(chan int, 1)
	go func() { probe <- postApply(t, applyBody(p2, false)).Code }()
	select {
	case code := <-probe:
		if code != http.StatusConflict {
			t.Errorf("the second profile is still running, a request for it must be 409, got %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Error("the request for a profile that is still running was neither refused nor finished")
	}
	close(releaseP2)
	if code := <-done2; code != 200 {
		t.Errorf("the second pass answered %d", code)
	}
}

// ---- includeRemoved (step 144) ----

func applyBodyWith(profileID string, dryRun bool, include any) string {
	raw, _ := json.Marshal(map[string]any{"profileId": profileID, "dryRun": dryRun, "includeRemoved": include})
	return string(raw)
}

func TestTheApplyRoutePassesIncludeRemovedOnAndItDefaultsToOff(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	var got [][2]bool
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		got = append(got, [2]bool{dryRun, includeRemoved})
		return profilejobmatchconsumer.ApplySummary{Jobs: 5, Removed: 2, Total: 1}, nil
	})
	handlertest.WantStatus(t, postApply(t, applyBodyWith(p1, true, true)), 200)
	handlertest.WantStatus(t, postApply(t, applyBodyWith(p1, false, true)), 200)
	handlertest.WantStatus(t, postApply(t, applyBodyWith(p1, true, false)), 200)
	handlertest.WantStatus(t, postApply(t, applyBody(p1, true)), 200) // includeRemoved left out
	handlertest.WantStatus(t, postApply(t, fmt.Sprintf(`{"profileId":%q}`, p1)), 200)
	want := [][2]bool{{true, true}, {false, true}, {true, false}, {true, false}, {false, false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("(dryRun, includeRemoved) = %v, want %v", got, want)
	}
	rec := postApply(t, applyBody(p1, true))
	var a applyAnswer
	_ = json.Unmarshal(rec.Body.Bytes(), &a)
	if a.Removed != 2 || a.Total != 1 || a.Jobs != 5 || !strings.Contains(rec.Body.String(), `"removed":2`) {
		t.Errorf("the removed number is answered as it is: %s", rec.Body.String())
	}
}

func TestIncludeRemovedMustBeABoolean(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	ran := false
	withApply(t, func(ctx context.Context, profileID string, dryRun, includeRemoved bool) (profilejobmatchconsumer.ApplySummary, error) {
		ran = true
		return profilejobmatchconsumer.ApplySummary{}, nil
	})
	for _, v := range []any{"yes", 1, []int{1}, map[string]int{"a": 1}} {
		if rec := postApply(t, applyBodyWith(p1, true, v)); rec.Code != 400 {
			t.Errorf("includeRemoved %v: status %d, want 400", v, rec.Code)
		}
	}
	if ran {
		t.Error("the pass must not run for a bad body")
	}
}

// The whole thing through the real routes and the real scorer: a card taken off through the board's own route is left out,
// the box adds it back, and a hand put-back is an ordinary card.
func TestTheApplyRouteLeavesOutWhatThePersonRemovedUnlessAskedTo(t *testing.T) {
	p1, _ := handlertest.BoardFixture(t)
	handlertest.SeedScoringPersona(t, p1, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`UPDATE jobs SET title = 'Backend Engineer (m/w/d)', location = 'Hamburg', description = ? WHERE id = ?`, handlertest.RecalcDesc, id); err != nil {
			t.Fatal(err)
		}
	}
	st, err := jobboard.CreateStatus(p1, "To check")
	if err != nil {
		t.Fatal(err)
	}
	createRule(t, p1, 60, st.ID)
	// j1 was on the board and the person took it off through the board's own route
	handlertest.WantStatus(t, handlertest.BoardCall(t, jobshandlers.JobBoardEntriesHandler, "PUT", "/job-board-entries", `{"profileId":"`+p1+`","jobId":"j1","statusId":"`+st.ID+`"}`), 200)
	handlertest.WantStatus(t, handlertest.BoardCall(t, jobshandlers.JobBoardEntriesHandler, "DELETE", "/job-board-entries?profileId="+p1+"&jobId=j1", ""), 204)

	decode := func(rec *httptest.ResponseRecorder) applyAnswer {
		t.Helper()
		handlertest.WantStatus(t, rec, 200)
		var a applyAnswer
		if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	preview := decode(postApply(t, applyBody(p1, true)))
	if preview.Jobs != 2 || preview.Removed != 1 || preview.Total != 1 || countBoard(t, p1) != 0 {
		t.Fatalf("preview = %+v, entries %d", preview, countBoard(t, p1))
	}
	ticked := decode(postApply(t, applyBodyWith(p1, true, true)))
	if ticked.Removed != 0 || ticked.Total != 2 || countBoard(t, p1) != 0 {
		t.Fatalf("preview with the box = %+v, entries %d", ticked, countBoard(t, p1))
	}
	real := decode(postApply(t, applyBody(p1, false)))
	if !reflect.DeepEqual(preview, real) || countBoard(t, p1) != 1 {
		t.Errorf("real %+v vs preview %+v, entries %d", real, preview, countBoard(t, p1))
	}
	if got, _ := jobboard.WasRemoved(p1, "j1"); !got {
		t.Error("leaving j1 out must not forget the removal")
	}
	both := decode(postApply(t, applyBodyWith(p1, false, true)))
	if both.Total != 1 || both.Removed != 0 || both.OnBoard != 1 || countBoard(t, p1) != 2 {
		t.Errorf("with the box = %+v, entries %d", both, countBoard(t, p1))
	}
	if got, _ := jobboard.WasRemoved(p1, "j1"); got {
		t.Error("adding j1 on request must forget the removal")
	}
	after := decode(postApply(t, applyBody(p1, true)))
	if after.Total != 0 || after.Removed != 0 || after.OnBoard != 2 {
		t.Errorf("after: %+v", after)
	}
}
