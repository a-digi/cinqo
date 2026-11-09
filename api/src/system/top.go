package system

import (
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/system/memory"
)

// TopCount is how many applications the memory dropdown lists (plan 226, Q6).
const TopCount = 10

// TopApps is the body of GET /api/v1/system/memory-top: the applications that use the most memory, biggest first, with the computer's total
// memory so the page can show each one's share. Apps is never null; Note says why it is empty when the system cannot be read.
type TopApps struct {
	SampledAt  time.Time    `json:"sampledAt"`
	TotalBytes uint64       `json:"totalBytes"`
	Apps       []memory.App `json:"apps"`
	Note       string       `json:"note,omitempty"`
}

// TopReader takes a reading of the process table and keeps it for the same short time as the resource reading, so many open dropdowns cost one
// `ps`. The sources are fields so the tests can replace them.
type TopReader struct {
	table  func() []memory.ProcRow
	total  func() uint64
	ownPID func() int
	now    func() time.Time

	mu     sync.Mutex
	cached TopApps
	at     time.Time
}

// NewTopReader reads the real computer.
func NewTopReader() *TopReader {
	return &TopReader{
		table:  memory.ReadProcesses,
		total:  func() uint64 { return memory.Memory().TotalBytes },
		ownPID: os.Getpid,
		now:    time.Now,
	}
}

// Read returns the cached reading while it is younger than cacheFor, else reads again.
func (r *TopReader) Read() TopApps {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if !r.at.IsZero() && now.Sub(r.at) >= 0 && now.Sub(r.at) < cacheFor {
		return r.cached
	}
	out := TopApps{SampledAt: now.UTC().Truncate(time.Second), TotalBytes: r.total(), Apps: []memory.App{}}
	if rows := r.table(); rows == nil {
		out.Note = "The list of running programs cannot be read on this system."
	} else {
		out.Apps = memory.TopApps(rows, r.ownPID(), TopCount)
	}
	r.cached, r.at = out, now
	return out
}

var sharedTop = NewTopReader()

// TopHandler handles GET /api/v1/system/memory-top. Read-only: no body, no query, nothing the caller sends reaches the operating system.
func TopHandler(reqCtx request.RequestContext) {
	response.SuccessResponse(reqCtx.GetWriter(), http.StatusOK, sharedTop.Read())
}
