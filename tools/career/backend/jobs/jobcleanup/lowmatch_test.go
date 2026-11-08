package jobcleanup

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

// fakeScorer answers from a table; a job missing from it is "unknown job", and calls records what was scored.
type fakeScorer struct {
	scores map[string][]int
	err    error
	calls  []string
}

func (f *fakeScorer) Scores(id string) ([]int, error) {
	f.calls = append(f.calls, id)
	if f.err != nil {
		return nil, f.err
	}
	s, ok := f.scores[id]
	if !ok {
		return nil, jobs.ErrUnknownJob
	}
	return s, nil
}

func useScorer(t *testing.T, f *fakeScorer) {
	t.Helper()
	old := newScorer
	newScorer = func() (Scorer, error) { return f, nil }
	t.Cleanup(func() { newScorer = old })
}

func low(min int) Options {
	return Options{Rule: RuleLowMatch, OlderThanDays: 90, MinMatchPercent: min}
}

func ids(list []Sample) map[string]bool {
	m := map[string]bool{}
	for _, s := range list {
		m[s.ID] = true
	}
	return m
}

// A job goes only when EVERY profile that judged it scored it under the limit; one good profile saves it, and
// "under" is strict.
func TestALowMatchNeedsEveryProfileUnderTheLimit(t *testing.T) {
	setup(t)
	fixedNow(t)
	for _, id := range []string{"allLow", "oneHigh", "exactly", "justUnder", "single", "highSingle"} {
		addJob(t, id, young, "crawled")
	}
	useScorer(t, &fakeScorer{scores: map[string][]int{
		"allLow": {20, 59, 10}, "oneHigh": {20, 85, 10}, "exactly": {60, 10}, "justUnder": {59, 59},
		"single": {5}, "highSingle": {95},
	}})
	p, err := PreviewCleanup(low(60))
	if err != nil {
		t.Fatal(err)
	}
	got := ids(p.Sample)
	if p.WouldDelete != 3 || !got["allLow"] || !got["justUnder"] || !got["single"] {
		t.Fatalf("would delete %d: %v", p.WouldDelete, got)
	}
	if p.ByRule != (ByRule{LowMatch: 3}) {
		t.Errorf("by rule %+v", p.ByRule)
	}
	best := map[string]int{}
	for _, s := range p.Sample {
		if s.BestPercent == nil {
			t.Fatalf("%s has no bestPercent", s.ID)
		}
		best[s.ID] = *s.BestPercent
	}
	if best["allLow"] != 59 || best["justUnder"] != 59 || best["single"] != 5 {
		t.Errorf("best percents %v", best)
	}
	res, err := RunCleanup(low(60))
	if err != nil || res.Deleted != 3 || res.ByRule != (ByRule{LowMatch: 3}) {
		t.Fatalf("run: %+v, %v", res, err)
	}
	for id, want := range map[string]bool{"allLow": false, "justUnder": false, "single": false, "oneHigh": true, "exactly": true, "highSingle": true} {
		if exists(t, id) != want {
			t.Errorf("%s exists = %v, want %v", id, !want, want)
		}
	}
	if s, _ := GetSettings(); s.LastRunSummary != "under 60% for every profile: 3 deleted" {
		t.Errorf("summary %q", s.LastRunSummary)
	}
}

// The percentage moves the line: the same scores, another limit.
func TestTheLimitMovesTheLine(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", young, "crawled")
	useScorer(t, &fakeScorer{scores: map[string][]int{"a": {40, 55}}})
	for min, want := range map[int]int{1: 0, 55: 0, 56: 1, 100: 1} {
		p, _ := PreviewCleanup(low(min))
		if p.WouldDelete != want {
			t.Errorf("limit %d: would delete %d, want %d", min, p.WouldDelete, want)
		}
	}
}

// A job nobody could judge is kept and counted, never deleted: no profile with personas, or too little data.
func TestAJobNobodyCouldJudgeIsKept(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "unknown1", young, "crawled")
	addJob(t, "unknown2", young, "crawled")
	addJob(t, "bad", young, "crawled")
	useScorer(t, &fakeScorer{scores: map[string][]int{"unknown1": {}, "unknown2": nil, "bad": {3}}})
	p, _ := PreviewCleanup(low(60))
	if p.WouldDelete != 1 || !ids(p.Sample)["bad"] || p.Kept.Unjudged != 2 {
		t.Errorf("%+v", p)
	}
	if res, _ := RunCleanup(low(60)); res.Deleted != 1 || !exists(t, "unknown1") || !exists(t, "unknown2") {
		t.Errorf("run %+v", res)
	}
}

