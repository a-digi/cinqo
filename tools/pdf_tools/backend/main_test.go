package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// extractorStyle is a page the way the extractor writes it: a word to a line, a white-space-only line between the words of a line, and a run
// of them between the lines of the page.
func extractorStyle(visual []string) string {
	var b strings.Builder
	b.WriteString("\n  \n")
	for i, line := range visual {
		words := strings.Fields(line)
		for j, w := range words {
			b.WriteString(w + "\n")
			if j < len(words)-1 {
				b.WriteString(" \n")
			}
		}
		if i < len(visual)-1 {
			b.WriteString(" \n  \n \n\n")
		}
	}
	return b.String() + " \n"
}

var page = []string{
	"~ whoami :", "Jane Example", "KONTAKT", "Email: jane@example.org Anschrift : Hamburg, Deutschland", "KURZPROFIL",
	"> 13+ Jahre Erfahrung als Angestellter in der Softwareentwicklung", "> Umfangreiche Erfahrung als Team und Tech Lead", "ABSCHLÜSSE",
	"2010 Bachelor of Informatik-Ingenieurwesen", "BERUFLICHE FÄHIGKEITEN", "PHP", "TYPESCRIPT", "Go", "Docker, Kubernetes, Terraform",
	"09/2024 - aktuell, München (remote), Deutschland", "Team Lead & Senior Software Developer", "VERSO GmbH",
}

type rig struct {
	t       *testing.T
	session *mcp.ClientSession
	calls   []string
}

func newRig(t *testing.T, conv func(r *rig, url string) (string, bool, string, error)) *rig {
	t.Helper()
	r := &rig{t: t}
	server := newMCPServer(t.TempDir(), t.TempDir(), func(ctx context.Context, rawURL string) (string, bool, string, error) {
		r.calls = append(r.calls, rawURL)
		return conv(r, rawURL)
	})
	ct, st := mcp.NewInMemoryTransports()
	ctx := t.Context()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	r.session = session
	return r
}

