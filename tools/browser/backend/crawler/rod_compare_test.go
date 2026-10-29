package crawler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"browser-tool-backend/shared"
)

// compareBudget bounds one side's fetch in the comparison tests.
const compareBudget = 90 * time.Second

// fetchWithChromedpWorker is the existing chromedp path, isolated from the
// engine switch: a headless worker tab + navigateAndReadWithCloudflareCheck,
// exactly what crawlOnWorkerTab does after its (Playwright or rod) first
// attempt. This is the baseline rod is compared against.
func fetchWithChromedpWorker(ctx context.Context, rawURL string) (crawlResponse, error) {
	tabCtx, release, err := shared.AcquireWorkerTab(ctx)
	if err != nil {
		return crawlResponse{}, err
	}
	defer release()
	tctx, cancel := context.WithTimeout(tabCtx, crawlTimeout)
	defer cancel()
	return navigateAndReadWithCloudflareCheck(tctx, rawURL, "", release, cloudflareAutoWait, nil, nil, nil, 0, nil)
}

type sideResult struct {
	resp    crawlResponse
	err     error
	elapsed time.Duration
}

func runSide(fetch func(context.Context, string) (crawlResponse, error), url string) sideResult {
	ctx, cancel := context.WithTimeout(context.Background(), compareBudget)
	defer cancel()
	start := time.Now()
	resp, err := fetch(ctx, url)
	return sideResult{resp: resp, err: err, elapsed: time.Since(start)}
}

func fetchRodSide(ctx context.Context, url string) (crawlResponse, error) {
	return fetchWithRod(ctx, url, "", nil, nil, nil, 0, nil)
}

var whitespaceRun = regexp.MustCompile(`\s+`)

func normalizeHTML(s string) string {
	return strings.TrimSpace(whitespaceRun.ReplaceAllString(s, " "))
}

// firstDifferences compares two HTML strings tag by tag and returns up to
// n human-readable descriptions of where they first diverge.
func firstDifferences(a, b string, n int) []string {
	split := func(s string) []string {
		compact := strings.ReplaceAll(normalizeHTML(s), "> <", "><") // whitespace between tags is not a difference
		return strings.Split(strings.ReplaceAll(compact, "><", ">\n<"), "\n")
	}
	ta, tb := split(a), split(b)
	var out []string
	for i := 0; i < len(ta) || i < len(tb); i++ {
		var x, y string
		if i < len(ta) {
			x = ta[i]
		}
		if i < len(tb) {
			y = tb[i]
		}
		if x != y {
			out = append(out, fmt.Sprintf("#%d  rod: %.110q | chromedp: %.110q", i, x, y))
			if len(out) == n {
				break
			}
		}
	}
	return out
}

func describe(label string, r sideResult) string {
	if r.err != nil {
		return fmt.Sprintf("%-9s ERROR after %s: %v", label, r.elapsed.Round(time.Millisecond), r.err)
	}
	return fmt.Sprintf("%-9s title=%q finalURL=%s html=%dB truncated=%v (%s) passed=%d failed=%d",
		label, r.resp.Title, r.resp.FinalURL, len(r.resp.HTML), r.resp.Truncated, r.elapsed.Round(time.Millisecond),
		strings.Count(strings.ToLower(r.resp.HTML), "(passed)")+strings.Count(strings.ToLower(r.resp.HTML), "passed"),
		strings.Count(strings.ToLower(r.resp.HTML), "(failed)")+strings.Count(strings.ToLower(r.resp.HTML), "failed"))
}

// failedCell matches an element whose tag or text says "failed" (a bot
// check page marks results with class="failed" or a "(failed)" text).
var failedCell = regexp.MustCompile(`(?i)<[^>]*failed[^>]*>[^<]*(?:<[^/][^>]*>[^<]*)?|[^<>]*\(failed\)[^<>]*`)

// failedCells lists up to n table cells that mention "failed" — what a
// bot-detection page marks as a failed check.
func failedCells(html string, n int) []string {
	m := failedCell.FindAllString(html, -1)
	if len(m) > n {
		m = m[:n]
	}
	return m
}

func stopBothEngines(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		StopRodEngine()
		shared.StopSharedSession()
	})
}

