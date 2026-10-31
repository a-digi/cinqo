package jobmatcher

import (
	"fmt"
	"testing"
)

// The calibration set (step 7 of plan/ai/tools/career/step-129-manual-job-match.md): realistic, written pairs of a persona
// and a job, each with the band a person would expect. No real person's data is in it. A pair that falls outside
// its band means the weights or a rule need another look; change them in weights.go, not here.

func juniorFrontend() Persona {
	return Persona{
		ID: "p-front", Name: "Frontend", Headline: "Junior Frontend Developer", DesiredTitles: "Frontend Developer, Junior Web Developer",
		Location: "Berlin", DesiredLocations: "Berlin, Remote",
		Skills: []string{"JavaScript", "React", "CSS", "HTML", "Git"},
		Experience: []Experience{
			{Title: "Web Development Intern", Description: "React components, CSS, JavaScript", StartDate: "2025-02", EndDate: "2025-09"},
		},
	}
}

func krankenpflegerin() Persona {
	return Persona{
		ID: "p-pflege", Name: "Pflege", Headline: "Gesundheits- und Krankenpflegerin", DesiredTitles: "Gesundheits- und Krankenpfleger/in, Pflegefachkraft",
		Location: "Hamburg", DesiredLocations: "Hamburg",
		Skills: []string{"Wundversorgung", "Pflegedokumentation", "Medikamentengabe", "Palliativpflege", "MS Office"},
		Experience: []Experience{
			{Title: "Gesundheits- und Krankenpflegerin", Description: "Station, Wundversorgung, Pflegedokumentation", StartDate: "2014-04", EndDate: "2020-12"},
			{Title: "Pflegefachkraft", Description: "Palliativstation, Medikamentengabe", StartDate: "2021-01", EndDate: ""},
		},
	}
}

func projectManager() Persona {
	return Persona{
		ID: "p-pm", Name: "PM", Headline: "Project Manager", DesiredTitles: "Project Manager, Delivery Manager",
		Location: "München", DesiredLocations: "München, Remote",
		Skills: []string{"Scrum", "Jira", "MS Project", "Stakeholder Management", "Budgeting"},
		Experience: []Experience{
			{Title: "Project Manager", Description: "Scrum teams, Jira, budgeting, stakeholder management", StartDate: "2016-03", EndDate: "2024-06"},
		},
	}
}

func seniorGoJob() Job {
	return Job{Title: "Senior Backend Engineer (m/w/d) Go", Location: "Hamburg", Description: "Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes und arbeitest mit PostgreSQL. " +
		"Mindestens 5 Jahre Berufserfahrung in der Softwareentwicklung. Hybrid möglich."}
}

func frontendJob() Job {
	return Job{Title: "Frontend Developer (m/w/d)", Location: "Berlin", Description: "Build user interfaces with React and JavaScript, style them with CSS and HTML, work in Git. We are happy to train juniors."}
}

func seniorFrontendJob() Job {
	return Job{Title: "Senior Frontend Lead (m/w/d)", Location: "Berlin", Description: "Lead our frontend team: React, JavaScript, TypeScript, CSS, architecture and mentoring. At least 8 years of experience."}
}

func pflegeJob() Job {
	return Job{Title: "Gesundheits- und Krankenpfleger/in (m/w/d) Palliativstation", Location: "Hamburg", Description: "Sie übernehmen Wundversorgung, Pflegedokumentation und Medikamentengabe auf unserer Palliativstation. 2 Jahre Berufserfahrung wünschenswert."}
}

func pmJob() Job {
	return Job{Title: "Project Manager (m/w/d)", Location: "München, Hybrid", Description: "You lead delivery of software projects: Scrum, Jira, budgeting and stakeholder management. 5 years of experience as a project manager."}
}