func (r *rig) call(name string, args map[string]any) *mcp.CallToolResult {
	r.t.Helper()
	res, err := r.session.CallTool(r.t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		r.t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("%d content items", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T", res.Content[0])
	}
	return tc.Text
}

func TestTheModelGetsTheCleanedText(t *testing.T) {
	raw := extractorStyle(page)
	r := newRig(t, func(*rig, string) (string, bool, string, error) { return raw, true, "abc123", nil })
	res := r.call("pdf_to_markdown", map[string]any{"url": "media:file-1"})
	if res.IsError {
		t.Fatalf("error: %s", text(t, res))
	}
	want := strings.Join(page, "\n")
	if got := text(t, res); got != want {
		t.Fatalf("the model's text:\n got %q\nwant %q", got, want)
	}
	if len(want) >= len(raw)*7/10 {
		t.Errorf("the fixture must be the kind of text the cleaning is for: %d bytes became %d", len(raw), len(want))
	}
	// the structured result carries the same cleaned text, and the diagnostics as they were
	var sc struct {
		Markdown string `json:"markdown"`
		Cached   bool   `json:"cached"`
		MD5      string `json:"md5"`
	}
	raws, _ := json.Marshal(res.StructuredContent)
	if err := json.Unmarshal(raws, &sc); err != nil {
		t.Fatal(err)
	}
	if sc.Markdown != want || !sc.Cached || sc.MD5 != "abc123" {
		t.Errorf("structured result: %+v", sc)
	}
}

func TestTheConverterGetsTheUrlAsGivenAndOrdinaryTextKeepsItsParagraphs(t *testing.T) {
	r := newRig(t, func(*rig, string) (string, bool, string, error) {
		return "Title  \n\n\n\nFirst paragraph,   line one\nline two\n\n \nSecond paragraph\n\n", false, "m", nil
	})
	res := r.call("pdf_to_markdown", map[string]any{"url": "https://example.org/cv.pdf?x=1&y=2"})
	if got, want := text(t, res), "Title\n\nFirst paragraph, line one\nline two\n\nSecond paragraph"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if len(r.calls) != 1 || r.calls[0] != "https://example.org/cv.pdf?x=1&y=2" {
		t.Errorf("the converter was asked for %v", r.calls)
	}
}

func TestAFailureIsReportedAsBeforeAndNeverCleaned(t *testing.T) {
	r := newRig(t, func(*rig, string) (string, bool, string, error) {
		return "", false, "", errors.New("md5 deadbeef:  could not   read\n\n the file ")
	})
	res := r.call("pdf_to_markdown", map[string]any{"url": "media:gone"})
	if !res.IsError || text(t, res) != "convert failed: md5 deadbeef:  could not   read\n\n the file " {
		t.Errorf("error result: %v %q", res.IsError, text(t, res))
	}
}

func TestAnEmptyUrlIsRefusedWithoutConverting(t *testing.T) {
	r := newRig(t, func(*rig, string) (string, bool, string, error) { return "x", false, "", nil })
	res := r.call("pdf_to_markdown", map[string]any{"url": ""})
	if !res.IsError || text(t, res) != "url is required" || len(r.calls) != 0 {
		t.Errorf("%v %q calls=%v", res.IsError, text(t, res), r.calls)
	}
}

func TestAnEmptyDocumentGivesAnEmptyAnswerNotAFailure(t *testing.T) {
	r := newRig(t, func(*rig, string) (string, bool, string, error) { return " \n\n  \n", false, "m", nil })
	res := r.call("pdf_to_markdown", map[string]any{"url": "media:blank"})
	if res.IsError || text(t, res) != "" {
		t.Errorf("%v %q", res.IsError, text(t, res))
	}
}

func TestTheToolsAreDeclaredAsBefore(t *testing.T) {
	r := newRig(t, func(*rig, string) (string, bool, string, error) { return "", false, "", nil })
	tools, err := r.session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*mcp.Tool{}
	for _, tool := range tools.Tools {
		byName[tool.Name] = tool
	}
	if len(byName) != 2 || byName["pdf_to_markdown"] == nil || byName["generate_pdf"] == nil {
		t.Fatalf("tools: %v", byName)
	}
	if byName["pdf_to_markdown"].Description != "Fetch a PDF document by URL and convert it to Markdown text." {
		t.Errorf("description: %q", byName["pdf_to_markdown"].Description)
	}
	schema, _ := json.Marshal(byName["pdf_to_markdown"].InputSchema)
	if !strings.Contains(string(schema), `"url"`) || !strings.Contains(string(schema), `"required":["url"]`) {
		t.Errorf("schema: %s", schema)
	}
}

// What is not cleaned: the cache and the HTTP surface keep the extractor's own text, and only the model's answer is cleaned. These read the
// source because the HTTP handler and the cache cannot be run here without a real PDF.
func TestOnlyTheModelsAnswerIsCleaned(t *testing.T) {
	main, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(main)
	handler := src[strings.Index(src, "func convertHandler"):]
	handler = handler[:strings.Index(handler, "\ntype generateRequest")]
	if strings.Contains(handler, "CleanText") {
		t.Error("the HTTP surface must keep the extractor's text as it was")
	}
	tool := src[strings.Index(src, "func pdfToMarkdown"):]
	tool = tool[:strings.Index(tool, "\n// runMCPServer")]
	if strings.Count(tool, "convert.CleanText(markdown)") != 1 || strings.Index(tool, "convert.CleanText(markdown)") < strings.Index(tool, "conv(ctx, args.URL)") {
		t.Error("the tool must clean the converter's text exactly once, after converting")
	}
	for _, f := range []string{"convert/convert.go", "convert/cache.go"} {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "CleanText(") {
			t.Errorf("%s: the cache keeps the extractor's text, so a better rule never needs a flush", f)
		}
	}
}
