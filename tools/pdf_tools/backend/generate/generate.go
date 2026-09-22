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

// PDF stages xhtml into a real file under tmpDir, points a fresh
// headless-Chrome context at it via a file:// URL, prints it to PDF,
// and stores the result under uploadsDir — exact same mechanism as
// pdf_generator's own generatePDF, renamed to an exported PDF (called
// as generate.PDF from main.go) since it now lives in its own package.
func PDF(tmpDir, uploadsDir, xhtml string) (id string, byteCount int, err error) {
	id = uuid.NewString()
	xhtmlPath := filepath.Join(tmpDir, id+".xhtml")
	if err := os.WriteFile(xhtmlPath, []byte(withDefaultPageMargin(xhtml)), 0o644); err != nil {
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

// defaultMarginInches is the top/bottom page margin (in inches)
// applied to every generated PDF, regardless of what the caller's own
// XHTML/CSS says.
//
// This needs two layers, not one — verified empirically against this
// chromedp/Chrome version, since the two documented mechanisms don't
// agree with real behavior in isolation:
//   - Page.printToPDF's own WithMarginTop/WithMarginBottom (set in
//     renderPDF below) is the baseline: it's honored whenever the
//     document has no `@page` margin rule of its own.
//   - But when the document DOES declare its own `@page { margin: 0 }`
//     (or similar), that CSS rule wins outright — printToPDF's margin
//     params are silently ignored. So a caller-authored document (e.g.
//     an AI-generated CV that resets `@page`/`body` margins to make a
//     full-bleed header) can defeat the printToPDF margin entirely.
//
// withDefaultPageMargin below closes that gap by appending our own
// `@page` rule as the LAST rule in <head>, so normal CSS cascade order
// makes it win over any earlier `@page` rule the document supplies —
// making the margin genuinely independent of the caller's own HTML/CSS,
// not just the common case.
const defaultMarginInches = 0.4

var headCloseTagPattern = regexp.MustCompile(`(?i)</head>`)

// withDefaultPageMargin appends a `@page` rule pinning the top/bottom
// margin, inserted immediately before the document's own </head> so it
// is the last-declared @page rule and therefore wins the cascade over
// any @page margin rule already in the document. If no </head> is
// found (a malformed/fragment document), the override is prepended
// instead so it still applies rather than being silently dropped.
func withDefaultPageMargin(xhtml string) string {
	override := fmt.Sprintf(
		"<style>@page { margin-top: %sin; margin-bottom: %sin; }</style>",
		strconv.FormatFloat(defaultMarginInches, 'f', -1, 64),
		strconv.FormatFloat(defaultMarginInches, 'f', -1, 64),
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
			buf, _, err := page.PrintToPDF().
				WithPrintBackground(true).
				WithMarginTop(defaultMarginInches).
				WithMarginBottom(defaultMarginInches).
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
