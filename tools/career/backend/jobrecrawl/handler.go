package jobrecrawl

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"

	"career-tool-backend/db"
	"career-tool-backend/domainevent"
)

const (
	// maxRequestBody bounds the size of one request's JSON (about two hundred thousand ids); it is a safety against a broken
	// client, not a limit on a batch.
	maxRequestBody = 8 << 20
	// publishParallelism is how many events are being published at once: each publish is a call to core.
	publishParallelism = 4
	defaultStatusPage  = 100
	maxStatusPage      = 500
)

// publish sends one event; a seam so a test can see and hold the events.
var publish = domainevent.Publish

// RequestHandler handles POST jobs/recrawl: {"jobIds": [...]} queues the jobs that can be recrawled (see Request), answers 202
// {batchId, accepted, skipped} and only then sends one event per accepted job, in the background. When nothing can be queued the
// answer is 400 with the reasons.
func RequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		JobIDs []string `json:"jobIds"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&body); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(body.JobIDs) == 0 {
		http.Error(w, "jobIds is required: at least one job id", http.StatusBadRequest)
		return
	}
	for _, id := range body.JobIDs {
		if id == "" {
			http.Error(w, "jobIds must not contain an empty id", http.StatusBadRequest)
			return
		}
	}
	res, err := Request(body.JobIDs)
	if err != nil {
		http.Error(w, "failed to queue the recrawl: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if len(res.Accepted) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "none of these jobs can be recrawled", "skipped": res.Skipped})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"batchId": res.BatchID, "accepted": len(res.Accepted), "skipped": res.Skipped})
	go publishAll(res.Accepted)
}

// publishAll sends one event per job id, a few at a time. Best effort like every domain event: a job whose event is lost stays
// queued and the sweeper (a later step) picks it up.
func publishAll(ids []string) {
	sem := make(chan struct{}, publishParallelism)
	var wg sync.WaitGroup
	for _, id := range ids {
		sem <- struct{}{}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			publish(Topic, map[string]string{"job_id": id})
		}(id)
	}
	wg.Wait()
}

// StatusHandler handles GET jobs/recrawl/status?batchId=&limit=&offset=: the progress of a batch, read from the table.
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	batchID := r.URL.Query().Get("batchId")
	if batchID == "" {
		http.Error(w, "batchId query parameter is required", http.StatusBadRequest)
		return
	}
	limit, offset := defaultStatusPage, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "limit must be a whole number", http.StatusBadRequest)
			return
		}
		limit = max(1, min(n, maxStatusPage))
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			http.Error(w, "offset must be a whole number", http.StatusBadRequest)
			return
		}
		offset = max(0, n)
	}
	st, err := Status(batchID, limit, offset)
	if err != nil {
		if errors.Is(err, ErrUnknownBatch) {
			http.Error(w, "unknown batch", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to read the batch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, st)
}

// CancelHandler handles POST jobs/recrawl/cancel: {"batchId": "…"} cancels the batch's queued requests.
func CancelHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		BatchID string `json:"batchId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BatchID == "" {
		http.Error(w, "batchId is required", http.StatusBadRequest)
		return
	}
	n, err := Cancel(body.BatchID)
	if err != nil {
		if errors.Is(err, ErrUnknownBatch) {
			http.Error(w, "unknown batch", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to cancel the batch: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]int{"cancelled": n})
}
