// Package livelog is the module that follows a log while it is being written ("tail -f"), apart from the file reader
// (api/src/filereader), which reads a finished file in windows and stays as it is. See plan/ai/file-reader/live-follow.md.
//
// The module is built to be removable and extendable without touching anything else:
//   - everything it knows about a log is the two interfaces below (a Provider says where a kind of log lives and who may read
//     it, a Follower reads what was appended since a cursor), so the HTTP side, the limits and the audit never learn what a
//     file, a table or a buffer is;
//   - a new kind of log is one file that implements Provider plus one line that registers it where the program is composed;
//   - nothing outside this folder imports it except that composition (boundary_test.go checks it), and this folder's core imports
//     nothing else of the program, so deleting it is a deletion.
package livelog

import (
	"context"
	"errors"
)

// Line is one line of a batch. Offset is the byte position in the log where the line starts (a live log has no line numbers:
// numbering needs a full scan); Bytes is the line's real length, Text is shorter when Truncated.
type Line struct {
	Offset    int64  `json:"offset"`
	Text      string `json:"text"`
	Bytes     int    `json:"bytes"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Batch is what one poll returns: the lines appended since the cursor (complete lines only), the cursor to ask with next, Reset
// when the log was truncated or replaced so the lines start over (the viewer clears what it holds), and EOF when nothing more is
// waiting right now (false: ask again at once).
type Batch struct {
	Lines  []Line `json:"lines"`
	Cursor string `json:"cursor"`
	Reset  bool   `json:"reset"`
	EOF    bool   `json:"eof"`
}

// Follower reads what was appended to ONE log since a cursor. It holds no connection and no state of its own: everything it needs
// to continue is in the cursor, so any number of requests, in any order, may share one.
//
// cursor is the opaque text a previous Poll returned, or "" for "start at the end" (the last limit lines, then follow). limit is
// the most lines to return. A cursor that is not valid is ErrBadCursor; one that does not fit the log (it was truncated, replaced or
// the cursor is beyond its end) is not an error: the Batch says Reset and starts again at the end.
type Follower interface {
	Poll(ctx context.Context, cursor string, limit int) (Batch, error)
}

// Provider is one kind of log that can be followed.
type Provider interface {
	// ID names the provider in a URL: lower-case words joined by hyphens ("ai-trace").
	ID() string
	// Title is what a person sees ("AI trace logs").
	Title() string
	// Scopes lists the scopes that may read this provider (any one is enough); AdminScope always may.
	Scopes() []string
	// Open turns a reference into the Follower of one log, or ErrNotFound. The reference is resolved by the provider under a root
	// the server owns: it never is a client's path, and every way it can fail to name a readable log is the same ErrNotFound, so a
	// caller learns nothing about what exists.
	Open(ref string) (Follower, error)
}

// The errors a provider or a follower answers with; the HTTP side maps each to one status.
var (
	// ErrNotFound: the reference does not name a log the caller may know about.
	ErrNotFound = errors.New("livelog: not found")
	// ErrBadCursor: the cursor is not one this module ever produced.
	ErrBadCursor = errors.New("livelog: bad cursor")
	// ErrNotText: the log holds binary data, which is never shown.
	ErrNotText = errors.New("livelog: not a text log")
	// ErrBusy: the provider cannot take another reader right now (the caller backs off and asks again).
	ErrBusy = errors.New("livelog: busy")
	// ErrTimeout: a poll took longer than the limit allows.
	ErrTimeout = errors.New("livelog: the poll took too long")
	// ErrBadBatch: a follower broke the contract (more lines than asked, a cursor that is not valid).
	ErrBadBatch = errors.New("livelog: the follower returned an invalid batch")
)

// AdminScope satisfies every provider's scope check, as everywhere else in Cinqo. It is written out here, not imported, so the
// module needs nothing of the program.
const AdminScope = "cinqo:super:admin"
