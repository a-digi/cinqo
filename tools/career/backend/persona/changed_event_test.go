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

// published is a fake core that records the "career.persona.changed" events it receives as "change:personaId:profileId".
type published struct {
	mu     sync.Mutex
	events []string
	other  int // events of any other topic
}

func capturePersonaEvents(t *testing.T) *published {
	t.Helper()
	p := &published{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var ev struct {
			Topic   string
			Payload map[string]any
		}
		_ = json.Unmarshal(raw, &ev)
		p.mu.Lock()
		if ev.Topic == "career.persona.changed" {
			p.events = append(p.events, str(ev.Payload["change"])+":"+str(ev.Payload["persona_id"])+":"+str(ev.Payload["profile_id"]))
			if len(ev.Payload) != 3 {
				p.events = append(p.events, "BAD-PAYLOAD-SIZE")
			}
		} else {
			p.other++
		}
		p.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	return p
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func (p *published) take() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string(nil), p.events...)
	p.events = nil
	return out
}

func sp(s string) *string { return &s }

func TestCreatingAPersonaAnnouncesIt(t *testing.T) {
	pid := setupRunsDB(t)
	ev := capturePersonaEvents(t)
	id, err := CreatePersona(pid, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"created:" + id + ":" + pid}) {
		t.Errorf("events = %v", got)
	}
}

func TestACreateThatFailsAnnouncesNothing(t *testing.T) {
	setupRunsDB(t)
	ev := capturePersonaEvents(t)
	if _, err := CreatePersona("no-such-profile", "X", ""); err == nil {
		t.Fatal("expected an error")
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events = %v", got)
	}
}

func TestChangingTheDetailsTheMatcherReadsAnnouncesItOnce(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := capturePersonaEvents(t)

	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: sp("Backend Engineer")}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"details:" + id + ":" + pid}) {
		t.Fatalf("first save: %v", got)
	}
	// each of the fields the matcher reads
	for name, args := range map[string]UpdatePersonaDetailsArgs{
		"summary":           {Summary: sp("Builds services")},
		"location":          {Location: sp("Hamburg")},
		"desired titles":    {DesiredTitles: sp("Software Engineer")},
		"desired locations": {DesiredLocations: sp("Remote")},
		"remote preference": {RemotePreference: sp("open")},
		"headline again":    {Headline: sp("Senior Backend Engineer")},
	} {
		args.PersonaID = id
		if err := UpdatePersonaDetails(args); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := ev.take(); len(got) != 1 {
			t.Errorf("%s: events = %v, want exactly one", name, got)
		}
	}
}

func TestADetailsSaveThatChangesNothingAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	_ = UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: sp("H"), Location: sp("Hamburg"), RemotePreference: sp("open")})
	ev := capturePersonaEvents(t)

	same := UpdatePersonaDetailsArgs{PersonaID: id, Headline: sp("H"), Location: sp("Hamburg"), RemotePreference: sp("open")}
	if err := UpdatePersonaDetails(same); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id}); err != nil { // nothing supplied
		t.Fatal(err)
	}
	// the very first save of an all-empty record changes nothing either
	fresh, _ := CreatePersona(pid, "Fresh", "")
	_ = ev.take() // the creation of "Fresh"
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: fresh, Headline: sp(""), Summary: sp("")}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events = %v, want none", got)
	}
}

// The minimum salary is not read by the matcher, so changing only it cannot move a score and sends nothing.
func TestChangingOnlyTheSalaryAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	_ = UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: sp("H")})
	ev := capturePersonaEvents(t)
	salary := int64(90000)
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, MinSalary: &salary}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events = %v", got)
	}
}

func TestADetailsSaveThatFailsAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := capturePersonaEvents(t)
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: sp("H"), RemotePreference: sp("sometimes")}); err == nil {
		t.Fatal("an invalid remote preference must fail")
	}
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: "nope", Headline: sp("H")}); err == nil {
		t.Fatal("an unknown persona must fail")
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events = %v", got)
	}
}

func TestSkillsAnnounceOnlyARealChange(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := capturePersonaEvents(t)

	if err := AddCareerSkill(id, "Go"); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"skill_added:" + id + ":" + pid}) {
		t.Fatalf("add: %v", got)
	}
	if err := AddCareerSkill(id, "Go"); err != nil { // already there
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a skill added twice: %v", got)
	}
	if err := RemoveCareerSkill(id, "Cobol"); err != nil { // never there
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("removing a skill that is not there: %v", got)
	}
	if err := RemoveCareerSkill(id, "Go"); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"skill_removed:" + id + ":" + pid}) {
		t.Errorf("remove: %v", got)
	}
	if err := AddCareerSkill("nope", "Go"); err == nil {
		t.Error("an unknown persona must fail")
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a failed add: %v", got)
	}
}

