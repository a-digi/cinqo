package jobs

import (
	"errors"
	"strings"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/match/jobmatcher"
)

func addTestJob(t *testing.T, id, title, desc string) {
	t.Helper()
	exec(t, `INSERT INTO jobs (id, source_url, title, description) VALUES (?, ?, ?, ?)`, id, "https://x.example/"+id, title, desc)
}

func termsOf(list []JobRequirement) string {
	var s []string
	for _, r := range list {
		s = append(s, r.Term+":"+string(rune('0'+r.Tier)))
	}
	return strings.Join(s, ",")
}

func TestRequirementsAreSavedReplacedAndReadInOrder(t *testing.T) {
	setupJobsDB(t)
	addTestJob(t, "j1", "Backend Engineer", "")
	if list, err := GetJobRequirements("j1"); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("a new job has an empty, non-nil list: %#v %v", list, err)
	}
	saved, err := SetJobRequirements("j1", []RequirementInput{{"  Go ", 3}, {"Docker", 2}, {"Terraform", 1}})
	if err != nil || termsOf(saved) != "Go:3,Docker:2,Terraform:1" {
		t.Fatalf("saved = %v, %v", saved, err)
	}
	for _, r := range saved {
		if r.ID == "" {
			t.Error("every requirement has an id")
		}
	}
	saved, err = SetJobRequirements("j1", []RequirementInput{{"Kubernetes", 3}, {"Go", 2}})
	if err != nil || termsOf(saved) != "Kubernetes:3,Go:2" {
		t.Fatalf("replaced = %v, %v", saved, err)
	}
	got, _ := GetJobRequirements("j1")
	if termsOf(got) != "Kubernetes:3,Go:2" {
		t.Errorf("read = %v", got)
	}
	if cleared, err := SetJobRequirements("j1", nil); err != nil || len(cleared) != 0 {
		t.Errorf("an empty list clears: %v %v", cleared, err)
	}
}

func TestRequirementsAreValidated(t *testing.T) {
	setupJobsDB(t)
	addTestJob(t, "j1", "Backend Engineer", "")
	if _, err := SetJobRequirements("j1", []RequirementInput{{"Keep me", 3}}); err != nil {
		t.Fatal(err)
	}
	many := make([]RequirementInput, jobmatcher.MaxRequirements+1)
	for i := range many {
		many[i] = RequirementInput{Term: "skill" + strings.Repeat("x", i), Tier: 1}
	}
	cases := map[string][]RequirementInput{
		"empty":    {{"  ", 2}},
		"too long": {{strings.Repeat("a", MaxRequirementTermLength+1), 2}},
		"tier 0":   {{"Go", 0}},
		"tier 4":   {{"Go", 4}},
		"twice":    {{"Go", 3}, {"go", 2}},
		"too many": many,
	}
	for name, in := range cases {
		_, err := SetJobRequirements("j1", in)
		if re, ok := IsRequirementsError(err); !ok || re.Message == "" {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	// the longest allowed term and the largest allowed list pass
	if _, err := SetJobRequirements("j1", []RequirementInput{{strings.Repeat("é", MaxRequirementTermLength), 2}}); err != nil {
		t.Errorf("a term of exactly the limit (in characters) passes: %v", err)
	}
	ok := many[:jobmatcher.MaxRequirements]
	if saved, err := SetJobRequirements("j1", ok); err != nil || len(saved) != jobmatcher.MaxRequirements {
		t.Errorf("a full list passes: %d %v", len(saved), err)
	}
	// a refused list changed nothing
	SetJobRequirements("j1", []RequirementInput{{"Keep me", 3}})
	if _, err := SetJobRequirements("j1", []RequirementInput{{"New", 3}, {"", 1}}); err == nil {
		t.Fatal("refused")
	}
	if got, _ := GetJobRequirements("j1"); termsOf(got) != "Keep me:3" {
		t.Errorf("a refused list must change nothing: %v", got)
	}
}

func TestRequirementsOfAnUnknownJobAreRefused(t *testing.T) {
	setupJobsDB(t)
	if _, err := GetJobRequirements("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("get: %v", err)
	}
	if _, err := SetJobRequirements("nope", nil); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("set: %v", err)
	}
	if _, err := SuggestJobRequirements("nope"); !errors.Is(err, ErrUnknownJob) {
		t.Errorf("suggest: %v", err)
	}
}

func TestDeletingAJobDeletesItsRequirements(t *testing.T) {
	setupJobsDB(t)
	addTestJob(t, "j1", "Backend Engineer", "")
	SetJobRequirements("j1", []RequirementInput{{"Go", 3}})
	if err := DeleteJob("j1"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.JobsDB.QueryRow(`SELECT COUNT(*) FROM job_requirements`).Scan(&n)
	if n != 0 {
		t.Errorf("%d requirements left", n)
	}
}

func TestSuggestionsComeFromTheJobTextAndSkipWhatIsAlreadyListed(t *testing.T) {
	setupJobsDB(t)
	addTestJob(t, "j1", "Platform Engineer", "Kubernetes Cluster betreiben. Kubernetes und Terraform, Terraform Module.")
	got, err := SuggestJobRequirements("j1")
	if err != nil || !contains(got, "Kubernetes") || !contains(got, "Terraform") {
		t.Fatalf("suggestions = %v, %v", got, err)
	}
	SetJobRequirements("j1", []RequirementInput{{"Kubernetes", 3}})
	got, _ = SuggestJobRequirements("j1")
	if contains(got, "Kubernetes") || !contains(got, "Terraform") {
		t.Errorf("suggestions = %v", got)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
