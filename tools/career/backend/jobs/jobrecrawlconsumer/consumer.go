// Package jobrecrawlconsumer is the consumer of "career.job.recrawl_requested" (step 148): the event the Jobs page's Recrawl action
// sends for each job. The route only validates the event and queues the job id; one worker crawls the queued jobs one after the
// other, and up to four helpers join it while more than ParallelThreshold are waiting, recording each job's outcome in job_recrawl_requests; a sweeper hands the worker
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
	"sync/atomic"
	"time"

	"career-tool-backend/autodiscovery"
	"career-tool-backend/crawl"
	"career-tool-backend/db"
	"career-tool-backend/jobs"
	"career-tool-backend/jobs/jobrecrawl"
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
)

// Consumer is a queue of job ids and the worker that crawls them. pending holds the ids that are waiting in the queue so an id
// is queued once.
type Consumer struct {
	// big is set once more than ParallelThreshold ids have been waiting and cleared when the queue runs dry: the helpers work while it is set.
	big     atomic.Bool
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
		if len(c.queue) > ParallelThreshold {
			c.big.Store(true)
		}
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

// The crawlers of a big batch: with more than ParallelThreshold ids waiting, up to MaxWorkers crawl at the same time (one main worker and
// helpers that only take an id while that many are waiting); a small batch is crawled by the main worker alone, one after the other. Each
// id is claimed through its row, so no two workers ever crawl the same job. The Browser tool still spaces the page loads of ONE platform
// (10 to 30 seconds, its anti-detection pacing, keyed by the platform): more crawlers speed up a batch whose jobs belong to several
// platforms, not one that belongs to a single platform.
const (
	ParallelThreshold = 20
	MaxWorkers        = 5
)

// helperPoll is how often a helper looks at the queue while it is short.
var helperPoll = 500 * time.Millisecond

// Start launches the worker and its helpers. First the main worker puts back to queued any request a restart left running, and queues
// what is waiting in the table; then it takes ids one at a time until ctx is cancelled (after the job in hand), and the helpers take
// ids only while more than ParallelThreshold are waiting. It returns a channel that closes when all of them have stopped. A failing or
// panicking job is recorded and logged and the workers go on.
func (c *Consumer) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	var workers sync.WaitGroup
	// The helpers wait until the main worker has put back what a restart cut off and queued what is waiting: a request a helper
	// is crawling must never be taken for one that a restart cut off.
	ready := make(chan struct{})
	workers.Add(1)
	go func() {
		defer workers.Done()
		if n, err := jobrecrawl.ReviveRunning(); err != nil {
			log.Printf("job recrawl consumer: could not revive the requests a restart cut off: %v", err)
		} else if n > 0 {
			log.Printf("job recrawl consumer: %d request(s) cut off by a restart go back to the queue", n)
		}
		c.sweepOnce()
		close(ready)
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-c.queue:
				c.take(id)
				c.process(crawl.WithPacingSlot(ctx, 0), id)
			}
		}
	}()
	for i := 1; i < MaxWorkers; i++ {
		workers.Add(1)
		go func(slot int) {
			defer workers.Done()
			select {
			case <-ctx.Done():
				return
			case <-ready:
			}
			c.helper(ctx, slot)
		}(i)
	}
	go func() {
		workers.Wait()
		close(done)
	}()
	return done
}

// helper takes ids while the batch is big: more than ParallelThreshold were waiting at some moment, and until the queue has run dry (so the
// last twenty of a big batch are still crawled by all the crawlers, not by the main worker alone). Each helper has a pacing slot of its own
// (see crawl.PacingKey), or the Browser tool's per-platform pacing would make it wait in the main worker's line.
func (c *Consumer) helper(ctx context.Context, slot int) {
	workCtx := crawl.WithPacingSlot(ctx, slot)
	for {
		if ctx.Err() != nil {
			return
		}
		if c.helping() {
			select {
			case id := <-c.queue:
				c.take(id)
				c.process(workCtx, id)
				continue
			default:
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(helperPoll):
		}
	}
}

// helping says whether the helpers should take ids now: the batch is big, and something is waiting. A drained queue ends the batch.
func (c *Consumer) helping() bool {
	n := len(c.queue)
	if n > ParallelThreshold {
		c.big.Store(true)
	}
	if n == 0 {
		c.big.Store(false)
		return false
	}
	return c.big.Load()
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
// "filled" or "refreshed" job also sends "career.job.description_changed" (sent by the save, see below), so its persona scores are made again.
func Process(ctx context.Context, id string) {
	claimed, err := jobrecrawl.ClaimSlot(id, crawl.PacingSlot(ctx))
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
	// The phase of the page load goes into the request's row while it crawls (the Crawl Monitor shows it). It is a hint: a failed write is
	// ignored, and a job that never gets a phase (the Browser tool answers nothing while it waits for the platform's pacing) shows none.
	crawlCtx = crawl.WithPhaseObserver(crawlCtx, func(phase, message string) {
		_ = jobrecrawl.SetPhase(id, phase, message)
	})
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
		// A refreshed or filled-in text is announced by the save itself (jobs.SaveJobDetailExtraction publishes
		// "career.job.description_changed" when the text really changed, step 164), before this request is marked done, so a
		// page that sees "done" and asks for the scores again finds them asked for.
		outcome := outcomeFor(t, after.Description)
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
