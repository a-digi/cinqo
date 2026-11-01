// Package jobrecrawlconsumer is the consumer of "career.job.recrawl_requested" (step 148): the event the Jobs page's Recrawl action
// sends for each job. The route only validates the event and queues the job id; one worker crawls the queued jobs one after the
// other (the Browser tool has one shared tab), recording each job's outcome in job_recrawl_requests; a sweeper hands the worker
// the requests whose event was missed. The request row, not the event, is the truth: a worker takes a job only by claiming its
// queued row, so a duplicate id, a cancelled request or a deleted job is dropped. See plan/ai/tools/career/step-148-recrawl-jobs.md.
package jobrecrawlconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"career-tool-backend/autodiscovery"
	"career-tool-backend/crawl"
	"career-tool-backend/db"
	"career-tool-backend/domainevent"
	"career-tool-backend/jobrecrawl"
	"career-tool-backend/jobs"
)

const (
	// QueueSize is how many job ids can wait for the worker; a full queue answers 503 so core's own retry gives the worker a
	// moment, and the sweeper hands over what did not fit.
	QueueSize = 5000

	// WaitingForSession is the reason shown for a job that waits because no signed-in session was seen lately.
	WaitingForSession = "waiting for a signed-in session"
)

// crawlTimeout bounds one job's crawl (a page that never loads, a challenge nobody answers); a var so a test can shrink it.
var crawlTimeout = 10 * time.Minute

// The seams a test replaces.
var (
	cachedToken = autodiscovery.CachedToken
	loadTarget  = jobs.GetRecrawlTarget
	recrawl     = crawl.RecrawlOneJob
	publish     = domainevent.Publish
)

// Consumer is a queue of job ids and the worker that crawls them. pending holds the ids that are waiting in the queue so an id
// is queued once.
type Consumer struct {
	queue   chan string
	mu      sync.Mutex
	pending map[string]bool
}

// New makes a consumer whose queue holds size ids.
func New(size int) *Consumer {
	return &Consumer{queue: make(chan string, size), pending: map[string]bool{}}
}

// Default is the consumer the tool runs; EventHandler, Start and StartSweeper are its route, worker and catch-up.
var Default = New(QueueSize)

// EventHandler is POST /events/job-recrawl, reached only by core's loopback dispatcher.
func EventHandler(w http.ResponseWriter, r *http.Request) { Default.Handler(w, r) }

// Start launches the worker of the default consumer (see Consumer.Start).
func Start(ctx context.Context) <-chan struct{} { return Default.Start(ctx) }

type enqueueResult int

const (
	queuedNow enqueueResult = iota
	alreadyWaiting
	queueFull
)

// enqueue puts an id on the queue unless it is already waiting there.
func (c *Consumer) enqueue(id string) enqueueResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[id] {
		return alreadyWaiting
	}
	select {
	case c.queue <- id:
		c.pending[id] = true
		return queuedNow
	default:
		return queueFull
	}
}

// take removes an id from the pending set when the worker takes it off the queue.
func (c *Consumer) take(id string) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

// Handler validates a "career.job.recrawl_requested" event and queues its job id. It answers 202 at once, before any crawling, so
// core never times out and retries a slow delivery; 400 for an unreadable event or an empty job id; 503 when the queue is full.
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
	if c.enqueue(payload.JobID) == queueFull {
		http.Error(w, "the recrawl queue is full", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// Start launches the single worker. First it puts back to queued any request a restart left running, and queues what is waiting in
// the table; then it takes ids one at a time until ctx is cancelled (after the job in hand) and returns a channel that closes when
// it has stopped. A failing or panicking job is recorded and logged and the worker goes on.
func (c *Consumer) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if n, err := jobrecrawl.ReviveRunning(); err != nil {
			log.Printf("job recrawl consumer: could not revive the requests a restart cut off: %v", err)
		} else if n > 0 {
			log.Printf("job recrawl consumer: %d request(s) cut off by a restart go back to the queue", n)
		}
		c.sweepOnce()
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-c.queue:
				c.take(id)
				c.process(ctx, id)
			}
		}
	}()
	return done
}

