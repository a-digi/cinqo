package persona

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

// deletedProbe records every event (topic and payload) and the number of persona score states in jobs.db at the moment it arrived.
type deletedProbe struct {
	mu       sync.Mutex
	topics   []string
	payloads []map[string]string
	statesAt []int
}

func probeDeleted(t *testing.T) *deletedProbe {
	t.Helper()
	p := &deletedProbe{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/events/publish") { // the comments purge also goes to core: not an event
			w.WriteHeader(http.StatusOK)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var ev struct {
			Topic   string
			Payload map[string]string
		}
		_ = json.Unmarshal(raw, &ev)
		var n int
		_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM persona_score_state`).Scan(&n)
		p.mu.Lock()
		p.topics = append(p.topics, ev.Topic)
		p.payloads = append(p.payloads, ev.Payload)
		p.statesAt = append(p.statesAt, n)
		p.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return p
}

func (p *deletedProbe) of(topic string) []map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]string
	for i, tp := range p.topics {
		if tp == topic {
			out = append(out, p.payloads[i])
		}
	}
	return out
}

func (p *deletedProbe) total() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.topics)
}

// Deleting a persona announces it once with its id and its profile's id, and nothing else is sent.
func TestDeletingAPersonaAnnouncesIt(t *testing.T) {
	pid := setupRunsDB(t)
	id, err := CreatePersona(pid, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	ev := probeDeleted(t)
	if err := DeletePersona(id); err != nil {
		t.Fatal(err)
	}
	got := ev.of("career.persona.deleted")
	if len(got) != 1 || !reflect.DeepEqual(got[0], map[string]string{"persona_id": id, "profile_id": pid}) || ev.total() != 1 {
		t.Errorf("events %v, total %d", got, ev.total())
	}
}

// The profile id is the persona's own profile, with two profiles around.
func TestTheEventNamesThePersonasOwnProfile(t *testing.T) {
	a := setupRunsDB(t)
	b, err := profile.CreateProfile("Other", "x")
	if err != nil {
		t.Fatal(err)
	}
	pa, _ := CreatePersona(a, "A", "")
	pb, _ := CreatePersona(b, "B", "")
	ev := probeDeleted(t)
	if err := DeletePersona(pb); err != nil {
		t.Fatal(err)
	}
	got := ev.of("career.persona.deleted")
	if len(got) != 1 || got[0]["persona_id"] != pb || got[0]["profile_id"] != b {
		t.Errorf("events %v (a: %s/%s)", got, a, pa)
	}
}

// A persona that does not exist, or was already deleted, announces nothing.
func TestDeletingAPersonaThatIsNotThereAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := probeDeleted(t)
	if err := DeletePersona("nope"); err != nil || ev.total() != 0 {
		t.Fatalf("unknown id: %v, %d events", err, ev.total())
	}
	if err := DeletePersona(id); err != nil {
		t.Fatal(err)
	}
	if err := DeletePersona(id); err != nil {
		t.Fatal(err)
	}
	if got := ev.of("career.persona.deleted"); len(got) != 1 {
		t.Errorf("a second delete announced again: %v", got)
	}
}

// A failed delete announces nothing.
func TestAFailedPersonaDeleteAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := probeDeleted(t)
	db.CareerDB.Close()
	if err := DeletePersona(id); err == nil {
		t.Fatal("expected an error")
	}
	if ev.total() != 0 {
		t.Errorf("%d events after a failed delete", ev.total())
	}
}

// Deleting a profile announces the profile only: its personas go with it and are not announced one by one.
func TestDeletingAProfileDoesNotAnnounceItsPersonas(t *testing.T) {
	pid := setupRunsDB(t)
	if _, err := CreatePersona(pid, "One", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePersona(pid, "Two", ""); err != nil {
		t.Fatal(err)
	}
	ev := probeDeleted(t)
	if err := profile.DeleteProfile(pid); err != nil {
		t.Fatal(err)
	}
	if len(ev.of("career.persona.deleted")) != 0 || len(ev.of("career.profile.deleted")) != 1 || ev.total() != 1 {
		t.Errorf("events %v", ev.topics)
	}
}

// The event comes after the existing cleanup: when it arrives the persona's score state is already gone.
func TestThePersonaEventComesAfterTheCleanup(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	if _, err := db.JobsDB.Exec(`INSERT INTO persona_score_state (persona_id, profile_id, fingerprint) VALUES (?, ?, 'x')`, id, pid); err != nil {
		t.Fatal(err)
	}
	ev := probeDeleted(t)
	if err := DeletePersona(id); err != nil {
		t.Fatal(err)
	}
	if len(ev.statesAt) != 1 || ev.statesAt[0] != 0 {
		t.Errorf("states left when the event arrived: %v", ev.statesAt)
	}
}

// Creating or editing a persona does not announce a delete.
func TestCreatingOrEditingAPersonaAnnouncesNoDelete(t *testing.T) {
	pid := setupRunsDB(t)
	ev := probeDeleted(t)
	id, _ := CreatePersona(pid, "Backend", "")
	n := "Renamed"
	if err := UpdatePersona(id, &n, nil); err != nil {
		t.Fatal(err)
	}
	if len(ev.of("career.persona.deleted")) != 0 {
		t.Errorf("events %v", ev.topics)
	}
}

// profileOfPersona answers "not found" both for a persona that is not there and for a read that fails, so nothing is announced then.
func TestProfileOfPersona(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	if got, ok := profileOfPersona(id); !ok || got != pid {
		t.Errorf("%q %v", got, ok)
	}
	if got, ok := profileOfPersona("nope"); ok || got != "" {
		t.Errorf("unknown persona: %q %v", got, ok)
	}
	db.CareerDB.Close()
	if got, ok := profileOfPersona(id); ok || got != "" {
		t.Errorf("failed read: %q %v", got, ok)
	}
}
