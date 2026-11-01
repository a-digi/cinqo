// Package jobmatchconsumer is the JobMatchConsumer (step 132): it listens to "career.job.found", the event every
// genuinely new job sends, and gives the job a manual (no AI) match over all profiles, so a new job has a match without
// anyone asking. The route only validates the event and queues the job id; one worker matches the queued ids in
// batches. See plan/ai/tools/career/step-132-job-match-consumer.md.
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

// autoMatch is what the worker runs on a batch; a var so a test can watch the batches and make them fail.
var autoMatch = jobs.AutoMatchJobs

// Consumer is a queue of job ids and the worker that matches them.
type Consumer struct {
	queue chan string
}

// New makes a consumer whose queue holds size ids.
func New(size int) *Consumer {
	return &Consumer{queue: make(chan string, size)}
}

// Default is the consumer the tool runs; EventHandler and Start are its route and worker.
var Default = New(QueueSize)

// EventHandler is POST /events/job-found, reached only by core's loopback dispatcher.
func EventHandler(w http.ResponseWriter, r *http.Request) { Default.Handler(w, r) }

// Start launches the worker of the default consumer (see Consumer.Start).
func Start(ctx context.Context) <-chan struct{} { return Default.Start(ctx) }

// Handler validates a "career.job.found" event and queues its job id. It answers 202 at once, before any scoring, so
// core never times out and retries a slow delivery; 400 for an unreadable event or an empty job id (only job_id of
// the payload is used); 503 when the queue is full.
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

// process matches one batch. Nothing here may stop the worker.
func (c *Consumer) process(ids []string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("job match consumer: a batch of %d job(s) panicked: %v", len(ids), r)
		}
	}()
	outcomes, err := autoMatch(ids)
	if err != nil {
		log.Printf("job match consumer: a batch of %d job(s) could not be matched: %v", len(ids), err)
		return
	}
	for _, o := range outcomes {
		if o.Status == jobs.AutoFailed {
			log.Printf("job match consumer: job %s could not be matched: %v", o.JobID, o.Err)
		}
	}
}
