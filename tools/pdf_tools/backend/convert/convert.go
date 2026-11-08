// Package convert holds the PDF -> Markdown capability's own core
// logic, kept in its own domain folder rather than flat inside
// backend/ — this tool is meant to grow more than one capability (see
// plan/ai/tools/pdf-tools/step-05-migrate-pdf-generator-capability.md,
// which adds a sibling `generate` package for XHTML -> PDF), and each
// one gets its own package rather than one growing main.go.
package convert

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

const (
	// maxPDFSourceBytes bounds memory use against a huge or malicious
	// response — a plain constant, not user-configurable, matching this
	// codebase's own "plain constants over premature configurability"
	// convention (fetch_cache.go's maxEntries, pdf_generator's 30s
	// render timeout).
	maxPDFSourceBytes = 25 * 1024 * 1024

	// fetchTimeout bounds the outbound HTTP fetch itself (connect +
	// read). convertTimeout, below, additionally bounds the whole
	// conversion call.
	fetchTimeout = 30 * time.Second

	// convertTimeout wraps the entire fetch+extract call — mirrors
	// pdf_generator's own 30s chromedp timeout convention, scaled up
	// slightly since this also includes text extraction after the
	// fetch. Note this only actually interrupts the network fetch
	// (context-aware); pdf.Reader's own text extraction is a
	// synchronous, non-context-aware call and cannot be preempted
	// mid-parse by this deadline — a pathological PDF could still run
	// past it. Flagged as a known limitation in the design doc
	// (step-02-pdf-to-markdown-core.md's Security considerations); not
	// solved here since neither dependency exposes a cancellable parse
	// API.
	convertTimeout = 45 * time.Second
)

// ToMarkdown fetches the PDF at rawURL and returns its plain text,
// alongside the MD5 hex of the source bytes and whether the result
// came from the cache. The cache key is the source PDF's own content
// hash, not the URL, so a fetch always happens even on a cache hit —
// only the (potentially much more expensive) text-extraction step is
// skipped. Call convert.Init once at startup before this is ever
// called. See plan/ai/tools/pdf-tools/step-03-content-addressed-cache.md.
//
// Deliberately does NOT use github.com/sukeesh/markitdown-go's own
// ConvertPDFToMarkdown: that function shells out to a separately
// installed `pdfcpu` CLI binary for image extraction, with no
// partial-success path (an image-extraction failure discards the
// already-extracted text too) — see pdf-tools.md's own verified-facts
// section. This calls github.com/ledongthuc/pdf directly instead (the
// same underlying library markitdown-go's own text extraction uses),
// text-only, with no new host/runtime dependency.
func ToMarkdown(ctx context.Context, rawURL string) (markdown string, cached bool, md5Hex string, err error) {
	ctx, cancel := context.WithTimeout(ctx, convertTimeout)
	defer cancel()

	pdfBytes, err := readSource(ctx, rawURL)
	if err != nil {
		return "", false, "", err
	}

	sum := md5.Sum(pdfBytes)
	md5Hex = hex.EncodeToString(sum[:])

	if hit, ok := pdfCacheLookup(md5Hex); ok {
		return hit, true, md5Hex, nil
	}

	text, err := extractText(pdfBytes)
	if err != nil {
		return "", false, "", fmt.Errorf("md5 %s: %w", md5Hex, err)
	}

	// A cache-store failure is a pure optimization-layer problem, not a
	// conversion failure — log it and still return the real result
	// rather than failing the whole call over it.
	if err := pdfCacheStore(md5Hex, rawURL, len(pdfBytes), text); err != nil {
		log.Printf("pdf_tools: failed to store cache entry for %s: %v", md5Hex, err)
	}

	return text, false, md5Hex, nil
}

// extractText mirrors markitdown-go's own unexported extractText
// function (same library, same two calls: pdf.Open then
// r.GetPlainText) — reimplemented directly rather than imported,
// since markitdown-go itself exports no text-only entry point (its one
// exported function always also does image extraction). Reading
// straight from an in-memory io.ReaderAt (bytes.NewReader implements
// it) avoids even needing to stage the PDF to a temp file first, since
// pdf.NewReader takes any io.ReaderAt, not just a real file path.
func extractText(pdfBytes []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("failed to open pdf: %w", err)
	}

	reader, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("failed to extract text: %w", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return "", fmt.Errorf("failed to read extracted text: %w", err)
	}

	return buf.String(), nil
}

