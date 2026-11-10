package jobmatcher

import (
	"strings"
	"testing"
)

// The five kinds of job a persona living in and wanting Berlin can meet.
func modeJob(kind string) Job {
	switch kind {
	case "remote":
		return Job{Title: "Engineer", Location: "Remote"}
	case "hybrid, Berlin":
		return Job{Title: "Engineer", Location: "Berlin", Description: "Hybrid möglich."}
	case "hybrid, Munich":
		return Job{Title: "Engineer", Location: "München", Description: "Hybrid möglich."}
	case "on-site, Berlin":
		return Job{Title: "Engineer", Location: "Berlin", Description: officeText}
	case "on-site, Munich":
		return Job{Title: "Engineer", Location: "München", Description: officeText}
	}
	panic(kind)
}

func locationOf(job Job, pref string, places string) (float64, bool, string) {
	return locationFit(prepareJob(job), Persona{RemotePreference: pref, Location: places, DesiredLocations: places})
}

// Every cell of the table of plan/ai/tools/career/step-130-remote-preference-and-location-score.md, with the numbers
// written out here (not read from the constants), so changing a constant has to change this test too.
func TestTheLocationTableCellByCell(t *testing.T) {
	kinds := []string{"remote", "hybrid, Berlin", "hybrid, Munich", "on-site, Berlin", "on-site, Munich"}
	table := map[string][]float64{
		//          remote  hybrid city  hybrid other                  on-site city  on-site other
		"onsite":      {0, 1, 0, 1, 0},
		"open":        {1, 1, 0.5, 1, 0},
		"remote_only": {1, 0.3, 0.3, 0, 0},
	}
	for pref, row := range table {
		for i, kind := range kinds {
			v, ok, note := locationOf(modeJob(kind), pref, "Berlin")
			if !ok || v != row[i] || note == "" {
				t.Errorf("%s persona, %s job: %v (evaluable %v, %q), want %v", pref, kind, v, ok, note, row[i])
			}
		}
	}
}

func TestAnUnknownJobCannotBeJudgedForAnExplicitPreference(t *testing.T) {
	job := Job{Title: "Engineer", Description: "Great team."}
	for _, pref := range []string{"onsite", "open", "remote_only"} {
		if _, ok, note := locationOf(job, pref, "Berlin"); ok || note == "" {
			t.Errorf("%s: an unknown job is not available: %v %q", pref, ok, note)
		}
	}
}

func TestAPersonaWithoutPlacesIsJudgedOnlyWhereThePlaceDoesNotMatter(t *testing.T) {
	// remote only: places never matter
	for _, kind := range []string{"remote", "hybrid, Berlin", "on-site, Munich"} {
		if _, ok, _ := locationOf(modeJob(kind), "remote_only", ""); !ok {
			t.Errorf("remote only, %s: should be judged without any place", kind)
		}
	}
	// a remote job does not need places either, for a persona that is open to it or refuses it
	if v, ok, _ := locationOf(modeJob("remote"), "open", ""); !ok || v != 1 {
		t.Errorf("open, remote job, no places: %v %v", v, ok)
	}
	if v, ok, _ := locationOf(modeJob("remote"), "onsite", ""); !ok || v != 0 {
		t.Errorf("on-site only, remote job, no places: %v %v", v, ok)
	}
	// a hybrid or on-site job needs a place to compare with
	for _, pref := range []string{"onsite", "open"} {
		for _, kind := range []string{"hybrid, Berlin", "on-site, Berlin"} {
			if _, ok, note := locationOf(modeJob(kind), pref, ""); ok || !strings.Contains(note, "no location") {
				t.Errorf("%s persona, %s job, no places: %v %q", pref, kind, ok, note)
			}
		}
	}
}

func TestAHybridJobWithoutAPlaceNeedsAPlaceUnlessThePersonaIsRemoteOnly(t *testing.T) {
	job := Job{Title: "Engineer", Description: "Hybrid arbeiten."}
	for _, pref := range []string{"onsite", "open"} {
		if _, ok, note := locationOf(job, pref, "Berlin"); ok || !strings.Contains(note, "names no place") {
			t.Errorf("%s: %v %q", pref, ok, note)
		}
	}
	if v, ok, _ := locationOf(job, "remote_only", "Berlin"); !ok || v != 0.3 {
		t.Errorf("remote only: %v %v", v, ok)
	}
}

