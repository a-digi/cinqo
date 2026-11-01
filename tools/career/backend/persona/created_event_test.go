package persona

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
)

// createdEvents is a fake core that records the "career.persona.created" events, and how many events of any topic arrived.
type createdEvents struct {
	mu      sync.Mutex
	created []map[string]string
	changed int // "career.persona.changed" events
	total   int
}

func captureCreated(t *testing.T) *createdEvents {
	t.Helper()
	c := &createdEvents{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev struct {
			Topic   string
			Payload map[string]string
		}
		_ = json.Unmarshal(raw, &ev)
		c.mu.Lock()
		c.total++
		switch ev.Topic {
		case "career.persona.created":
			c.created = append(c.created, ev.Payload)
		case "career.persona.changed":
			c.changed++
		}
		c.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return c
}

func (c *createdEvents) snapshot() (created []map[string]string, changed, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]map[string]string(nil), c.created...), c.changed, c.total
}

// Creating a persona announces it once with its id and its profile's id, next to the "changed / created" event that stays.
func TestCreatingAPersonaAnnouncesItAsCreated(t *testing.T) {
	pid := setupRunsDB(t)
	ev := captureCreated(t)
	id, err := CreatePersona(pid, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	created, changed, total := ev.snapshot()
	if len(created) != 1 || !reflect.DeepEqual(created[0], map[string]string{"persona_id": id, "profile_id": pid}) {
		t.Errorf("created events %v", created)
	}
	if changed != 1 || total != 2 {
		t.Errorf("the changed/created event must stay and nothing else may be sent: changed=%d total=%d", changed, total)
	}
}

func TestAPersonaThatCouldNotBeCreatedIsNotAnnouncedAsCreated(t *testing.T) {
	setupRunsDB(t)
	ev := captureCreated(t)
	if _, err := CreatePersona("no-such-profile", "X", ""); err == nil {
		t.Fatal("expected an error")
	}
	if _, _, total := ev.snapshot(); total != 0 {
		t.Errorf("%d events for a failed create", total)
	}
}

// The personas the AI proposed and the person accepted are announced after the transaction committed, one event each, under the run's
// profile; a declined one is not.
func TestAcceptedAiPersonasAreAnnouncedAsCreated(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta", "Gamma")
	ev := captureCreated(t)
	made, err := ApplyDecisions(run, []Decision{accept(0, pp("Alpha", []string{"Go"})), decline(1), accept(2, pp("Gamma", []string{"SQL"}))})
	if err != nil || len(made) != 2 {
		t.Fatalf("%v %v", made, err)
	}
	created, changed, _ := ev.snapshot()
	want := []map[string]string{{"persona_id": made[0].ID, "profile_id": pid}, {"persona_id": made[1].ID, "profile_id": pid}}
	if !reflect.DeepEqual(created, want) {
		t.Errorf("created events %v, want %v", created, want)
	}
	if changed != 2 {
		t.Errorf("the changed/created events of step 142 must still go out: %d", changed)
	}
}

// A decision that fails creates nothing and announces nothing.
func TestAFailedApplyAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha")
	ev := captureCreated(t)
	if _, err := ApplyDecisions(run, []Decision{accept(5, pp("Nope", nil))}); err == nil { // a position that was never proposed
		t.Fatal("expected an error")
	}
	if _, _, total := ev.snapshot(); total != 0 {
		t.Errorf("%d events for a refused apply", total)
	}
	if n := count(t, `SELECT COUNT(*) FROM personas WHERE profile_id = ?`, pid); n != 0 {
		t.Errorf("%d personas created", n)
	}
}

// Skills, details and experience written to an existing persona are not a creation.
func TestEditingAPersonaIsNotAnnouncedAsCreated(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := captureCreated(t)
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: sp("Backend Engineer")}); err != nil {
		t.Fatal(err)
	}
	if err := AddCareerSkill(id, "Go"); err != nil {
		t.Fatal(err)
	}
	if created, changed, _ := ev.snapshot(); len(created) != 0 || changed != 2 {
		t.Errorf("created=%v changed=%d", created, changed)
	}
}
