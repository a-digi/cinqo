package main

import (
	"encoding/json"
	"net/http"
	"os"
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
			_ = json.NewEncoder(w).Encode([]string{"replace me with real data"})
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	_ = http.ListenAndServe("127.0.0.1:"+port, nil)
}
