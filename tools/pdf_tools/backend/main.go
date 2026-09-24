package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"pdf-tools-backend/convert"
	"pdf-tools-backend/generate"
)

// The host's entire contract with this process (tool framework step
// 4): bind 127.0.0.1:$PORT, expose GET /healthz returning 200,
// implement whatever routes this tool's own manifest.json declared —
// exact same contract pdf_generator's own main.go already documents
// and satisfies.
//
// A second, additive entrypoint mode: run with a single "--mcp" argv,
// this process instead speaks MCP over stdin/stdout for exactly one
// spawn -> exchange -> exit cycle. Both modes call the exact same
// convert.ToMarkdown / generate.PDF cores; nothing about either
// capability differs between the two entrypoints. See
// plan/ai/tools/pdf-tools/step-02-pdf-to-markdown-core.md and
// plan/ai/tools/pdf-tools/step-05-migrate-pdf-generator-capability.md
// (the second capability, ported from tools/pdf_generator).
func main() {
	if err := convert.Init(os.Getenv("TOOL_CACHE_DIR")); err != nil {
		log.Fatalf("failed to init pdf_to_md cache: %v", err)
	}

	// CORE_API_URL is always http://127.0.0.1:<corePort> (manager.go's
	// ToolEnvVars) — the one, narrow, explicit SSRF exception
	// convert.SetAllowedInternalHost documents. A parse failure here
	// just means no exception is set (empty string), not a fatal error
	// — this tool's own core fetch functionality doesn't depend on it.
	if coreAPIURL := os.Getenv("CORE_API_URL"); coreAPIURL != "" {
		if parsed, err := url.Parse(coreAPIURL); err == nil {
			convert.SetAllowedInternalHost(parsed.Host)
		}
	}

	tmpDir := os.Getenv("TOOL_TMP_DIR")
	uploadsDir := os.Getenv("TOOL_UPLOADS_DIR")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		log.Fatalf("failed to create TOOL_TMP_DIR %q: %v", tmpDir, err)
	}
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		log.Fatalf("failed to create TOOL_UPLOADS_DIR %q: %v", uploadsDir, err)
	}

	if len(os.Args) > 1 && os.Args[1] == "--mcp" {
		runMCPServer(tmpDir, uploadsDir)
		return
	}
	runHTTPServer(tmpDir, uploadsDir)
}

func runHTTPServer(tmpDir, uploadsDir string) {
	port := os.Getenv("PORT")

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	http.HandleFunc("/convert-to-markdown", convertHandler)
	http.HandleFunc("/generate", generateHandler(tmpDir, uploadsDir))
	http.HandleFunc("/files", filesHandler(uploadsDir))

	log.Printf("pdf_tools listening on 127.0.0.1:%s", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		log.Fatal(err)
	}
}

type convertArgs struct {
	URL string `json:"url" jsonschema:"the URL of the PDF document to convert to Markdown"`
}

type convertResponse struct {
	Markdown string `json:"markdown"`
	Cached   bool   `json:"cached"`
	MD5      string `json:"md5"`
}

type generatePDFArgs struct {
	Xhtml          string  `json:"xhtml" jsonschema:"the XHTML document to render into a PDF"`
	MarginTopIn    float64 `json:"marginTopIn,omitempty" jsonschema:"optional top page margin in inches; omit for no margin"`
	MarginBottomIn float64 `json:"marginBottomIn,omitempty" jsonschema:"optional bottom page margin in inches; omit for no margin"`
	MarginLeftIn   float64 `json:"marginLeftIn,omitempty" jsonschema:"optional left page margin in inches; omit for no margin"`
	MarginRightIn  float64 `json:"marginRightIn,omitempty" jsonschema:"optional right page margin in inches; omit for no margin"`
}

func (a generatePDFArgs) margins() generate.Margins {
	return generate.Margins{Top: a.MarginTopIn, Bottom: a.MarginBottomIn, Left: a.MarginLeftIn, Right: a.MarginRightIn}
}

