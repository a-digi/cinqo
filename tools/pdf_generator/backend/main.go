package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The host's entire contract with this process (tool framework step 4):
// bind 127.0.0.1:$PORT, expose GET /healthz returning 200, implement
// whatever routes this tool's own manifest.json declared. TOOL_TMP_DIR/
// TOOL_UPLOADS_DIR are handed to every tool as env vars but are never
// pre-created by the host — this process creates its own subdirectories
// on startup. See
// plan/ai/tools/pdf-generator/step-02-xhtml-to-pdf-core.md.
//
// A second, additive entrypoint mode: run with a single "--mcp" argv,
// this process instead speaks MCP over stdin/stdout for exactly one
// spawn → exchange → exit cycle (the host never keeps this mode
// running) — the AI-model-facing invocation path. Both modes call the
// exact same generatePDF core; nothing about PDF generation itself
// differs between them. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
func main() {
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
	http.HandleFunc("/generate", generateHandler(tmpDir, uploadsDir))
	http.HandleFunc("/files", filesHandler(uploadsDir))

	log.Printf("pdf_generator listening on 127.0.0.1:%s", port)
	if err := http.ListenAndServe("127.0.0.1:"+port, nil); err != nil {
		log.Fatal(err)
	}
}

type generatePDFArgs struct {
	Xhtml string `json:"xhtml" jsonschema:"the XHTML document to render into a PDF"`
}

// runMCPServer declares this tool's one MCP tool and speaks MCP over
// stdin/stdout until the client (cinqo's own backend, per
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md) closes
// the session — at which point this process exits. InputSchema is
// inferred from generatePDFArgs by AddTool's own reflection, not
// hand-written.
func runMCPServer(tmpDir, uploadsDir string) {
	server := mcp.NewServer(&mcp.Implementation{Name: "pdf_generator", Version: "0.2.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate_pdf",
		Description: "Render an XHTML document into a PDF and return a link to the generated file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args generatePDFArgs) (*mcp.CallToolResult, any, error) {
		if args.Xhtml == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "xhtml is required"}},
				IsError: true,
			}, nil, nil
		}

		id, _, err := generatePDF(tmpDir, uploadsDir, args.Xhtml)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("render failed: %v", err)}},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.ResourceLink{
				URI:      fmt.Sprintf("/api/v1/tools/pdf_generator/proxy/files?id=%s", id),
				Name:     id + ".pdf",
				MIMEType: "application/pdf",
			}},
		}, nil, nil
	})

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}

type generateRequest struct {
	Xhtml string `json:"xhtml"`
}

type generateResponse struct {
	ID    string `json:"id"`
	Bytes int    `json:"bytes"`
}

// generateHandler stages the caller's XHTML and renders it via
// generatePDF — the HTTP surface's own thin wrapper around the same
// core both entrypoint modes share.
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

		id, n, err := generatePDF(tmpDir, uploadsDir, body.Xhtml)
		if err != nil {
			http.Error(w, fmt.Sprintf("render failed: %v", err), http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(generateResponse{ID: id, Bytes: n})
	}
}

// generatePDF stages xhtml into a real file under tmpDir, points a
// fresh headless-Chrome context at it via a file:// URL, prints it to
// PDF, and stores the result under uploadsDir — the exact mechanism
// described in the design: store the code in a file, let Chrome
// convert it. Shared, unchanged core between the HTTP surface
// (generateHandler) and the MCP surface (runMCPServer's tool
// handler).
func generatePDF(tmpDir, uploadsDir, xhtml string) (id string, byteCount int, err error) {
	id = uuid.NewString()
	xhtmlPath := filepath.Join(tmpDir, id+".xhtml")
	if err := os.WriteFile(xhtmlPath, []byte(xhtml), 0o644); err != nil {
		return "", 0, fmt.Errorf("failed to stage xhtml: %w", err)
	}
	defer os.Remove(xhtmlPath)

	absPath, err := filepath.Abs(xhtmlPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve staged xhtml path: %w", err)
	}

	pdfBytes, err := renderPDF(absPath)
	if err != nil {
		return "", 0, err
	}

	pdfPath := filepath.Join(uploadsDir, id+".pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
		return "", 0, fmt.Errorf("failed to store pdf: %w", err)
	}

	return id, len(pdfBytes), nil
}

// filesHandler serves back a previously generated PDF by id — the
// caller-supplied query value is validated as a real UUID shape before
// ever touching filepath.Join, since (unlike every other path this
// tool's proxy route narrows by manifest declaration) this one segment
// is genuinely caller-controlled. Query parameter, not a path segment
// — tool_routes matching is an exact string match on path_suffix, so
// a dynamic /files/{id} route could never match a real request (see
// plan/ai/tools/pdf-generator/step-01-manifest-and-package-skeleton.md's
// own correction).
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

		path := filepath.Join(uploadsDir, id+".pdf")
		data, err := os.ReadFile(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.pdf"`)
		w.Write(data)
	}
}

// renderPDF drives a fresh, short-lived headless-Chrome context per
// call — simpler and crash-safer for a v1 than a pooled/reused browser
// (see the design doc's open questions). A 30s request-scoped timeout
// bounds a hung/pathological render.
func renderPDF(absXhtmlPath string) ([]byte, error) {
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), allocatorOptions()...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	var pdfBytes []byte
	err := chromedp.Run(ctx,
		chromedp.Navigate("file://"+absXhtmlPath),
		chromedp.ActionFunc(func(ctx context.Context) error {
			buf, _, err := page.PrintToPDF().WithPrintBackground(true).Do(ctx)
			if err != nil {
				return err
			}
			pdfBytes = buf
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return pdfBytes, nil
}

// allocatorOptions falls back to chromedp's own default auto-discovery
// (DefaultExecAllocatorOptions) unless PDF_GENERATOR_CHROME_PATH is
// set — an escape hatch for a host where auto-discovery can't find a
// system Chrome/Chromium install. No behavior change for the common
// case. See
// plan/ai/tools/pdf-generator/step-05-packaging-and-deploy.md.
func allocatorOptions() []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if p := os.Getenv("PDF_GENERATOR_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	return opts
}