// process handles one id. Nothing here may stop the worker.
func (c *Consumer) process(ctx context.Context, id string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("job recrawl consumer: job %s panicked: %v", id, r)
			_ = jobrecrawl.Finish(id, jobrecrawl.OutcomeFailed, "internal error")
		}
	}()
	Process(ctx, id)
}

// Process recrawls one job if its request is still queued, and records the outcome:
//   - a job added by hand that already has a description is "kept" without visiting its page (its text is never replaced);
//   - with no fresh session token the job goes back to queued and waits;
//   - a job that cannot be crawled (no portal, no or invalid instructions) is "skipped" with the reason;
//   - otherwise its page is crawled: "failed" when the crawl or save failed or the page gave no description (the old text is kept),
//     "filled" when a manual job had none and now has one, "refreshed" when the text changed, "unchanged" when it did not. A
//
// "filled" or "refreshed" job also sends "career.job.description_changed", so its persona scores are made again.
func Process(ctx context.Context, id string) {
	claimed, err := jobrecrawl.Claim(id)
	if err != nil {
		log.Printf("job recrawl consumer: could not claim job %s: %v", id, err)
		return
	}
	if !claimed {
		return // cancelled, done, already taken or deleted
	}
	t, err := loadTarget(id)
	if err != nil {
		if errors.Is(err, jobs.ErrUnknownJob) {
			return // the request went with the job
		}
		finish(id, jobrecrawl.OutcomeFailed, "could not read the job: "+err.Error())
		return
	}
	if t.Source == jobs.SourceManual && strings.TrimSpace(t.Description) != "" {
		finish(id, jobrecrawl.OutcomeKept, "added by hand: its own text is kept")
		return
	}
	token, fresh := cachedToken()
	if token == "" || !fresh {
		if err := jobrecrawl.Release(id, WaitingForSession); err != nil {
			log.Printf("job recrawl consumer: could not put job %s back to wait: %v", id, err)
		}
		return
	}

	crawlCtx, cancel := context.WithTimeout(ctx, crawlTimeout)
	saved, err := recrawl(crawlCtx, t, token)
	timedOut := errors.Is(crawlCtx.Err(), context.DeadlineExceeded)
	cancel()

	var notPossible *crawl.NotPossibleError
	switch {
	case errors.As(err, &notPossible):
		finish(id, jobrecrawl.OutcomeSkipped, notPossible.Reason)
	case ctx.Err() != nil:
		// The tool is shutting down: the job was not done, so it goes back to the queue for the next start.
		_ = jobrecrawl.Release(id, "")
	case timedOut:
		finish(id, jobrecrawl.OutcomeFailed, "the page took too long; the old text was kept")
	case err != nil:
		finish(id, jobrecrawl.OutcomeFailed, err.Error())
	case !saved:
		finish(id, jobrecrawl.OutcomeFailed, "the page gave no description; the old text was kept")
	default:
		after, err := loadTarget(id)
		if err != nil {
			finish(id, jobrecrawl.OutcomeFailed, "could not read the job after the crawl: "+err.Error())
			return
		}
		outcome := outcomeFor(t, after.Description)
		// The old persona scores were made from the old text: say so before the request is marked done, so a page that sees "done" and
		// asks for the scores again finds them brought up to date. Best effort like every event.
		if outcome == jobrecrawl.OutcomeRefreshed || outcome == jobrecrawl.OutcomeFilled {
			publish(jobrecrawl.DescriptionChangedTopic, map[string]string{"job_id": id})
		}
		finish(id, outcome, "")
	}
}

// outcomeFor is the outcome of a crawl that saved a description: the job's text before (in t) and after.
func outcomeFor(before jobs.RecrawlTarget, after string) string {
	switch {
	case before.Source == jobs.SourceManual && strings.TrimSpace(before.Description) == "":
		return jobrecrawl.OutcomeFilled
	case after != before.Description:
		return jobrecrawl.OutcomeRefreshed
	default:
		return jobrecrawl.OutcomeUnchanged
	}
}

func finish(id, outcome, reason string) {
	if err := jobrecrawl.Finish(id, outcome, reason); err != nil {
		log.Printf("job recrawl consumer: could not record the outcome of job %s: %v", id, err)
	}
}
