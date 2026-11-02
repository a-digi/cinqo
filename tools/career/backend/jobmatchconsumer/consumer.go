// Package jobmatchconsumer is the JobMatchConsumer (step 132): it listens to "career.job.found", the event every
// genuinely new job sends, and gives the job a manual (no AI) match over all profiles, so a new job has a match without
// anyone asking. The route only validates the event and queues the job id; one worker matches the queued ids in
// batches. Since step 164 it also listens to "career.job.description_changed": such a job goes on a second queue and is
// matched again from its new text (a manual result is replaced, a missing one is created, an AI result and a run in
// progress are never touched). See plan/ai/tools/career/step-132-job-match-consumer.md and
// plan/ai/tools/career/step-164-job-description-changed-rematch.md.
package jobmatchconsumer

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/jobs"
)

const (
	// QueueSize is how many job ids can wait for the worker. A crawl of many jobs fits easily; a full queue answers 503
	// so core's own retry gives the worker a moment.
	QueueSize = 1000
	// BatchSize is how many waiting ids one pass of the worker takes together (the profiles and the corpus are read
	// once per pass, not once per job).
	BatchSize = 50
)

// autoMatch is what the worker runs on the new jobs of a batch, rematch on the jobs whose text changed (step 164); vars so a
// test can watch the batches and make them fail.
var (
	autoMatch = jobs.AutoMatchJobs
	rematch   = jobs.RematchChangedJobs
)

// Consumer is a queue of job ids of new jobs, a queue of job ids whose text changed, and the worker that matches them.
type Consumer struct {
	queue   chan string
	changed chan string
}

// New makes a consumer whose queues hold size ids each.
func New(size int) *Consumer {
	return &Consumer{queue: make(chan string, size), changed: make(chan string, 1)}
}

// Default is the consumer the tool runs; EventHandler and Start are its route and worker.
var Default = New(QueueSize)

// EventHandler is POST /events/job-found, reached only by core's loopback dispatcher.
func EventHandler(w http.ResponseWriter, r *http.Request) { Default.Handler(w, r) }

// ChangedEventHandler is POST /events/job-description-changed-job-match (step 164), reached only by core's loopback dispatcher.
func ChangedEventHandler(w http.ResponseWriter, r *http.Request) { Default.ChangedHandler(w, r) }

// Start launches the worker of the default consumer (see Consumer.Start).
func Start(ctx context.Context) <-chan struct{} { return Default.Start(ctx) }

// Handler validates a "career.job.found" event and queues its job id. It answers 202 at once, before any scoring, so
// core never times out and retries a slow delivery; 400 for an unreadable event or an empty job id (only job_id of
// the payload is used); 503 when the queue is full.
func (c *Consumer) Handler(w http.ResponseWriter, r *http.Request) { c.accept(w, r, c.queue) }

// ChangedHandler validates a "career.job.description_changed" event and queues its job id on the second queue, with the same
// answers as Handler: 202 at once, 400 for an unreadable event or an empty job id (only job_id of the payload is used, the
// source is ignored), 503 when the queue is full.
func (c *Consumer) ChangedHandler(w http.ResponseWriter, r *http.Request) { c.accept(w, r, c.changed) }

func (c *Consumer) accept(w http.ResponseWriter, r *http.Request, queue chan string) {
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
	case queue <- payload.JobID:
		w.WriteHeader(http.StatusAccepted)
	default:
		http.Error(w, "the job match queue is full", http.StatusServiceUnavailable)
	}
}

// Enqueue queues job ids for the worker without waiting (step 152: the jobs that lost their match when a profile or persona was
// deleted, so the profiles that remain match them at once). A blank id is skipped; when the queue is full it stops and does not
// block: the ids that did not fit are left to the catch-up sweeper, which finds every job without a match. It returns how many ids it
// queued.
func (c *Consumer) Enqueue(ids []string) int {
	queued := 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		select {
		case c.queue <- id:
			queued++
		default:
			return queued
		}
	}
	return queued
}

// Enqueue queues job ids for the worker of the default consumer (see Consumer.Enqueue).
func Enqueue(ids []string) int { return Default.Enqueue(ids) }

// Start launches the single worker: it waits for an id, takes up to BatchSize-1 more that are already waiting, and
// matches them together. It runs until ctx is cancelled (after the batch in hand) and returns a channel that closes
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
				c.process(c.drain(batch{found: []string{id}}))
			case id := <-c.changed:
				c.process(c.drain(batch{changed: []string{id}}))
			}
		}
	}()
	return done
}

// batch is what one pass of the worker handles: ids of new jobs and ids of jobs whose text changed.
type batch struct {
	found, changed []string
}

func (b batch) size() int { return len(b.found) + len(b.changed) }

// drain collects the ids already waiting on either queue, up to a batch in all, after the first one.
func (c *Consumer) drain(b batch) batch {
	for b.size() < BatchSize {
		select {
		case id := <-c.queue:
			b.found = append(b.found, id)
		case id := <-c.changed:
			b.changed = append(b.changed, id)
		default:
			return b
		}
	}
	return b
}

// process matches one batch. Nothing here may stop the worker. A job that is both new and changed in one batch is only
// re-matched: that creates the match when there is none, so the job is not handled twice.
func (c *Consumer) process(b batch) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("job match consumer: a batch of %d job(s) panicked: %v", b.size(), r)
		}
	}()
	changed := map[string]bool{}
	for _, id := range b.changed {
		changed[id] = true
	}
	var found []string
	for _, id := range b.found {
		if !changed[id] {
			found = append(found, id)
		}
	}
	if len(found) > 0 {
		outcomes, err := autoMatch(found)
		if err != nil {
			log.Printf("job match consumer: a batch of %d job(s) could not be matched: %v", len(found), err)
		}
		for _, o := range outcomes {
			if o.Status == jobs.AutoFailed {
				log.Printf("job match consumer: job %s could not be matched: %v", o.JobID, o.Err)
			}
		}
	}
	if len(b.changed) > 0 {
		outcomes, err := rematch(b.changed)
		if err != nil {
			log.Printf("job match consumer: a batch of %d changed job(s) could not be matched again: %v", len(b.changed), err)
		}
		for _, o := range outcomes {
			if o.Status == jobs.ChangedJobFailed {
				log.Printf("job match consumer: job %s could not be matched again: %v", o.JobID, o.Err)
			}
		}
	}
}