func TestCalibrationSetSitsInItsBands(t *testing.T) {
	c := NewCorpus([]string{goJobDesc, kitaJobDesc, seniorGoJob().Description, frontendJob().Description, seniorFrontendJob().Description, pflegeJob().Description, pmJob().Description})
	cases := []struct {
		name   string
		job    Job
		p      Persona
		lo, hi int
	}{
		// the persona is the right person for the job
		{"Go developer, senior Go job", seniorGoJob(), goDev(), 72, 100},
		{"junior frontend, frontend job", frontendJob(), juniorFrontend(), 70, 100},
		{"Erzieherin, Kita job", kitaJob(), erzieher(), 65, 100},
		{"nurse, palliative nursing job", pflegeJob(), krankenpflegerin(), 70, 100},
		{"project manager, project manager job", pmJob(), projectManager(), 70, 100},
		// related, but not quite
		{"junior frontend, senior frontend lead (level and years are far off)", seniorFrontendJob(), juniorFrontend(), 25, 62},
		{"Go developer, frontend job (same craft, other stack)", frontendJob(), goDev(), 5, 40},
		// the wrong person
		{"Erzieherin, nursing job (social work, not nursing)", pflegeJob(), erzieher(), 0, 35},
		{"nurse, Kita job", kitaJob(), krankenpflegerin(), 0, 35},
		{"project manager, Go job", seniorGoJob(), projectManager(), 0, 30},
		{"Go developer, project manager job", pmJob(), goDev(), 0, 30},
		{"nurse, Go job", seniorGoJob(), krankenpflegerin(), 0, 25},
	}
	for _, x := range cases {
		r := mustScore(t, x.job, []Persona{x.p}, c)
		t.Logf("%-70s %3d  (%s, coverage %.2f)", x.name, r.Score, r.Mode, r.Coverage)
		inRange(t, x.name, r.Score, x.lo, x.hi)
	}
}

func TestCalibrationOrdering(t *testing.T) {
	c := NewCorpus([]string{goJobDesc, seniorGoJob().Description, frontendJob().Description, pmJob().Description})
	score := func(j Job, p Persona) int { return mustScore(t, j, []Persona{p}, c).Score }

	// the same persona ranks its own field above another, and a fitting level above a mismatched one
	if a, b := score(frontendJob(), juniorFrontend()), score(seniorFrontendJob(), juniorFrontend()); a <= b {
		t.Errorf("a junior should fit the junior-friendly job (%d) above the senior lead job (%d)", a, b)
	}
	if a, b := score(seniorGoJob(), goDev()), score(frontendJob(), goDev()); a <= b+25 {
		t.Errorf("a Go developer: the Go job (%d) must clearly outrank the frontend job (%d)", a, b)
	}

	// a job with only a title to go on must not outrank the same job fully described
	bare := seniorGoJob()
	bare.Description, bare.Location = "", ""
	if described, thin := score(seniorGoJob(), goDev()), score(bare, goDev()); thin >= described {
		t.Errorf("a described job (%d) must outrank the same job without a description (%d)", described, thin)
	}

	// requirements: covering more of them scores higher, in steps that follow the tiers
	base := Job{Title: "Backend Engineer", Location: "Hamburg"}
	three := func(terms ...string) int {
		j := base
		for i, term := range terms {
			j.Requirements = append(j.Requirements, Requirement{Term: term, Tier: 3 - i%2})
		}
		return score(j, goDev())
	}
	all, two, none := three("Go", "Docker", "Kubernetes"), three("Go", "Docker", "Terraform"), three("Terraform", "Ansible", "Puppet")
	if !(all > two && two > none) {
		t.Errorf("covering 3, 2 and 0 of 3 requirements must score %d > %d > %d", all, two, none)
	}
	// With nothing covered the title, the years and the place still count (no cap, by decision): a middling match,
	// far below the same job fully covered.
	if all < 80 || none > 55 || all-none < 30 {
		t.Errorf("all requirements covered should be a strong match (%d), none a clearly weaker one (%d)", all, none)
	}
}

func TestCalibrationPrintsTheTable(t *testing.T) {
	// a reader's view of the set: one line per pair, run with -v
	c := corpusOf()
	r := mustScore(t, goJob(), []Persona{goDev()}, c)
	t.Log(fmt.Sprintf("example breakdown: %s", r.Describe()))
	for _, comp := range r.Components {
		t.Logf("  %-10s %5.1f / %d  %s", comp.Key, comp.Points, comp.Weight, comp.Note)
	}
}
