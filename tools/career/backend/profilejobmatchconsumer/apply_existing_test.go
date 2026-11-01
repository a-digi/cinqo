package profilejobmatchconsumer

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/profile"
	"career-tool-backend/profilerules"
)

func ruleIDs(t *testing.T, profileID string) []string {
	t.Helper()
	rules, err := profilerules.List(profileID)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, r := range rules {
		out = append(out, r.ID)
	}
	return out
}

func statusLine(s ApplySummary) string {
	out := ""
	for _, c := range s.Statuses {
		out += fmt.Sprintf("%s=%d ", c.StatusName, c.Count)
	}
	return out
}

func boardEntries(t *testing.T) int { return entries(t) }

// ---- each row of the table ----

func TestApplyDoesWhatTheTableSays(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check := customStatus(t, p, "To check")
	rule(t, p, 60, check)
	for _, id := range []string{"added", "below", "judgedNot", "onBoard"} {
		addJob(t, id)
	}
	mine := customStatus(t, p, "Mine")
	if _, err := jobboard.SetEntry(p, "onBoard", mine); err != nil { // fits the rule, but a person put it in "Mine"
		t.Fatal(err)
	}
	var before string
	_ = db.JobsDB.QueryRow(`SELECT updated_at FROM job_board_entries WHERE job_id = 'onBoard'`).Scan(&before)
	useFake(t, &fake{by: map[string]map[string]int{
		"added":     {p: 80},
		"below":     {p: 30},
		"judgedNot": {}, // the profile is absent: it cannot judge this job
		"onBoard":   {p: 95},
	}})

	sum, err := ApplyToExisting(context.Background(), p, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Jobs != 4 || sum.NotJudged != 1 || sum.Below != 1 || sum.OnBoard != 1 || sum.Total != 1 {
		t.Errorf("summary = %+v", sum)
	}
	if len(sum.Statuses) != 1 || sum.Statuses[0].StatusID != check || sum.Statuses[0].StatusName != "To check" || sum.Statuses[0].Count != 1 {
		t.Errorf("statuses = %+v", sum.Statuses)
	}
	if onBoard(t, p, "added") != check {
		t.Error("the job that fits and was not on the board must be added with the rule's status")
	}
	for _, id := range []string{"below", "judgedNot"} {
		if onBoard(t, p, id) != "" {
			t.Errorf("%s must not be on the board", id)
		}
	}
	var after string
	_ = db.JobsDB.QueryRow(`SELECT updated_at FROM job_board_entries WHERE job_id = 'onBoard'`).Scan(&after)
	if onBoard(t, p, "onBoard") != mine || after != before {
		t.Errorf("a card already on the board must not move: status %q, moved-at %q -> %q", onBoard(t, p, "onBoard"), before, after)
	}
	if sum.Jobs != sum.NotJudged+sum.Below+sum.OnBoard+sum.Total {
		t.Errorf("every job is in exactly one bucket: %+v", sum)
	}
}

// The first rule of the list that fits wins, also when a later rule has a higher percentage; reordering changes the result.
func TestApplyUsesTheFirstFittingRuleOfTheList(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check, top := customStatus(t, p, "To check"), customStatus(t, p, "Top pick")
	rule(t, p, 60, check) // first
	rule(t, p, 85, top)   // second: it fits only jobs the first rule fits too
	for _, id := range []string{"j50", "j70", "j90"} {
		addJob(t, id)
	}
	useFake(t, &fake{by: map[string]map[string]int{"j50": {p: 50}, "j70": {p: 70}, "j90": {p: 90}}})

	preview, err := ApplyToExisting(context.Background(), p, true)
	if err != nil {
		t.Fatal(err)
	}
	if statusLine(preview) != "To check=2 " || preview.Total != 2 || preview.Below != 1 {
		t.Fatalf("preview = %+v (%s)", preview, statusLine(preview))
	}
	if _, err := ApplyToExisting(context.Background(), p, false); err != nil {
		t.Fatal(err)
	}
	for job, want := range map[string]string{"j50": "", "j70": check, "j90": check} {
		if got := onBoard(t, p, job); got != want {
			t.Errorf("%s: %q, want %q", job, got, want)
		}
	}
}

func TestReorderingTheRulesChangesWhatApplyAdds(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	check, top := customStatus(t, p, "To check"), customStatus(t, p, "Top pick")
	rule(t, p, 60, check)
	rule(t, p, 85, top)
	addJob(t, "j90")
	addJob(t, "j70")
	useFake(t, &fake{by: map[string]map[string]int{"j90": {p: 90}, "j70": {p: 70}}})
	ids := ruleIDs(t, p)
	if _, err := profilerules.Reorder(p, []string{ids[1], ids[0]}); err != nil { // 85% first
		t.Fatal(err)
	}
	sum, err := ApplyToExisting(context.Background(), p, false)
	if err != nil {
		t.Fatal(err)
	}
	if onBoard(t, p, "j90") != top || onBoard(t, p, "j70") != check {
		t.Errorf("j90 %q (want %q), j70 %q (want %q)", onBoard(t, p, "j90"), top, onBoard(t, p, "j70"), check)
	}
	if statusLine(sum) != "Top pick=1 To check=1 " {
		t.Errorf("statuses in the order of the rules: %q", statusLine(sum))
	}
}

func TestARuleThatDoesNotFitIsSkippedAndTheNextOneDecides(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	top, low := customStatus(t, p, "Top"), customStatus(t, p, "Low")
	rule(t, p, 90, top)
	rule(t, p, 50, low)
	for _, id := range []string{"j40", "j70", "j95"} {
		addJob(t, id)
	}
	useFake(t, &fake{by: map[string]map[string]int{"j40": {p: 40}, "j70": {p: 70}, "j95": {p: 95}}})
	if _, err := ApplyToExisting(context.Background(), p, false); err != nil {
		t.Fatal(err)
	}
	for job, want := range map[string]string{"j40": "", "j70": low, "j95": top} {
		if got := onBoard(t, p, job); got != want {
			t.Errorf("%s: %q, want %q", job, got, want)
		}
	}
}

// ---- the dry run ----

func TestADryRunWritesNothingAndSaysWhatARealRunDoes(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	a, b := customStatus(t, p, "A"), customStatus(t, p, "B")
	rule(t, p, 80, a)
	rule(t, p, 50, b)
	scores := map[string]map[string]int{}
	for i, s := range []int{95, 85, 70, 55, 52, 20, 10} {
		id := fmt.Sprintf("j%d", i)
		addJob(t, id)
		scores[id] = map[string]int{p: s}
	}
	addJob(t, "unjudged")
	scores["unjudged"] = map[string]int{}
	if _, err := jobboard.SetEntry(p, "j1", b); err != nil { // one fitting job is on the board already
		t.Fatal(err)
	}
	useFake(t, &fake{by: scores})
	entriesBefore := boardEntries(t)

	preview, err := ApplyToExisting(context.Background(), p, true)
	if err != nil {
		t.Fatal(err)
	}
	if boardEntries(t) != entriesBefore {
		t.Fatal("a dry run wrote to the board")
	}
	if preview.Jobs != 8 || preview.NotJudged != 1 || preview.Below != 2 || preview.OnBoard != 1 || preview.Total != 4 || statusLine(preview) != "A=1 B=3 " {
		t.Fatalf("preview = %+v (%s)", preview, statusLine(preview))
	}
	real, err := ApplyToExisting(context.Background(), p, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview, real) {
		t.Errorf("a dry run and a real run must give the same numbers:\n dry  %+v\n real %+v", preview, real)
	}
	if boardEntries(t) != entriesBefore+4 {
		t.Errorf("entries = %d, want %d", boardEntries(t), entriesBefore+4)
	}
}

func TestASecondRunAddsNothingMore(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "S")
	rule(t, p, 50, s)
	scores := map[string]map[string]int{}
	for _, id := range []string{"a", "b", "c"} {
		addJob(t, id)
		scores[id] = map[string]int{p: 60}
	}
	addJob(t, "d")
	scores["d"] = map[string]int{p: 10}
	useFake(t, &fake{by: scores})
	first, _ := ApplyToExisting(context.Background(), p, false)
	second, err := ApplyToExisting(context.Background(), p, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || second.Total != 0 || second.OnBoard != 3 || second.Below != 1 || len(second.Statuses) != 0 {
		t.Errorf("first %+v, second %+v", first, second)
	}
	if entries(t) != 3 {
		t.Errorf("entries = %d", entries(t))
	}
}

// ---- other profiles, empty cases ----

func TestApplyNeverTouchesAnotherProfilesBoard(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	sa, sb := customStatus(t, a, "A status"), customStatus(t, b, "B status")
	rule(t, a, 50, sa)
	rule(t, b, 50, sb)
	addJob(t, "j1")
	if _, err := jobboard.SetEntry(b, "j1", sb); err != nil { // Ben has the job; Ana does not
		t.Fatal(err)
	}
	useFake(t, &fake{by: map[string]map[string]int{"j1": {a: 80, b: 80}}})
	sum, err := ApplyToExisting(context.Background(), a, false)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Total != 1 || sum.OnBoard != 0 {
		t.Errorf("Ana's summary = %+v: Ben's card says nothing about her board", sum)
	}
	if onBoard(t, a, "j1") != sa || onBoard(t, b, "j1") != sb || entries(t) != 2 {
		t.Errorf("Ana %q Ben %q entries %d", onBoard(t, a, "j1"), onBoard(t, b, "j1"), entries(t))
	}
}

func TestApplyOfAProfileWithoutRulesIsRefusedBeforeAnythingIsRead(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	addJob(t, "j1")
	built := useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 90}}})
	if _, err := ApplyToExisting(context.Background(), p, false); !errors.Is(err, ErrNoRules) {
		t.Errorf("err = %v, want ErrNoRules", err)
	}
	if _, err := ApplyToExisting(context.Background(), p, true); !errors.Is(err, ErrNoRules) {
		t.Errorf("dry run err = %v", err)
	}
	if len(*built) != 0 || entries(t) != 0 {
		t.Errorf("scorers built %d, entries %d", len(*built), entries(t))
	}
}

