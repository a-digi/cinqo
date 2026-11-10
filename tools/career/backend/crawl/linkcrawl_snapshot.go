// linkcrawl_snapshot.go (plan 247, step 2) — the page copy the Job list picker shows.
//
// The Browser tool makes the copy (POST /fetch-links with "snapshot": true) and keeps it in its own temporary folder; Career only forwards: the
// candidates route asks for it, and the four routes below read and delete it through core's proxy with the person's own session. Career keeps no copy and
// no record of one. The id is checked here as well as in the Browser tool, so nothing but 32 hex characters is ever put into an address.
package crawl

import (
	"context"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	browserSnapshotPath  = "/api/v1/tools/browser/proxy/page-snapshot"
	browserSnapshotsPath = "/api/v1/tools/browser/proxy/page-snapshots"
	// maxSnapshotRelay bounds the copy that is relayed (the Browser tool keeps it under 8 MB of html).
	maxSnapshotRelay = 16 << 20
)

var snapshotIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// PageSnapshot is what the Browser tool reports about the copy it made.
type PageSnapshot struct {
	ID        string `json:"id"`
	Bytes     int    `json:"bytes"`
	CreatedAt string `json:"createdAt"`
	Truncated bool   `json:"truncated,omitempty"`
}

type snapshotRequestKey struct{}

// withSnapshotRequest asks the next link fetch to also make a page copy.
func withSnapshotRequest(ctx context.Context, want bool) context.Context {
	if !want {
		return ctx
	}
	return context.WithValue(ctx, snapshotRequestKey{}, true)
}

func snapshotRequested(ctx context.Context) bool {
	v, _ := ctx.Value(snapshotRequestKey{}).(bool)
	return v
}

// relayToBrowser sends method to the Browser tool's url with the person's session and answers with the response; the caller closes the body.
func relayToBrowser(r *http.Request, w http.ResponseWriter, method, path, query string) (*http.Response, bool) {
	cookie, err := r.Cookie("access_token")
	if err != nil {
		http.Error(w, "no active session cookie found", http.StatusUnauthorized)
		return nil, false
	}
	coreURL, err := coreAPIURL()
	if err != nil {
		http.Error(w, "core API address is not configured", http.StatusInternalServerError)
		return nil, false
	}
	req, err := http.NewRequestWithContext(r.Context(), method, coreURL+path+query, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, false
	}
	req.Header.Set("Authorization", "Bearer "+cookie.Value)
	resp, err := crawlNowHTTPClient.Do(req)
	if err != nil {
		http.Error(w, "the Browser tool could not be reached: "+err.Error(), http.StatusBadGateway)
		return nil, false
	}
	return resp, true
}

// failRelay answers a Browser status that is not a success. missing says what a 404 means for this call.
func failRelay(w http.ResponseWriter, resp *http.Response, missing string) {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		http.Error(w, "session expired — please sign in again", http.StatusUnauthorized)
	case http.StatusNotFound:
		http.Error(w, missing, http.StatusNotFound)
	case http.StatusBadRequest:
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		http.Error(w, strings.TrimSpace(string(b)), http.StatusBadRequest)
	default:
		http.Error(w, "the Browser tool answered "+resp.Status, http.StatusBadGateway)
	}
}

// LinkCrawlSnapshotHandler handles GET /portal-links/link-crawl/snapshot?id= (the page copy, as html) and DELETE (204, also when it is gone).
func LinkCrawlSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if !snapshotIDPattern.MatchString(id) {
		http.Error(w, "id must be a page copy id", http.StatusBadRequest)
		return
	}
	resp, ok := relayToBrowser(r, w, r.Method, browserSnapshotPath, "?id="+id)
	if !ok {
		return
	}
	defer resp.Body.Close()
	switch {
	case r.Method == http.MethodGet && resp.StatusCode == http.StatusOK:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; style-src 'unsafe-inline' https: http:; img-src https: http: data:; font-src https: http: data:")
		_, _ = io.Copy(w, io.LimitReader(resp.Body, maxSnapshotRelay))
	case r.Method == http.MethodDelete && (resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK):
		w.WriteHeader(http.StatusNoContent)
	default:
		failRelay(w, resp, "the page copy is gone (it may have been deleted or have expired)")
	}
}

// LinkCrawlSnapshotsHandler handles GET /portal-links/link-crawl/snapshots ({count, bytes}) and DELETE ({deleted}): the temporary page files of the Browser tool.
func LinkCrawlSnapshotsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp, ok := relayToBrowser(r, w, r.Method, browserSnapshotsPath, "")
	if !ok {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		failRelay(w, resp, "the Browser tool does not keep page copies; install its latest version")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 4096))
}