// localFileScheme is what the host (cinqo core's own
// conversation/chat.go, invokeToolCall's resolveMediaArgument)
// rewrites a "media:<fileId>" reference into before this tool is ever
// invoked — never a value the AI model or any caller of this MCP tool
// constructs itself. Reading it directly off local disk, rather than
// fetching it over HTTP the way every other rawURL is, is what makes a
// 401 structurally impossible for this path: there's no request to
// authenticate. See plan/ai/media/step-02-career-media-migration.md.
const localFileScheme = "file://"

// readSource dispatches to a local disk read for a file:// reference,
// or the existing SSRF-guarded HTTP fetch for everything else.
func readSource(ctx context.Context, rawURL string) ([]byte, error) {
	if path, ok := strings.CutPrefix(rawURL, localFileScheme); ok {
		return readLocalFile(path)
	}
	return fetchPDF(ctx, rawURL)
}

// readLocalFile enforces the same maxPDFSourceBytes ceiling fetchPDF
// does, just via a stat instead of a LimitReader (no streaming response
// body to bound here).
func readLocalFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read local file: %w", err)
	}
	if info.Size() > maxPDFSourceBytes {
		return nil, fmt.Errorf("pdf exceeds max size of %d bytes", maxPDFSourceBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read local file: %w", err)
	}
	return data, nil
}

// fetchPDF downloads rawURL, enforcing the SSRF hygiene and size cap
// the design doc calls for: only http/https, reject connections to
// loopback/private/link-local addresses (checked against the actually
// resolved+dialed IP, not just the URL's own hostname string, so this
// also covers redirect targets — the same http.Client/Transport is
// reused across redirects), and a hard maxPDFSourceBytes ceiling
// enforced via io.LimitReader.
func fetchPDF(ctx context.Context, rawURL string) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("url must be http or https")
	}

	client := &http.Client{
		Timeout: fetchTimeout,
		Transport: &http.Transport{
			DialContext: dialContextRejectingPrivateIPs,
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	// Some hosts reject Go's default "Go-http-client/..." User-Agent
	// outright (confirmed live against a real host during
	// implementation: 403 with the default UA, 200 with this one) —
	// same UA string tools/browser already uses for exactly this
	// reason, reused here for consistency rather than inventing a
	// second one.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pdf: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed: %s", resp.Status)
	}

	limited := io.LimitReader(resp.Body, maxPDFSourceBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(data) > maxPDFSourceBytes {
		return nil, fmt.Errorf("pdf exceeds max size of %d bytes", maxPDFSourceBytes)
	}

	return data, nil
}

// allowedInternalHost is the one loopback address this tool's own SSRF
// guard makes an explicit, narrow exception for — the host process's
// own CORE_API_URL, which every tool subprocess receives as a fixed,
// trusted env var (never caller/model-controlled) and needs to reach
// for exactly one reason: fetching a CV another tool (career) has made
// available through its own capability-token route. Every other
// loopback/private/link-local target stays rejected exactly as
// before. Empty by default (SetAllowedInternalHost not called, or
// called with an empty value) — dialContextRejectingPrivateIPs then
// behaves identically to before this exception existed. See
// plan/ai/tools/career/import-cv/step-03-pdf-tools-ssrf-exception.md.
var allowedInternalHost string

// SetAllowedInternalHost is called once from main() with
// CORE_API_URL's own parsed host:port — see allowedInternalHost's own
// doc for why this is safe to exempt and nothing else.
func SetAllowedInternalHost(hostPort string) {
	allowedInternalHost = hostPort
}

// dialContextRejectingPrivateIPs is wired into fetchPDF's own
// http.Transport so the SSRF check applies to the address actually
// resolved and dialed, not just the scheme of the URL string — a
// hostname can resolve to a loopback/private address regardless of how
// it's written, and this also covers any redirect Location the same
// client follows.
func dialContextRejectingPrivateIPs(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no addresses found for %s", host)
	}
	for _, ip := range ips {
		resolved := net.JoinHostPort(ip.IP.String(), port)
		if allowedInternalHost != "" && resolved == allowedInternalHost {
			continue // the one explicit, narrow exception — see allowedInternalHost's own doc
		}
		if isDisallowedIP(ip.IP) {
			return nil, fmt.Errorf("refusing to connect to disallowed address %s", ip.IP)
		}
	}

	dialer := &net.Dialer{Timeout: fetchTimeout}
	return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

func isDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
