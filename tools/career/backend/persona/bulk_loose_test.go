package persona

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The exact texts the free model sent in the live pass (the trace of conversation 81c64211…, 2026-11-05).
const (
	modelSkillsAsJSONText    = `["Java", "Spring Boot", "Kubernetes", "PostgreSQL", "Kafka", "Docker", "AWS", "Terraform", "Microservices", "REST APIs", "JUnit", "CI/CD"]`
	modelSkillsAsQuotedComma = `"Java, Spring Boot, Kubernetes, PostgreSQL, Kafka, Docker, AWS, Terraform, Microservices, REST APIs, JUnit, CI/CD"`
	modelExperiencesAsText   = `[{"company": "Acme Software GmbH", "title": "Senior Java Developer", "startDate": "2021-03", "description": "Led a team building Spring Boot microservices."}, {"company": "Beta Systems AG", "title": "Java Developer", "startDate": "2017-06", "endDate": "2021-02", "description": "Built REST APIs and Kafka consumers."}, {"company": "Gamma Labs", "title": "Junior Developer", "startDate": "2014-09", "endDate": "2017-05", "description": "Maintained Java backends."}]`
)

var twelveSkills = []string{"Java", "Spring Boot", "Kubernetes", "PostgreSQL", "Kafka", "Docker", "AWS", "Terraform", "Microservices", "REST APIs", "JUnit", "CI/CD"}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestSkillsAreReadFromAListOrFromATextHoldingIt(t *testing.T) {
	for name, c := range map[string]struct {
		in   string // the raw JSON of the argument
		want []string
	}{
		"a list":                                {`["Go","SQL"]`, []string{"Go", "SQL"}},
		"a list keeps a skill with a comma":     {`["C, C++","Go"]`, []string{"C, C++", "Go"}},
		"an empty list":                         {`[]`, []string{}},
		"null":                                  {`null`, nil},
		"a JSON list in a text (the model)":     {jsonOf(t, modelSkillsAsJSONText), twelveSkills},
		"a comma text in quotes (the model)":    {jsonOf(t, modelSkillsAsQuotedComma), twelveSkills},
		"a comma text":                          {`"Go, SQL, Rust"`, []string{"Go", "SQL", "Rust"}},
		"a semicolon text":                      {`"Go; SQL;Rust"`, []string{"Go", "SQL", "Rust"}},
		"a line text":                           {`"Go\nSQL\r\nRust"`, []string{"Go", "SQL", "Rust"}},
		"mixed separators":                      {`"Go, SQL;Rust\nC"`, []string{"Go", "SQL", "Rust", "C"}},
		"a trailing comma and blank items drop": {`"Go, , SQL,\n"`, []string{"Go", "SQL"}},
		"white space around items":              {`"  Go ,   SQL  "`, []string{"Go", "SQL"}},
		"a single skill as text":                {`"Go"`, []string{"Go"}},
		"an empty text":                         {`""`, []string{}},
		"a text of only separators":             {`" , ; "`, []string{}},
		"a list wrapped twice":                  {jsonOf(t, jsonOf(t, `["Go","SQL"]`)), []string{"Go", "SQL"}},
		"a list wrapped three times":            {jsonOf(t, jsonOf(t, jsonOf(t, `["Go","SQL"]`))), []string{"Go", "SQL"}},
		"a comma text wrapped three times":      {jsonOf(t, jsonOf(t, jsonOf(t, "Go, SQL"))), []string{"Go", "SQL"}},
		"a lone carriage return separates":      {`"Go\rSQL\rRust"`, []string{"Go", "SQL", "Rust"}},
		"a JSON list in a text with spaces":     {jsonOf(t, `  ["Go", "SQL"]  `), []string{"Go", "SQL"}},
	} {
		var got LooseStrings
		if err := json.Unmarshal([]byte(c.in), &got); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !reflect.DeepEqual([]string(got), c.want) {
			t.Errorf("%s: got %#v, want %#v", name, []string(got), c.want)
		}
	}
}

func TestSkillsThatAreNotAListOfStringsAreRefusedWithAMessageSayingWhatWasExpected(t *testing.T) {
	for name, in := range map[string]string{
		"a number":                  `5`,
		"a boolean":                 `true`,
		"an object":                 `{"skill":"Go"}`,
		"a list of numbers":         `[1,2]`,
		"a list with an object":     `["Go",{"a":1}]`,
		"a JSON list of numbers":    `"[1,2]"`,
		"a text with a broken list": `"[\"Go\", "`,
		"a broken list":             `["Go"`,
	} {
		var got LooseStrings
		err := json.Unmarshal([]byte(in), &got)
		if err == nil {
			t.Errorf("%s: want an error, got %#v", name, []string(got))
			continue
		}
		if !strings.Contains(err.Error(), "expected a list of strings") && !strings.Contains(err.Error(), "unexpected end") {
			t.Errorf("%s: the message must say what was expected: %v", name, err)
		}
	}
}

