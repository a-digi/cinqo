package profiledeletedconsumer

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
	"career-tool-backend/rematchworker"
)

func send(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	EventHandler(rec, httptest.NewRequest(method, "/events/profile-deleted-matches", strings.NewReader(body)))
	return rec
}

func event(profileID string) string {
	return fmt.Sprintf(`{"id":"e1","topic":"career.profile.deleted","payload":{"profile_id":%q}}`, profileID)
}

// calls replaces what the handler uses with recorders; order is the sequence of the calls.
type calls struct {
	order    []string
	purged   []string
	handed   [][]string
	freed    []string
	purgeErr error
}

func watch(t *testing.T, freed []string, purgeErr error) *calls {
	t.Helper()
	c := &calls{freed: freed, purgeErr: purgeErr}
	op, of, oh := purge, forget, handOver
	purge = func(id string) ([]string, error) {
		c.order = append(c.order, "purge")
		c.purged = append(c.purged, id)
		return c.freed, c.purgeErr
	}
	forget = func(id string) bool { c.order = append(c.order, "forget"); return true }
	handOver = func(ids []string) int {
		c.order = append(c.order, "handover")
		c.handed = append(c.handed, ids)
		return len(ids)
	}
	t.Cleanup(func() { purge, forget, handOver = op, of, oh })
	return c
}

func TestAProfileDeletedEventRemovesItsMatchesAndHandsTheJobsOver(t *testing.T) {
	c := watch(t, []string{"j1", "j2"}, nil)
	if rec := send(http.MethodPost, event("prof")); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if !slices.Equal(c.purged, []string{"prof"}) {
		t.Errorf("purged %v", c.purged)
	}
	if len(c.handed) != 1 || !slices.Equal(c.handed[0], []string{"j1", "j2"}) {
		t.Errorf("handed over %v", c.handed)
	}
	// forgotten from the waiting set first, then purged, then the jobs handed over
	if !slices.Equal(c.order, []string{"forget", "purge", "handover"}) {
		t.Errorf("order %v", c.order)
	}
}

func TestNothingIsHandedOverWhenNothingWasFreed(t *testing.T) {
	c := watch(t, nil, nil)
	if rec := send(http.MethodPost, event("prof")); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if len(c.handed) != 0 || !slices.Equal(c.purged, []string{"prof"}) {
		t.Errorf("handed %v, purged %v", c.handed, c.purged)
	}
}

// A failed removal answers 500 (core retries) and hands nothing over; the retry then succeeds.
func TestAFailedRemovalAnswers500AndHandsNothingOver(t *testing.T) {
	c := watch(t, []string{"j1"}, errors.New("db gone"))
	if rec := send(http.MethodPost, event("prof")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
	if len(c.handed) != 0 {
		t.Errorf("handed over %v after a failure", c.handed)
	}
	c.purgeErr = nil
	if rec := send(http.MethodPost, event("prof")); rec.Code != http.StatusOK || len(c.handed) != 1 {
		t.Errorf("the redelivery: %d, handed %v", rec.Code, c.handed)
	}
}

func TestTheRouteRefusesWhatItCannotUseAndRemovesNothing(t *testing.T) {
	c := watch(t, []string{"j1"}, nil)
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":              {http.MethodGet, "", http.StatusMethodNotAllowed},
		"PUT":              {http.MethodPut, event("p"), http.StatusMethodNotAllowed},
		"DELETE":           {http.MethodDelete, event("p"), http.StatusMethodNotAllowed},
		"not json":         {http.MethodPost, "nope", http.StatusBadRequest},
		"empty body":       {http.MethodPost, "", http.StatusBadRequest},
		"no payload":       {http.MethodPost, `{"topic":"t"}`, http.StatusBadRequest},
		"payload not json": {http.MethodPost, `{"payload":"x"}`, http.StatusBadRequest},
		"empty id":         {http.MethodPost, `{"payload":{"profile_id":""}}`, http.StatusBadRequest},
		"missing id":       {http.MethodPost, `{"payload":{"x":"y"}}`, http.StatusBadRequest},
		"a persona event":  {http.MethodPost, `{"payload":{"persona_id":"p"}}`, http.StatusBadRequest},
		"id is a number":   {http.MethodPost, `{"payload":{"profile_id":5}}`, http.StatusBadRequest},
		"overlong id":      {http.MethodPost, fmt.Sprintf(`{"payload":{"profile_id":%q}}`, strings.Repeat("x", 201)), http.StatusBadRequest},
	} {
		if rec := send(tc.method, tc.body); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, tc.want)
		}
	}
	if len(c.order) != 0 {
		t.Errorf("refused events caused calls: %v", c.order)
	}
	if rec := send(http.MethodPost, fmt.Sprintf(`{"payload":{"profile_id":%q}}`, strings.Repeat("x", 200))); rec.Code != http.StatusOK {
		t.Errorf("an id of exactly 200 characters: %d", rec.Code)
	}
}

