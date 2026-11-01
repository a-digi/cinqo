// Package profilejobmatchconsumer is the ProfileJobMatchConsumer (step 140): it listens to "career.job.found", the event
// every genuinely new job sends, and for each profile that has auto-add rules it scores the job with the manual engine
// (no AI) and puts the job on that profile's board with the status of the rule the score reaches. It scores by itself
// instead of reading job_matches, which holds only one profile's result. A job already on a board is never moved. The
// route only validates the event and queues the job id; one worker handles the queued ids in batches, like the
// JobMatchConsumer. See plan/ai/tools/career/step-140-profile-job-match-consumer.md.
package profilejobmatchconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"

	"career-tool-backend/db"
	"career-tool-backend/jobboard"
	"career-tool-backend/jobs"
	"career-tool-backend/profilerules"
)

const (
	// QueueSize is how many job ids can wait for the worker; a full queue answers 503 so core's own retry gives the
	// worker a moment.
	QueueSize = 1000
	// BatchSize is how many waiting ids one pass of the worker takes together (the rules and the personas are read once
	// per pass, not once per job).
	BatchSize = 50
)

// scorer is what the worker needs of jobs.ProfileScorer; a seam so a test can give exact scores.
type scorer interface {
	ScoreByProfile(jobID string) (map[string]int, error)
}

// newScorer builds the scorer of one batch for the profiles that have rules.
var newScorer = func(profileIDs map[string]bool) (scorer, error) { return jobs.NewProfileScorerFor(profileIDs) }

// What happened to one job for one profile.
const (
	Added    = "added"    // put on the board with the rule's status
	Below    = "below"    // the score reaches none of the profile's rules
	OnBoard  = "on_board" // already on the profile's board, left alone
	Unjudged = "unjudged" // the job's own lookup failed for this pair (see Err)
)

// Outcome is the result for one (job, profile) pair that had rules and could judge the job.
type Outcome struct {
	JobID, ProfileID string
	Status           string
	Score            int
	StatusID         string // the rule's status, for Added and OnBoard
	Err              error
}

// ProcessBatch handles a batch of new jobs: for every profile with rules that can judge a job, the rule with the
// highest percentage the job's score reaches decides the status, and the job is added to that board unless it is
// already there. An id is handled once however often it appears; a job that no longer exists is skipped; a failure
// for one job is logged and the rest go on. An error is returned only when the batch cannot start (the rules or the
// profiles cannot be read). With no rule anywhere nothing else is read at all.
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
	rules, err := profilerules.All()
	if err != nil {
		return nil, fmt.Errorf("could not read the rules: %w", err)
	}
	if len(rules) == 0 {
		return nil, nil
	}
	only := make(map[string]bool, len(rules))
	for profileID := range rules {
		only[profileID] = true
	}
	sc, err := newScorer(only)
	if err != nil {
		return nil, fmt.Errorf("could not read the profiles: %w", err)
	}
	var out []Outcome
	for _, jobID := range order {
		scores, err := sc.ScoreByProfile(jobID)
		if errors.Is(err, jobs.ErrUnknownJob) {
			continue
		}
		if err != nil {
			out = append(out, Outcome{JobID: jobID, Status: Unjudged, Err: err})
			continue
		}
		profileIDs := make([]string, 0, len(scores))
		for profileID := range scores {
			profileIDs = append(profileIDs, profileID)
		}
		slices.Sort(profileIDs)
		for _, profileID := range profileIDs {
			out = append(out, apply(jobID, profileID, scores[profileID], rules[profileID]))
		}
	}
	return out, nil
}

func apply(jobID, profileID string, score int, rules []profilerules.Rule) Outcome {
	o := Outcome{JobID: jobID, ProfileID: profileID, Score: score}
	rule, ok := profilerules.RuleFor(rules, score)
	if !ok {
		o.Status = Below
		return o
	}
	o.StatusID = rule.StatusID
	added, err := jobboard.AddEntryIfAbsent(profileID, jobID, rule.StatusID)
	switch {
	case err != nil:
		// including a job deleted between the scoring and the insert (jobboard.ErrUnknownJob)
		o.Status = Unjudged
		o.Err = err
	case added:
		o.Status = Added
	default:
		o.Status = OnBoard
	}
	return o
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

// Default is the consumer the tool runs; EventHandler and Start are its route and worker.
var Default = New(QueueSize)

// EventHandler is POST /events/job-found-profile-rules, reached only by core's loopback dispatcher.
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
		http.Error(w, "the profile rules queue is full", http.StatusServiceUnavailable)
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
			log.Printf("profile job match consumer: a batch of %d job(s) panicked: %v", len(ids), r)
		}
	}()
	outcomes, err := run(ids)
	if err != nil {
		log.Printf("profile job match consumer: a batch of %d job(s) could not be handled: %v", len(ids), err)
		return
	}
	for _, o := range outcomes {
		if o.Err != nil {
			log.Printf("profile job match consumer: job %s, profile %q: %v", o.JobID, o.ProfileID, o.Err)
		}
	}
}
