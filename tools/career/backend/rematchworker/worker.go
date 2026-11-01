// Package rematchworker is the debounced worker behind the consumers of "career.profile.created" and "career.persona.created" (step
// 151): it holds the profiles that are waiting for a re-match of their jobs (jobs.RematchProfile) and runs each one once it has been
// quiet for a while. A new profile has no personas yet, and a CV import or the AI adds personas and skills one call at a time, so a
// re-match that ran the instant the event arrived would score an empty profile and never run again. The two consumers share one
// worker, so a profile named by both events is re-matched once. See plan/ai/tools/career/step-151-profile-persona-created-rematch.md.
package rematchworker

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

// The timing of the worker; vars so a test can shrink them. A profile is re-matched once no event for it arrived for quietPeriod, or
// maxWait after its first pending event, whichever comes first; the worker looks every tick.
var (
	quietPeriod = 10 * time.Second
	maxWait     = 2 * time.Minute
	tick        = time.Second
	// maxIDLength bounds the profile id of an event (real ids are UUIDs).
	maxIDLength = 200
)

// rematch is what the worker runs for one profile; a var so a test can watch the runs and make them fail.
var rematch = jobs.RematchProfile

// pending is one profile waiting to be re-matched: when its first and its latest event arrived.
type pending struct {
	first, last time.Time
}

// Worker is the set of profiles waiting to be re-matched and the worker that handles them.
type Worker struct {
	mu      sync.Mutex
	waiting map[string]pending
	now     func() time.Time
}

// New makes a worker; now is its clock.
func New(now func() time.Time) *Worker {
	return &Worker{waiting: map[string]pending{}, now: now}
}

// Default is the worker the tool runs; both consumers note into it.
var Default = New(time.Now)

// Start launches the worker of the default set (see Worker.Start).
func Start(ctx context.Context) <-chan struct{} { return Default.Start(ctx) }

// Note records that a profile needs a re-match: a profile already waiting keeps its first time and gets a new latest time.
func (w *Worker) Note(profileID string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if p, ok := w.waiting[profileID]; ok {
		p.last = now
		w.waiting[profileID] = p
		return
	}
	w.waiting[profileID] = pending{first: now, last: now}
}

// Rearm moves the quiet timer of a profile that is already waiting (a persona of it changed, so more may follow). It never starts a
// wait: a profile that is not pending is left alone, and it reports whether there was one.
func (w *Worker) Rearm(profileID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	p, ok := w.waiting[profileID]
	if !ok {
		return false
	}
	p.last = w.now()
	w.waiting[profileID] = p
	return true
}

// Forget removes a profile from the waiting set (it was deleted, step 152) and reports whether it was waiting. A re-match that is
// already running is not stopped; the purge of the profile's matches waits for it.
func (w *Worker) Forget(profileID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.waiting[profileID]
	delete(w.waiting, profileID)
	return ok
}

// Forget removes a profile from the waiting set of the default worker (see Worker.Forget).
func Forget(profileID string) bool { return Default.Forget(profileID) }

// Waiting is how many profiles are waiting.
func (w *Worker) Waiting() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.waiting)
}

// NoteHandler is the route of a consumer that notes the profile named in the event's payload under the given field ("profile_id").
// It answers 202 at once, before any scoring, so core never times out and retries a slow delivery; 400 for an unreadable event or an
// empty or overlong id; 405 for a method other than POST. It never answers 503: there is one entry per profile.
func (w *Worker) NoteHandler(field string) http.HandlerFunc { return w.handler(field, w.Note) }

// RearmHandler is the route that listens to "career.persona.changed": it only re-arms a profile that is already pending (same answers
// as NoteHandler; 202 whether or not the profile was pending).
func (w *Worker) RearmHandler(field string) http.HandlerFunc {
	return w.handler(field, func(id string) { w.Rearm(id) })
}

func (w *Worker) handler(field string, act func(id string)) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(rw, r.Body, 64*1024)
		var evt db.EventEnvelope
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			http.Error(rw, "invalid event payload", http.StatusBadRequest)
			return
		}
		var payload map[string]any
		_ = json.Unmarshal(evt.Payload, &payload)
		id, _ := payload[field].(string)
		if id == "" || len(id) > maxIDLength {
			http.Error(rw, "invalid event payload", http.StatusBadRequest)
			return
		}
		act(id)
		rw.WriteHeader(http.StatusAccepted)
	}
}

// due takes (removes) and returns the profiles that are ready, oldest first: quiet for quietPeriod, or waiting for maxWait. A
// profile that is not ready stays. A profile noted again while it is being re-matched is a new entry.
func (w *Worker) due() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	var ids []string
	for id, p := range w.waiting {
		if now.Sub(p.last) >= quietPeriod || now.Sub(p.first) >= maxWait {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := w.waiting[ids[i]], w.waiting[ids[j]]
		if !a.first.Equal(b.first) {
			return a.first.Before(b.first)
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		delete(w.waiting, id)
	}
	return ids
}

// processDue re-matches every profile that is ready, one after the other, and returns how many it handled. Nothing here may stop the
// worker: a panic or an error for one profile is logged and the next one goes on. A profile that no longer exists changes nothing.
// A failed run is not retried here (the catch-up of step 4 finds the profile).
func (w *Worker) processDue(ctx context.Context) int {
	ids := w.due()
	for _, id := range ids {
		if ctx.Err() != nil {
			return len(ids)
		}
		rematchOne(id)
	}
	return len(ids)
}

func rematchOne(profileID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("profile rematch: profile %s panicked: %v", profileID, r)
		}
	}()
	rep, err := rematch(profileID)
	switch {
	case err != nil:
		log.Printf("profile rematch: profile %s: %v", profileID, err)
	case rep.Stored+rep.Improved+rep.Failed > 0:
		log.Printf("profile rematch: profile %s: %d job(s) looked at, %d stored, %d improved, %d failed", profileID, rep.Jobs, rep.Stored, rep.Improved, rep.Failed)
	}
}

// Start launches the single worker: every tick it re-matches the profiles that are ready. It runs until ctx is cancelled and returns
// a channel that closes when it has stopped.
func (w *Worker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.processDue(ctx)
			}
		}
	}()
	return done
}