func TestTheNotesSayWhichRowApplied(t *testing.T) {
	cases := []struct {
		pref, kind, want string
	}{
		{"remote_only", "remote", "remote work only and the job is remote"},
		{"remote_only", "on-site, Berlin", "remote work only and the job is on-site"},
		{"remote_only", "hybrid, Berlin", "remote work only and the job is hybrid"},
		{"open", "remote", "open to remote work"},
		{"onsite", "remote", "on-site work only"},
		{"open", "hybrid, Munich", "open to hybrid work"},
		{"onsite", "on-site, Berlin", "a place the target role wants"},
		{"onsite", "on-site, Munich", "a place the target role does not want"},
	}
	for _, c := range cases {
		_, _, note := locationOf(modeJob(c.kind), c.pref, "Berlin")
		if !strings.Contains(note, c.want) {
			t.Errorf("%s / %s: %q does not say %q", c.pref, c.kind, note, c.want)
		}
	}
}

func TestAPersonaWithoutAPreferenceIsJudgedAsBefore(t *testing.T) {
	// not set (or a value the engine does not know) keeps the old reading, free text included
	for _, pref := range []string{"", "bogus", "Remote", "hybrid"} {
		// the old reading: a remote or hybrid job in another place is 0.7 for a persona without the word "remote"
		if v, ok, _ := locationOf(modeJob("remote"), pref, "Berlin"); !ok || v != LocationRemote {
			t.Errorf("pref %q, remote job: %v %v, want %v", pref, v, ok, LocationRemote)
		}
		if v, _, _ := locationOf(modeJob("on-site, Berlin"), pref, "Berlin"); v != 1 {
			t.Errorf("pref %q, on-site job in the persona's city: %v", pref, v)
		}
		if v, _, _ := locationOf(modeJob("on-site, Munich"), pref, "Berlin"); v != 0 {
			t.Errorf("pref %q, on-site job elsewhere: %v", pref, v)
		}
	}
	// the word "remote" typed into the places still means "wants remote" when nothing is set
	if v, _, _ := locationOf(modeJob("remote"), "", "Berlin, Remote"); v != 1 {
		t.Errorf("unset persona that typed Remote: %v", v)
	}
	// but once a preference is chosen the typed word is only a place word, and the preference decides
	if v, _, _ := locationOf(modeJob("remote"), "onsite", "Berlin, Remote"); v != 0 {
		t.Errorf("on-site only beats the typed word: %v", v)
	}
	if v, _, _ := locationOf(modeJob("on-site, Berlin"), "remote_only", "Berlin, Remote"); v != 0 {
		t.Errorf("remote only: an on-site job never fits, whatever the places say: %v", v)
	}
}

func TestThePreferenceReachesTheScore(t *testing.T) {
	persona := func(pref string) Persona {
		p := goDev()
		p.RemotePreference = pref
		return p
	}
	inOffice := strings.Replace(goJobDesc, " Hybrid möglich.", "", 1) // goJobDesc ends by saying hybrid is possible
	remote := Job{Title: "Backend Engineer", Location: "Remote", Description: inOffice}
	office := Job{Title: "Backend Engineer", Location: "Hamburg", Description: inOffice}
	score := func(j Job, pref string) (int, float64) {
		r := mustScore(t, j, []Persona{persona(pref)}, corpusOf())
		return r.Score, comp(t, r, "location").Points
	}
	if _, pts := score(remote, "remote_only"); pts != WeightLocation {
		t.Errorf("remote only on a remote job: %v points", pts)
	}
	if _, pts := score(office, "remote_only"); pts != 0 {
		t.Errorf("remote only on an office job in the persona's own city: %v points", pts)
	}
	remoteScore, _ := score(remote, "remote_only")
	officeScore, _ := score(office, "remote_only")
	if remoteScore <= officeScore {
		t.Errorf("a remote-only persona must prefer the remote job: %d vs %d", remoteScore, officeScore)
	}
	onsiteRemote, _ := score(remote, "onsite")
	onsiteOffice, _ := score(office, "onsite")
	if onsiteRemote >= onsiteOffice {
		t.Errorf("an on-site-only persona must prefer the office job: %d vs %d", onsiteRemote, onsiteOffice)
	}
}
