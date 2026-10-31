package persona

import (
	"errors"
	"strings"
	"testing"
)

const okPersona = `{"name":"Backend","description":"d","possibleDuplicateOf":null,
 "details":{"headline":"h","summary":"s","location":"l","desiredTitles":"t","desiredLocations":"x","minSalary":0},
 "skills":["Go"],"experience":[{"company":"A","title":"B","startDate":"2020","endDate":"","description":""}]}`

func personasJSON(parts ...string) string {
	return `{"personas":[` + strings.Join(parts, ",") + `],"note":""}`
}

func TestParseProposal_Valid(t *testing.T) {
	p, err := ParseProposal([]byte(personasJSON(okPersona)))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Personas) != 1 || p.Personas[0].Name != "Backend" || p.Personas[0].Skills[0] != "Go" || p.Personas[0].Experience[0].Company != "A" {
		t.Fatalf("parsed %+v", p)
	}
}

// An empty list is how the AI says the data is too thin; it is valid, with the reason in note.
func TestParseProposal_EmptyListIsValid(t *testing.T) {
	p, err := ParseProposal([]byte(`{"personas":[],"note":"not enough data"}`))
	if err != nil || len(p.Personas) != 0 || p.Note != "not enough data" {
		t.Fatalf("p=%+v err=%v", p, err)
	}
}

func TestParseProposal_Refusals(t *testing.T) {
	long := strings.Repeat("x", maxShortField+1)
	cases := map[string]string{
		"not json":             `nope`,
		"personas missing":     `{"note":"x"}`,
		"personas null":        `{"personas":null}`,
		"too many":             personasJSON(okPersona, strings.Replace(okPersona, "Backend", "B2", 1), strings.Replace(okPersona, "Backend", "B3", 1), strings.Replace(okPersona, "Backend", "B4", 1)),
		"name missing":         personasJSON(strings.Replace(okPersona, `"name":"Backend"`, `"name":"  "`, 1)),
		"duplicate name":       personasJSON(okPersona, strings.Replace(okPersona, "Backend", "backend", 1)),
		"markup in summary":    personasJSON(strings.Replace(okPersona, `"summary":"s"`, `"summary":"<script>x</script>"`, 1)),
		"name too long":        personasJSON(strings.Replace(okPersona, "Backend", long, 1)),
		"negative salary":      personasJSON(strings.Replace(okPersona, `"minSalary":0`, `"minSalary":-1`, 1)),
		"empty skill":          personasJSON(strings.Replace(okPersona, `"skills":["Go"]`, `"skills":[" "]`, 1)),
		"too many skills":      personasJSON(strings.Replace(okPersona, `"skills":["Go"]`, `"skills":[`+strings.Repeat(`"a",`, maxProposedSkills)+`"b"]`, 1)),
		"role needs a company": personasJSON(strings.Replace(okPersona, `"company":"A"`, `"company":""`, 1)),
		"role needs a title":   personasJSON(strings.Replace(okPersona, `"title":"B"`, `"title":""`, 1)),
		"markup in a role":     personasJSON(strings.Replace(okPersona, `"description":""}]`, `"description":"<b>x</b>"}]`, 1)),
		"note with markup":     `{"personas":[],"note":"<img src=x>"}`,
	}
	for name, raw := range cases {
		_, err := ParseProposal([]byte(raw))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if !errors.Is(err, ErrInvalidProposal) {
			t.Errorf("%s: error %v does not wrap ErrInvalidProposal", name, err)
		}
	}
}

func TestHasMarkup(t *testing.T) {
	for s, want := range map[string]bool{
		"plain text":      false,
		"a < b and c > d": false,
		"5 <3 love":       false,
		"<b>x</b>":        true,
		"x </p>":          true,
		"<!-- c -->":      true,
		"<Script>":        true,
	} {
		if got := hasMarkup(s); got != want {
			t.Errorf("hasMarkup(%q) = %v, want %v", s, got, want)
		}
	}
}
