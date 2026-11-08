package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

// The host's entire contract with this process (see
// plan/ai/tools/step-04-enable-disable-and-tool-manager.md): bind
// 127.0.0.1:$PORT, expose GET /healthz returning 200, implement
// whatever routes this tool's own manifest.json declared.
func main() {
	port := os.Getenv("PORT")

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/items", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(items())
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			// A real tool would delete the item here. What matters for comments is the
			// next line: tell Cinqo the entity is gone so the comments about it go too.
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "id query parameter is required", http.StatusBadRequest)
				return
			}
			purgeComments(os.Getenv("CORE_API_URL"), os.Getenv("TOOL_SERVICE_TOKEN"), "item", id)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Home dashboard widgets (see README.md, "Adding a widget to the Home
	// dashboard"). Each is a plain GET route declared in manifest.json's routes
	// and named by a widgets[] entry. Cinqo calls them with the signed-in user's
	// bearer token, after checking the route's required_scope itself.
	http.HandleFunc("/dashboard/items-count", getOnly(itemsCountWidget))
	http.HandleFunc("/dashboard/latest-items", getOnly(latestItemsWidget))
	http.HandleFunc("/dashboard/custom-demo", getOnly(customDemoWidget))

	_ = http.ListenAndServe("127.0.0.1:"+port, nil)
}

func items() []string {
	return []string{"replace me with real data"}
}

func getOnly(h func() any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h())
	}
}

// kind "stat": one headline number. label and value are required; unit,
// delta, caption and link are optional. A link must be an internal path.
func itemsCountWidget() any {
	n := len(items())
	return map[string]any{
		"label":   "Items",
		"value":   n,
		"caption": strconv.Itoa(n) + " in total",
		"link":    "/tools/example_full/home",
	}
}

// kind "list": up to 10 rows. Return {"items": []} when there is nothing to
// show; the card then says "Nothing to show yet".
func latestItemsWidget() any {
	rows := []map[string]any{}
	for _, name := range items() {
		rows = append(rows, map[string]any{
			"title":    name,
			"subtitle": "Example item",
			"badge":    map[string]any{"text": "New", "variant": "green"},
			"link":     "/tools/example_full/home",
		})
	}
	return map[string]any{"items": rows}
}

// kind "custom": any JSON object (at most 8 levels deep). Core only checks
// that it is an object; frontend/bundle.js draws it.
func customDemoWidget() any {
	return map[string]any{"message": "Hello from the example tool", "items": items()}
}

// purgeComments tells Cinqo that an entity no longer exists, so the comments about it
// are removed (see README.md, "Comments on your tool's entities"). The comments live in
// Cinqo, in this tool's own comments database; this is the only thing the tool does.
// The caller is authenticated as this tool by its service token, and the owner is
// whichever tool owns that token, so nothing here names an owner. Best effort: a
// failure is logged and ignored, because failing to tidy up comments must never fail
// the delete itself.
func purgeComments(coreURL, token, entityType, entityID string) {
	if coreURL == "" || token == "" {
		return
	}
	body, _ := json.Marshal(map[string]string{"entityType": entityType, "entityId": entityID})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, coreURL+"/api/v1/comments/purge", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "ToolService "+token) // not "Bearer": this is the tool, not a user
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "purge comments: %v\n", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "purge comments: core answered %d\n", resp.StatusCode)
	}
}
