package persona

import (
	"errors"
	"testing"

	"career-tool-backend/db"
)

func newPersona(t *testing.T) string {
	t.Helper()
	profileID := setupRunsDB(t)
	id, err := CreatePersona(profileID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func strp(s string) *string { return &s }

func remoteOf(t *testing.T, personaID string) string {
	t.Helper()
	d, err := FetchPersonaDetails(personaID)
	if err != nil {
		t.Fatal(err)
	}
	if d.PersonaDetails == nil {
		return "<no row>"
	}
	return d.PersonaDetails.RemotePreference
}

func TestRemotePreferenceStartsUnsetAndRoundTrips(t *testing.T) {
	id := newPersona(t)
	if got := remoteOf(t, id); got != "<no row>" {
		t.Fatalf("a persona with no details has no row: %q", got)
	}
	// the row created by another field leaves the preference unset
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: strp("Backend Engineer")}); err != nil {
		t.Fatal(err)
	}
	if got := remoteOf(t, id); got != "" {
		t.Errorf("a new row must be unset, got %q", got)
	}
	for _, v := range []string{RemoteOnSite, RemoteOpen, RemoteOnly, "", RemoteOpen} {
		if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, RemotePreference: strp(v)}); err != nil {
			t.Fatalf("%q: %v", v, err)
		}
		if got := remoteOf(t, id); got != v {
			t.Errorf("stored %q, read %q", v, got)
		}
	}
}

func TestRemotePreferenceIsKeptByOtherUpdatesAndTrimmed(t *testing.T) {
	id := newPersona(t)
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, RemotePreference: strp("  remote_only  ")}); err != nil {
		t.Fatal(err)
	}
	if got := remoteOf(t, id); got != RemoteOnly {
		t.Errorf("trimmed value = %q", got)
	}
	// another field's update (a partial update) must not clear it
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Summary: strp("Ten years of Go"), MinSalary: new(int64)}); err != nil {
		t.Fatal(err)
	}
	if got := remoteOf(t, id); got != RemoteOnly {
		t.Errorf("a partial update changed the preference: %q", got)
	}
	d, _ := FetchPersonaDetails(id)
	if d.PersonaDetails.Summary != "Ten years of Go" {
		t.Errorf("summary = %q", d.PersonaDetails.Summary)
	}
}

func TestAnInvalidRemotePreferenceIsRefusedAndWritesNothing(t *testing.T) {
	id := newPersona(t)
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: strp("Before"), RemotePreference: strp(RemoteOpen)}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"remote", "Remote", "ONSITE", "remote-only", "yes", "hybrid", "onsite;DROP TABLE persona_details"} {
		err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: strp("After"), RemotePreference: strp(bad)})
		if !errors.Is(err, ErrInvalidRemotePreference) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	d, _ := FetchPersonaDetails(id)
	if d.PersonaDetails.Headline != "Before" || d.PersonaDetails.RemotePreference != RemoteOpen {
		t.Errorf("a refused update must change nothing: %+v", d.PersonaDetails)
	}
}

func TestRemotePreferenceOfAnUnknownPersonaIsStillUnknownPersona(t *testing.T) {
	newPersona(t)
	err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: "nope", RemotePreference: strp(RemoteOpen)})
	if !errors.Is(err, ErrUnknownPersona) {
		t.Errorf("err = %v", err)
	}
}

func TestTheDatabaseRefusesAnInvalidValueEvenIfTheCodeDoesNot(t *testing.T) {
	id := newPersona(t)
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: id, Headline: strp("x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CareerDB.Exec(`UPDATE persona_details SET remote_preference = 'remote' WHERE persona_id = ?`, id); err == nil {
		t.Error("the CHECK constraint must refuse a value outside the four")
	}
}