// Only profile_id of the payload is used.
func TestOnlyTheProfileIDIsUsed(t *testing.T) {
	c := watch(t, nil, nil)
	send(http.MethodPost, `{"payload":{"profile_id":"prof","persona_id":"per","extra":[1,2]}}`)
	if !slices.Equal(c.purged, []string{"prof"}) {
		t.Errorf("purged %v", c.purged)
	}
}

// The real defaults are the ones that are meant: the package's own seams point at the real functions.
func TestTheDefaultsAreTheRealFunctions(t *testing.T) {
	// A waiting profile is forgotten by the real forget, and a repeated delivery finds nothing to purge.
	rematchworker.Default.Note("pending-profile-deleted-test")
	before := rematchworker.Default.Waiting()
	setupDB(t)
	if rec := send(http.MethodPost, event("pending-profile-deleted-test")); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if rematchworker.Default.Waiting() != before-1 {
		t.Error("the profile is still waiting for a re-match")
	}
}

func setupDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
}

func seedProfile(t *testing.T, name string, skills []string) string {
	t.Helper()
	pid, err := profile.CreateProfile(name, "x")
	if err != nil {
		t.Fatal(err)
	}
	per, err := persona.CreatePersona(pid, name+" persona", "")
	if err != nil {
		t.Fatal(err)
	}
	h, d, l := "Backend Engineer", "Software Engineer, Backend Developer", "Hamburg"
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: per, Headline: &h, DesiredTitles: &d, Location: &l, DesiredLocations: &l}); err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if err := persona.AddCareerSkill(per, s); err != nil {
			t.Fatal(err)
		}
	}
	return pid
}

func rows(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The whole chain with real tables and the real engine: a profile's matches are zombies after it is deleted; the event removes them
// (and only them), the freed jobs go to the job-match worker's hand-over, and matching them again gives the profile that remains.
func TestTheEventRemovesTheZombieMatchesOfADeletedProfile(t *testing.T) {
	setupDB(t)
	desc := "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, id, "https://x.example/"+id, desc); err != nil {
			t.Fatal(err)
		}
	}
	gone := seedProfile(t, "Gone", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	kept := seedProfile(t, "Kept", []string{"Go"})
	for _, p := range []string{gone, kept} {
		if _, err := jobs.RematchProfile(p); err != nil {
			t.Fatal(err)
		}
	}
	if rows(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = ?`, gone) != 2 {
		t.Fatal("setup: the profile that fits best should hold both matches")
	}
	// the persona scores of both profiles (the persona consumer makes them; seeded directly)
	var gonePersona, keptPersona string
	_ = db.CareerDB.QueryRow(`SELECT id FROM personas WHERE profile_id = ?`, gone).Scan(&gonePersona)
	_ = db.CareerDB.QueryRow(`SELECT id FROM personas WHERE profile_id = ?`, kept).Scan(&keptPersona)
	for _, j := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, ?, ?, 40)`, j, keptPersona, kept); err != nil {
			t.Fatal(err)
		}
	}
	if err := profile.DeleteProfile(gone); err != nil {
		t.Fatal(err)
	}
	// scores that appear for the deleted profile after its direct cleanup (a run that was going, a lost cleanup): the consumer removes them
	for _, j := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, ?, ?, 40)`, j, gonePersona, gone); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.JobsDB.Exec(`INSERT INTO profile_match_state (profile_id, fingerprint) VALUES (?, 'x') ON CONFLICT(profile_id) DO NOTHING`, gone); err != nil {
		t.Fatal(err)
	}
	if rows(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = ?`, gone) != 2 {
		t.Fatal("the zombies should still be there before the event")
	}
	var handed []string
	oh := handOver
	handOver = func(ids []string) int { handed = append(handed, ids...); return len(ids) }
	defer func() { handOver = oh }()

	if rec := send(http.MethodPost, event(gone)); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if n := rows(t, `SELECT COUNT(*) FROM job_matches WHERE profile_id = ?`, gone); n != 0 {
		t.Errorf("%d zombie matches left", n)
	}
	if rows(t, `SELECT COUNT(*) FROM job_persona_matches WHERE profile_id = ?`, gone) != 0 || rows(t, `SELECT COUNT(*) FROM profile_match_state WHERE profile_id = ?`, gone) != 0 {
		t.Error("scores or state of the deleted profile are left")
	}
	if rows(t, `SELECT COUNT(*) FROM job_persona_matches WHERE profile_id = ?`, kept) != 2 || rows(t, `SELECT COUNT(*) FROM profile_match_state WHERE profile_id = ?`, kept) != 1 {
		t.Error("the remaining profile's scores or state were touched")
	}
	if !slices.Equal(handed, []string{"j1", "j2"}) {
		t.Fatalf("handed over %v", handed)
	}
	out, err := jobs.AutoMatchJobs(handed)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		if o.Status != jobs.AutoMatched || o.ProfileID != kept {
			t.Errorf("%+v, want matched by %s", o, kept)
		}
	}
	// a redelivery finds nothing and still answers 200
	handed = nil
	if rec := send(http.MethodPost, event(gone)); rec.Code != http.StatusOK || len(handed) != 0 {
		t.Errorf("redelivery: %d, handed %v", rec.Code, handed)
	}
}
