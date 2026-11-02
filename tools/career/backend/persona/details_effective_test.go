package persona

import (
	"errors"
	"reflect"
	"testing"

	"career-tool-backend/db"
)

// setProfileDetails writes the profile's summary and goals (its own writer is step 3 of plan 174).
func setProfileDetails(t *testing.T, profileID, summary, titles, locations string, salary int64, remote string) {
	t.Helper()
	if _, err := db.CareerDB.Exec(
		`INSERT INTO profile_details (profile_id, summary, desired_titles, desired_locations, min_salary, remote_preference)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(profile_id) DO UPDATE SET summary = excluded.summary, desired_titles = excluded.desired_titles,
		   desired_locations = excluded.desired_locations, min_salary = excluded.min_salary, remote_preference = excluded.remote_preference`,
		profileID, summary, titles, locations, salary, remote,
	); err != nil {
		t.Fatal(err)
	}
}

func detailsOf(t *testing.T, personaID string) personaDetails {
	t.Helper()
	r, err := FetchPersonaDetails(personaID)
	if err != nil {
		t.Fatal(err)
	}
	if r.PersonaDetails == nil {
		t.Fatalf("persona %s has no details", personaID)
	}
	return *r.PersonaDetails
}

func i64(v int64) *int64 { return &v }

func TestAPersonaWithoutARowFollowsTheProfileAndIsNotConfiguredWhenThereIsNothingToFollow(t *testing.T) {
	a := newPersona(t)
	if r, err := FetchPersonaDetails(a); err != nil || r.PersonaDetails != nil {
		t.Fatalf("nothing set anywhere: details = %+v, err %v", r.PersonaDetails, err)
	}
	setProfileDetails(t, profileOf(t, a), "Profile text", "CTO", "Berlin", 80000, "open")
	d := detailsOf(t, a)
	if d.Summary != "Profile text" || d.DesiredTitles != "CTO" || d.DesiredLocations != "Berlin" || d.MinSalary != 80000 || d.RemotePreference != "open" {
		t.Errorf("the persona must show the profile's values: %+v", d)
	}
	if d.Headline != "" || d.Location != "" || len(d.Overridden) != 0 || d.Overridden == nil {
		t.Errorf("headline/location empty, overridden an empty (non-null) list: %+v", d)
	}
}

func TestAPersonasOwnValueReplacesTheProfilesForThatPersonaOnly(t *testing.T) {
	a := newPersona(t)
	profileID := profileOf(t, a)
	b, err := CreatePersona(profileID, "Frontend", "")
	if err != nil {
		t.Fatal(err)
	}
	setProfileDetails(t, profileID, "Profile text", "CTO", "Berlin", 80000, "open")

	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Summary: strp("Own text"), DesiredTitles: strp("Staff Eng"), MinSalary: i64(95000), RemotePreference: strp("remote_only")}); err != nil {
		t.Fatal(err)
	}
	da, db2 := detailsOf(t, a), detailsOf(t, b)
	if da.Summary != "Own text" || da.DesiredTitles != "Staff Eng" || da.MinSalary != 95000 || da.RemotePreference != "remote_only" {
		t.Errorf("A = %+v", da)
	}
	if da.DesiredLocations != "Berlin" {
		t.Errorf("A did not override the locations: %q", da.DesiredLocations)
	}
	if want := []string{FieldSummary, FieldDesiredTitles, FieldMinSalary, FieldRemotePreference}; !reflect.DeepEqual(da.Overridden, want) {
		t.Errorf("A overridden = %v, want %v", da.Overridden, want)
	}
	// the other persona (no row at all) and the profile are untouched
	if db2.Summary != "Profile text" || db2.DesiredTitles != "CTO" || db2.MinSalary != 80000 || db2.RemotePreference != "open" || len(db2.Overridden) != 0 {
		t.Errorf("B must keep following: %+v", db2)
	}
	var s string
	if err := db.CareerDB.QueryRow(`SELECT summary FROM profile_details WHERE profile_id = ?`, profileID).Scan(&s); err != nil || s != "Profile text" {
		t.Errorf("the profile's summary = %q (err %v)", s, err)
	}
}

func TestAProfileChangeReachesAFollowingPersonaAndNotAnOverridingOne(t *testing.T) {
	a := newPersona(t)
	profileID := profileOf(t, a)
	b, _ := CreatePersona(profileID, "Frontend", "")
	setProfileDetails(t, profileID, "v1", "T1", "L1", 1, "")
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: b, Summary: strp("B own")}); err != nil {
		t.Fatal(err)
	}
	setProfileDetails(t, profileID, "v2", "T2", "L2", 2, "onsite")
	da, dbb := detailsOf(t, a), detailsOf(t, b)
	if da.Summary != "v2" || da.DesiredTitles != "T2" || da.MinSalary != 2 || da.RemotePreference != "onsite" {
		t.Errorf("A follows: %+v", da)
	}
	if dbb.Summary != "B own" || dbb.DesiredTitles != "T2" || dbb.DesiredLocations != "L2" {
		t.Errorf("B keeps its own summary and follows the rest: %+v", dbb)
	}
}