// runMCPServer declares this tool's two MCP tools and speaks MCP over
// stdin/stdout until the client closes the session, at which point
// this process exits — exact same shape as pdf_generator's own
// runMCPServer, now with a second mcp.AddTool call for the ported
// generate_pdf capability. InputSchema for each is inferred from its
// own args struct by AddTool's own reflection, not hand-written.
func runMCPServer(tmpDir, uploadsDir string) {
	server := mcp.NewServer(&mcp.Implementation{Name: "pdf_tools", Version: "0.2.0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pdf_to_markdown",
		Description: "Fetch a PDF document by URL and convert it to Markdown text.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args convertArgs) (*mcp.CallToolResult, any, error) {
		if args.URL == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "url is required"}},
				IsError: true,
			}, nil, nil
		}

		markdown, cached, md5Hex, err := convert.ToMarkdown(ctx, args.URL)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("convert failed: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		// The model gets the Markdown text itself as the tool's return
		// content (not a ResourceLink) — you were explicit the agent
		// should receive the text directly, not a pointer to a file it
		// would have to fetch separately. (cached, md5) ride along as
		// the structured `any` return value for diagnostics only.
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: markdown}},
		}, convertResponse{Markdown: markdown, Cached: cached, MD5: md5Hex}, nil
	})

	// The old MANDATORY "verify via pdf_to_markdown, retry up to 2
	// times" instruction that used to live here was written for a world
	// where the AI hand-authored the XHTML itself, character by
	// character — real risk of an unescaped &/<, an unclosed tag, or
	// broken CSS silently producing a garbled PDF. Its one real consumer
	// (Career's "Generate CV PDF") no longer works that way: the AI only
	// supplies structured data to cvbuilder's own render_cv_document,
	// which renders it through Go's html/template — free text fields are
	// auto-escaped by the template engine itself, so that whole failure
	// mode can't occur through this path anymore. Removed rather than
	// kept "just in case" per your own instruction — it was adding a
	// real round trip (generate_pdf -> pdf_to_markdown -> re-render) to
	// every single CV generation for a risk that no longer exists on its
	// only call path. If a FUTURE caller goes back to hand-authoring raw
	// XHTML itself, that caller's own prompt should reintroduce whatever
	// verification it actually needs — this tool no longer mandates one
	// for everyone.
	mcp.AddTool(server, &mcp.Tool{
		Name: "generate_pdf",
		Description: "Render an XHTML document into a PDF and return a link to the generated file. xhtml must be " +
			"well-formed XML (this is checked up front and rejected immediately if not) — if another tool handed you " +
			"already-rendered XHTML (e.g. cvbuilder's render_cv_document), pass it straight through completely " +
			"UNCHANGED; retyping or reformatting it yourself is the most common way to accidentally break its " +
			"escaping (e.g. a bare \"&\" instead of \"&amp;\") and get this call rejected.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args generatePDFArgs) (*mcp.CallToolResult, any, error) {
		if args.Xhtml == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "xhtml is required"}},
				IsError: true,
			}, nil, nil
		}

		id, _, err := generate.PDF(tmpDir, uploadsDir, args.Xhtml, args.margins())
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("render failed: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.ResourceLink{
				URI:      fmt.Sprintf("/api/v1/tools/pdf_tools/proxy/files?id=%s", id),
				Name:     id + ".pdf",
				MIMEType: "application/pdf",
			}},
		}, nil, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

type convertRequest struct {
	URL string `json:"url"`
}

// convertHandler is the HTTP surface's own thin wrapper around the
// same convert.ToMarkdown core the MCP surface calls — no duplicated
// logic between the two surfaces.
func convertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body convertRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}

	markdown, cached, md5Hex, err := convert.ToMarkdown(r.Context(), body.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf("convert failed: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(convertResponse{Markdown: markdown, Cached: cached, MD5: md5Hex})
}

type generateRequest struct {
	Xhtml          string  `json:"xhtml"`
	MarginTopIn    float64 `json:"marginTopIn,omitempty"`
	MarginBottomIn float64 `json:"marginBottomIn,omitempty"`
	MarginLeftIn   float64 `json:"marginLeftIn,omitempty"`
	MarginRightIn  float64 `json:"marginRightIn,omitempty"`
}

func (b generateRequest) margins() generate.Margins {
	return generate.Margins{Top: b.MarginTopIn, Bottom: b.MarginBottomIn, Left: b.MarginLeftIn, Right: b.MarginRightIn}
}

type generateResponse struct {
	ID    string `json:"id"`
	Bytes int    `json:"bytes"`
}

// generateHandler is the HTTP surface's own thin wrapper around the
// same generate.PDF core the MCP surface calls — ported from
// pdf_generator's own generateHandler.
func generateHandler(tmpDir, uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body generateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Xhtml == "" {
			http.Error(w, "xhtml is required", http.StatusBadRequest)
			return
		}

		id, n, err := generate.PDF(tmpDir, uploadsDir, body.Xhtml, body.margins())
		if err != nil {
			http.Error(w, fmt.Sprintf("render failed: %v", err), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generateResponse{ID: id, Bytes: n})
	}
}

// filesHandler serves back a previously generated PDF by id — ported
// from pdf_generator's own filesHandler; generate.ReadFile does the
// UUID validation, this stays responsible only for HTTP status codes.
func filesHandler(uploadsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		id := r.URL.Query().Get("id")
		if _, err := uuid.Parse(id); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		data, err := generate.ReadFile(uploadsDir, id)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="`+id+`.pdf"`)
		w.Write(data)
	}
}