func TestExperiencesAreReadFromAListOrFromATextHoldingIt(t *testing.T) {
	one := []BulkExperience{{Company: "Acme", Title: "Dev", StartDate: "2020", EndDate: "2021", Description: "d"}}
	asList := `[{"company":"Acme","title":"Dev","startDate":"2020","endDate":"2021","description":"d"}]`
	for name, c := range map[string]struct {
		in   string
		want []BulkExperience
	}{
		"a list":                         {asList, one},
		"an empty list":                  {`[]`, []BulkExperience{}},
		"null":                           {`null`, nil},
		"a JSON list in a text":          {jsonOf(t, asList), one},
		"a JSON list in extra quotes":    {jsonOf(t, `"`+strings.ReplaceAll(asList, `"`, `\"`)+`"`), one},
		"a list wrapped twice":           {jsonOf(t, jsonOf(t, asList)), one},
		"a list wrapped three times":     {jsonOf(t, jsonOf(t, jsonOf(t, asList))), one},
		"white space around the text":    {jsonOf(t, "  \n"+asList+"\n "), one},
		"the text the model sent (live)": {jsonOf(t, modelExperiencesAsText), nil}, // checked below
	} {
		var got LooseExperiences
		if err := json.Unmarshal([]byte(c.in), &got); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if name == "the text the model sent (live)" {
			if len(got) != 3 || got[0].Company != "Acme Software GmbH" || got[1].EndDate != "2021-02" || got[2].Title != "Junior Developer" || got[2].Description != "Maintained Java backends." {
				t.Errorf("%s: %+v", name, got)
			}
			continue
		}
		if !reflect.DeepEqual([]BulkExperience(got), c.want) {
			t.Errorf("%s: got %#v, want %#v", name, []BulkExperience(got), c.want)
		}
	}
}

func TestExperiencesThatAreNotAListOfObjectsAreRefusedWithAMessageSayingWhatWasExpected(t *testing.T) {
	for name, in := range map[string]string{
		"a number":                      `5`,
		"an object, not a list":         `{"company":"Acme","title":"Dev"}`,
		"a list of strings":             `["Acme"]`,
		"a list of numbers":             `[1]`,
		"free text":                     `"Acme, Senior Developer, 2020"`,
		"a text that is not a list":     `"{\"company\":\"Acme\"}"`,
		"a text with a broken list":     `"[{\"company\": "`,
		"a misspelt field in a list":    `[{"company":"Acme","title":"Dev","start":"2020"}]`,
		"a misspelt field in a text":    `"[{\"company\":\"Acme\",\"title\":\"Dev\",\"start\":\"2020\"}]"`,
		"a field of the wrong type":     `[{"company":"Acme","title":"Dev","startDate":2020}]`,
		"a field of the wrong type (t)": `"[{\"company\":\"Acme\",\"title\":\"Dev\",\"startDate\":2020}]"`,
	} {
		var got LooseExperiences
		err := json.Unmarshal([]byte(in), &got)
		if err == nil {
			t.Errorf("%s: want an error, got %#v", name, []BulkExperience(got))
			continue
		}
		if !strings.Contains(err.Error(), "expected a list of experience objects") {
			t.Errorf("%s: the message must say what was expected: %v", name, err)
		}
		if name == "free text" || name == "a text that is not a list" {
			// text that is not a JSON list is refused with the expectation alone, without a JSON parser's own words
			if strings.Contains(err.Error(), "invalid character") || strings.Contains(err.Error(), "cannot unmarshal") {
				t.Errorf("%s: the message must not carry the parser's text: %v", name, err)
			}
		}
	}
}

func TestATextWrappedInTooManyLayersIsGivenUpOn(t *testing.T) {
	text := `["Go"]`
	for i := 0; i < maxTextLayers+2; i++ {
		text = jsonOf(t, text)
	}
	var got LooseStrings
	// the outermost layer is the JSON value itself; one more than the layers that are taken off leaves a quoted text, which reads as one item
	if err := json.Unmarshal([]byte(text), &got); err == nil && len(got) == 1 && got[0] == "Go" {
		t.Errorf("a text wrapped %d times must not be unwrapped for ever: %#v", maxTextLayers+2, []string(got))
	}
}

