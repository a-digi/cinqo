package profilerules

import (
	"errors"
	"fmt"
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

func TestCreateAndListHighestFirstWithTheStatusName(t *testing.T) {
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
	if got := percents(rules); fmt.Sprint(got) != "[85 70 60]" {
		t.Fatalf("order = %v, want highest first", got)
	}
	if rules[0].StatusName != "Top pick" || rules[0].StatusID != top || rules[0].ProfileID != p || rules[0].ID == "" || rules[1].StatusName != "Not Reviewed" {
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
	if _, err := Update(rules[0].ID, 99, jobboard.AppliedID); err != nil {
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
	if fmt.Sprint(percents(rules)) != "[80 60]" {
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

func TestRuleForPicksTheHighestThresholdTheScoreReaches(t *testing.T) {
	rules := []Rule{{ID: "a", MinPercent: 60}, {ID: "c", MinPercent: 85}, {ID: "b", MinPercent: 70}} // any order
	cases := []struct {
		score int
		want  string // "" = none
	}{
		{0, ""}, {59, ""}, {60, "a"}, {61, "a"}, {69, "a"}, {70, "b"}, {84, "b"}, {85, "c"}, {100, "c"},
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
	if _, ok := RuleFor(nil, 100); ok {
		t.Error("no rules must never apply")
	}
}

func TestAllGroupsByProfileHighestFirst(t *testing.T) {
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
	if len(all) != 2 || fmt.Sprint(percents(all[a])) != "[90 60]" || fmt.Sprint(percents(all[b])) != "[50]" {
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
	if fmt.Sprint(percents(rules)) != "[80 70]" {
		t.Errorf("Ana's rules = %v, want 80 and 70", percents(rules))
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