// With no profile at all (the real scorer) the rule deletes nothing and everything counts as unjudged.
func TestWithoutProfilesTheRuleDoesNothing(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", young, "crawled")
	addJob(t, "b", old30, "crawled")
	p, err := PreviewCleanup(low(100))
	if err != nil || p.WouldDelete != 0 || p.Kept.Unjudged != 2 {
		t.Fatalf("%+v, %v", p, err)
	}
	if res, _ := RunCleanup(low(100)); res.Deleted != 0 || !exists(t, "a") || !exists(t, "b") {
		t.Errorf("%+v", res)
	}
}

// The safety rules apply to a low scorer like to an old job, with the same order and counts; a protected job
// that scores well is not counted at all.
func TestSafetyRulesKeepLowScorers(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "free", young, "crawled")
	addJob(t, "board", young, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'board', 'fixed-applied')`)
	addJob(t, "cv", young, "crawled")
	exec(t, `INSERT INTO job_cv_pdfs (job_id, profile_id) VALUES ('cv', 'p')`)
	addJob(t, "progress", young, "crawled")
	exec(t, `INSERT INTO job_matches (job_id, profile_id, status) VALUES ('progress', 'p', 'matching')`)
	addJob(t, "manual", young, "manual")
	addJob(t, "boardGood", young, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'boardGood', 'fixed-applied')`)
	useScorer(t, &fakeScorer{scores: map[string][]int{"free": {10}, "board": {10}, "cv": {10}, "progress": {10}, "manual": {10}, "boardGood": {90}}})
	p, _ := PreviewCleanup(low(60))
	if p.WouldDelete != 1 || !ids(p.Sample)["free"] || p.Kept != (Kept{OnBoard: 1, HasCv: 1, InProgress: 1, Manual: 1}) {
		t.Fatalf("%+v", p)
	}
	if res, _ := RunCleanup(low(60)); res.Deleted != 1 {
		t.Fatalf("%+v", res)
	}
	for _, id := range []string{"board", "cv", "progress", "manual", "boardGood"} {
		if !exists(t, id) {
			t.Errorf("%s was deleted", id)
		}
	}
	// included manual jobs go
	if res, _ := RunCleanup(Options{Rule: RuleLowMatch, OlderThanDays: 90, MinMatchPercent: 60, IncludeManual: true}); res.Deleted != 1 || exists(t, "manual") {
		t.Errorf("manual included: %+v", res)
	}
}

// "both": a job is picked by age or by score; an old job is not scored; a job both rules would pick counts as old,
// so the two counts add up; the summary names both parts.
func TestBothRulesTogether(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "oldBad", old30, "crawled")
	addJob(t, "oldGood", old30, "crawled")
	addJob(t, "youngBad", young, "crawled")
	addJob(t, "youngGood", young, "crawled")
	f := &fakeScorer{scores: map[string][]int{"oldBad": {1}, "oldGood": {99}, "youngBad": {1}, "youngGood": {99}}}
	useScorer(t, f)
	o := Options{Rule: RuleBoth, OlderThanDays: 30, MinMatchPercent: 60}
	p, _ := PreviewCleanup(o)
	if p.WouldDelete != 3 || p.ByRule != (ByRule{Older: 2, LowMatch: 1}) {
		t.Fatalf("%+v", p)
	}
	for _, c := range f.calls {
		if strings.HasPrefix(c, "old") {
			t.Errorf("an old job was scored: %s", c)
		}
	}
	for _, s := range p.Sample {
		if (s.ID == "youngBad") != (s.BestPercent != nil) {
			t.Errorf("%s bestPercent %v", s.ID, s.BestPercent)
		}
	}
	res, _ := RunCleanup(o)
	if res.Deleted != 3 || res.ByRule != (ByRule{Older: 2, LowMatch: 1}) || !exists(t, "youngGood") {
		t.Fatalf("%+v", res)
	}
	if s, _ := GetSettings(); s.LastRunSummary != "older than 30 days or under 60% for every profile: 3 deleted (2 old, 1 low match)" {
		t.Errorf("summary %q", s.LastRunSummary)
	}
}

// The older rule alone never builds or calls the scorer; the low-match rule alone ignores a job's age.
func TestTheRulesStayApart(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "oldGood", old30, "crawled")
	addJob(t, "youngBad", young, "crawled")
	built := 0
	f := &fakeScorer{scores: map[string][]int{"oldGood": {99}, "youngBad": {1}}}
	old := newScorer
	newScorer = func() (Scorer, error) { built++; return f, nil }
	defer func() { newScorer = old }()

	if p, _ := PreviewCleanup(opts(30, false)); p.WouldDelete != 1 || !ids(p.Sample)["oldGood"] || built != 0 || len(f.calls) != 0 {
		t.Errorf("older only: %+v, built %d, calls %v", p, built, f.calls)
	}
	if p, _ := PreviewCleanup(low(60)); p.WouldDelete != 1 || !ids(p.Sample)["youngBad"] || built != 1 {
		t.Errorf("low only: %+v, built %d", p, built)
	}
}