func TestAnOwnValueCanBeEmptyOrZeroAndFollowProfileClearsIt(t *testing.T) {
	a := newPersona(t)
	profileID := profileOf(t, a)
	setProfileDetails(t, profileID, "Profile text", "CTO", "Berlin", 80000, "open")
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Summary: strp(""), MinSalary: i64(0), RemotePreference: strp("")}); err != nil {
		t.Fatal(err)
	}
	d := detailsOf(t, a)
	if d.Summary != "" || d.MinSalary != 0 || d.RemotePreference != "" || len(d.Overridden) != 3 {
		t.Errorf("an own empty value must show as empty: %+v", d)
	}
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, FollowProfile: []string{FieldSummary, FieldMinSalary}}); err != nil {
		t.Fatal(err)
	}
	d = detailsOf(t, a)
	if d.Summary != "Profile text" || d.MinSalary != 80000 || d.RemotePreference != "" || !reflect.DeepEqual(d.Overridden, []string{FieldRemotePreference}) {
		t.Errorf("after following again: %+v", d)
	}
}

func TestHeadlineAndLocationStayThePersonasAndAreKeptByOtherUpdates(t *testing.T) {
	a := newPersona(t)
	setProfileDetails(t, profileOf(t, a), "P", "", "", 0, "")
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Headline: strp("H"), Location: strp("Hamburg")}); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Summary: strp("S")}); err != nil {
		t.Fatal(err)
	}
	d := detailsOf(t, a)
	if d.Headline != "H" || d.Location != "Hamburg" || d.Summary != "S" {
		t.Errorf("%+v", d)
	}
}

func TestARefusedUpdateWritesNothing(t *testing.T) {
	a := newPersona(t)
	setProfileDetails(t, profileOf(t, a), "P", "", "", 0, "")
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Headline: strp("H")}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args UpdatePersonaDetailsArgs
		want error
	}{
		{"unknown field", UpdatePersonaDetailsArgs{PersonaID: a, Headline: strp("X"), FollowProfile: []string{"headline"}}, ErrUnknownFollowField},
		{"set and follow", UpdatePersonaDetailsArgs{PersonaID: a, Headline: strp("X"), Summary: strp("own"), FollowProfile: []string{FieldSummary}}, ErrSetAndFollow},
		{"bad remote", UpdatePersonaDetailsArgs{PersonaID: a, Headline: strp("X"), RemotePreference: strp("sometimes")}, ErrInvalidRemotePreference},
		{"unknown persona", UpdatePersonaDetailsArgs{PersonaID: "nope", Headline: strp("X")}, ErrUnknownPersona},
	}
	for _, c := range cases {
		if err := UpdatePersonaDetails(c.args); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	if d := detailsOf(t, a); d.Headline != "H" || len(d.Overridden) != 0 {
		t.Errorf("a refusal changed something: %+v", d)
	}
}

func TestDetailsEventsAreSentOnlyWhenWhatThePersonaShowsChanges(t *testing.T) {
	a := newPersona(t)
	profileID := profileOf(t, a)
	b, _ := CreatePersona(profileID, "Frontend", "")
	setProfileDetails(t, profileID, "Same", "", "", 0, "")
	ev := capturePersonaEvents(t)

	// an own value equal to the one it already shows changes nothing it shows: no event
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Summary: strp("Same")}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("same shown value: events = %v", got)
	}
	// a different own value: one event
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, Summary: strp("Other")}); err != nil {
		t.Fatal(err)
	}
	if got, want := ev.take(), []string{"details:" + a + ":" + profileID}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	// back to following: shows "Same" again: an event
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, FollowProfile: []string{FieldSummary}}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 1 {
		t.Errorf("following again: events = %v", got)
	}
	// the minimum salary is not read by the matcher: no event
	if err := UpdatePersonaDetails(UpdatePersonaDetailsArgs{PersonaID: a, MinSalary: i64(5)}); err != nil {
		t.Fatal(err)
	}
	if got := ev.take(); len(got) != 0 {
		t.Errorf("salary only: events = %v", got)
	}
	// a change of the profile's values is announced for every persona of the profile
	PublishProfileDetailsChanged(profileID)
	got := ev.take()
	if len(got) != 2 {
		t.Fatalf("events = %v, want one per persona", got)
	}
	want := map[string]bool{"details:" + a + ":" + profileID: true, "details:" + b + ":" + profileID: true}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected event %q", g)
		}
	}
}

