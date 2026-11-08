package catalog

import (
	"reflect"
	"strings"
	"testing"
)

const repoURL = "https://e.com/r.json"

func withRequires(requires string) string {
	return v2(topOK, versionOK+`,"requires":`+requires)
}

func TestARepositoryVersionCanSayWhatItNeeds(t *testing.T) {
	c, err := Parse([]byte(withRequires(`[{"id":"pdf_tools","minVersion":"0.0.1"},{"id":"notes","minVersion":"1.0.0","maxVersion":"1.9.9"},{"id":"scraper"},{"id":"other","maxVersion":"2"}]`)), repoURL)
	if err != nil {
		t.Fatal(err)
	}
	want := []Requirement{
		{ID: "pdf_tools", MinVersion: "0.0.1"},
		{ID: "notes", MinVersion: "1.0.0", MaxVersion: "1.9.9"},
		{ID: "scraper"},
		{ID: "other", MaxVersion: "2"},
	}
	if got := c.Tools[0].Versions[0].Requires; !reflect.DeepEqual(got, want) {
		t.Fatalf("requires, in the order declared: %+v", got)
	}
	// a file without `requires`, or with an empty list, is as valid as before and has none
	for _, raw := range []string{v2(topOK, versionOK), withRequires(`[]`), withRequires(`null`)} {
		c, err := Parse([]byte(raw), repoURL)
		if err != nil || len(c.Tools[0].Versions[0].Requires) != 0 {
			t.Fatalf("%v %v", err, c)
		}
	}
	// a file of the older format (no authors) can carry it too
	legacy := `{"formatVersion":1,"tools":[{"id":"browser","currentVersionId":"v1","versions":[{"versionId":"v1","name":"B","version":"1.0.0","requires":[{"id":"pdf_tools"}],"packages":[{"os":"any","arch":"any","url":"a.zip","sha256":"` + sum + `"}]}]}]}`
	if c, err := Parse([]byte(legacy), repoURL); err != nil || len(c.Tools[0].Versions[0].Requires) != 1 {
		t.Fatalf("%v", err)
	}
}

func TestEveryBadRequirementRejectsTheFileAndNamesIt(t *testing.T) {
	path := "tools[0].versions[0].requires"
	many := make([]string, 21)
	for i := range many {
		many[i] = `{"id":"tool` + strings.Repeat("a", i+1) + `"}`
	}
	for name, tc := range map[string]struct{ requires, path, msg string }{
		"bad id":               {`[{"id":"Bad-Id"}]`, path + "[0].id", "must match"},
		"empty id":             {`[{"id":""}]`, path + "[0].id", "must match"},
		"itself":               {`[{"id":"browser"}]`, path + "[0].id", "cannot require itself"},
		"twice":                {`[{"id":"pdf_tools"},{"id":"pdf_tools","minVersion":"1.0.0"}]`, path + "[1].id", "duplicate"},
		"bad minimum":          {`[{"id":"pdf_tools","minVersion":"1.x"}]`, path + "[0].minVersion", "dotted numbers"},
		"bad maximum":          {`[{"id":"pdf_tools","maxVersion":"beta"}]`, path + "[0].maxVersion", "dotted numbers"},
		"leading dot":          {`[{"id":"pdf_tools","minVersion":".1"}]`, path + "[0].minVersion", "dotted numbers"},
		"maximum below min":    {`[{"id":"pdf_tools","minVersion":"2.0.0","maxVersion":"1.9.9"}]`, path + "[0].maxVersion", "no version could satisfy"},
		"numbers, not text":    {`[{"id":"pdf_tools","minVersion":"10.0.0","maxVersion":"9.0.0"}]`, path + "[0].maxVersion", "no version could satisfy"},
		"the second is wrong":  {`[{"id":"ok_tool"},{"id":"pdf_tools","minVersion":"x"}]`, path + "[1].minVersion", "dotted numbers"},
		"more than 20 entries": {"[" + strings.Join(many, ",") + "]", path, "at most 20"},
	} {
		ps := problems(t, withRequires(tc.requires), repoURL)
		if !hasProblem(ps, tc.path, tc.msg) {
			t.Errorf("%s: want a problem at %s mentioning %q, got %v", name, tc.path, tc.msg, ps)
		}
	}
	// exactly the limit, a maximum equal to the minimum and a maximum above it are fine
	exactly := make([]string, 20)
	for i := range exactly {
		exactly[i] = `{"id":"tool` + strings.Repeat("a", i+1) + `"}`
	}
	if _, err := Parse([]byte(withRequires("["+strings.Join(exactly, ",")+"]")), repoURL); err != nil {
		t.Errorf("20 entries are allowed: %v", err)
	}
	if _, err := Parse([]byte(withRequires(`[{"id":"a","minVersion":"1.0.0","maxVersion":"1.0.0"},{"id":"b","minVersion":"1.0.0","maxVersion":"1.0.1"}]`)), repoURL); err != nil {
		t.Errorf("max equal to min, and above it: %v", err)
	}
}

// The field is a hint with its own name; fields it does not know inside an entry are ignored like everywhere else in the file.
func TestRequiresIgnoresWhatItDoesNotKnow(t *testing.T) {
	c, err := Parse([]byte(withRequires(`[{"id":"pdf_tools","minVersion":"0.0.1","note":"x"}]`)), repoURL)
	if err != nil || c.Tools[0].Versions[0].Requires[0].ID != "pdf_tools" {
		t.Fatalf("%v", err)
	}
}