// A scorer failure stops everything before any delete; a job deleted while the list was being scored is skipped.
func TestAScorerFailureDeletesNothing(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", young, "crawled")
	addJob(t, "old", old30, "crawled")
	useScorer(t, &fakeScorer{err: errors.New("boom")})
	o := Options{Rule: RuleBoth, OlderThanDays: 30, MinMatchPercent: 60}
	if _, err := PreviewCleanup(o); err == nil {
		t.Error("preview hid the failure")
	}
	if _, err := RunCleanup(o); err == nil || !exists(t, "old") || !exists(t, "a") {
		t.Errorf("run err %v", err)
	}
	if s, _ := GetSettings(); !strings.HasPrefix(s.LastRunSummary, "failed: ") {
		t.Errorf("summary %q", s.LastRunSummary)
	}
	oldNew := newScorer
	newScorer = func() (Scorer, error) { return nil, errors.New("no scorer") }
	defer func() { newScorer = oldNew }()
	if _, err := RunCleanup(low(60)); err == nil || !exists(t, "a") {
		t.Errorf("constructor failure: %v", err)
	}
}

func TestAJobGoneMidScoringIsSkipped(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", young, "crawled")
	addJob(t, "b", young, "crawled")
	useScorer(t, &fakeScorer{scores: map[string][]int{"b": {1}}}) // "a" is unknown to the scorer
	p, err := PreviewCleanup(low(60))
	if err != nil || p.WouldDelete != 1 || !ids(p.Sample)["b"] {
		t.Errorf("%+v, %v", p, err)
	}
}

