// Package personarecomputeconsumer is the consumer of "career.persona.changed" (step 142): when a persona changed it
// scores that persona against every job again (jobs.RecomputePersona) and replaces its rows. A persona is edited in
// bursts (a CV import adds dozens of skills one call at a time), so the route only notes which persona changed; the worker
// recomputes a persona once it has been quiet for a while, and at the latest after a maximum wait. See
// plan/ai/tools/career/step-142-persona-changed-rescore.md.
package personarecomputeconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/match"
	"career-tool-backend/persona"
)

// The timing of the worker; vars so a test can shrink them. A persona is recomputed once no event for it arrived for
// quietPeriod, or maxWait after its first pending event, whichever comes first; the worker looks every tick.
var (
	quietPeriod = 10 * time.Second
	maxWait     = 2 * time.Minute
	tick        = time.Second
	// maxIDLength bounds the persona id of an event (real ids are UUIDs).
	maxIDLength = 200
)

// recompute is what the worker runs for one persona; a var so a test can watch the runs and make them fail.
var recompute = match.RecomputePersona

// pending is one persona waiting to be recomputed: when its first and its latest event arrived.
type pending struct {
	first, last time.Time
}

// Consumer is the set of personas waiting to be recomputed and the worker that handles them.
type Consumer struct {
	mu      sync.Mutex
	waiting map[string]pending
	now     func() time.Time
}

// New makes a consumer; now is its clock.
func New(now func() time.Time) *Consumer {
	return &Consumer{waiting: map[string]pending{}, now: now}
}

// Default is the consumer the tool runs; EventHandler and Start are its route and worker.
var Default = New(time.Now)

// EventHandler is POST /events/persona-changed-recompute, reached only by core's loopback dispatcher.
func EventHandler(w http.ResponseWriter, r *http.Request) { Default.Handler(w, r) }

// Start launches the worker of the default consumer (see Consumer.Start).
func Start(ctx context.Context) <-chan struct{} { return Default.Start(ctx) }

// Note records that a persona changed: a persona already waiting keeps its first time and gets a new latest time.
func (c *Consumer) Note(personaID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if p, ok := c.waiting[personaID]; ok {
		p.last = now
		c.waiting[personaID] = p
		return
	}
	c.waiting[personaID] = pending{first: now, last: now}
}

// Waiting is how many personas are waiting.
func (c *Consumer) Waiting() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiting)
}

// Handler validates a "career.persona.changed" event and notes its persona. It answers 202 at once, before any scoring,
// so core never times out and retries a slow delivery; 400 for an unreadable event or an empty or overlong persona id
// (only persona_id of the payload is used); 405 for a method other than POST. It never answers 503: there is one entry
// per persona, so the set does not fill up.
func (c *Consumer) Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var evt db.EventEnvelope
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	var payload struct {
		PersonaID string `json:"persona_id"`
	}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.PersonaID == "" || len(payload.PersonaID) > maxIDLength {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	c.Note(payload.PersonaID)
	w.WriteHeader(http.StatusAccepted)
}

// due takes (removes) and returns the personas that are ready, oldest first: quiet for quietPeriod, or waiting for
// maxWait. A persona that is not ready stays. A persona noted again while it is being recomputed is a new entry.
func (c *Consumer) due() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	var ids []string
	for id, p := range c.waiting {
		if now.Sub(p.last) >= quietPeriod || now.Sub(p.first) >= maxWait {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := c.waiting[ids[i]], c.waiting[ids[j]]
		if !a.first.Equal(b.first) {
			return a.first.Before(b.first)
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		delete(c.waiting, id)
	}
	return ids
}

// processDue recomputes every persona that is ready, one after the other, and returns how many it handled. Nothing here
// may stop the worker: a panic or an error for one persona is logged and the next one goes on. A persona that no longer
// exists is dropped silently (its rows were purged with it). A failed run is not retried here; the catch-up sweeper
// finds the persona by its fingerprint.
func (c *Consumer) processDue(ctx context.Context) int {
	ids := c.due()
	for _, id := range ids {
		if ctx.Err() != nil {
			return len(ids)
		}
		c.recomputeOne(ctx, id)
	}
	return len(ids)
}

func (c *Consumer) recomputeOne(ctx context.Context, personaID string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("persona recompute: persona %s panicked: %v", personaID, r)
		}
	}()
	out, err := recompute(ctx, personaID)
	switch {
	case errors.Is(err, persona.ErrUnknownPersona):
		return
	case errors.Is(err, context.Canceled):
		return
	case err != nil:
		log.Printf("persona recompute: persona %s: %v", personaID, err)
	case !out.Skipped:
		log.Printf("persona recompute: persona %s: %d job(s) scored, %d judged", personaID, out.Jobs, out.Scored)
	}
}

// Start launches the single worker: every tick it recomputes the personas that are ready. It runs until ctx is cancelled
// (a run in progress stops between its chunks) and returns a channel that closes when it has stopped.
func (c *Consumer) Start(ctx context.Context) <-chan struct{} {
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
				c.processDue(ctx)
			}
		}
	}()
	return done
}