func TestExperienceAnnouncesOnlyARealChange(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := capturePersonaEvents(t)

	expID, err := AddCareerExperience(id, "Acme", "Developer", "2020-01", "", "Built things")
	if err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"experience_added:" + id + ":" + pid}) {
		t.Fatalf("add: %v", got)
	}
	// each of the four fields the matcher reads
	for name, args := range map[string]UpdateCareerExperienceArgs{
		"title":       {Title: sp("Senior Developer")},
		"start date":  {StartDate: sp("2019-06")},
		"end date":    {EndDate: sp("2022-01")},
		"description": {Description: sp("Led a team")},
	} {
		args.PersonaID, args.ID = id, expID
		if err := UpdateCareerExperience(args); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := ev.take(); !reflect.DeepEqual(got, []string{"experience_updated:" + id + ":" + pid}) {
			t.Errorf("%s: %v", name, got)
		}
	}
	// the company is not read by the matcher; the same values change nothing; an unknown entry is a no-op
	for name, args := range map[string]UpdateCareerExperienceArgs{
		"company only":  {PersonaID: id, ID: expID, Company: sp("Other Corp")},
		"same title":    {PersonaID: id, ID: expID, Title: sp("Senior Developer")},
		"nothing given": {PersonaID: id, ID: expID},
		"unknown entry": {PersonaID: id, ID: "nope", Title: sp("X")},
	} {
		if err := UpdateCareerExperience(args); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := ev.take(); len(got) != 0 {
			t.Errorf("%s: events = %v, want none", name, got)
		}
	}
	if err := RemoveCareerExperience(id, "nope"); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("removing an entry that is not there: %v", got)
	}
	if err := RemoveCareerExperience(id, expID); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); !reflect.DeepEqual(got, []string{"experience_removed:" + id + ":" + pid}) {
		t.Errorf("remove: %v", got)
	}
	if _, err := AddCareerExperience("nope", "A", "T", "2020", "", ""); err == nil {
		t.Error("an unknown persona must fail")
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a failed add: %v", got)
	}
}

// The name and the description are not read by the matcher, and a delete leaves nothing to score: no events.
func TestRenamingAndDeletingAPersonaAnnounceNothing(t *testing.T) {
	pid := setupRunsDB(t)
	id, _ := CreatePersona(pid, "Backend", "")
	ev := capturePersonaEvents(t)
	if err := UpdatePersona(id, sp("Renamed"), sp("A description")); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a rename: %v", got)
	}
	if err := DeletePersona(id); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("a delete: %v", got)
	}
}

// Personas created from an AI run are announced once each, after the transaction has committed.
func TestPersonasCreatedFromAnAiRunAreAnnouncedAfterTheCommit(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha", "Beta", "Gamma")
	ev := capturePersonaEvents(t)

	created, err := ApplyDecisions(run, []Decision{accept(0, pp("Alpha", []string{"Go"})), decline(1), accept(2, pp("Gamma", []string{"SQL"}))})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created %d", len(created))
	}
	got := ev.take()
	want := []string{"created:" + created[0].ID + ":" + pid, "created:" + created[1].ID + ":" + pid}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
}

func TestADeclinedOrFailedAiDecisionAnnouncesNothing(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "Alpha")
	ev := capturePersonaEvents(t)
	if _, err := ApplyDecisions(run, []Decision{decline(0)}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyDecisions("no-such-run", []Decision{accept(0, pp("X", nil))}); err == nil {
		t.Fatal("an unknown run must fail")
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events = %v", got)
	}
}

// A core that cannot be reached never fails the change itself.
func TestTheChangeStillWorksWhenCoreIsUnreachable(t *testing.T) {
	pid := setupRunsDB(t)
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	t.Setenv("TOOL_SERVICE_TOKEN", "tok")
	id, err := CreatePersona(pid, "Backend", "")
	if err != nil {
		t.Fatalf("a failed announcement must not fail the create: %v", err)
	}
	if err := AddCareerSkill(id, "Go"); err != nil {
		t.Fatal(err)
	}
	if n := count(t, `SELECT COUNT(*) FROM career_skills WHERE persona_id = ?`, id); n != 1 {
		t.Errorf("the skill was not saved: %d", n)
	}
}

// An unknown persona (a race with a delete) is announced as nothing and never panics.
func TestAPersonaThatIsGoneAnnouncesNothing(t *testing.T) {
	setupRunsDB(t)
	ev := capturePersonaEvents(t)
	publishPersonaChanged("ghost", ChangeDetails)
	if got := ev.take(); len(got) != 0 {
		t.Errorf("events = %v", got)
	}
}
