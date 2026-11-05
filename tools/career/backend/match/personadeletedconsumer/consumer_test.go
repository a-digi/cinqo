package personadeletedconsumer

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/match"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func send(method, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	EventHandler(rec, httptest.NewRequest(method, "/events/persona-deleted-matches", strings.NewReader(body)))
	return rec
}

func event(personaID, profileID string) string {
	return fmt.Sprintf(`{"id":"e1","topic":"career.persona.deleted","payload":{"persona_id":%q,"profile_id":%q}}`, personaID, profileID)
}

type calls struct {
	purged   [][2]string
	handed   [][]string
	freed    []string
	purgeErr error
}

func watch(t *testing.T, freed []string, purgeErr error) *calls {
	t.Helper()
	c := &calls{freed: freed, purgeErr: purgeErr}
	op, oh := purge, handOver
	purge = func(personaID, profileID string) ([]string, error) {
		c.purged = append(c.purged, [2]string{personaID, profileID})
		return c.freed, c.purgeErr
	}
	handOver = func(ids []string) int { c.handed = append(c.handed, ids); return len(ids) }
	t.Cleanup(func() { purge, handOver = op, oh })
	return c
}

func TestAPersonaDeletedEventRemovesItsMatchesAndHandsTheJobsOver(t *testing.T) {
	c := watch(t, []string{"j1", "j2"}, nil)
	if rec := send(http.MethodPost, event("per", "prof")); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if len(c.purged) != 1 || c.purged[0] != [2]string{"per", "prof"} {
		t.Errorf("purged %v (persona first, then its profile)", c.purged)
	}
	if len(c.handed) != 1 || !slices.Equal(c.handed[0], []string{"j1", "j2"}) {
		t.Errorf("handed over %v", c.handed)
	}
}

func TestNothingIsHandedOverWhenNothingWasFreed(t *testing.T) {
	c := watch(t, nil, nil)
	if rec := send(http.MethodPost, event("per", "prof")); rec.Code != http.StatusOK || len(c.handed) != 0 {
		t.Errorf("status %d, handed %v", rec.Code, c.handed)
	}
}

func TestAFailedRemovalAnswers500AndHandsNothingOver(t *testing.T) {
	c := watch(t, []string{"j1"}, errors.New("db gone"))
	if rec := send(http.MethodPost, event("per", "prof")); rec.Code != http.StatusInternalServerError || len(c.handed) != 0 {
		t.Fatalf("status %d, handed %v", rec.Code, c.handed)
	}
	c.purgeErr = nil
	if rec := send(http.MethodPost, event("per", "prof")); rec.Code != http.StatusOK || len(c.handed) != 1 {
		t.Errorf("the redelivery: %d, handed %v", rec.Code, c.handed)
	}
}