// TestRodMatchesChromedpOnALocalPage is a parity check that runs whenever
// Chrome is installed: the same local page through rod and through the
// existing chromedp worker path must produce the same title, final URL and
// (whitespace-normalized) HTML. No network involved.
func TestRodMatchesChromedpOnALocalPage(t *testing.T) {
	requireRodChrome(t)
	stopBothEngines(t)
	srv := rodTestServer(t, map[string]http.HandlerFunc{
		"/final": htmlPage(`<!doctype html><html lang="en"><head><title>Parity page</title><style>p{color:red}</style></head>
			<body class="home"><header><h1>Jobs</h1></header>
			<ul id="list"><li data-id="1"><a href="/job/1?x=1&y=2">Go developer</a> <span>Berlin</span></li>
			<li data-id="2"><a href="/job/2">Rust developer</a> <span>Hamburg</span></li></ul>
			<script>document.getElementById("list").setAttribute("data-built", "js")</script>
			<svg width="10" height="10"><rect/></svg><footer>© Example</footer></body></html>`),
		"/start": func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) },
	})
	url := srv.URL + "/start"

	rod := runSide(fetchRodSide, url)
	chr := runSide(fetchWithChromedpWorker, url)
	t.Log(describe("rod", rod))
	t.Log(describe("chromedp", chr))
	if rod.err != nil || chr.err != nil {
		t.Fatalf("both sides must succeed: rod=%v chromedp=%v", rod.err, chr.err)
	}
	if rod.resp.Title != chr.resp.Title {
		t.Errorf("titles differ: rod %q, chromedp %q", rod.resp.Title, chr.resp.Title)
	}
	if rod.resp.FinalURL != chr.resp.FinalURL {
		t.Errorf("final URLs differ: rod %q, chromedp %q", rod.resp.FinalURL, chr.resp.FinalURL)
	}
	if normalizeHTML(rod.resp.HTML) != normalizeHTML(chr.resp.HTML) {
		for _, d := range firstDifferences(rod.resp.HTML, chr.resp.HTML, 8) {
			t.Log(d)
		}
		t.Errorf("the HTML differs between rod and chromedp")
	}
}

func TestFirstDifferencesHelper(t *testing.T) {
	if d := firstDifferences("<a>1</a><b>2</b>", "<a>1</a> <b>2</b>", 5); len(d) != 0 {
		t.Errorf("whitespace-only differences must not count: %v", d)
	}
	d := firstDifferences("<a>1</a><b>2</b><c>3</c>", "<a>1</a><b>X</b><c>3</c>", 5)
	if len(d) != 1 || !strings.Contains(d[0], "2") {
		t.Errorf("one differing tag expected: %v", d)
	}
}

// TestRodVersusChromedpOnRealURLs is the side-by-side diagnostic from the
// plan (step 49, step 4): set BROWSER_TOOL_COMPARE_URLS to a comma-separated
// list of URLs (e.g. "https://bot.sannysoft.com,https://some-cloudflare-
// site.example") and run
//
//	BROWSER_TOOL_COMPARE_URLS=... go test ./crawler -run RodVersusChromedp -v -count=1
//
// For each URL it fetches with rod and with the existing chromedp worker
// path and LOGS both outcomes (error or title / final URL / HTML size /
// timing / "passed"/"failed" counts) and the first places the HTML
// differs. It asserts nothing about the sites — whether rod does better or
// worse on a real Cloudflare page is for a person to judge from the log.
// Skipped when the variable is unset, so it never touches the network
// during a normal test run.
func TestRodVersusChromedpOnRealURLs(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("BROWSER_TOOL_COMPARE_URLS"))
	if raw == "" {
		t.Skip("set BROWSER_TOOL_COMPARE_URLS=url1,url2 to run the side-by-side diagnostic")
	}
	requireRodChrome(t)
	stopBothEngines(t)

	for _, url := range strings.Split(raw, ",") {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}
		t.Run(url, func(t *testing.T) {
			rod := runSide(fetchRodSide, url)
			chr := runSide(fetchWithChromedpWorker, url)
			t.Log(describe("rod", rod))
			t.Log(describe("chromedp", chr))
			for _, f := range failedCells(rod.resp.HTML, 6) {
				t.Logf("rod      failed cell: %.160s", f)
			}
			for _, f := range failedCells(chr.resp.HTML, 6) {
				t.Logf("chromedp failed cell: %.160s", f)
			}
			if rod.err == nil && chr.err == nil {
				diffs := firstDifferences(rod.resp.HTML, chr.resp.HTML, 10)
				if len(diffs) == 0 {
					t.Log("HTML identical (whitespace-normalized)")
				}
				for _, d := range diffs {
					t.Log(d)
				}
			}
		})
	}
}
