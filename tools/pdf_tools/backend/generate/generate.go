// Package generate holds the XHTML -> PDF capability's own core logic
// — ported from tools/pdf_generator/backend/main.go's own generatePDF/
// renderPDF/filesHandler as this tool's second capability, in its own
// domain folder alongside convert/ (PDF -> Markdown). See
// plan/ai/tools/pdf-tools/step-05-migrate-pdf-generator-capability.md.
package generate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

// Margins holds the four page-margin values (in inches) a caller may
// ask PDF to apply. The zero value means "no margin on that side" —
// there is no implicit default any more; a margin only ever appears
// because the caller asked for it.
type Margins struct {
	Top    float64
	Bottom float64
	Left   float64
	Right  float64
}

func (m Margins) isZero() bool {
	return m.Top == 0 && m.Bottom == 0 && m.Left == 0 && m.Right == 0
}

// PDF stages xhtml into a real file under tmpDir, points a fresh
// headless-Chrome context at it via a file:// URL, prints it to PDF,
// and stores the result under uploadsDir — exact same mechanism as
// pdf_generator's own generatePDF, renamed to an exported PDF (called
// as generate.PDF from main.go) since it now lives in its own package.
func PDF(tmpDir, uploadsDir, xhtml string, margins Margins) (id string, byteCount int, err error) {
	id = uuid.NewString()
	xhtmlPath := filepath.Join(tmpDir, id+".xhtml")
	staged := xhtml
	if !margins.isZero() {
		staged = withPageMargin(xhtml, margins)
	}
	if err := os.WriteFile(xhtmlPath, []byte(staged), 0o644); err != nil {
		return "", 0, fmt.Errorf("failed to stage xhtml: %w", err)
	}
	defer os.Remove(xhtmlPath)

	absPath, err := filepath.Abs(xhtmlPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve staged xhtml path: %w", err)
	}

	pdfBytes, err := renderPDF(absPath, margins)
	if err != nil {
		return "", 0, err
	}

	pdfPath := filepath.Join(uploadsDir, id+".pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
		return "", 0, fmt.Errorf("failed to store pdf: %w", err)
	}

	return id, len(pdfBytes), nil
}

// ReadFile serves back a previously generated PDF by id — the
// caller-supplied id is validated as a real UUID shape before ever
// touching filepath.Join, since (unlike every other path this tool's
// proxy route narrows by manifest declaration) this one segment is
// genuinely caller-controlled. Exact same validation pdf_generator's
// own filesHandler already did, factored out of the HTTP-specific
// status-code handling (which stays in main.go).
func ReadFile(uploadsDir, id string) ([]byte, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("not found")
	}
	return os.ReadFile(filepath.Join(uploadsDir, id+".pdf"))
}

// withPageMargin only ever runs when the caller explicitly asked for a
// non-zero margin (see PDF above) — there is no implicit default any
// more. It appends our own `@page` rule as the LAST rule in <head>, so
// normal CSS cascade order makes it win over any earlier `@page` rule
// the document supplies, and over Page.printToPDF's own margin params,
// which are silently ignored whenever the document declares its own
// `@page` margin rule — verified empirically against this
// chromedp/Chrome version. Without this, a caller-authored document
// (e.g. an AI-generated CV that resets `@page`/`body` margins) could
// defeat a caller-requested printToPDF margin entirely.
var headCloseTagPattern = regexp.MustCompile(`(?i)</head>`)

func withPageMargin(xhtml string, margins Margins) string {
	override := fmt.Sprintf(
		"<style>@page { margin-top: %sin; margin-bottom: %sin; margin-left: %sin; margin-right: %sin; }</style>",
		strconv.FormatFloat(margins.Top, 'f', -1, 64),
		strconv.FormatFloat(margins.Bottom, 'f', -1, 64),
		strconv.FormatFloat(margins.Left, 'f', -1, 64),
		strconv.FormatFloat(margins.Right, 'f', -1, 64),
	)
	if loc := headCloseTagPattern.FindStringIndex(xhtml); loc != nil {
		return xhtml[:loc[0]] + override + xhtml[loc[0]:]
	}
	// No </head> — a malformed or fragment document. Still apply the
	// override rather than dropping it, inserting after any leading
	// XML declaration so it doesn't break XML parsing by preceding it.
	if end := strings.Index(xhtml, "?>"); strings.HasPrefix(strings.TrimLeft(xhtml, " \t\r\n"), "<?xml") && end != -1 {
		return xhtml[:end+2] + override + xhtml[end+2:]
	}
	return override + xhtml
}

// renderPDF drives a fresh, short-lived headless-Chrome context per
// call — same convention pdf_generator's own renderPDF already
// established. A 30s request-scoped timeout bounds a hung/pathological
// render.
func renderPDF(absXhtmlPath string, margins Margins) ([]byte, error) {
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
			// Explicit on every side, even when zero: Page.printToPDF's
			// own default (when a margin isn't set at all) is 0.4in, not
			// 0 — leaving any of these unset would silently reintroduce
			// a margin nobody asked for.
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithMarginTop(margins.Top).
				WithMarginBottom(margins.Bottom).
				WithMarginLeft(margins.Left).
				WithMarginRight(margins.Right).
				Do(ctx)
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
// (DefaultExecAllocatorOptions) unless PDF_TOOLS_CHROME_PATH is set —
// same escape hatch pdf_generator's own PDF_GENERATOR_CHROME_PATH
// provided, renamed for this tool per
// plan/ai/tools/pdf-tools/step-05-migrate-pdf-generator-capability.md's
// own Open Question 1.
func allocatorOptions() []chromedp.ExecAllocatorOption {
	opts := chromedp.DefaultExecAllocatorOptions[:]
	if p := os.Getenv("PDF_TOOLS_CHROME_PATH"); p != "" {
		opts = append(opts, chromedp.ExecPath(p))
	}
	return opts
}