// A job picked by its score is deleted by the guarded statement without an age check (it is young), but every
// safety rule still applies at that moment.
func TestTheGuardedDeleteOfALowScorerStillRechecksSafety(t *testing.T) {
	setup(t)
	fixedNow(t)
	cutoff := low(60).cutoff()
	addJob(t, "x", young, "crawled")
	if done, err := deleteBatch([]Sample{{ID: "x", reason: RuleOlder}}, cutoff, false); err != nil || len(done) != 0 {
		t.Fatalf("a young job picked as old was deleted: %v, %v", done, err)
	}
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'x', 'fixed-applied')`)
	if done, err := deleteBatch([]Sample{{ID: "x", reason: RuleLowMatch}}, cutoff, false); err != nil || len(done) != 0 || !exists(t, "x") {
		t.Fatalf("board ignored: %v, %v", done, err)
	}
	exec(t, `DELETE FROM job_board_entries`)
	if done, err := deleteBatch([]Sample{{ID: "x", reason: RuleLowMatch}}, cutoff, false); err != nil || len(done) != 1 || exists(t, "x") {
		t.Fatalf("a low scorer was not deleted: %v, %v", done, err)
	}
}

func TestMinMatchPercentOverrideInTheBody(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "a", young, "crawled")
	useScorer(t, &fakeScorer{scores: map[string][]int{"a": {50}}})
	exec(t, `UPDATE job_cleanup_settings SET min_match_percent = 60, updated_at = '2026-06-01 00:00:00'`)
	// saved limit 60 would pick it; the body says 40
	if _, body := post(PreviewHandler, `{"rule":"low_match"}`); !strings.Contains(body, `"wouldDelete":1`) || !strings.Contains(body, `"bestPercent":50`) {
		t.Errorf("saved 60: %s", body)
	}
	if _, body := post(PreviewHandler, `{"rule":"low_match","minMatchPercent":40}`); !strings.Contains(body, `"wouldDelete":0`) {
		t.Errorf("override 40: %s", body)
	}
	exec(t, `UPDATE job_cleanup_settings SET min_match_percent = 30`)
	if _, body := post(PreviewHandler, `{"rule":"low_match"}`); !strings.Contains(body, `"wouldDelete":0`) {
		t.Errorf("saved 30: %s", body)
	}
	if code, _ := post(PreviewHandler, `{"rule":"low_match","minMatchPercent":101}`); code != 400 {
		t.Errorf("101: %d", code)
	}
	// an older-only sample carries no bestPercent
	if _, body := post(PreviewHandler, `{"rule":"older","olderThanDays":3650}`); strings.Contains(body, "bestPercent") {
		t.Errorf("older-only: %s", body)
	}
}

// End to end with the real engine and two real profiles: the job the first profile fits is kept even though the
// second scores it low; a job neither fits is deleted.
func TestLowMatchWithTheRealEngineAndTwoProfiles(t *testing.T) {
	setup(t)
	fixedNow(t)
	seed := func(first string, skills ...string) {
		pid, err := profile.CreateProfile(first, "Test")
		if err != nil {
			t.Fatal(err)
		}
		per, err := persona.CreatePersona(pid, first+" persona", "")
		if err != nil {
			t.Fatal(err)
		}
		h, titles, loc := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
		if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &titles, Location: &loc, DesiredLocations: &loc}); err != nil {
			t.Fatal(err)
		}
		for _, s := range skills {
			if err := persona.AddCareerSkill(per, s); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed("Fit", "Go", "Docker", "Kubernetes", "PostgreSQL")
	seed("Other", "Cobol", "Fortran")
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description, crawled_at) VALUES
		('fits', 'u1', 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', 'Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung.', ?),
		('nobody', 'u2', 'Gärtner', 'Acme', 'Hamburg', 'Pflege von Grünanlagen, Rasen mähen und Bäume schneiden im Stadtgebiet. Führerschein Klasse B erforderlich.', ?)`, young, young)
	p, err := PreviewCleanup(low(60))
	if err != nil || p.WouldDelete != 1 || !ids(p.Sample)["nobody"] || p.Kept.Unjudged != 0 {
		t.Fatalf("%+v, %v", p, err)
	}
	if res, err := RunCleanup(low(60)); err != nil || res.Deleted != 1 || !exists(t, "fits") || exists(t, "nobody") {
		t.Errorf("%+v, %v", res, err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 0 {
		t.Error("the cleanup stored a match")
	}
}

func memoryRows(t *testing.T) map[string]string {
	t.Helper()
	r, err := db.DeletedJobsDB.Query(`SELECT source_url, reason FROM deleted_jobs`)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := map[string]string{}
	for r.Next() {
		var u, reason string
		_ = r.Scan(&u, &reason)
		out[u] = reason
	}
	return out
}

// The cleanup remembers what the low-match rule deletes (not what the age rule deletes, not what is kept), so the next
// crawl does not bring a deleted job back.
func TestTheLowMatchRuleRemembersWhatItDeletesAndTheAgeRuleDoesNot(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "lowYoung", young, "crawled")
	addJob(t, "goodYoung", young, "crawled")
	addJob(t, "oldGood", old30, "crawled")
	addJob(t, "lowOnBoard", young, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'lowOnBoard', 'fixed-applied')`)
	addJob(t, "lowManual", young, "manual")
	useScorer(t, &fakeScorer{scores: map[string][]int{"lowYoung": {5}, "goodYoung": {95}, "oldGood": {95}, "lowOnBoard": {5}, "lowManual": {5}}})

	res, err := RunCleanup(Options{Rule: RuleBoth, OlderThanDays: 30, MinMatchPercent: 60})
	if err != nil || res.Deleted != 2 || res.ByRule != (ByRule{Older: 1, LowMatch: 1}) {
		t.Fatalf("%+v, %v", res, err)
	}
	got := memoryRows(t)
	if len(got) != 1 || got["https://x.example/lowYoung"] != "low_match" {
		t.Errorf("remembered %v: only the low-match deletion belongs there", got)
	}
}

// A job the guarded delete holds back (it gained a board entry after the list was made) is not deleted and its entry is
// taken back; a job that is deleted keeps its entry. Both in one batch.
func TestAnEntryIsTakenBackForAJobTheGuardHeldBack(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "held", young, "crawled")
	addJob(t, "goes", young, "crawled")
	exec(t, `INSERT INTO job_board_entries (profile_id, job_id, status_id) VALUES ('p', 'held', 'fixed-applied')`)
	done, err := deleteBatch([]Sample{{ID: "held", reason: RuleLowMatch}, {ID: "goes", reason: RuleLowMatch}}, low(60).cutoff(), false)
	if err != nil || len(done) != 1 || done[0].ID != "goes" {
		t.Fatalf("%v, %v", done, err)
	}
	got := memoryRows(t)
	if len(got) != 1 || got["https://x.example/goes"] != "low_match" {
		t.Errorf("remembered %v", got)
	}
	if !exists(t, "held") || exists(t, "goes") {
		t.Error("the wrong job was deleted")
	}
}

// With the memory unavailable the cleanup still deletes.
func TestTheCleanupStillDeletesWithoutTheMemory(t *testing.T) {
	setup(t)
	fixedNow(t)
	addJob(t, "low", young, "crawled")
	useScorer(t, &fakeScorer{scores: map[string][]int{"low": {5}}})
	saved := db.DeletedJobsDB
	db.DeletedJobsDB = nil
	defer func() { db.DeletedJobsDB = saved }()
	if res, err := RunCleanup(low(60)); err != nil || res.Deleted != 1 || exists(t, "low") {
		t.Errorf("%+v, %v", res, err)
	}
}