func TestApplyOfAnUnknownProfile(t *testing.T) {
	setup(t)
	built := useFake(t, &fake{})
	if _, err := ApplyToExisting(context.Background(), "nope", false); !errors.Is(err, profile.ErrUnknownProfile) {
		t.Errorf("err = %v", err)
	}
	if len(*built) != 0 {
		t.Error("nothing may be read for an unknown profile")
	}
}

func TestApplyWithNoJobsIsAnEmptySummary(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, customStatus(t, p, "S"))
	useFake(t, &fake{})
	sum, err := ApplyToExisting(context.Background(), p, false)
	if err != nil || sum.Jobs != 0 || sum.Total != 0 || sum.Statuses == nil || len(sum.Statuses) != 0 {
		t.Errorf("%+v %v", sum, err)
	}
}

func TestAScorerThatCannotBeBuiltFailsTheApplyAndAddsNothing(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, customStatus(t, p, "S"))
	addJob(t, "j1")
	old := newScorer
	newScorer = func(map[string]bool) (scorer, error) { return nil, errors.New("no profile") }
	t.Cleanup(func() { newScorer = old })
	if _, err := ApplyToExisting(context.Background(), p, false); err == nil || entries(t) != 0 {
		t.Errorf("err = %v, entries %d", err, entries(t))
	}
}

