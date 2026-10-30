package main

import (
	"encoding/json"
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
