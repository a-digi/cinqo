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