func TestTheScorerIsBuiltOnceAndOnlyForThisProfile(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	newProfile(t, "Ben")
	rule(t, p, 50, customStatus(t, p, "S"))
	for _, id := range []string{"a", "b", "c"} {
		addJob(t, id)
	}
	built := useFake(t, &fake{by: map[string]map[string]int{"a": {p: 60}, "b": {p: 60}, "c": {p: 60}}})
	if _, err := ApplyToExisting(context.Background(), p, false); err != nil {
		t.Fatal(err)
	}
	if len(*built) != 1 || len((*built)[0]) != 1 || !(*built)[0][p] {
		t.Errorf("built for %v, want once and only for the profile", *built)
	}
}

// ---- statuses in the summary ----

func TestAStatusUsedByTwoRulesIsOneEntryInTheOrderOfTheRules(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	x, y := customStatus(t, p, "X"), customStatus(t, p, "Y")
	rule(t, p, 90, x)
	rule(t, p, 70, y)
	rule(t, p, 50, x) // X again
	scores := map[string]map[string]int{}
	for i, s := range []int{95, 80, 60, 55, 75} {
		id := fmt.Sprintf("j%d", i)
		addJob(t, id)
		scores[id] = map[string]int{p: s}
	}
	useFake(t, &fake{by: scores})
	sum, err := ApplyToExisting(context.Background(), p, true)
	if err != nil {
		t.Fatal(err)
	}
	// 95 -> X (rule 90); 80 -> Y; 75 -> Y; 60 -> X (rule 50); 55 -> X
	if statusLine(sum) != "X=3 Y=2 " || sum.Total != 5 || len(sum.Statuses) != 2 {
		t.Errorf("statuses = %s (%+v)", statusLine(sum), sum)
	}
}