func TestAnAcceptedProposalFillsTheProfilesEmptyValuesAndKeepsWhatDiffers(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "First", "Second", "Third")
	first := pp("First", nil) // summary "s", titles "Dev", locations "Remote", salary 70000
	second := pp("Second", nil)
	second.Details.Summary = " s "        // same as the profile's, trimmed: follows
	second.Details.DesiredTitles = "Lead" // differs: own
	second.Details.DesiredLocations = ""  // no opinion: follows
	second.Details.MinSalary = 0          // no opinion: follows
	third := pp("Third", nil)
	third.Details.MinSalary = 90000 // differs: own

	created, err := ApplyDecisions(run, []Decision{accept(0, first), accept(1, second), accept(2, third)})
	if err != nil {
		t.Fatal(err)
	}
	var s, titles, locs string
	var salary int64
	if err := db.CareerDB.QueryRow(`SELECT summary, desired_titles, desired_locations, min_salary FROM profile_details WHERE profile_id = ?`, pid).Scan(&s, &titles, &locs, &salary); err != nil {
		t.Fatal(err)
	}
	if s != "s" || titles != "Dev" || locs != "Remote" || salary != 70000 {
		t.Errorf("the profile got the first persona's values: %q %q %q %d", s, titles, locs, salary)
	}
	d1, d2, d3 := detailsOf(t, created[0].ID), detailsOf(t, created[1].ID), detailsOf(t, created[2].ID)
	if len(d1.Overridden) != 0 {
		t.Errorf("first follows: %v", d1.Overridden)
	}
	if !reflect.DeepEqual(d2.Overridden, []string{FieldDesiredTitles}) || d2.DesiredTitles != "Lead" || d2.DesiredLocations != "Remote" || d2.MinSalary != 70000 || d2.Summary != "s" {
		t.Errorf("second = %+v", d2)
	}
	if !reflect.DeepEqual(d3.Overridden, []string{FieldMinSalary}) || d3.MinSalary != 90000 || d3.DesiredTitles != "Dev" {
		t.Errorf("third = %+v", d3)
	}
	// headline and location are the persona's own as always
	if d2.Headline != "h Second" || d2.Location != "Hamburg" {
		t.Errorf("headline/location = %q %q", d2.Headline, d2.Location)
	}
}

func TestAnAcceptedProposalNeverOverwritesAnExistingProfileValue(t *testing.T) {
	pid := setupRunsDB(t)
	setProfileDetails(t, pid, "Profile summary", "", "Berlin", 0, "open")
	run := runWith(t, pid, "One")
	p := pp("One", nil) // summary "s" (differs), titles "Dev" (profile empty: fills), locations "Remote" (differs), salary 70000 (fills)
	created, err := ApplyDecisions(run, []Decision{accept(0, p)})
	if err != nil {
		t.Fatal(err)
	}
	var s, titles, locs, remote string
	var salary int64
	if err := db.CareerDB.QueryRow(`SELECT summary, desired_titles, desired_locations, COALESCE(min_salary, 0), remote_preference FROM profile_details WHERE profile_id = ?`, pid).
		Scan(&s, &titles, &locs, &salary, &remote); err != nil {
		t.Fatal(err)
	}
	if s != "Profile summary" || titles != "Dev" || locs != "Berlin" || salary != 70000 || remote != "open" {
		t.Errorf("profile = %q %q %q %d %q", s, titles, locs, salary, remote)
	}
	d := detailsOf(t, created[0].ID)
	if d.Summary != "s" || d.DesiredLocations != "Remote" || d.DesiredTitles != "Dev" || d.RemotePreference != "open" {
		t.Errorf("persona = %+v", d)
	}
	if want := []string{FieldSummary, FieldDesiredLocations}; !reflect.DeepEqual(d.Overridden, want) {
		t.Errorf("overridden = %v, want %v", d.Overridden, want)
	}
}

func TestAProposalWithOnlyATextStillFillsTheProfile(t *testing.T) {
	pid := setupRunsDB(t)
	run := runWith(t, pid, "One")
	p := pp("One", nil)
	p.Details.DesiredTitles, p.Details.DesiredLocations, p.Details.MinSalary = "", "", 0
	if _, err := ApplyDecisions(run, []Decision{accept(0, p)}); err != nil {
		t.Fatal(err)
	}
	var s string
	if err := db.CareerDB.QueryRow(`SELECT summary FROM profile_details WHERE profile_id = ?`, pid).Scan(&s); err != nil || s != "s" {
		t.Errorf("the profile's summary = %q (err %v)", s, err)
	}
}
