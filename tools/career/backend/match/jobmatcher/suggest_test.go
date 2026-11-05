package jobmatcher

import (
	"strings"
	"testing"
)

// The text that came out noisy when the suggestions first met real German: verb forms and fillers ranked beside the
// real skills.
const noisyJobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung. Hybrid möglich."

func noisyJob() Job {
	return Job{Title: "Backend Engineer (m/w/d)", Location: "Hamburg", Description: noisyJobDesc}
}

func TestSuggestionsOnRealGermanTextAreSkillsNotVerbsOrFillers(t *testing.T) {
	got := Suggest(noisyJob(), NewCorpus(nil))
	for _, want := range []string{"Docker", "Kubernetes", "PostgreSQL", "Backend", "Engineer"} {
		if !contains(got, want) {
			t.Errorf("%q should be suggested: %v", want, got)
		}
	}
	for _, noise := range []string{"betreibst", "entwickelst", "arbeitest", "Mindestens", "Verstärkung", "Berufserfahrung", "Hybrid", "Hamburg", "Jahre", "suchen"} {
		for _, g := range got {
			if strings.EqualFold(g, noise) {
				t.Errorf("%q is noise and must not be suggested: %v", noise, got)
			}
		}
	}
}

func TestOnlyTermLikeWordsAreCandidates(t *testing.T) {
	c := NewCorpus(nil)
	cases := []struct {
		name string
		job  Job
		want []string // must be suggested
		not  []string // must not be
	}{
		{"lower-case words in the body are not terms",
			Job{Title: "Engineer", Description: "we deploy services and manage clusters, services and clusters daily."},
			nil, []string{"services", "clusters", "deploy", "manage", "daily"}},
		{"a capitalised name in the middle of a sentence is a term",
			Job{Title: "Engineer", Description: "You will run Kubernetes clusters. Kubernetes experience helps."},
			[]string{"Kubernetes"}, nil},
		{"a word that only opens a sentence once is not a term",
			Job{Title: "Engineer", Description: "Flexibel arbeiten wir. Es bleibt das Team."},
			nil, []string{"Flexibel"}},
		{"the start of a line is the start of a sentence",
			Job{Title: "Engineer", Description: "Wir nutzen Docker\nFlexibel arbeiten wir\nEs gibt Kaffee"},
			[]string{"Docker", "Kaffee"}, []string{"Flexibel"}},
		{"a name that opens sentences twice is a term",
			Job{Title: "Engineer", Description: "Terraform ist wichtig. Terraform wird täglich genutzt."},
			[]string{"Terraform"}, nil},
		{"a title word counts even in lower case",
			Job{Title: "kubernetes platform", Description: "something else entirely"},
			[]string{"kubernetes", "platform"}, nil},
		{"letters and digits make a term",
			Job{Title: "Engineer", Description: "we use k8s2 and s3 here, k8s2 and s3 again"},
			[]string{"k8s2", "s3"}, nil},
		{"the job's own place is never suggested",
			Job{Title: "Engineer", Location: "Hamburg", Description: "Wir sitzen in Hamburg und Hamburg ist schön, Hamburg."},
			nil, []string{"Hamburg"}},
		{"boilerplate is never suggested",
			Job{Title: "Engineer", Description: "Ihre Bewerbung an unser Team. Das Team freut sich auf Ihre Bewerbung und die Unterlagen, Team Bewerbung."},
			nil, []string{"Bewerbung", "Team", "Unterlagen"}},
	}
	for _, x := range cases {
		got := Suggest(x.job, c)
		for _, w := range x.want {
			if !contains(got, w) {
				t.Errorf("%s: %q should be suggested: %v", x.name, w, got)
			}
		}
		for _, n := range x.not {
			if contains(got, n) {
				t.Errorf("%s: %q must not be suggested: %v", x.name, n, got)
			}
		}
	}
}

func TestGapsUseTheSameFilter(t *testing.T) {
	r := mustScore(t, noisyJob(), []Persona{{Name: "x", DesiredTitles: "Backend Developer", Skills: []string{"Go"}}}, NewCorpus(nil))
	for _, noise := range []string{"betreibst", "entwickelst", "arbeitest", "Mindestens", "Verstärkung", "Berufserfahrung", "Hybrid"} {
		if contains(r.Gaps, noise) {
			t.Errorf("%q is noise and must not be a gap: %v", noise, r.Gaps)
		}
	}
	for _, want := range []string{"Docker", "Kubernetes", "PostgreSQL"} {
		if !contains(r.Gaps, want) {
			t.Errorf("%q is a real gap: %v", want, r.Gaps)
		}
	}
}

func TestCapitalisedMidSentence(t *testing.T) {
	m := capitalisedMidSentence("Wir nutzen Docker. Kubernetes läuft hier.\nMonitoring ist wichtig. Monitoring auch.")
	for w, want := range map[string]bool{
		"docker":     true,  // mid-sentence capital
		"kubernetes": false, // opens a sentence once
		"wir":        false,
		"monitoring": true, // opens sentences twice, never lower-case
		"nutzen":     false,
	} {
		if m[w] != want {
			t.Errorf("%q = %v, want %v (%v)", w, m[w], want, m)
		}
	}
	// a word that is also written in lower case is not a name that only opens sentences
	m = capitalisedMidSentence("Testing ist gut. Testing wird gemacht. Wir testing nicht.")
	if m["testing"] {
		t.Errorf("a word seen in lower case is no repeated sentence opener: %v", m)
	}
}
