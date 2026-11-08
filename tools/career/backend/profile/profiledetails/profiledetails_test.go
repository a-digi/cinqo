package profiledetails

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func setup(t *testing.T) (p1, p2 string) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.JobsDB.Close(); db.CareerDB.Close() })
	var err error
	if p1, err = profile.CreateProfile("Ada", "L"); err != nil {
		t.Fatal(err)
	}
	if p2, err = profile.CreateProfile("Bob", "M"); err != nil {
		t.Fatal(err)
	}
	return
}

func sp(s string) *string { return &s }
func ip(v int64) *int64   { return &v }

func TestAProfileWithoutARowHasEmptyValuesAndAnUnknownOneIsAnError(t *testing.T) {
	p1, _ := setup(t)
	v, err := Get(p1)
	if err != nil || v != (Values{ProfileID: p1}) {
		t.Errorf("Get = %+v, %v", v, err)
	}
	if _, err := Get("nope"); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
	if _, err := Update("nope", Patch{Summary: sp("x")}); !errors.Is(err, ErrUnknownProfile) {
		t.Errorf("update of an unknown profile: %v", err)
	}
}

func TestUpdateChangesOnlyTheFieldsGivenAndEachProfileIsItsOwn(t *testing.T) {
	p1, p2 := setup(t)
	v, err := Update(p1, Patch{Summary: sp("About"), DesiredTitles: sp("CTO"), MinSalary: ip(80000), RemotePreference: sp(" open ")})
	if err != nil {
		t.Fatal(err)
	}
	if v.Summary != "About" || v.DesiredTitles != "CTO" || v.MinSalary != 80000 || v.RemotePreference != "open" || v.DesiredLocations != "" {
		t.Errorf("%+v", v)
	}
	v, err = Update(p1, Patch{DesiredLocations: sp("Berlin")})
	if err != nil || v.Summary != "About" || v.DesiredLocations != "Berlin" || v.MinSalary != 80000 {
		t.Errorf("a partial update must keep the rest: %+v %v", v, err)
	}
	// an empty text and 0 clear a value; 0 is stored as "no minimum" (NULL)
	v, err = Update(p1, Patch{Summary: sp(""), MinSalary: ip(0), RemotePreference: sp("")})
	if err != nil || v.Summary != "" || v.MinSalary != 0 || v.RemotePreference != "" {
		t.Errorf("clearing: %+v %v", v, err)
	}
	var isNull int
	if err := db.CareerDB.QueryRow(`SELECT min_salary IS NULL FROM profile_details WHERE profile_id = ?`, p1).Scan(&isNull); err != nil || isNull != 1 {
		t.Errorf("no minimum must be NULL: %d %v", isNull, err)
	}
	if other, _ := Get(p2); other != (Values{ProfileID: p2}) {
		t.Errorf("another profile changed: %+v", other)
	}
}

func TestARefusedUpdateWritesNothing(t *testing.T) {
	p1, _ := setup(t)
	if _, err := Update(p1, Patch{Summary: sp("Keep")}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"remote", "Remote", "ONSITE", "hybrid", "yes"} {
		if _, err := Update(p1, Patch{Summary: sp("Lost"), RemotePreference: sp(bad)}); !errors.Is(err, ErrInvalidRemotePreference) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if _, err := Update(p1, Patch{Summary: sp("Lost"), MinSalary: ip(-1)}); !errors.Is(err, ErrNegativeSalary) {
		t.Errorf("negative salary: %v", err)
	}
	if v, _ := Get(p1); v.Summary != "Keep" {
		t.Errorf("a refusal wrote something: %+v", v)
	}
}

func TestThePersonasOfTheProfileFollowAndAnOverridingPersonaKeepsItsOwn(t *testing.T) {
	p1, _ := setup(t)
	a, _ := persona.CreatePersona(p1, "A", "")
	b, _ := persona.CreatePersona(p1, "B", "")
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: b, DesiredTitles: sp("B titles")}); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(p1, Patch{Summary: sp("Shared"), DesiredTitles: sp("Shared titles"), MinSalary: ip(70000)}); err != nil {
		t.Fatal(err)
	}
	da, _ := persona.FetchPersonaDetails(a)
	dbb, _ := persona.FetchPersonaDetails(b)
	if da.PersonaDetails.Summary != "Shared" || da.PersonaDetails.DesiredTitles != "Shared titles" || da.PersonaDetails.MinSalary != 70000 {
		t.Errorf("A follows: %+v", da.PersonaDetails)
	}
	if dbb.PersonaDetails.Summary != "Shared" || dbb.PersonaDetails.DesiredTitles != "B titles" {
		t.Errorf("B follows the summary and keeps its own titles: %+v", dbb.PersonaDetails)
	}
}

type recorded struct {
	mu     sync.Mutex
	events []string
}

func captureEvents(t *testing.T) *recorded {
	t.Helper()
	r := &recorded{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var ev struct {
			Topic   string
			Payload map[string]any
		}
		_ = json.Unmarshal(raw, &ev)
		if ev.Topic == "career.persona.changed" {
			r.mu.Lock()
			s, _ := ev.Payload["persona_id"].(string)
			r.events = append(r.events, s)
			r.mu.Unlock()
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return r
}

func (r *recorded) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]string(nil), r.events...)
	r.events = nil
	sort.Strings(out)
	return out
}

func TestEveryPersonaOfTheProfileIsAnnouncedOnlyWhenAValueChanged(t *testing.T) {
	p1, p2 := setup(t)
	a, _ := persona.CreatePersona(p1, "A", "")
	b, _ := persona.CreatePersona(p1, "B", "")
	if _, err := persona.CreatePersona(p2, "Other", ""); err != nil {
		t.Fatal(err)
	}
	ev := captureEvents(t)
	if _, err := Update(p1, Patch{Summary: sp("New")}); err != nil {
		t.Fatal(err)
	}
	want := []string{a, b}
	sort.Strings(want)
	got := ev.take()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("events = %v, want the two personas of the profile %v", got, want)
	}
	if _, err := Update(p1, Patch{Summary: sp("New")}); err != nil { // the same value: nothing changed
		t.Fatal(err)
	}
	if _, err := Update(p1, Patch{}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a change that changed nothing sent %v", got)
	}
	if _, err := Update(p1, Patch{Summary: sp("x"), MinSalary: ip(-1)}); err == nil {
		t.Fatal("expected a refusal")
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a refusal sent %v", got)
	}
}

func TestDeletingTheProfileDeletesItsDetails(t *testing.T) {
	p1, _ := setup(t)
	if _, err := Update(p1, Patch{Summary: sp("x")}); err != nil {
		t.Fatal(err)
	}
	if err := profile.DeleteProfile(p1); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profile_details WHERE profile_id = ?`, p1).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows left: %d %v", n, err)
	}
}
