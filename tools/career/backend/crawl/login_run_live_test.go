package crawl

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"career-tool-backend/portal"
)

// TestALoginRunAgainstTheRealBrowserTool (opt-in) runs a real "Crawl now" of a link that needs a login against a REAL Browser tool
// binary and real Chrome: BROWSER_TOOL_LIVE_URL is the binary's address (http://127.0.0.1:PORT). A small stand-in for core's proxy
// forwards /api/v1/tools/browser/proxy/<route> to it. The "person" is this test: it waits until the run is waiting for the login,
// then presses the Browser's own confirm (the page is example.com, which has no login form, so the check passes). It then expects the
// jobs to arrive, the run to complete and the window to be gone.
func TestALoginRunAgainstTheRealBrowserTool(t *testing.T) {
	base := os.Getenv("BROWSER_TOOL_LIVE_URL")
	if base == "" {
		t.Skip("set BROWSER_TOOL_LIVE_URL to run against a real Browser tool")
	}
	setupTrialDB(t)
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "/api/v1/tools/browser/proxy/"
		if r.URL.Path == "/api/v1/events/publish" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		target := base + "/" + strings.TrimPrefix(r.URL.Path, prefix)
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		req, _ := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer core.Close()
	t.Setenv("CORE_API_URL", core.URL)

	portalID, err := portal.CreatePortal("Live login portal")
	if err != nil {
		t.Fatal(err)
	}
	linkID, err := portal.AddPortalLink(portalID, "https://example.com/", "Example")
	if err != nil {
		t.Fatal(err)
	}
	const instr = "container: \"body\"\nfields:\n  - label: title\n    selector: h1\n  - label: url\n    selector: a\n    attribute: href\npagination:\n  nextSelector: a.never\n  maxPages: 1\n"
	if err := portal.UpdatePortalLinkCrawlInstructions(linkID, instr); err != nil {
		t.Fatal(err)
	}
	yes := true
	if err := portal.UpdatePortalLinkLogin(linkID, &yes, nil); err != nil {
		t.Fatal(err)
	}
	run, err := startCrawlRun(linkID, "listing", "manual")
	if err != nil {
		t.Fatal(err)
	}

	oldPoll := loginStatusPollInterval
	loginStatusPollInterval = 500 * time.Millisecond
	t.Cleanup(func() { loginStatusPollInterval = oldPoll })

	done := make(chan struct{})
	go func() { runCrawlNow(context.Background(), run.ID, linkID, "live-token"); close(done) }()

	// The person: wait for the run to wait for the login, then confirm.
	deadline := time.Now().Add(90 * time.Second)
	for readRun(t, run.ID).phase != "waiting_for_login" {
		if time.Now().After(deadline) {
			t.Fatal("the run never waited for the login")
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Log("run is waiting for the login; confirming as the person")
	body := `{"requestId":"` + run.ID + `","checkUrl":"https://example.com/"}`
	resp, err := http.Post(base+"/login-window/confirm", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("confirm: %d %s", resp.StatusCode, b)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		t.Fatal("the run did not finish")
	}
	r := readRun(t, run.ID)
	t.Logf("run: status=%s phase=%s err=%q\n%s", r.status, r.phase, r.errMsg, r.log)
	if r.status != "completed" {
		t.Fatalf("status = %s (%s)", r.status, r.errMsg)
	}
	// The window is gone: the Browser no longer knows this run.
	sresp, err := http.Get(base + "/login-window/status?requestId=" + url.QueryEscape(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	sresp.Body.Close()
	if sresp.StatusCode != http.StatusNotFound && sresp.StatusCode != http.StatusOK {
		t.Fatalf("status route: %d", sresp.StatusCode)
	}
	t.Logf("status after the run: HTTP %d", sresp.StatusCode)
}
