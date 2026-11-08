// Package filereader is Cinqo's file reader engine: it reads big text files a window of lines at a
// time without ever loading them. Everything is bounded: one fixed buffer, a cap on the lines, the
// bytes of a response and of a single line, the size of a file and the number of reads running at once.
// See plan/ai/file-reader/file-reader-engine.md.
//
// The engine takes an absolute path and trusts it: deciding WHICH file a caller may read (a source
// reference resolved under a root, the scope check, symlink containment) is the source layer's job, never
// the engine's, and a client never sends a path.
package filereader

import (
	"errors"
	"time"
)

var (
	// ErrBadRange: from or count is out of range (from < 1, count < 1).
	ErrBadRange = errors.New("filereader: line range out of bounds")
	// ErrNotIndexed: the requested line is further than the index has got so far. Ask Info (each call
	// advances the index) and try again.
	ErrNotIndexed = errors.New("filereader: line not indexed yet")
	// ErrBusy: too many reads are running at once, and none finished within MaxWait.
	ErrBusy = errors.New("filereader: too many reads in progress")
	// ErrBinary: the file contains NUL bytes where text is expected.
	ErrBinary = errors.New("filereader: file is not text")
	// ErrTooLarge: the file is bigger than Limits.MaxFileBytes.
	ErrTooLarge = errors.New("filereader: file is too large")
	// ErrBadQuery: the search text is empty, too long, not valid text, or contains a NUL.
	ErrBadQuery = errors.New("filereader: invalid search text")
	// ErrSlow: a read ran past Limits.ReadBudget (a huge line on a slow disk). Ask again, or ask for less.
	ErrSlow = errors.New("filereader: the read took too long")
	// ErrNotRegular: the path is not a regular file (a directory, a device, a pipe).
	ErrNotRegular = errors.New("filereader: not a regular file")
)

// Limits is every bound the engine enforces. The zero value is not usable: start from DefaultLimits.
type Limits struct {
	// MaxLines is the most lines one window may hold.
	MaxLines int
	// MaxResponseBytes caps the text bytes of one window; a window that would exceed it ends early and says
	// there is more (HasMore, NextFrom).
	MaxResponseBytes int
	// MaxLineBytes is how much of one line is kept; the rest is counted (Line.Bytes) but dropped, and the
	// line is flagged Truncated.
	MaxLineBytes int
	// MaxFileBytes refuses bigger files outright, which bounds the worst-case time to index one.
	MaxFileBytes int64
	// IndexSlice is how long one Info call may spend advancing the index.
	IndexSlice time.Duration
	// IndexChunkBytes is the size of one read while indexing.
	IndexChunkBytes int
	// CheckpointEvery is how many lines lie between two index checkpoints.
	CheckpointEvery int
	// MaxConcurrent is how many reads may run at once, engine-wide; MaxWait is how long a read waits
	// for a place before it gives up with ErrBusy.
	MaxConcurrent int
	MaxWait       time.Duration
	// MaxCachedFiles bounds the index cache (least recently used goes first).
	MaxCachedFiles int

	// LongLineBytes is the length above which the index remembers where a line starts and how long it is, so
	// windows jump over it instead of reading it through, and a long line can be read in segments by byte offset.
	LongLineBytes int
	// MaxSegmentBytes is the most one segment of a long line may hold; ReadBudget is how long one window or
	// segment read may run before it stops with ErrSlow (also checked in the middle of a long line).
	MaxSegmentBytes int
	ReadBudget      time.Duration

	// Search: the most matches one call returns, the most file bytes and the most time one call may spend
	// scanning (a call that runs out says where to continue), the longest search text, and how many characters
	// of a matching line a match carries as its preview.
	SearchMaxMatches int
	SearchMaxBytes   int64
	SearchBudget     time.Duration
	MaxQueryBytes    int
	PreviewRunes     int
	// SearchChunkBytes is the size of one read while searching.
	SearchChunkBytes int
}

func DefaultLimits() Limits {
	return Limits{
		MaxLines:         500,
		MaxResponseBytes: 2 << 20,
		MaxLineBytes:     16 << 10,
		MaxFileBytes:     1 << 30,
		IndexSlice:       750 * time.Millisecond,
		IndexChunkBytes:  256 << 10,
		CheckpointEvery:  1000,
		MaxConcurrent:    4,
		MaxWait:          2 * time.Second,
		MaxCachedFiles:   64,

		LongLineBytes:   64 << 10,
		MaxSegmentBytes: 16 << 10,
		ReadBudget:      5 * time.Second,

		SearchMaxMatches: 50,
		SearchMaxBytes:   64 << 20,
		SearchBudget:     time.Second,
		MaxQueryBytes:    200,
		PreviewRunes:     200,
		SearchChunkBytes: 256 << 10,
	}
}
