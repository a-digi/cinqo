package livelog

import "time"

// Limits bound one poll. The defaults are what the plan states; a provider may use a smaller bound of its own, never a larger one.
type Limits struct {
	// DefaultLines is how many lines a first poll (no cursor) returns when the caller does not say.
	DefaultLines int
	// MaxLines is the most lines one poll returns.
	MaxLines int
	// MaxBatchBytes is the most bytes of log one poll reads.
	MaxBatchBytes int
	// MaxLineBytes is the longest line returned whole: longer ones are cut and flagged.
	MaxLineBytes int
	// PollTimeout is how long one poll may take before the caller gets ErrTimeout.
	PollTimeout time.Duration
}

// DefaultLimits are 200 lines for a first poll, at most 500 lines and 256 KiB per poll, lines cut at 16 KiB (as the file reader
// does), a poll bounded at 10 seconds.
func DefaultLimits() Limits {
	return Limits{DefaultLines: 200, MaxLines: 500, MaxBatchBytes: 256 << 10, MaxLineBytes: 16 << 10, PollTimeout: 10 * time.Second}
}

// Lines is the number of lines to ask a follower for: the caller's ask (0 or less means the default), at most MaxLines.
func (l Limits) Lines(asked int) int {
	if asked <= 0 {
		asked = l.DefaultLines
	}
	if asked > l.MaxLines {
		asked = l.MaxLines
	}
	return asked
}

// normalized fills every bound that is zero or negative with its default, so a Limits built by hand (or left empty in a test) can
// neither end every poll at once nor allow an unbounded one.
func (l Limits) normalized() Limits {
	d := DefaultLimits()
	if l.DefaultLines <= 0 {
		l.DefaultLines = d.DefaultLines
	}
	if l.MaxLines <= 0 {
		l.MaxLines = d.MaxLines
	}
	if l.MaxBatchBytes <= 0 {
		l.MaxBatchBytes = d.MaxBatchBytes
	}
	if l.MaxLineBytes <= 0 {
		l.MaxLineBytes = d.MaxLineBytes
	}
	if l.PollTimeout <= 0 {
		l.PollTimeout = d.PollTimeout
	}
	return l
}

// Normalized is the limits with every unset bound at its default (providers use it to read their own bounds the same way).
func (l Limits) Normalized() Limits { return l.normalized() }
