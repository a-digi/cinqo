package jobcleanup

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"career-tool-backend/db"
)

// cleanupRequest is the body of preview and run. The thresholds default to the saved settings; olderThanDays and
// includeManual override them for this call only. confirm is required by run and ignored by preview.
type cleanupRequest struct {
	Rule            string `json:"rule"`
	OlderThanDays   *int   `json:"olderThanDays"`
	MinMatchPercent *int   `json:"minMatchPercent"`
	IncludeManual   *bool  `json:"includeManual"`
	Confirm         bool   `json:"confirm"`
}

func readRequest(w http.ResponseWriter, r *http.Request) (cleanupRequest, Options, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return cleanupRequest{}, Options{}, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var req cleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `body must be {"rule": "older" | "low_match" | "both"}`, http.StatusBadRequest)
		return req, Options{}, false
	}
	s, err := GetSettings()
	if err != nil {
		http.Error(w, "failed to read the cleanup settings: "+err.Error(), http.StatusInternalServerError)
		return req, Options{}, false
	}
	o := Options{Rule: req.Rule, OlderThanDays: s.OlderThanDays, MinMatchPercent: s.MinMatchPercent, IncludeManual: s.IncludeManual}
	if req.MinMatchPercent != nil {
		o.MinMatchPercent = *req.MinMatchPercent
	}
	if req.OlderThanDays != nil {
		o.OlderThanDays = *req.OlderThanDays
	}
	if req.IncludeManual != nil {
		o.IncludeManual = *req.IncludeManual
	}
	if err := o.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return req, o, false
	}
	return req, o, true
}

// PreviewHandler handles POST /jobs/cleanup/preview. It never writes.
func PreviewHandler(w http.ResponseWriter, r *http.Request) {
	_, o, ok := readRequest(w, r)
	if !ok {
		return
	}
	p, err := PreviewCleanup(o)
	if err != nil {
		http.Error(w, "failed to preview the cleanup: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, p)
}

// RunHandler handles POST /jobs/cleanup/run. It refuses without "confirm": true.
func RunHandler(w http.ResponseWriter, r *http.Request) {
	req, o, ok := readRequest(w, r)
	if !ok {
		return
	}
	if !req.Confirm {
		http.Error(w, `deleting needs "confirm": true`, http.StatusBadRequest)
		return
	}
	res, err := RunCleanup(o)
	switch {
	case errors.Is(err, ErrBusy):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, "the cleanup stopped after deleting "+strconv.Itoa(res.Deleted)+" jobs: "+err.Error(), http.StatusInternalServerError)
	default:
		db.WriteJSON(w, res)
	}
}
