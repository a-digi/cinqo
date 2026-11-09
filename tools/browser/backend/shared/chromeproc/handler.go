package chromeproc

import (
	"encoding/json"
	"net/http"
)

// StatusHandler answers GET /chrome-processes: the Chromes the tool started and still has registered (read only; nothing a caller sends
// reaches the operating system, and there is no route that stops a process).
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(CurrentStatus())
}