func TestTheRouteRefusesWhatItCannotUseAndRemovesNothing(t *testing.T) {
	c := watch(t, []string{"j1"}, nil)
	long := strings.Repeat("x", 201)
	for name, tc := range map[string]struct {
		method, body string
		want         int
	}{
		"GET":               {http.MethodGet, "", http.StatusMethodNotAllowed},
		"DELETE":            {http.MethodDelete, event("a", "b"), http.StatusMethodNotAllowed},
		"not json":          {http.MethodPost, "nope", http.StatusBadRequest},
		"empty body":        {http.MethodPost, "", http.StatusBadRequest},
		"no payload":        {http.MethodPost, `{"topic":"t"}`, http.StatusBadRequest},
		"empty persona id":  {http.MethodPost, event("", "prof"), http.StatusBadRequest},
		"empty profile id":  {http.MethodPost, event("per", ""), http.StatusBadRequest},
		"only a persona id": {http.MethodPost, `{"payload":{"persona_id":"p"}}`, http.StatusBadRequest},
		"only a profile id": {http.MethodPost, `{"payload":{"profile_id":"p"}}`, http.StatusBadRequest},
		"persona is number": {http.MethodPost, `{"payload":{"persona_id":5,"profile_id":"p"}}`, http.StatusBadRequest},
		"profile is number": {http.MethodPost, `{"payload":{"persona_id":"p","profile_id":5}}`, http.StatusBadRequest},
		"overlong persona":  {http.MethodPost, event(long, "prof"), http.StatusBadRequest},
		"overlong profile":  {http.MethodPost, event("per", long), http.StatusBadRequest},
	} {
		if rec := send(tc.method, tc.body); rec.Code != tc.want {
			t.Errorf("%s: status %d, want %d", name, rec.Code, tc.want)
		}
	}
	if len(c.purged) != 0 {
		t.Errorf("refused events purged %v", c.purged)
	}
	ok := strings.Repeat("x", 200)
	if rec := send(http.MethodPost, event(ok, ok)); rec.Code != http.StatusOK {
		t.Errorf("ids of exactly 200 characters: %d", rec.Code)
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

func seedPersona(t *testing.T, pid, name string, skills []string) string {
	t.Helper()
	per, err := persona.CreatePersona(pid, name, "")
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
	return per
}

func rows(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.JobsDB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The whole chain with real tables and the real engine: the profile has two personas, the better one holds the job matches; when it is
// deleted its matches go (the other persona's scores and the profile's state stay), and matching the freed jobs again gives the
// profile's remaining persona.
func TestTheEventRemovesTheMatchesOfADeletedPersonaOnly(t *testing.T) {
	setupDB(t)
	desc := "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
	for _, id := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`, id, "https://x.example/"+id, desc); err != nil {
			t.Fatal(err)
		}
	}
	pid, err := profile.CreateProfile("Ada", "L")
	if err != nil {
		t.Fatal(err)
	}
	best := seedPersona(t, pid, "best", []string{"Go", "Docker", "Kubernetes", "PostgreSQL"})
	other := seedPersona(t, pid, "other", []string{"Go"})
	if _, err := match.RematchProfile(pid); err != nil {
		t.Fatal(err)
	}
	if rows(t, `SELECT COUNT(*) FROM job_matches WHERE persona_id = ?`, best) != 2 {
		t.Fatal("setup: the better persona should hold both matches")
	}
	// the persona scores of both personas exist (a score row is made by the persona consumer; seed them directly)
	for _, p := range []string{best, other} {
		for _, j := range []string{"j1", "j2"} {
			if _, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, ?, ?, 40)`, j, p, pid); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := persona.DeletePersona(best); err != nil { // removes its scores directly; the job matches are what the consumer removes
		t.Fatal(err)
	}
	// scores that appear for the deleted persona after its direct cleanup (a run that was going): the consumer removes them too
	for _, j := range []string{"j1", "j2"} {
		if _, err := db.JobsDB.Exec(`INSERT INTO job_persona_matches (job_id, persona_id, profile_id, score) VALUES (?, ?, ?, 40)`, j, best, pid); err != nil {
			t.Fatal(err)
		}
	}
	if rows(t, `SELECT COUNT(*) FROM job_matches WHERE persona_id = ?`, best) != 2 {
		t.Fatal("the zombies should still be there before the event")
	}
	var handed []string
	oh := handOver
	handOver = func(ids []string) int { handed = append(handed, ids...); return len(ids) }
	defer func() { handOver = oh }()

	if rec := send(http.MethodPost, event(best, pid)); rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if n := rows(t, `SELECT COUNT(*) FROM job_matches WHERE persona_id = ?`, best); n != 0 {
		t.Errorf("%d zombie matches left", n)
	}
	if rows(t, `SELECT COUNT(*) FROM job_persona_matches WHERE persona_id = ?`, best) != 0 {
		t.Error("the deleted persona's scores are left")
	}
	if rows(t, `SELECT COUNT(*) FROM job_persona_matches WHERE persona_id = ?`, other) != 2 {
		t.Error("the other persona's scores were removed")
	}
	if rows(t, `SELECT COUNT(*) FROM profile_match_state WHERE profile_id = ?`, pid) != 1 {
		t.Error("the profile's re-match state was removed")
	}
	if !slices.Equal(handed, []string{"j1", "j2"}) {
		t.Fatalf("handed over %v", handed)
	}
	out, err := match.AutoMatchJobs(handed)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range out {
		if o.Status != match.AutoMatched || o.ProfileID != pid {
			t.Errorf("%+v, want matched by the profile %s", o, pid)
		}
	}
	if m := rows(t, `SELECT COUNT(*) FROM job_matches WHERE persona_id = ?`, other); m != 2 {
		t.Errorf("the freed jobs should now be held by the remaining persona: %d", m)
	}
	// a redelivery finds nothing and still answers 200
	handed = nil
	if rec := send(http.MethodPost, event(best, pid)); rec.Code != http.StatusOK || len(handed) != 0 {
		t.Errorf("redelivery: %d, handed %v", rec.Code, handed)
	}
}
