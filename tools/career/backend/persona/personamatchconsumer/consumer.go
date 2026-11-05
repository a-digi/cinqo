// Package personamatchconsumer is the PersonaJobMatchConsumer (step 141): it listens to "career.job.found", the event
// every genuinely new job sends (and, since step 148, to "career.job.description_changed", sent when a recrawl changed a job's text,
// so the job is scored again), and scores the job against every persona of every profile with the manual engine (no AI),
// storing one row per persona that could judge it (personamatch). The route only validates the event and queues the job
// id; one worker handles the queued ids in batches, like the other consumers of this event, and a sweeper hands it the
// jobs that have no rows (existing jobs, missed events). See plan/ai/tools/career/step-141-persona-job-match.md.
package personamatchconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/persona/personamatch"
)

const (
	// QueueSize is how many job ids can wait for the worker; a full queue answers 503 so core's own retry gives the
	// worker a moment.
	QueueSize = 1000
	// BatchSize is how many waiting ids one pass of the worker takes together (the personas and the corpus are read once
	// per pass, not once per job).
	BatchSize = 50
)

// scorer is what the worker needs of jobs.PersonaScorer; a seam so a test can give exact scores.
type scorer interface {
	HasPersonas() bool
	Rows(jobID string) ([]personamatch.Row, error)
}

// newScorer builds the scorer of one batch.
var newScorer = func() (scorer, error) { return jobs.NewPersonaScorer() }

// What happened to one job.
const (
	Scored   = "scored"   // rows written for the personas that could judge it
	Unjudged = "unjudged" // no persona could judge it (or there is none): no rows
	Failed   = "failed"   // scoring or storing failed (see Err)
)

// Outcome is the result for one job of a batch.
type Outcome struct {
	JobID    string
	Status   string
	Personas int // how many personas got a row
	Err      error
}

// ProcessBatch scores a batch of new jobs. Each job's rows replace its earlier ones in one transaction, so a redelivered
// event or a recalculation only brings them up to date. An id is handled once however often it appears; a job that no
// longer exists is skipped (no outcome); a failure for one job is its Failed outcome and the rest go on. An error is
// returned only when the batch cannot start (the personas cannot be read). With no persona at all nothing is scored and
// the corpus is not read.
func ProcessBatch(ids []string) ([]Outcome, error) {
	var order []string
	seen := map[string]bool{}
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	if len(order) == 0 {
		return nil, nil
	}
	sc, err := newScorer()
	if err != nil {
		return nil, fmt.Errorf("could not read the personas: %w", err)
	}
	if !sc.HasPersonas() {
		return nil, nil
	}
	var out []Outcome
	for _, jobID := range order {
		rows, err := sc.Rows(jobID)
		if errors.Is(err, jobs.ErrUnknownJob) {
			continue
		}
		if err != nil {
			out = append(out, Outcome{JobID: jobID, Status: Failed, Err: err})
			continue
		}
		if err := personamatch.Replace(jobID, rows); err != nil {
			if errors.Is(err, personamatch.ErrUnknownJob) { // deleted while being scored
				continue
			}
			out = append(out, Outcome{JobID: jobID, Status: Failed, Err: err})
			continue
		}
		status := Scored
		if len(rows) == 0 {
			status = Unjudged
		}
		out = append(out, Outcome{JobID: jobID, Status: status, Personas: len(rows)})
	}
	return out, nil
}

// run is what the worker calls on a batch; a var so a test can watch the batches and make them fail.
var run = ProcessBatch

// Consumer is a queue of job ids and the worker that handles them.
type Consumer struct {
	queue chan string
}

// New makes a consumer whose queue holds size ids.
func New(size int) *Consumer {
	return &Consumer{queue: make(chan string, size)}
}

// Default is the consumer the tool runs; EventHandler, Start and StartSweeper are its route, worker and catch-up.
var Default = New(QueueSize)

// EventHandler is POST /events/job-found-persona-matches, reached only by core's loopback dispatcher.
func EventHandler(w http.ResponseWriter, r *http.Request) { Default.Handler(w, r) }

// Start launches the worker of the default consumer (see Consumer.Start).
func Start(ctx context.Context) <-chan struct{} { return Default.Start(ctx) }

// Handler validates a "career.job.found" event and queues its job id. It answers 202 at once, before any scoring, so
// core never times out and retries a slow delivery; 400 for an unreadable event or an empty job id (only job_id of the
// payload is used); 503 when the queue is full.
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
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(evt.Payload, &payload); err != nil || payload.JobID == "" {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}
	select {
	case c.queue <- payload.JobID:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "the persona match queue is full", http.StatusServiceUnavailable)
	}
}

// Start launches the single worker: it waits for an id, takes up to BatchSize-1 more that are already waiting, and
// handles them together. It runs until ctx is cancelled (after the batch in hand) and returns a channel that closes
// when it has stopped. A failing or panicking batch is logged and the worker goes on.
func (c *Consumer) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-c.queue:
				c.process(c.drain(id))
			}
		}
	}()
	return done
}

// drain collects the ids already waiting, up to a batch, after the first one.
func (c *Consumer) drain(first string) []string {
	batch := []string{first}
	for len(batch) < BatchSize {
		select {
		case id := <-c.queue:
			batch = append(batch, id)
		default:
			return batch
		}
	}
	return batch
}

// process handles one batch. Nothing here may stop the worker.
func (c *Consumer) process(ids []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("persona match consumer: a batch of %d job(s) panicked: %v", len(ids), r)
		}
	}()
	outcomes, err := run(ids)
	if err != nil {
		log.Printf("persona match consumer: a batch of %d job(s) could not be handled: %v", len(ids), err)
		return
	}
	for _, o := range outcomes {
		if o.Status == Failed {
			log.Printf("persona match consumer: job %s could not be scored: %v", o.JobID, o.Err)
		}
	}
}