func TestOnlyStatusesWithAJobAreListed(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 90, customStatus(t, p, "Never reached"))
	used := customStatus(t, p, "Used")
	rule(t, p, 50, used)
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 60}}})
	sum, _ := ApplyToExisting(context.Background(), p, true)
	if len(sum.Statuses) != 1 || sum.Statuses[0].StatusID != used {
		t.Errorf("statuses = %+v", sum.Statuses)
	}
}

// ---- chunks, cancellation, failures ----

func TestApplyVisitsEveryJobInChunks(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, customStatus(t, p, "S"))
	scores := map[string]map[string]int{}
	var all []string
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		addJob(t, id)
		scores[id] = map[string]int{p: 70}
		all = append(all, id)
	}
	f := &fake{by: scores}
	useFake(t, f)
	old, oldHook := applyChunk, afterApplyChunk
	applyChunk = 2
	chunks := 0
	afterApplyChunk = func() { chunks++ }
	t.Cleanup(func() { applyChunk, afterApplyChunk = old, oldHook })

	sum, err := ApplyToExisting(context.Background(), p, false)
	if err != nil || sum.Jobs != 5 || sum.Total != 5 || chunks != 3 {
		t.Fatalf("%+v chunks %d err %v", sum, chunks, err)
	}
	if !reflect.DeepEqual(f.asked, all) {
		t.Errorf("jobs scored in the order %v, want %v (each once, by id)", f.asked, all)
	}
}

// A cancellation stops between chunks; what was added stays, and the summary says how far it got; a repeat finishes.
func TestACancelledApplyKeepsWhatWasAddedAndARepeatFinishes(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "S")
	rule(t, p, 50, s)
	scores := map[string]map[string]int{}
	for _, id := range []string{"a", "b", "c", "d"} {
		addJob(t, id)
		scores[id] = map[string]int{p: 70}
	}
	useFake(t, &fake{by: scores})
	old, oldHook := applyChunk, afterApplyChunk
	applyChunk = 2
	ctx, cancel := context.WithCancel(context.Background())
	afterApplyChunk = func() { cancel() }
	t.Cleanup(func() { applyChunk, afterApplyChunk = old, oldHook })

	sum, err := ApplyToExisting(ctx, p, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if sum.Jobs != 2 || sum.Total != 2 || entries(t) != 2 {
		t.Errorf("after the first chunk: %+v, entries %d", sum, entries(t))
	}
	afterApplyChunk = func() {}
	again, err := ApplyToExisting(context.Background(), p, false)
	if err != nil || again.Total != 2 || again.OnBoard != 2 || entries(t) != 4 {
		t.Errorf("the repeat: %+v %v entries %d", again, err, entries(t))
	}
}

