// Package system reports what the computer Cinqo runs on is doing right now, for the resource bar under the top bar of the shell
// (plan 222): the memory, and how much of it Cinqo itself holds. It stores nothing; every number is read from the operating system.
package system

import (
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/security/scopes/entity"
	"github.com/a-digi/cinqo/src/system/memory"
)

// ScopeGroup is registered with the scope registry in routes.Init, next to the other domains' groups.
var ScopeGroup = entity.ScopeGroup{
	ID:          "cinqo:system",
	Description: "What the computer Cinqo runs on is doing (the resource bar).",
	Scopes: []entity.Scope{
		{ID: "cinqo:system:read", Description: "Can view the memory of the computer and how much of it Cinqo uses."},
	},
}

// cacheFor is how long one reading answers every caller: many open tabs cannot multiply the calls to the operating system.
const cacheFor = 2 * time.Second

// Resources is the body of GET /api/v1/system/resources. A value that could not be read is 0 / "unknown": the bar then shows it
// grey and never as an alarm.
type Resources struct {
	Memory    MemoryBody `json:"memory"`
	Cinqo     CinqoBody  `json:"cinqo"`
	SampledAt time.Time  `json:"sampledAt"`
}

type MemoryBody struct {
	TotalBytes     uint64 `json:"totalBytes"`
	AvailableBytes uint64 `json:"availableBytes"`
	SwapUsedBytes  uint64 `json:"swapUsedBytes"`
	Pressure       string `json:"pressure"` // normal, warn, critical or unknown
}

type CinqoBody struct {
	AppBytes     uint64 `json:"appBytes"`     // the resident memory of the Cinqo program itself
	LocalAiBytes uint64 `json:"localAiBytes"` // the resident memory of the local model program; 0 while it does not run
}

// Reader takes a reading and keeps it for cacheFor. The sources are fields so the tests can replace them.
type Reader struct {
	memory   func() memory.MemoryInfo
	resident func(pid int) uint64
	pid      func() int
	localPid func() int // the pid of the local model program, 0 when none runs
	now      func() time.Time

	mu     sync.Mutex
	cached Resources
	at     time.Time
}

// NewReader reads the real computer.
func NewReader() *Reader {
	return &Reader{memory: memory.Memory, resident: memory.Resident, pid: os.Getpid, localPid: localAiPid, now: time.Now}
}

// The local AI module tells the shell which process is its model program, so this package never imports the module.
var localAiSource atomic.Pointer[func() int]

// SetLocalAiPid is called once by the local AI module at start-up with a function that returns the pid of its model program now
// (0 when it is not running). Without it the share of the local AI is 0.
func SetLocalAiPid(f func() int) { localAiSource.Store(&f) }

func localAiPid() int {
	if f := localAiSource.Load(); f != nil {
		return (*f)()
	}
	return 0
}

// Read returns the cached reading while it is younger than cacheFor, else reads again.
func (r *Reader) Read() Resources {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.now()
	if !r.at.IsZero() && now.Sub(r.at) >= 0 && now.Sub(r.at) < cacheFor {
		return r.cached
	}
	m := r.memory()
	r.cached = Resources{
		Memory: MemoryBody{
			TotalBytes:     m.TotalBytes,
			AvailableBytes: m.AvailableBytes,
			SwapUsedBytes:  m.SwapUsedBytes,
			Pressure:       string(m.Pressure),
		},
		Cinqo:     CinqoBody{AppBytes: r.resident(r.pid()), LocalAiBytes: r.residentOf(r.localPid)},
		SampledAt: now.UTC().Truncate(time.Second),
	}
	r.at = now
	return r.cached
}

// residentOf is the resident memory of the process a function names; 0 when there is no function or no process.
func (r *Reader) residentOf(pid func() int) uint64 {
	if pid == nil {
		return 0
	}
	return r.resident(pid())
}

// shared is the one reader (and so the one cache) of the program.
var shared = NewReader()

// GetHandler handles GET /api/v1/system/resources. Read-only: no body, no query, nothing the caller sends reaches the operating system.
func GetHandler(reqCtx request.RequestContext) {
	response.SuccessResponse(reqCtx.GetWriter(), http.StatusOK, shared.Read())
}
