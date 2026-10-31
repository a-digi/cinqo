package jobmatcher

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// A persona that fits the job in every other way, so only the remote preference differs between the cases.
func dealbreakerPersona(pref string) Persona {
	p := goDev()
	p.RemotePreference = pref
	p.DesiredLocations = "Hamburg" // never the typed word "remote"
	return p
}

func dealbreakerJob(kind string) Job {
	base := "Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."
	switch kind {
	case "remote":
		return Job{Title: "Backend Engineer (m/w/d) Go", Location: "Remote", Description: base}
	case "hybrid":
		return Job{Title: "Backend Engineer (m/w/d) Go", Location: "Hamburg", Description: base + " Hybrid möglich."}
	case "onsite":
		return Job{Title: "Backend Engineer (m/w/d) Go", Location: "Hamburg", Description: base + " Arbeit im Büro."}
	case "onsiteFar":
		return Job{Title: "Backend Engineer (m/w/d) Go", Location: "München", Description: base + " Arbeit im Büro."}
	case "unknown":
		return Job{Title: "Backend Engineer (m/w/d) Go", Description: base}
	}
	panic(kind)
}

func TestAnExplicitPreferenceThatRulesTheJobOutHalvesTheScore(t *testing.T) {
	c := NewCorpus([]string{dealbreakerJob("remote").Description})
	cases := []struct {
		pref, kind string
		ruledOut   bool
	}{
		{"remote_only", "onsite", true},
		{"remote_only", "onsiteFar", true},
		{"onsite", "remote", true},
		// everything else is left alone
		{"remote_only", "remote", false},
		{"remote_only", "hybrid", false}, // a hybrid job is never ruled out
		{"remote_only", "unknown", false},
		{"onsite", "hybrid", false},
		{"onsite", "onsite", false},
		{"onsite", "onsiteFar", false}, // a city the persona does not want is not a dealbreaker
		{"onsite", "unknown", false},
		{"open", "remote", false},
		{"open", "hybrid", false},
		{"open", "onsite", false},
		{"open", "onsiteFar", false},
		{"", "remote", false},
		{"", "onsite", false},
		{"bogus", "onsite", false},
	}
	for _, x := range cases {
		r := mustScore(t, dealbreakerJob(x.kind), []Persona{dealbreakerPersona(x.pref)}, c)
		want := 1.0
		if x.ruledOut {
			want = 0.5 // written out on purpose: changing DealbreakerScale must change this test too
		}
		if r.DealbreakerScale != want {
			t.Errorf("%q persona, %s job: dealbreaker scale %v, want %v", x.pref, x.kind, r.DealbreakerScale, want)
		}
	}
}

func TestTheDealbreakerMultipliesTheWholeScore(t *testing.T) {
	c := NewCorpus([]string{dealbreakerJob("remote").Description})
	r := mustScore(t, dealbreakerJob("onsite"), []Persona{dealbreakerPersona("remote_only")}, c)
	var earned, evaluable float64
	for _, comp := range r.Components {
		if comp.Evaluable {
			earned += comp.Points
			evaluable += float64(comp.Weight)
		}
	}
	unscaled := 100 * earned / evaluable * (ConfidenceBase + ConfidenceSpan*r.Coverage) * r.RelevanceScale
	if math.Abs(float64(r.Score)-math.Round(unscaled*0.5)) > 1 {
		t.Errorf("score %d, want about %.1f (half of %.1f)", r.Score, unscaled*0.5, unscaled)
	}
	if unscaled < 55 || r.Score > 40 {
		t.Errorf("a fitting job should have scored %.1f, ruled out it is %d", unscaled, r.Score)
	}
	// the same job for the persona who does not rule it out
	open := mustScore(t, dealbreakerJob("onsite"), []Persona{dealbreakerPersona("onsite")}, c)
	if open.Score < r.Score+25 {
		t.Errorf("on-site only on an on-site job (%d) must be far above remote only on it (%d)", open.Score, r.Score)
	}
}

func TestTheDealbreakerAndTheRelevanceGateMultiply(t *testing.T) {
	// an unrelated on-site job for a remote-only persona: scaled down twice
	job := Job{Title: "Assistenz der Leitung Jugendhilfe (m/w/d)", Location: "Hamburg", Description: officeText}
	r := mustScore(t, job, []Persona{dealbreakerPersona("remote_only")}, NewCorpus(nil))
	if r.DealbreakerScale != 0.5 || r.RelevanceScale >= 0.5 || r.Score > 5 {
		t.Errorf("dealbreaker %v, relevance %v, score %d", r.DealbreakerScale, r.RelevanceScale, r.Score)
	}
}

func TestRulesOut(t *testing.T) {
	remoteJob := prepareJob(dealbreakerJob("remote"))
	hybridJob := prepareJob(dealbreakerJob("hybrid"))
	officeJob := prepareJob(dealbreakerJob("onsite"))
	unknownJob := prepareJob(dealbreakerJob("unknown"))
	for _, c := range []struct {
		pref string
		job  preparedJob
		want bool
	}{
		{"remote_only", officeJob, true}, {"remote_only", hybridJob, false}, {"remote_only", remoteJob, false}, {"remote_only", unknownJob, false},
		{"onsite", remoteJob, true}, {"onsite", hybridJob, false}, {"onsite", officeJob, false}, {"onsite", unknownJob, false},
		{"open", remoteJob, false}, {"open", officeJob, false}, {"", remoteJob, false}, {"", officeJob, false},
	} {
		if got := rulesOut(c.job, Persona{RemotePreference: c.pref}); got != c.want {
			t.Errorf("rulesOut(%q, %s) = %v, want %v", c.pref, c.job.workMode, got, c.want)
		}
	}
}

func TestTheDealbreakerScaleIsInTheJSON(t *testing.T) {
	r := mustScore(t, dealbreakerJob("onsite"), []Persona{dealbreakerPersona("remote_only")}, NewCorpus(nil))
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"dealbreakerScale":0.5`) {
		t.Errorf("json = %s", b)
	}
	r = mustScore(t, goJob(), []Persona{goDev()}, corpusOf())
	b, _ = json.Marshal(r)
	if !strings.Contains(string(b), `"dealbreakerScale":1`) {
		t.Errorf("json = %s", b)
	}
}

// The finding from the real jobs: many have a place and no description. That says nothing about remote work, so it
// must not count as on-site and must not rule the job out.
func TestAJobWithAPlaceButNoTextIsNeverRuledOut(t *testing.T) {
	bare := Job{Title: "Backend Engineer (m/w/d) Go", Location: "Hamburg"}
	for _, pref := range []string{"remote_only", "onsite", "open"} {
		r := mustScore(t, bare, []Persona{dealbreakerPersona(pref)}, NewCorpus(nil))
		if r.DealbreakerScale != 1 {
			t.Errorf("%s: a job with no text was ruled out", pref)
		}
		loc := comp(t, r, "location")
		if loc.Evaluable {
			t.Errorf("%s: the way it is worked cannot be told, so the place is not judged: %+v", pref, loc)
		}
	}
	// a persona that has not chosen is judged as before (the old reading does not need the mode)
	r := mustScore(t, bare, []Persona{dealbreakerPersona("")}, NewCorpus(nil))
	if !comp(t, r, "location").Evaluable || comp(t, r, "location").Points != WeightLocation {
		t.Errorf("not set: %+v", comp(t, r, "location"))
	}
}