func TestAJobDeletedBeforeItIsScoredOrBeforeItIsAddedIsSkippedAndNotCounted(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, customStatus(t, p, "S"))
	for _, id := range []string{"a", "gone-before", "gone-during", "d"} {
		addJob(t, id)
	}
	f := &fake{
		by: map[string]map[string]int{"a": {p: 70}, "gone-during": {p: 70}, "d": {p: 70}}, // gone-before is unknown to the scorer
		hook: func(id string) {
			if id == "gone-during" {
				exec(t, `DELETE FROM jobs WHERE id = 'gone-during'`) // deleted after the listing, while being scored
			}
		},
	}
	useFake(t, f)
	sum, err := ApplyToExisting(context.Background(), p, false)
	if err != nil {
		t.Fatalf("a deleted job must not fail the run: %v", err)
	}
	if sum.Jobs != 2 || sum.Total != 2 || entries(t) != 2 {
		t.Errorf("%+v entries %d, want only a and d", sum, entries(t))
	}
}

func TestAFailureMidRunReturnsWhatWasDoneAndKeepsIt(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "S")
	rule(t, p, 50, s)
	scores := map[string]map[string]int{}
	for _, id := range []string{"a", "b", "c"} {
		addJob(t, id)
		scores[id] = map[string]int{p: 70}
	}
	useFake(t, &fake{by: scores, errs: map[string]error{"c": errors.New("boom")}})
	sum, err := ApplyToExisting(context.Background(), p, false)
	if err == nil {
		t.Fatal("a scoring failure must be reported")
	}
	if sum.Jobs != 2 || sum.Total != 2 || entries(t) != 2 {
		t.Errorf("summary %+v entries %d: a and b were done and stay", sum, entries(t))
	}
}

// A rule whose status is deleted while the run is on (the status goes, the rule with it) fails the add with the status
// error; the jobs added before it stay.
func TestAStoreFailureMidRunIsReported(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	s := customStatus(t, p, "S")
	rule(t, p, 50, s)
	for _, id := range []string{"a", "b", "c", "d"} {
		addJob(t, id)
	}
	f := &fake{
		by: map[string]map[string]int{"a": {p: 70}, "b": {p: 70}, "c": {p: 70}, "d": {p: 70}},
		hook: func(id string) {
			if id == "c" {
				if err := jobboard.DeleteStatus(s); err != nil { // the rule goes with the status, but the pass holds its copy
					t.Error(err)
				}
			}
		},
	}
	useFake(t, f)
	sum, err := ApplyToExisting(context.Background(), p, false)
	if !errors.Is(err, jobboard.ErrUnknownStatus) {
		t.Fatalf("err = %v, want ErrUnknownStatus", err)
	}
	if sum.Total != 2 || entries(t) != 2 {
		t.Errorf("summary %+v entries %d: a and b were added before the failure", sum, entries(t))
	}
}

// ---- no personas, the real scorer ----

func TestAProfileWithoutPersonasJudgesNothingAndAddsNothing(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana") // no persona
	rule(t, p, 1, customStatus(t, p, "S"))
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES ('j2', 'https://x.example/j2', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, jobDesc)
	sum, err := ApplyToExisting(context.Background(), p, false)
	if err != nil || sum.Jobs != 2 || sum.NotJudged != 2 || sum.Total != 0 || entries(t) != 0 {
		t.Errorf("%+v %v entries %d", sum, err, entries(t))
	}
}

