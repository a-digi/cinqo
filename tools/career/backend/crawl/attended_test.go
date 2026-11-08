package crawl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// A person's work asks the Browser for `attended`; anything else does not, so only a person can cause a window to open (plan 198).
func TestAttendedIsSentToTheBrowserOnlyForAPersonsWork(t *testing.T) {
	var got []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		got = append(got, m)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"finalUrl":"https://x.test/","title":"t","links":[]}`))
	}))
	defer srv.Close()
	for _, attended := range []bool{false, true} {
		ctx := withAttended(context.Background(), attended)
		if _, err := fetchPageLinksViaCore(ctx, srv.URL, "tok", "https://x.test/", "p1", ""); err != nil {
			t.Fatalf("links: %v", err)
		}
		if _, err := fetchDetailPageViaCore(ctx, srv.URL, "tok", "https://x.test/job", "p1"); err != nil {
			t.Logf("detail answer not scripted (%v): the request body is what is checked", err)
		}
	}
	if len(got) != 4 {
		t.Fatalf("requests = %d, want 4", len(got))
	}
	for i, want := range []bool{false, false, true, true} {
		_, has := got[i]["attended"]
		if has != want {
			t.Errorf("request %d: attended present = %v, want %v (%v)", i, has, want, got[i])
		}
	}
}

// A job-page request says engine katana only for a link that Katana reads (its own engine, else the Browser's), so a Rod link never reaches
// the Katana window (plan 198, V4).
func TestOnlyAKatanaLinkIsMarkedForTheKatanaWindow(t *testing.T) {
	id := duapuneLink(t)
	core := newFakeCore(t, "katana")
	if got := engineField(withKatanaIfSelected(context.Background(), os.Getenv("CORE_API_URL"), "tok", id)); got != "katana" {
		t.Errorf("Browser on Katana: engine = %q", got)
	}
	core.mu.Lock()
	core.engine = "rod"
	core.mu.Unlock()
	if got := engineField(withKatanaIfSelected(context.Background(), os.Getenv("CORE_API_URL"), "tok", id)); got != "" {
		t.Errorf("Browser on Rod: engine = %q, want none", got)
	}
}
