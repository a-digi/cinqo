package crawler

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The list the user's failing run sent, as a real list and as the string form.
const indeedFields = `[{"label":"title","selector":"h3.jobTitle a"},{"label":"company","selector":"div.companyName"}]`

var indeedWant = []extractField{
	{Label: "title", Selector: "h3.jobTitle a"},
	{Label: "company", Selector: "div.companyName"},
}

func TestFieldListDecoding(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want []extractField
		bad  string // a part the error must contain; empty = no error
	}{
		"array":                        {indeedFields, indeedWant, ""},
		"null":                         {`null`, nil, ""},
		"empty string":                 {`"  "`, nil, ""},
		"JSON list in a string":        {`"` + strings.ReplaceAll(indeedFields, `"`, `\"`) + `"`, indeedWant, ""},
		"attribute and multiple kept":  {`"[{\"label\":\"u\",\"selector\":\"a\",\"attribute\":\"href\",\"multiple\":true}]"`, []extractField{{Label: "u", Selector: "a", Attribute: "href", Multiple: true}}, ""},
		"plain text":                   {`"title: h1"`, nil, "not a JSON list"},
		"broken JSON list in a string": {`"[{\"label\":\"t\""`, nil, "not valid"},
		"number":                       {`5`, nil, "list of objects"},
		"object":                       {`{"label":"t","selector":"h1"}`, nil, "list of objects"},
		"array of strings":             {`["title"]`, nil, "label and a selector"},
	} {
		var got FieldList
		err := json.Unmarshal([]byte(c.in), &got)
		if c.bad == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", name, err)
			} else if !reflect.DeepEqual([]extractField(got), c.want) {
				t.Errorf("%s: got %+v, want %+v", name, got, c.want)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: want an error", name)
		} else if !strings.Contains(err.Error(), c.bad) || !strings.Contains(err.Error(), `{"label": "title", "selector": "h2 a"}`) || !strings.Contains(err.Error(), "not as text") {
			t.Errorf("%s: the message should say %q and show the right shape: %v", name, c.bad, err)
		}
	}
}

// callViaMCP calls one registered tool through a real MCP session, with a stub
// standing in for the browser tool's HTTP side; it returns the request body the
// tool sent on (nil when it sent none).
func callViaMCP(t *testing.T, tool string, register func(*mcp.Server), arguments map[string]any) ([]byte, *mcp.CallToolResult, error) {
	t.Helper()
	var (
		mu       sync.Mutex
		captured []byte
	)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = b
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"notFound":[]}`))
	}))
	t.Cleanup(stub.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(stub.URL, "http://"))
	t.Setenv("TOOL_OWN_PORT", port)

	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	register(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	res, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: arguments})
	mu.Lock()
	defer mu.Unlock()
	return captured, res, callErr
}

func sentFields(t *testing.T, body []byte) []extractField {
	t.Helper()
	var sent struct {
		Fields []extractField `json:"fields"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("the tool sent no readable body: %q (%v)", body, err)
	}
	return sent.Fields
}

func TestFieldsArriveInBothFormsAtBothTools(t *testing.T) {
	tools := map[string]struct {
		register func(*mcp.Server)
		extra    map[string]any
	}{
		"extract_page_data": {RegisterExtractPageData, map[string]any{"container": "div.job"}},
		"extract_from_url":  {RegisterExtractFromURL, map[string]any{"url": "https://example.com/jobs"}},
	}
	for tool, tc := range tools {
		for form, fields := range map[string]any{
			"a real list":   []any{map[string]any{"label": "title", "selector": "h3.jobTitle a"}, map[string]any{"label": "company", "selector": "div.companyName"}},
			"a JSON string": indeedFields,
		} {
			args := map[string]any{"fields": fields}
			for k, v := range tc.extra {
				args[k] = v
			}
			body, res, err := callViaMCP(t, tool, tc.register, args)
			if err != nil || res == nil {
				t.Errorf("%s / %s: the call was rejected: %v", tool, form, err)
				continue
			}
			if got := sentFields(t, body); !reflect.DeepEqual(got, indeedWant) {
				t.Errorf("%s / %s: sent %+v, want %+v", tool, form, got, indeedWant)
			}
		}
	}
}

func TestAStringThatIsNotAListIsAClearErrorAndNothingIsSent(t *testing.T) {
	for tool, tc := range map[string]struct {
		register func(*mcp.Server)
		extra    map[string]any
	}{
		"extract_page_data": {RegisterExtractPageData, nil},
		"extract_from_url":  {RegisterExtractFromURL, map[string]any{"url": "https://example.com/"}},
	} {
		for name, fields := range map[string]any{"plain text": "title: h1", "broken list": `[{"label":"t"`, "a number": 5} {
			args := map[string]any{"fields": fields}
			for k, v := range tc.extra {
				args[k] = v
			}
			body, res, err := callViaMCP(t, tool, tc.register, args)
			text := ""
			if err != nil {
				text = err.Error()
			} else if res != nil && res.IsError && len(res.Content) > 0 {
				if tc, ok := res.Content[0].(*mcp.TextContent); ok {
					text = tc.Text
				}
			} else {
				t.Errorf("%s / %s: should have been rejected: %+v", tool, name, res)
				continue
			}
			if body != nil {
				t.Errorf("%s / %s: nothing should have been sent on", tool, name)
			}
			// A number is turned away by the published schema itself; the others by our own message.
			if name != "a number" && !strings.Contains(text, `{"label": "title", "selector": "h2 a"}`) {
				t.Errorf("%s / %s: the message should show the right shape: %q", tool, name, text)
			}
		}
	}
}

func TestAllowStringKeepsTheExistingTypes(t *testing.T) {
	one := &jsonschema.Schema{Type: "array"}
	allowString(one)
	if one.Type != "" || !reflect.DeepEqual(one.Types, []string{"array", "string"}) {
		t.Errorf("a single type: got %q / %q", one.Type, one.Types)
	}
	many := &jsonschema.Schema{Types: []string{"null", "array"}}
	allowString(many)
	if !reflect.DeepEqual(many.Types, []string{"null", "array", "string"}) {
		t.Errorf("several types: got %q", many.Types)
	}
}

func TestPublishedSchemasAllowAStringForFields(t *testing.T) {
	for tool, build := range map[string]func() (any, error){
		"extract_page_data": func() (any, error) { return withStringFields[extractPageDataArgs]("extract_page_data") },
		"extract_from_url":  func() (any, error) { return withStringFields[extractFromURLArgs]("extract_from_url") },
	} {
		schema, err := build()
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(schema)
		var parsed struct {
			Required   []string `json:"required"`
			Properties map[string]struct {
				Type        any    `json:"type"`
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			t.Fatal(err)
		}
		p, ok := parsed.Properties["fields"]
		types, _ := json.Marshal(p.Type)
		if !ok || !strings.Contains(string(types), `"array"`) || !strings.Contains(string(types), `"string"`) {
			t.Errorf("%s: fields must allow an array and a string, got %s", tool, types)
		}
		if !strings.Contains(p.Description, "one entry per piece of data") || !strings.Contains(p.Description, "not as a string") {
			t.Errorf("%s: fields lost its description or the hint: %q", tool, p.Description)
		}
		if !reflect.DeepEqual(parsed.Required, []string{"fields"}) && !strings.Contains(strings.Join(parsed.Required, ","), "fields") {
			t.Errorf("%s: fields must stay required: %v", tool, parsed.Required)
		}
		if _, ok := parsed.Properties["mapping"]; !ok {
			t.Errorf("%s: the other properties must be unchanged (mapping missing)", tool)
		}
	}
}