// With the real scorer: the fitting profile gets the existing jobs, the other (below the rule) none, and nothing else moves.
func TestTheRealScorerAppliesTheRulesToTheExistingJobsOfTheFittingProfileOnly(t *testing.T) {
	setup(t)
	fit := seedProfile(t, "Fit", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other := seedProfile(t, "Other", []string{"Cobol", "Fortran"})
	sFit, sOther := customStatus(t, fit, "To check"), customStatus(t, other, "To check")
	rule(t, fit, 60, sFit)
	rule(t, other, 60, sOther)
	for _, id := range []string{"j1", "j2", "j3"} {
		exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, id, "https://x.example/"+id, jobDesc)
	}
	// the profile is told about the three jobs it has in common with its rule
	preview, err := ApplyToExisting(context.Background(), fit, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Jobs != 3 || preview.Total != 3 || len(preview.Statuses) != 1 || preview.Statuses[0].StatusID != sFit {
		t.Fatalf("preview = %+v", preview)
	}
	if entries(t) != 0 {
		t.Fatal("the preview wrote")
	}
	sum, err := ApplyToExisting(context.Background(), fit, false)
	if err != nil || sum.Total != 3 {
		t.Fatalf("%+v %v", sum, err)
	}
	for _, id := range []string{"j1", "j2", "j3"} {
		if onBoard(t, fit, id) != sFit || onBoard(t, other, id) != "" {
			t.Errorf("%s: Fit %q Other %q", id, onBoard(t, fit, id), onBoard(t, other, id))
		}
	}
	// the other profile's own apply finds its jobs below its rule
	otherSum, err := ApplyToExisting(context.Background(), other, false)
	if err != nil || otherSum.Below != 3 || otherSum.Total != 0 {
		t.Errorf("%+v %v", otherSum, err)
	}
}

func TestAJobListingThatFailsFailsTheApply(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, customStatus(t, p, "S"))
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 80}}})
	old := listJobIDs
	listJobIDs = func(string, int) ([]string, error) { return nil, errors.New("jobs.db gone") }
	t.Cleanup(func() { listJobIDs = old })
	if _, err := ApplyToExisting(context.Background(), p, false); err == nil || entries(t) != 0 {
		t.Errorf("err = %v, entries %d", err, entries(t))
	}
}

func TestADryRunThatCannotReadTheBoardFails(t *testing.T) {
	setup(t)
	p := newProfile(t, "Ana")
	rule(t, p, 50, customStatus(t, p, "S"))
	addJob(t, "j1")
	useFake(t, &fake{by: map[string]map[string]int{"j1": {p: 80}}})
	old := isOnBoard
	isOnBoard = func(string, string) (bool, error) { return false, errors.New("jobs.db gone") }
	t.Cleanup(func() { isOnBoard = old })
	if _, err := ApplyToExisting(context.Background(), p, true); err == nil {
		t.Error("a dry run that cannot tell whether a job is on the board must fail")
	}
}

// In a dry run too, another profile's card says nothing about this profile's board.
func TestADryRunIgnoresAnotherProfilesCard(t *testing.T) {
	setup(t)
	a, b := newProfile(t, "Ana"), newProfile(t, "Ben")
	sa := customStatus(t, a, "A status")
	rule(t, a, 50, sa)
	addJob(t, "j1")
	if _, err := jobboard.SetEntry(b, "j1", jobboard.AppliedID); err != nil {
		t.Fatal(err)
	}
	useFake(t, &fake{by: map[string]map[string]int{"j1": {a: 80}}})
	sum, err := ApplyToExisting(context.Background(), a, true)
	if err != nil || sum.OnBoard != 0 || sum.Total != 1 {
		t.Errorf("%+v %v: Ben's card must not count as Ana's", sum, err)
	}
}
