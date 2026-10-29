package crawler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestSelectorListDecoding(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want []string
		bad  bool
	}{
		"array":                         {`["script","style"]`, []string{"script", "style"}, false},
		"array with a comma string":     {`["script,style,.banner"]`, []string{"script,style,.banner"}, false},
		"null":                          {`null`, nil, false},
		"empty string":                  {`"   "`, nil, false},
		"comma string is one selector":  {`"script, style, .cookie-banner"`, []string{"script, style, .cookie-banner"}, false},
		"selector with its own comma":   {`".card:is(.a, .b)"`, []string{".card:is(.a, .b)"}, false},
		"JSON array in a string":        {`"[\"script\", \"style\"]"`, []string{"script", "style"}, false},
		"broken JSON array in a string": {`"[\"script\", \"style\""`, nil, true},
		"number":                        {`123`, nil, true},
		"object":                        {`{"a":1}`, nil, true},
		"array of numbers":              {`[1,2]`, nil, true},
	} {
		var got SelectorList
		err := json.Unmarshal([]byte(c.in), &got)
		if (err != nil) != c.bad {
			t.Errorf("%s: err = %v, want error %v", name, err, c.bad)
			continue
		}
		if !c.bad && !reflect.DeepEqual([]string(got), c.want) {
			t.Errorf("%s: got %q, want %q", name, []string(got), c.want)
		}
	}
}

func TestAttributeNameListDecoding(t *testing.T) {
	for name, c := range map[string]struct {
		in   string
		want []string
		bad  bool
	}{
		"array":                  {`["style","onclick"]`, []string{"style", "onclick"}, false},
		"comma string":           {`"style,onclick,data-testid"`, []string{"style", "onclick", "data-testid"}, false},
		"comma and spaces":       {`"style, onclick  data-testid"`, []string{"style", "onclick", "data-testid"}, false},
		"single name":            {`"style"`, []string{"style"}, false},
		"empty string":           {`""`, nil, false},
		"JSON array in a string": {`"[\"style\",\"onclick\"]"`, []string{"style", "onclick"}, false},
		"broken":                 {`"[\"style\""`, nil, true},
		"number":                 {`5`, nil, true},
	} {
		var got AttributeNameList
		err := json.Unmarshal([]byte(c.in), &got)
		if (err != nil) != c.bad {
			t.Errorf("%s: err = %v, want error %v", name, err, c.bad)
			continue
		}
		if !c.bad && !reflect.DeepEqual([]string(got), c.want) {
			t.Errorf("%s: got %q, want %q", name, []string(got), c.want)
		}
	}
}

// fetchViaMCP calls the registered fetch_page_html tool through a real MCP
// session, with a stub standing in for the browser tool's HTTP side, and
// returns the /crawl request the tool sent (nil when it sent none).
func fetchViaMCP(t *testing.T, arguments map[string]any) (*crawlRequest, *mcp.CallToolResult, error) {
	t.Helper()
	var (
		mu       sync.Mutex
		captured *crawlRequest
	)
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req crawlRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		captured = &req
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"html":"<p>ok</p>","title":"T","finalUrl":"https://x/","truncated":false}`))
	}))
	t.Cleanup(stub.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(stub.URL, "http://"))
	t.Setenv("TOOL_OWN_PORT", port)

	server := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "0"}, nil)
	RegisterFetchPageHTML(server)
	ct, st := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })

	res, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "fetch_page_html", Arguments: arguments})
	mu.Lock()
	defer mu.Unlock()
	return captured, res, callErr
}

func TestListArgumentsArriveInEveryAcceptedForm(t *testing.T) {
	for name, c := range map[string]struct {
		args           map[string]any
		wantSelectors  []string
		wantAttributes []string
	}{
		"arrays, unchanged": {
			map[string]any{"url": "https://x/", "removeSelectors": []any{"header", ".banner"}, "removeAttributes": []any{"style", "onclick"}},
			[]string{"header", ".banner"}, []string{"style", "onclick"},
		},
		"array holding one comma string": {
			map[string]any{"url": "https://x/", "removeSelectors": []any{"script,style,header"}},
			[]string{"script,style,header"}, nil,
		},
		"comma-separated strings": {
			map[string]any{"url": "https://x/", "removeSelectors": "script, style, .cookie-banner", "removeAttributes": "style,onclick,data-testid"},
			[]string{"script, style, .cookie-banner"}, []string{"style", "onclick", "data-testid"},
		},
		"JSON arrays inside strings": {
			map[string]any{"url": "https://x/", "removeSelectors": `["script", "style"]`, "removeAttributes": `["style"]`},
			[]string{"script", "style"}, []string{"style"},
		},
	} {
		req, res, err := fetchViaMCP(t, c.args)
		if err != nil || res.IsError || req == nil {
			t.Errorf("%s: call failed: err=%v res=%+v", name, err, res)
			continue
		}
		if !reflect.DeepEqual(req.RemoveSelectors, c.wantSelectors) || !reflect.DeepEqual(req.RemoveAttributes, c.wantAttributes) {
			t.Errorf("%s: sent selectors %q attributes %q, want %q / %q", name, req.RemoveSelectors, req.RemoveAttributes, c.wantSelectors, c.wantAttributes)
		}
	}
}

func TestAMalformedListStringIsAClearErrorAndNothingIsFetched(t *testing.T) {
	req, res, err := fetchViaMCP(t, map[string]any{"url": "https://x/", "removeSelectors": `["script", "style"`})
	text := ""
	switch {
	case err != nil:
		text = err.Error()
	case res != nil && res.IsError:
		text = res.Content[0].(*mcp.TextContent).Text
	default:
		t.Fatalf("a malformed list must fail: %+v", res)
	}
	if !strings.Contains(text, "removeSelectors") || !strings.Contains(text, "not valid JSON") {
		t.Errorf("the message should name the argument and say what to do: %q", text)
	}
	if req != nil {
		t.Error("nothing should have been fetched")
	}
}

func TestANumberIsStillRejected(t *testing.T) {
	req, res, err := fetchViaMCP(t, map[string]any{"url": "https://x/", "removeAttributes": 5})
	if err == nil && (res == nil || !res.IsError) {
		t.Fatalf("a number is not a list: %+v", res)
	}
	if req != nil {
		t.Error("nothing should have been fetched")
	}
}

func TestPublishedSchemaAllowsStringsForBothLists(t *testing.T) {
	schema, err := fetchPageHTMLSchema()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(schema)
	var parsed struct {
		Properties map[string]struct {
			Type        any    `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"removeSelectors", "removeAttributes"} {
		p, ok := parsed.Properties[name]
		if !ok {
			t.Fatalf("no %s property", name)
		}
		types, _ := json.Marshal(p.Type)
		if !strings.Contains(string(types), `"string"`) || !strings.Contains(string(types), `"array"`) {
			t.Errorf("%s must allow an array and a string, got %s", name, types)
		}
		if p.Description == "" {
			t.Errorf("%s lost its description", name)
		}
	}
	if _, ok := parsed.Properties["offset"]; !ok {
		t.Error("the other properties must be unchanged (offset missing)")
	}
}
