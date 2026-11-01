package profilerules

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
)

func setup(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatalf("InitDatabases: %v", err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func newProfile(t *testing.T, name string) string {
	t.Helper()
	id, err := profile.CreateProfile(name, "Test")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func status(t *testing.T, profileID, name string) string {
	t.Helper()
	s, err := jobboard.CreateStatus(profileID, name)
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func mustCreate(t *testing.T, profileID string, percent int, statusID string) Rule {
	t.Helper()
	r, err := Create(profileID, percent, statusID)
	if err != nil {
		t.Fatalf("Create(%d): %v", percent, err)
	}
	return r
}

func count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func percents(rules []Rule) []int {
	out := []int{}
	for _, r := range rules {
		out = append(out, r.MinPercent)
	}
	return out
}

func TestCreateAndListInTheirOrderWithTheStatusNameAndPosition(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check := status(t, p, "To check")
	top := status(t, p, "Top pick")
	mustCreate(t, p, 60, check)
	mustCreate(t, p, 85, top)
	mustCreate(t, p, 70, jobboard.NotReviewedID)

	rules, err := List(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := percents(rules); fmt.Sprint(got) != "[60 85 70]" {
		t.Fatalf("order = %v, want the order they were made in (a new rule goes to the end), not by percentage", got)
	}
	for i, r := range rules {
		if r.Position != i {
			t.Errorf("rule %d has position %d", i, r.Position)
		}
	}
	if rules[1].StatusName != "Top pick" || rules[1].StatusID != top || rules[1].ProfileID != p || rules[1].ID == "" || rules[2].StatusName != "Not Reviewed" {
		t.Errorf("rules = %+v", rules)
	}
}

func TestAProfileWithoutRulesListsAnEmptyListNotNil(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rules, err := List(p)
	if err != nil || rules == nil || len(rules) != 0 {
		t.Errorf("rules=%v err=%v", rules, err)
	}
}

func TestThePercentageBounds(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	for _, bad := range []int{0, -1, 101, 1000} {
		if _, err := Create(p, bad, jobboard.NotReviewedID); !errors.Is(err, ErrInvalidPercent) {
			t.Errorf("Create(%d) = %v, want ErrInvalidPercent", bad, err)
		}
	}
	for _, good := range []int{1, 100} {
		mustCreate(t, p, good, jobboard.NotReviewedID)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules`); n != 2 {
		t.Errorf("rules = %d, want only the two valid ones", n)
	}
}

func TestAnUnknownProfileIsRefused(t *testing.T) {
	setup(t)
	if _, err := Create("nope", 60, jobboard.NotReviewedID); !errors.Is(err, profile.ErrUnknownProfile) {
		t.Errorf("Create = %v", err)
	}
	if _, err := List("nope"); !errors.Is(err, profile.ErrUnknownProfile) {
		t.Errorf("List = %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules`); n != 0 {
		t.Errorf("rules = %d", n)
	}
}

func TestTheStatusMustExistAndBelongToTheProfileOrBeFixed(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	own := status(t, a, "Mine")
	others := status(t, b, "Theirs")

	for _, fixed := range []string{jobboard.NotReviewedID, jobboard.AppliedID, jobboard.DeclinedID, jobboard.StoppedID} {
		mustCreate(t, a, 10+count(t, `SELECT COUNT(*) FROM profile_match_rules`), fixed)
	}
	mustCreate(t, a, 90, own)
	if _, err := Create(a, 95, others); !errors.Is(err, jobboard.ErrStatusNotOnBoard) {
		t.Errorf("another profile's status: %v", err)
	}
	if _, err := Create(a, 96, "no-such-status"); !errors.Is(err, jobboard.ErrUnknownStatus) {
		t.Errorf("unknown status: %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules`); n != 5 {
		t.Errorf("rules = %d, want 5", n)
	}
}

func TestOneRulePerPercentageAndProfile(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	mustCreate(t, a, 60, jobboard.NotReviewedID)
	if _, err := Create(a, 60, jobboard.AppliedID); !errors.Is(err, ErrDuplicatePercent) {
		t.Errorf("duplicate = %v", err)
	}
	mustCreate(t, b, 60, jobboard.NotReviewedID) // another profile may use it
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, a); n != 1 {
		t.Errorf("Ana's rules = %d, want 1", n)
	}
}

func TestAtMostTenRulesPerProfile(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	for i := 1; i <= MaxRulesPerProfile; i++ {
		mustCreate(t, a, i*5, jobboard.NotReviewedID)
	}
	if _, err := Create(a, 99, jobboard.NotReviewedID); !errors.Is(err, ErrTooManyRules) {
		t.Errorf("11th rule = %v", err)
	}
	mustCreate(t, b, 99, jobboard.NotReviewedID) // the limit is per profile
	// a rule at the limit can still be changed
	rules, _ := List(a)
	if _, err := Update(rules[len(rules)-1].ID, 99, jobboard.AppliedID); err != nil { // the 50% rule
		t.Errorf("Update at the limit: %v", err)
	}
	// a duplicate is reported as a duplicate even at the limit
	if _, err := Create(a, 5, jobboard.NotReviewedID); !errors.Is(err, ErrDuplicatePercent) {
		t.Errorf("duplicate at the limit = %v", err)
	}
}

func TestUpdateChangesThePercentageAndTheStatusButNeverTheProfile(t *testing.T) {
	setup(t)
	a := newProfile(t, "Ana")
	s := status(t, a, "Later")
	r := mustCreate(t, a, 60, jobboard.NotReviewedID)

	got, err := Update(r.ID, 75, s)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != r.ID || got.ProfileID != a || got.MinPercent != 75 || got.StatusID != s || got.StatusName != "Later" {
		t.Errorf("updated = %+v", got)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules`); n != 1 {
		t.Errorf("rules = %d, want still 1", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE updated_at IS NOT NULL`); n != 1 {
		t.Error("updated_at must be set")
	}
	// the same percentage as itself is no duplicate
	if _, err := Update(r.ID, 75, jobboard.AppliedID); err != nil {
		t.Errorf("keeping its own percentage: %v", err)
	}
}

func TestUpdateRefusals(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	others := status(t, b, "Theirs")
	r1 := mustCreate(t, a, 60, jobboard.NotReviewedID)
	mustCreate(t, a, 80, jobboard.NotReviewedID)

	if _, err := Update("nope", 50, jobboard.NotReviewedID); !errors.Is(err, ErrUnknownRule) {
		t.Errorf("unknown rule = %v", err)
	}
	if _, err := Update(r1.ID, 0, jobboard.NotReviewedID); !errors.Is(err, ErrInvalidPercent) {
		t.Errorf("percent 0 = %v", err)
	}
	if _, err := Update(r1.ID, 101, jobboard.NotReviewedID); !errors.Is(err, ErrInvalidPercent) {
		t.Errorf("percent 101 = %v", err)
	}
	if _, err := Update(r1.ID, 80, jobboard.NotReviewedID); !errors.Is(err, ErrDuplicatePercent) {
		t.Errorf("another rule's percentage = %v", err)
	}
	if _, err := Update(r1.ID, 61, others); !errors.Is(err, jobboard.ErrStatusNotOnBoard) {
		t.Errorf("another profile's status = %v", err)
	}
	if _, err := Update(r1.ID, 61, "no-such-status"); !errors.Is(err, jobboard.ErrUnknownStatus) {
		t.Errorf("unknown status = %v", err)
	}
	rules, _ := List(a)
	if fmt.Sprint(percents(rules)) != "[60 80]" {
		t.Errorf("a refused update changed something: %v", percents(rules))
	}
}

func TestDeleteRemovesOnlyThatRuleAndAnUnknownIdIsANoOp(t *testing.T) {
	setup(t)
	a := newProfile(t, "Ana")
	r1 := mustCreate(t, a, 60, jobboard.NotReviewedID)
	mustCreate(t, a, 80, jobboard.NotReviewedID)
	if err := Delete("nope"); err != nil {
		t.Errorf("unknown id: %v", err)
	}
	if err := Delete(r1.ID); err != nil {
		t.Fatal(err)
	}
	rules, _ := List(a)
	if fmt.Sprint(percents(rules)) != "[80]" {
		t.Errorf("rules = %v", percents(rules))
	}
}

func TestRuleForPicksTheFirstRuleOfTheListThatFits(t *testing.T) {
	rules := []Rule{{ID: "a", MinPercent: 60}, {ID: "c", MinPercent: 85}, {ID: "b", MinPercent: 70}} // this order
	cases := []struct {
		score int
		want  string // "" = none
	}{
		{0, ""}, {59, ""}, {60, "a"}, {61, "a"}, {69, "a"}, {70, "a"}, {84, "a"}, {85, "a"}, {100, "a"}, // 60% comes first and fits all
	}
	for _, c := range cases {
		got, ok := RuleFor(rules, c.score)
		if c.want == "" {
			if ok {
				t.Errorf("score %d: got rule %s, want none", c.score, got.ID)
			}
			continue
		}
		if !ok || got.ID != c.want {
			t.Errorf("score %d: got %v %v, want %s", c.score, got.ID, ok, c.want)
		}
	}
	// the same rules with the highest percentage first: now the percentage order decides
	rules = []Rule{{ID: "c", MinPercent: 85}, {ID: "b", MinPercent: 70}, {ID: "a", MinPercent: 60}}
	for score, want := range map[int]string{59: "", 60: "a", 69: "a", 70: "b", 84: "b", 85: "c", 100: "c"} {
		got, ok := RuleFor(rules, score)
		if (want == "") == ok || (ok && got.ID != want) {
			t.Errorf("highest first, score %d: got %v %v, want %q", score, got.ID, ok, want)
		}
	}
	// a rule that does not fit is skipped, the next one that fits wins
	mixed := []Rule{{ID: "high", MinPercent: 90}, {ID: "low", MinPercent: 50}}
	if got, ok := RuleFor(mixed, 70); !ok || got.ID != "low" {
		t.Errorf("a rule that does not fit must be skipped: %v %v", got.ID, ok)
	}
	if _, ok := RuleFor(nil, 100); ok {
		t.Error("no rules must never apply")
	}
}

func TestAllGroupsByProfileInTheirOrder(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	newProfile(t, "Cy") // no rules: absent
	mustCreate(t, a, 60, jobboard.NotReviewedID)
	mustCreate(t, a, 90, jobboard.AppliedID)
	mustCreate(t, b, 50, jobboard.NotReviewedID)
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || fmt.Sprint(percents(all[a])) != "[60 90]" || fmt.Sprint(percents(all[b])) != "[50]" {
		t.Errorf("all = %+v", all)
	}
}

// A deleted custom status takes its rules with it (its cards move to Not Reviewed as before); other rules stay.
func TestDeletingACustomStatusDeletesItsRules(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	gone := status(t, a, "Gone")
	keep := status(t, a, "Keep")
	bs := status(t, b, "Bens")
	mustCreate(t, a, 60, gone)
	mustCreate(t, a, 70, keep)
	mustCreate(t, a, 80, jobboard.AppliedID)
	mustCreate(t, b, 60, bs)

	if err := jobboard.DeleteStatus(gone); err != nil {
		t.Fatal(err)
	}
	rules, _ := List(a)
	if fmt.Sprint(percents(rules)) != "[70 80]" {
		t.Errorf("Ana's rules = %v, want 70 and 80 in their order", percents(rules))
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, b); n != 1 {
		t.Errorf("Ben's rules = %d, want 1", n)
	}
}

func TestAFixedStatusCannotBeDeletedSoItsRulesStay(t *testing.T) {
	setup(t)
	a := newProfile(t, "Ana")
	mustCreate(t, a, 60, jobboard.AppliedID)
	if err := jobboard.DeleteStatus(jobboard.AppliedID); !errors.Is(err, jobboard.ErrFixedStatus) {
		t.Fatalf("DeleteStatus = %v", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules`); n != 1 {
		t.Errorf("rules = %d", n)
	}
}

func TestPurgeProfileRemovesThatProfilesRulesOnly(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	sa := status(t, a, "A status")
	mustCreate(t, a, 60, sa)
	mustCreate(t, a, 70, jobboard.NotReviewedID)
	mustCreate(t, b, 60, jobboard.NotReviewedID)

	if err := jobboard.PurgeProfile(a); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, a); n != 0 {
		t.Errorf("Ana's rules = %d, want 0", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, b); n != 1 {
		t.Errorf("Ben's rules = %d, want 1", n)
	}
}

// End to end: deleting the profile itself removes its rules (profile.DeleteProfile calls jobboard.PurgeProfile).
func TestDeletingAProfileRemovesItsRules(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	mustCreate(t, a, 60, jobboard.NotReviewedID)
	mustCreate(t, b, 60, jobboard.NotReviewedID)
	if err := profile.DeleteProfile(a); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, a); n != 0 {
		t.Errorf("rules left for the deleted profile: %d", n)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, b); n != 1 {
		t.Errorf("the other profile's rules = %d, want 1", n)
	}
}

func TestTheTableRefusesBadValuesByItself(t *testing.T) {
	setup(t)
	if _, err := db.JobsDB.Exec(`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id) VALUES ('x','p',0,'fixed-applied')`); err == nil {
		t.Error("the CHECK must refuse 0")
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id) VALUES ('x','p',101,'fixed-applied')`); err == nil {
		t.Error("the CHECK must refuse 101")
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id) VALUES ('x','p',50,'no-status')`); err == nil {
		t.Error("the foreign key must refuse an unknown status")
	}
	for i := 0; i < 2; i++ {
		_, err := db.JobsDB.Exec(`INSERT INTO profile_match_rules (id, profile_id, min_percent, status_id) VALUES (?, 'p', 50, 'fixed-applied')`, fmt.Sprint("y", i))
		if (i == 0) != (err == nil) {
			t.Errorf("insert %d: err=%v (the UNIQUE must refuse the second)", i, err)
		}
	}
}

// The limit is ten, written out (not the constant, so changing the constant is noticed).
func TestTheLimitIsExactlyTenRules(t *testing.T) {
	setup(t)
	a := newProfile(t, "Ana")
	for i := 1; i <= 10; i++ {
		if _, err := Create(a, i, jobboard.NotReviewedID); err != nil {
			t.Fatalf("rule %d: %v", i, err)
		}
	}
	if _, err := Create(a, 11, jobboard.NotReviewedID); !errors.Is(err, ErrTooManyRules) {
		t.Errorf("rule 11 = %v, want ErrTooManyRules", err)
	}
	if n := count(t, `SELECT COUNT(*) FROM profile_match_rules WHERE profile_id = ?`, a); n != 10 {
		t.Errorf("rules = %d, want 10", n)
	}
}

func ids(rules []Rule) []string {
	out := []string{}
	for _, r := range rules {
		out = append(out, r.ID)
	}
	return out
}

func TestANewRuleGoesToTheEndAndAChangeOrADeleteKeepTheOrder(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	r1 := mustCreate(t, p, 60, jobboard.NotReviewedID)
	r2 := mustCreate(t, p, 85, jobboard.NotReviewedID)
	r3 := mustCreate(t, p, 70, jobboard.NotReviewedID)
	if _, err := Update(r2.ID, 90, jobboard.AppliedID); err != nil { // a change keeps the place
		t.Fatal(err)
	}
	if rules, _ := List(p); !reflect.DeepEqual(ids(rules), []string{r1.ID, r2.ID, r3.ID}) {
		t.Errorf("after a change: %v", percents(rules))
	}
	if err := Delete(r1.ID); err != nil {
		t.Fatal(err)
	}
	r4 := mustCreate(t, p, 40, jobboard.NotReviewedID) // after the delete a new rule is still last
	rules, _ := List(p)
	if !reflect.DeepEqual(ids(rules), []string{r2.ID, r3.ID, r4.ID}) {
		t.Errorf("after a delete and a new rule: %v", percents(rules))
	}
	if rules[2].Position <= rules[1].Position || rules[1].Position <= rules[0].Position {
		t.Errorf("positions must keep rising: %d %d %d", rules[0].Position, rules[1].Position, rules[2].Position)
	}
}

func TestPositionsOfOneProfileDoNotDependOnAnother(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	mustCreate(t, a, 60, jobboard.NotReviewedID)
	mustCreate(t, a, 70, jobboard.NotReviewedID)
	rb := mustCreate(t, b, 60, jobboard.NotReviewedID)
	if rb.Position != 0 {
		t.Errorf("the first rule of another profile starts at 0, got %d", rb.Position)
	}
}

func TestReorderSetsTheOrderAndReturnsIt(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	r1 := mustCreate(t, a, 60, jobboard.NotReviewedID)
	r2 := mustCreate(t, a, 85, jobboard.NotReviewedID)
	r3 := mustCreate(t, a, 70, jobboard.NotReviewedID)
	other := mustCreate(t, b, 50, jobboard.NotReviewedID)

	got, err := Reorder(a, []string{r3.ID, r1.ID, r2.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(got), []string{r3.ID, r1.ID, r2.ID}) {
		t.Errorf("returned order = %v", percents(got))
	}
	for i, r := range got {
		if r.Position != i {
			t.Errorf("position of %d%% = %d, want %d", r.MinPercent, r.Position, i)
		}
	}
	if again, _ := List(a); !reflect.DeepEqual(ids(again), ids(got)) {
		t.Errorf("List = %v, Reorder returned %v", percents(again), percents(got))
	}
	if otherRules, _ := List(b); len(otherRules) != 1 || otherRules[0].ID != other.ID || otherRules[0].Position != 0 {
		t.Errorf("another profile's rules changed: %+v", otherRules)
	}
	// the same order again, and back, both fine
	if _, err := Reorder(a, ids(got)); err != nil {
		t.Errorf("the same order again: %v", err)
	}
	back, err := Reorder(a, []string{r1.ID, r2.ID, r3.ID})
	if err != nil || !reflect.DeepEqual(ids(back), []string{r1.ID, r2.ID, r3.ID}) {
		t.Errorf("back: %v %v", ids(back), err)
	}
	// a rule created after a reorder is last
	r4 := mustCreate(t, a, 40, jobboard.NotReviewedID)
	if rules, _ := List(a); rules[len(rules)-1].ID != r4.ID {
		t.Error("a rule created after a reorder must be last")
	}
}

func TestReorderRefusesAnythingButExactlyTheProfilesRules(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	r1 := mustCreate(t, a, 60, jobboard.NotReviewedID)
	r2 := mustCreate(t, a, 85, jobboard.NotReviewedID)
	r3 := mustCreate(t, a, 70, jobboard.NotReviewedID)
	foreign := mustCreate(t, b, 50, jobboard.NotReviewedID)
	want := []string{r1.ID, r2.ID, r3.ID}

	for name, order := range map[string][]string{
		"a rule missing":      {r3.ID, r1.ID},
		"nothing":             {},
		"nil":                 nil,
		"an extra unknown id": {r1.ID, r2.ID, r3.ID, "nope"},
		"another profile's":   {r1.ID, r2.ID, foreign.ID},
		"a rule twice":        {r1.ID, r1.ID, r2.ID},
		"a rule twice, all":   {r1.ID, r2.ID, r3.ID, r1.ID},
		"unknown ids":         {"x", "y", "z"},
		"too many":            {r1.ID, r2.ID, r3.ID, foreign.ID},
	} {
		if _, err := Reorder(a, order); !errors.Is(err, ErrBadOrder) {
			t.Errorf("%s: err = %v, want ErrBadOrder", name, err)
		}
		if rules, _ := List(a); !reflect.DeepEqual(ids(rules), want) {
			t.Errorf("%s: a refused reorder changed the order to %v", name, percents(rules))
		}
	}
	if rules, _ := List(b); len(rules) != 1 || rules[0].Position != 0 {
		t.Errorf("another profile's rule changed: %+v", rules)
	}
	if _, err := Reorder("nope", []string{r1.ID}); !errors.Is(err, profile.ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
}

func TestReorderOfAProfileWithoutRulesIsAnEmptyList(t *testing.T) {
	setup(t)
	a := newProfile(t, "Ana")
	got, err := Reorder(a, nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("%v %v", got, err)
	}
}

// With equal positions (a database that has not been reordered since the column was added) ties keep the creation order.
func TestEqualPositionsFallBackToTheCreationOrder(t *testing.T) {
	setup(t)
	a := newProfile(t, "Ana")
	r1 := mustCreate(t, a, 60, jobboard.NotReviewedID)
	r2 := mustCreate(t, a, 85, jobboard.NotReviewedID)
	r3 := mustCreate(t, a, 70, jobboard.NotReviewedID)
	for i, r := range []Rule{r1, r2, r3} {
		if _, err := db.JobsDB.Exec(`UPDATE profile_match_rules SET position = 0, created_at = ? WHERE id = ?`, fmt.Sprintf("2026-01-0%d 10:00:00", i+1), r.ID); err != nil {
			t.Fatal(err)
		}
	}
	if rules, _ := List(a); !reflect.DeepEqual(ids(rules), []string{r1.ID, r2.ID, r3.ID}) {
		t.Errorf("order = %v", percents(rules))
	}
	if all, _ := All(); !reflect.DeepEqual(ids(all[a]), []string{r1.ID, r2.ID, r3.ID}) {
		t.Errorf("All order = %v", percents(all[a]))
	}
}
