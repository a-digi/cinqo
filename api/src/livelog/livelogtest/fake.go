// Package livelogtest is an in-memory Provider and Follower for tests of the live log module and of everything that sits on top of
// it (the HTTP handler, a provider's wiring): a log that can grow, be truncated and be replaced, with the same cursor rules a real
// follower keeps. It imports only the module's core.
package livelogtest

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/a-digi/cinqo/src/livelog"
)

// Log is one in-memory log: lines with byte offsets, and an identity that changes when the log is replaced.
type Log struct {
	mu            sync.Mutex
	lines         []string
	version       int
	Delay         time.Duration // how long a poll takes (honours the context unless IgnoreContext)
	Err           error         // returned by every poll when set
	IgnoreContext bool
	// Asked records the limit of every poll, newest last.
	Asked []int
}

func NewLog(lines ...string) *Log { return &Log{lines: append([]string(nil), lines...)} }

func (l *Log) Append(lines ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, lines...)
}

// Truncate empties the log (the same identity, a smaller size than any cursor that was behind it).
func (l *Log) Truncate() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = nil
}

// Replace swaps the content for new lines and changes the identity (a rotated file).
func (l *Log) Replace(lines ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append([]string(nil), lines...)
	l.version++
}

func (l *Log) identity() string { return "v" + strconv.Itoa(l.version) }

func (l *Log) size() int64 {
	var n int64
	for _, s := range l.lines {
		n += int64(len(s)) + 1
	}
	return n
}

// Poll follows the contract of livelog.Follower.
func (l *Log) Poll(ctx context.Context, cursor string, limit int) (livelog.Batch, error) {
	l.mu.Lock()
	l.Asked = append(l.Asked, limit)
	delay, err, ignore := l.Delay, l.Err, l.IgnoreContext
	l.mu.Unlock()
	if delay > 0 {
		if ignore {
			time.Sleep(delay)
		} else {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return livelog.Batch{}, ctx.Err()
			}
		}
	}
	if err != nil {
		return livelog.Batch{}, err
	}
	c, present, perr := livelog.ParseCursor(cursor)
	if perr != nil {
		return livelog.Batch{}, perr
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	end := l.size()
	reset := false
	if present && (c.Identity != l.identity() || c.Offset > end) {
		reset, present = true, false
	}
	var from int64
	first := 0
	if !present {
		// start at the end: the last `limit` lines
		first = len(l.lines) - limit
		if first < 0 {
			first = 0
		}
		for _, s := range l.lines[:first] {
			from += int64(len(s)) + 1
		}
	} else {
		from = c.Offset
		var pos int64
		for first < len(l.lines) && pos < from {
			pos += int64(len(l.lines[first])) + 1
			first++
		}
	}
	b := livelog.Batch{Lines: []livelog.Line{}, Reset: reset}
	pos := from
	i := first
	for ; i < len(l.lines) && len(b.Lines) < limit; i++ {
		s := l.lines[i]
		b.Lines = append(b.Lines, livelog.Line{Offset: pos, Text: s, Bytes: len(s)})
		pos += int64(len(s)) + 1
	}
	b.EOF = i >= len(l.lines)
	b.Cursor = livelog.Cursor{Offset: pos, Identity: l.identity()}.Encode()
	return b, nil
}

// Provider serves named logs under a fixed id, title and scopes.
type Provider struct {
	IDValue    string
	TitleValue string
	ScopeList  []string
	Logs       map[string]*Log
}

func (p *Provider) ID() string       { return p.IDValue }
func (p *Provider) Title() string    { return p.TitleValue }
func (p *Provider) Scopes() []string { return p.ScopeList }

func (p *Provider) Open(ref string) (livelog.Follower, error) {
	if l, ok := p.Logs[ref]; ok {
		return l, nil
	}
	return nil, livelog.ErrNotFound
}
