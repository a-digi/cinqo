package jobs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/jobmatcher"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

func str(s string) *string { return &s }

// seedProfile makes a profile with one persona (skills, details, experience) and returns both ids.
func seedProfile(t *testing.T, name string, skills []string) (profileID, personaID string) {
	t.Helper()
	profileID, err := profile.CreateProfile(name, "Test")
	if err != nil {
		t.Fatal(err)
	}
	personaID, err = persona.CreatePersona(profileID, name+" persona", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{
		PersonaID: personaID, Headline: str("Backend Engineer"), DesiredTitles: str("Software Engineer, Backend Developer"),
		Location: str("Hamburg"), DesiredLocations: str("Hamburg, Remote"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if err := persona.AddCareerSkill(personaID, s); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := persona.AddCareerExperience(personaID, "Acme", "Backend Developer", "2019-03", "", "Go services with Docker"); err != nil {
		t.Fatal(err)
	}
	return profileID, personaID
}

const matchJobDesc = "Wir suchen Verstärkung in Hamburg. Du entwickelst Backend-Services in Go, betreibst sie mit Docker und Kubernetes " +
	"und arbeitest mit PostgreSQL. Mindestens 3 Jahre Berufserfahrung."

func addMatchJob(t *testing.T, id string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, company, location, description) VALUES (?, ?, 'Backend Engineer (m/w/d)', 'Acme', 'Hamburg', ?)`,
		id, "https://x.example/"+id, matchJobDesc)
}

type matchRow struct {
	profileID, personaID, status, kind string
	score                              int
	breakdown                          *string
}

func readMatch(t *testing.T, jobID string) (matchRow, bool) {
	t.Helper()
	var m matchRow
	var persona, breakdown *string
	var score *int
	err := db.JobsDB.QueryRow(`SELECT profile_id, persona_id, status, kind, score, breakdown_json FROM job_matches WHERE job_id = ?`, jobID).
		Scan(&m.profileID, &persona, &m.status, &m.kind, &score, &breakdown)
	if err != nil {
		return matchRow{}, false
	}
	if persona != nil {
		m.personaID = *persona
	}
	if score != nil {
		m.score = *score
	}
	m.breakdown = breakdown
	return m, true
}

func matchSkills(t *testing.T, jobID string) string {
	t.Helper()
	list, err := GetJobMatchSkills(jobID)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(list, ",")
}

func TestARunWritesTheSameRowsAnAIMatchDoes(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, personaID := seedProfile(t, "Ada", []string{"Go", "Docker", "Kubernetes", "PostgreSQL", "Rust"})

	res, err := RunManualMatch("j1", profileID)
	if err != nil {
		t.Fatal(err)
	}
	if res.PersonaID != personaID || res.Score < 60 || res.Score > 100 || res.Mode != jobmatcher.ModeText {
		t.Fatalf("result = %s persona %s", res.Describe(), res.PersonaID)
	}
	row, ok := readMatch(t, "j1")
	if !ok || row.status != "completed" || row.kind != "manual" || row.score != res.Score || row.profileID != profileID || row.personaID != personaID {
		t.Fatalf("row = %+v", row)
	}
	if row.breakdown == nil {
		t.Fatal("breakdown_json is empty")
	}
	var stored jobmatcher.Result
	if err := json.Unmarshal([]byte(*row.breakdown), &stored); err != nil || stored.Score != res.Score || stored.EngineVersion != jobmatcher.EngineVersion || len(stored.Components) != 5 {
		t.Errorf("stored breakdown = %+v, %v", stored, err)
	}
	if got := matchSkills(t, "j1"); got != "Docker,Go,Kubernetes,PostgreSQL" {
		t.Errorf("matched skills = %q", got)
	}

	// the job payloads carry the engine and the breakdown
	one, err := GetJobByID("j1")
	if err != nil || one.MatchKind != "manual" || len(one.MatchBreakdown) == 0 || one.MatchScore == nil || *one.MatchScore != res.Score || one.MatchStatus != "completed" {
		t.Fatalf("job = kind %q breakdown %d score %v, %v", one.MatchKind, len(one.MatchBreakdown), one.MatchScore, err)
	}
	if strings.Join(one.MatchedSkills, ",") != "Docker,Go,Kubernetes,PostgreSQL" {
		t.Errorf("matchedSkills = %v", one.MatchedSkills)
	}
	list, err := SearchJobs("", "", nil, "", "", 25, 0)
	if err != nil || len(list.Jobs) != 1 || list.Jobs[0].MatchKind != "manual" || len(list.Jobs[0].MatchBreakdown) == 0 {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

func TestARerunReplacesTheResultAndItsSkills(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, personaID := seedProfile(t, "Ada", []string{"Go", "Docker"})
	first, err := RunManualMatch("j1", profileID)
	if err != nil {
		t.Fatal(err)
	}
	if err := persona.AddCareerSkill(personaID, "Kubernetes"); err != nil {
		t.Fatal(err)
	}
	second, err := RunManualMatch("j1", profileID)
	if err != nil || second.Score <= first.Score {
		t.Fatalf("a new skill raises the score: %d -> %d, %v", first.Score, second.Score, err)
	}
	if got := matchSkills(t, "j1"); got != "Docker,Go,Kubernetes" {
		t.Errorf("matched skills = %q", got)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_matches`).Scan(&n)
	if n != 1 {
		t.Errorf("one row per job, found %d", n)
	}
}

func TestAnAIResultTakesTheEngineLabelBack(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, personaID := seedProfile(t, "Ada", []string{"Go", "Docker"})
	if _, err := RunManualMatch("j1", profileID); err != nil {
		t.Fatal(err)
	}
	if err := saveJobMatchResult("j1", profileID, personaID, 42, []string{"Go"}); err != nil {
		t.Fatal(err)
	}
	row, _ := readMatch(t, "j1")
	if row.kind != "ai" || row.breakdown != nil || row.score != 42 {
		t.Errorf("after the AI saved: %+v", row)
	}
	one, _ := GetJobByID("j1")
	if one.MatchKind != "ai" || len(one.MatchBreakdown) != 0 {
		t.Errorf("job = kind %q breakdown %d", one.MatchKind, len(one.MatchBreakdown))
	}
	// and the other way round
	if _, err := RunManualMatch("j1", profileID); err != nil {
		t.Fatal(err)
	}
	if row, _ := readMatch(t, "j1"); row.kind != "manual" || row.breakdown == nil {
		t.Errorf("after a manual run: %+v", row)
	}
	// starting an AI match clears the manual result's label and breakdown too
	if err := StartJobMatch("j1", profileID, "conv-1"); err != nil {
		t.Fatal(err)
	}
	if row, _ := readMatch(t, "j1"); row.kind != "ai" || row.breakdown != nil || row.status != "matching" {
		t.Errorf("after StartJobMatch: %+v", row)
	}
}

func TestRequirementsModeIsUsedWhenAJobHasRequirements(t *testing.T) {
	setupJobsDB(t)
	exec(t, `INSERT INTO jobs (id, source_url, title, location) VALUES ('j1', 'https://x.example/j1', 'Backend Engineer', 'Hamburg')`)
	profileID, _ := seedProfile(t, "Ada", []string{"Go", "Docker"})
	if _, err := SetJobRequirements("j1", []RequirementInput{{"Go", 3}, {"Docker", 2}, {"Terraform", 3}}); err != nil {
		t.Fatal(err)
	}
	res, err := RunManualMatch("j1", profileID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Mode != jobmatcher.ModeRequirements || len(res.Missing) != 1 || res.Missing[0].Term != "Terraform" {
		t.Fatalf("result = %+v", res)
	}
	one, _ := GetJobByID("j1")
	var stored jobmatcher.Result
	if err := json.Unmarshal(one.MatchBreakdown, &stored); err != nil || stored.Mode != jobmatcher.ModeRequirements || len(stored.Missing) != 1 {
		t.Errorf("stored = %+v, %v", stored, err)
	}
}

func TestTheBestPersonaOfTheProfileIsStored(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, _ := seedProfile(t, "Ada", []string{"Go", "Docker"})
	other, err := persona.CreatePersona(profileID, "Teacher", "")
	if err != nil {
		t.Fatal(err)
	}
	persona.UpdatePersonaDetails(persona.UpdatePersonaDetailsArgs{PersonaID: other, DesiredTitles: str("Erzieher")})
	persona.AddCareerSkill(other, "Elternarbeit")
	res, err := RunManualMatch("j1", profileID)
	if err != nil {
		t.Fatal(err)
	}
	if res.PersonaName != "Ada persona" {
		t.Errorf("persona = %q", res.PersonaName)
	}
}

func TestAFailedRunWritesNothingAndKeepsTheEarlierResult(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, _ := seedProfile(t, "Ada", []string{"Go", "Docker"})
	good, err := RunManualMatch("j1", profileID)
	if err != nil {
		t.Fatal(err)
	}

	// a profile with a persona that has nothing to judge
	emptyProfile, _ := profile.CreateProfile("Empty", "Test")
	persona.CreatePersona(emptyProfile, "Blank", "")
	_, err = RunManualMatch("j1", emptyProfile)
	var ins *jobmatcher.InsufficientDataError
	if !errors.As(err, &ins) || len(ins.Reasons) == 0 {
		t.Fatalf("err = %v", err)
	}
	// a profile with no persona at all
	noPersona, _ := profile.CreateProfile("None", "Test")
	if _, err := RunManualMatch("j1", noPersona); !errors.Is(err, jobmatcher.ErrNoPersonas) {
		t.Errorf("err = %v", err)
	}
	row, _ := readMatch(t, "j1")
	if row.score != good.Score || row.profileID != profileID || row.kind != "manual" {
		t.Errorf("a failed run must leave the earlier result: %+v", row)
	}
}

func TestUnknownJobsAndProfilesAreRefused(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	profileID, _ := seedProfile(t, "Ada", []string{"Go"})
	if _, err := RunManualMatch("nope", profileID); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("unknown job: %v", err)
	}
	if _, err := RunManualMatch("j1", "nope"); !errors.Is(err, profile.ErrUnknownProfile) {
		t.Errorf("unknown profile: %v", err)
	}
	if _, ok := readMatch(t, "j1"); ok {
		t.Error("nothing is written for a refused run")
	}
}

func TestTheCorpusIsEveryJobsTitleAndDescription(t *testing.T) {
	setupJobsDB(t)
	addMatchJob(t, "j1")
	for i := 0; i < jobmatcher.CorpusMinJobs; i++ {
		exec(t, `INSERT INTO jobs (id, source_url, title, description) VALUES (?, ?, 'Office Clerk', 'MS Office und Excel täglich')`,
			"office"+string(rune('a'+i)), "https://x.example/office"+string(rune('a'+i)))
	}
	c, err := loadCorpus()
	if err != nil || c.N() != jobmatcher.CorpusMinJobs+1 {
		t.Fatalf("corpus = %d jobs, %v", c.N(), err)
	}
	// with a real corpus a rare skill (Kubernetes: 1 of 21 jobs) weighs more than a common one (MS Office: 20 of 21)
	profileID, _ := seedProfile(t, "Ada", []string{"Kubernetes"})
	rare, err := RunManualMatch("j1", profileID)
	if err != nil {
		t.Fatal(err)
	}
	if rare.Score <= 0 {
		t.Errorf("score = %d", rare.Score)
	}
}
