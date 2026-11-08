package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The widget routes answer GET with a JSON object of the shape their kind needs,
// and refuse everything else.
func TestWidgetRoutes(t *testing.T) {
	for name, tc := range map[string]struct {
		handler func() any
		keys    []string
	}{
		"stat":   {itemsCountWidget, []string{"label", "value"}},
		"list":   {latestItemsWidget, []string{"items"}},
		"custom": {customDemoWidget, []string{"message"}},
	} {
		h := getOnly(tc.handler)

		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		var got map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %q: %v", name, rec.Code, rec.Body.String(), err)
		}
		for _, k := range tc.keys {
			if _, ok := got[k]; !ok {
				t.Errorf("%s: missing %q in %v", name, k, got)
			}
		}

		rec = httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/x", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: POST answered %d", name, rec.Code)
		}
	}
}

// Deleting an item tells core to purge its comments: right route, the tool's own
// ToolService token, and only the entity in the body (never an owner).
func TestPurgeCommentsRequest(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer core.Close()

	purgeComments(core.URL, "secret", "item", "8d3c5b1e-2f4a-4c6d-9e7f-0a1b2c3d4e5f")
	if gotPath != "/api/v1/comments/purge" || gotAuth != "ToolService secret" {
		t.Errorf("path %q, auth %q", gotPath, gotAuth)
	}
	if len(gotBody) != 2 || gotBody["entityType"] != "item" || gotBody["entityId"] != "8d3c5b1e-2f4a-4c6d-9e7f-0a1b2c3d4e5f" {
		t.Errorf("body = %v", gotBody)
	}

	// Without the environment core gives a tool, or with core unreachable, nothing breaks.
	gotPath = ""
	purgeComments("", "secret", "item", "x")
	purgeComments(core.URL, "", "item", "x")
	if gotPath != "" {
		t.Error("a request went out without a complete environment")
	}
	core.Close()
	purgeComments(core.URL, "secret", "item", "x")
}