// The two tools through a real in-memory MCP session, with the texts the free model sent.
func TestTheBulkToolsAcceptTheTextsTheFreeModelSent(t *testing.T) {
	_, id := newBulkPersona(t)
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterAddCareerSkills(server)
	RegisterAddCareerExperiences(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	call := func(tool string, args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		text := ""
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				text += tc.Text
			}
		}
		return text, res.IsError
	}

	// the first call of the live pass: the JSON list in a text
	text, isErr := call("add_career_skills", map[string]any{"personaId": id, "skills": modelSkillsAsJSONText})
	if isErr {
		t.Fatalf("a JSON list in a text must be read: %s", text)
	}
	var skills BulkSkillsResult
	if err := json.Unmarshal([]byte(text), &skills); err != nil || !reflect.DeepEqual(skills.Added, twelveSkills) {
		t.Fatalf("%s %v", text, err)
	}
	// the third call of the live pass: the comma text in extra quotes (every skill is there already: all skipped)
	text, isErr = call("add_career_skills", map[string]any{"personaId": id, "skills": modelSkillsAsQuotedComma})
	if isErr || !strings.Contains(text, `"added": []`) {
		t.Fatalf("a quoted comma text must be read: %v %s", isErr, text)
	}
	if got := ownSkills(t, id); !reflect.DeepEqual(got, twelveSkills) {
		t.Errorf("stored %v", got)
	}
	// a plain list still works, and a bad shape is refused with the expectation
	if text, isErr := call("add_career_skills", map[string]any{"personaId": id, "skills": []string{"Rust"}}); isErr {
		t.Errorf("a list: %s", text)
	}
	if text, isErr := call("add_career_skills", map[string]any{"personaId": id, "skills": 5}); !isErr || !strings.Contains(text, "skills") {
		t.Errorf("a number: %v %s", isErr, text)
	}
	if text, isErr := call("add_career_skills", map[string]any{"personaId": id, "skills": ""}); !isErr || !strings.Contains(text, "skills is empty") {
		t.Errorf("an empty text is an empty list: %v %s", isErr, text)
	}

	// the first call of the experiences turn: the JSON list in a text
	text, isErr = call("add_career_experiences", map[string]any{"personaId": id, "experiences": modelExperiencesAsText})
	if isErr {
		t.Fatalf("a JSON list in a text must be read: %s", text)
	}
	var out BulkExperiencesResult
	if err := json.Unmarshal([]byte(text), &out); err != nil || len(out.Added) != 3 {
		t.Fatalf("%s %v", text, err)
	}
	want := []storedRole{
		{"Acme Software GmbH", "Senior Java Developer", "2021-03", "", "Led a team building Spring Boot microservices."},
		{"Beta Systems AG", "Java Developer", "2017-06", "2021-02", "Built REST APIs and Kafka consumers."},
		{"Gamma Labs", "Junior Developer", "2014-09", "2017-05", "Maintained Java backends."},
	}
	if got := ownRoles(t, id); !reflect.DeepEqual(got, want) {
		t.Errorf("stored %+v", got)
	}
	if text, isErr := call("add_career_experiences", map[string]any{"personaId": id, "experiences": "Acme, Dev, 2020"}); !isErr || !strings.Contains(text, "expected a list of experience objects") {
		t.Errorf("free text: %v %s", isErr, text)
	}
	if text, isErr := call("add_career_experiences", map[string]any{"personaId": id, "experiences": []map[string]any{{"company": "Delta", "title": "Dev", "bogus": 1}}}); !isErr {
		t.Errorf("an unknown field in a list must still be refused: %s", text)
	}
	if got := len(ownRoles(t, id)); got != 3 {
		t.Errorf("the refused calls must add nothing: %d", got)
	}
}

// The tools' schemas say that a text is accepted: this is what a model reads when it chooses its arguments.
func TestTheSchemasSayThatATextIsAcceptedToo(t *testing.T) {
	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1"}, nil)
	RegisterAddCareerSkills(server)
	RegisterAddCareerExperiences(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		raw, _ := json.Marshal(tool.InputSchema)
		var sch struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Description string `json:"description"`
				AnyOf       []struct {
					Type        any    `json:"type"`
					Items       any    `json:"items"`
					Description string `json:"description"`
				} `json:"anyOf"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &sch); err != nil {
			t.Fatal(err)
		}
		prop := "skills"
		if tool.Name == "add_career_experiences" {
			prop = "experiences"
		}
		p, ok := sch.Properties[prop]
		if !ok || len(p.AnyOf) != 2 || !reflect.DeepEqual(p.AnyOf[1].Type, "string") || p.AnyOf[0].Items == nil {
			t.Errorf("%s: %s must be a list or a string: %s", tool.Name, prop, raw)
		}
		if !strings.Contains(p.Description, "holding") {
			t.Errorf("%s: the description must tell that a text holding the list is read too: %q", tool.Name, p.Description)
		}
		if len(p.AnyOf) == 2 && p.AnyOf[0].Description != "" {
			t.Errorf("%s: the description belongs on the property, not repeated on the list branch: %q", tool.Name, p.AnyOf[0].Description)
		}
		if !reflect.DeepEqual(sch.Required, []string{"personaId", prop}) {
			t.Errorf("%s: required = %v", tool.Name, sch.Required)
		}
		if _, ok := sch.Properties["personaId"]; !ok {
			t.Errorf("%s: personaId is missing from the schema", tool.Name)
		}
	}
	if len(list.Tools) != 2 {
		t.Errorf("tools = %d", len(list.Tools))
	}
}
