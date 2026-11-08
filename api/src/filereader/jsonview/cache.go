package jsonview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrBusy: too many conversions are running and none finished within MaxWait. Ask again in a moment.
	ErrBusy = errors.New("jsonview: too many files are being formatted")
	// ErrSlow: formatting ran past ConvertTimeout.
	ErrSlow = errors.New("jsonview: formatting took too long")
	// ErrChanged: the file changed while it was being formatted, so the copy would mix two versions and is not kept. Ask again.
	ErrChanged = errors.New("jsonview: the file changed while it was formatted")
	// ErrNotRegular: the path is not a regular file.
	ErrNotRegular = errors.New("jsonview: not a regular file")
)

// CacheLimits bounds the derived copies and the work of making them. A zero field takes its default.
type CacheLimits struct {
	// MaxFiles and MaxBytes bound what is kept on disk; the least recently used copy goes first.
	MaxFiles int
	MaxBytes int64
	// MaxConcurrent is how many conversions may run at once, for the whole program; a conversion that cannot start within MaxWait
	// answers ErrBusy.
	MaxConcurrent int
	MaxWait       time.Duration
	// ConvertTimeout bounds one conversion.
	ConvertTimeout time.Duration
	// Format bounds one document (size, output size, nesting).
	Format Limits
}

// DefaultCacheLimits are the limits of plan 202: 64 files and 256 MiB on disk, two conversions at a time, 2 s to wait for a turn,
// 5 s to convert.
func DefaultCacheLimits() CacheLimits {
	return CacheLimits{MaxFiles: 64, MaxBytes: 256 << 20, MaxConcurrent: 2, MaxWait: 2 * time.Second, ConvertTimeout: 5 * time.Second}
}

func (l CacheLimits) withDefaults() CacheLimits {
	d := DefaultCacheLimits()
	if l.MaxFiles <= 0 {
		l.MaxFiles = d.MaxFiles
	}
	if l.MaxBytes <= 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxConcurrent <= 0 {
		l.MaxConcurrent = d.MaxConcurrent
	}
	if l.MaxWait <= 0 {
		l.MaxWait = d.MaxWait
	}
	if l.ConvertTimeout <= 0 {
		l.ConvertTimeout = d.ConvertTimeout
	}
	l.Format = l.Format.withDefaults()
	return l
}

// Derived is an indented copy of a file.
type Derived struct {
	// Path is the copy, an absolute path inside the cache folder, named by a hash: a caller hands it to the file reader engine and never
	// to a client.
	Path string
	// SourceBytes is the size of the original, Bytes the size of the copy.
	SourceBytes int64
	Bytes       int64
}

type entry struct {
	path     string
	bytes    int64
	lastUsed time.Time
}

type flight struct {
	done   chan struct{}
	result Derived
	err    error
}

// Cache makes and keeps the indented copies. A copy is named by a hash of the file's identity (the caller's source and reference), its
// size and its modification time, so a file that changes simply gets a new copy. One Cache serves the whole program (it owns the
// conversion semaphore).
type Cache struct {
	dir string
	lim CacheLimits
	sem chan struct{}

	mu       sync.Mutex
	entries  map[string]*entry
	total    int64
	inflight map[string]*flight

	conversions atomic.Int64 // how many conversions ran (for tests)

	// test hooks: called while a conversion holds its turn, and right after the formatting succeeded
	hookBefore func(ctx context.Context)
	hookAfter  func()
}

// ourFile is the shape of a file this cache makes: a hash and ".txt", or a half-written ".tmp".
var ourFile = regexp.MustCompile(`^[0-9a-f]{64}\.txt$|^[0-9a-f]{64}\..+\.tmp$`)

// NewCache creates the cache folder (private to this program) and empties it of copies an earlier run left behind: a copy is cheap to
// make again and a stale one is dead weight. Anything in the folder that is not named like a copy is left alone.
func NewCache(dir string, lim CacheLimits) (*Cache, error) {
	if dir == "" {
		return nil, errors.New("jsonview: no cache folder")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("jsonview: cache folder: %w", err)
	}
	items, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if !it.IsDir() && ourFile.MatchString(it.Name()) {
			_ = os.Remove(filepath.Join(abs, it.Name()))
		}
	}
	lim = lim.withDefaults()
	return &Cache{dir: abs, lim: lim, sem: make(chan struct{}, lim.MaxConcurrent), entries: map[string]*entry{}, inflight: map[string]*flight{}}, nil
}

func keyOf(identity string, size int64, mtime time.Time) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", identity, size, mtime.UnixNano())))
	return hex.EncodeToString(sum[:])
}

// Ensure returns the indented copy of the file at srcPath, making it when there is none for this version of the file. identity names
// the file for the cache (the source and reference the caller resolved srcPath from): the same file reached under two identities has two
// copies, so a copy is never shared across a scope boundary by accident. The caller has already decided that this person may read the
// file; the cache does not know about scopes.
//
// Errors: *SyntaxError (not valid JSON), ErrTooLarge, ErrTooDeep, ErrBusy, ErrSlow, ErrChanged, ErrNotRegular, an error of the file
// system, or the caller's context error. Nothing is kept for a file that failed.
func (c *Cache) Ensure(ctx context.Context, identity, srcPath string) (Derived, error) {
	for {
		st, err := os.Stat(srcPath)
		if err != nil {
			return Derived{}, err
		}
		if !st.Mode().IsRegular() {
			return Derived{}, ErrNotRegular
		}
		if st.Size() > c.lim.Format.MaxBytes {
			return Derived{}, ErrTooLarge
		}
		key := keyOf(identity, st.Size(), st.ModTime())

		c.mu.Lock()
		if e, ok := c.entries[key]; ok {
			if _, err := os.Stat(e.path); err == nil {
				e.lastUsed = time.Now()
				d := Derived{Path: e.path, SourceBytes: st.Size(), Bytes: e.bytes}
				c.mu.Unlock()
				return d, nil
			}
			c.total -= e.bytes // the copy vanished from the disk: forget it and make it again
			delete(c.entries, key)
		}
		if fl, ok := c.inflight[key]; ok {
			c.mu.Unlock()
			select {
			case <-fl.done:
			case <-ctx.Done():
				return Derived{}, ctx.Err()
			}
			// Whoever was converting was stopped by its own context (not this caller's): try again, now as the converter.
			if fl.err != nil && (errors.Is(fl.err, context.Canceled) || errors.Is(fl.err, context.DeadlineExceeded)) && ctx.Err() == nil {
				continue
			}
			return fl.result, fl.err
		}
		fl := &flight{done: make(chan struct{})}
		c.inflight[key] = fl
		c.mu.Unlock()

		fl.result, fl.err = c.convert(ctx, key, srcPath, st)

		c.mu.Lock()
		delete(c.inflight, key)
		c.mu.Unlock()
		close(fl.done)
		return fl.result, fl.err
	}
}

// convert takes a turn, formats the file into a temporary file and renames it into place.
func (c *Cache) convert(ctx context.Context, key, srcPath string, st fs.FileInfo) (Derived, error) {
	timer := time.NewTimer(c.lim.MaxWait)
	defer timer.Stop()
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return Derived{}, ctx.Err()
	case <-timer.C:
		return Derived{}, ErrBusy
	}
	c.conversions.Add(1)

	cctx, cancel := context.WithTimeout(ctx, c.lim.ConvertTimeout)
	defer cancel()
	if c.hookBefore != nil {
		c.hookBefore(cctx)
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return Derived{}, err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(c.dir, key+".*.tmp")
	if err != nil {
		return Derived{}, err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := Format(cctx, tmp, src, c.lim.Format); err != nil {
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return Derived{}, ErrSlow
		}
		return Derived{}, err
	}
	if c.hookAfter != nil {
		c.hookAfter()
	}
	// The copy is only good for the version of the file it was made from.
	after, err := os.Stat(srcPath)
	if err != nil {
		return Derived{}, err
	}
	if after.Size() != st.Size() || !after.ModTime().Equal(st.ModTime()) {
		return Derived{}, ErrChanged
	}
	info, err := tmp.Stat()
	if err != nil {
		return Derived{}, err
	}
	if err := tmp.Close(); err != nil {
		return Derived{}, err
	}
	final := filepath.Join(c.dir, key+".txt")
	if err := os.Rename(tmpPath, final); err != nil {
		return Derived{}, err
	}
	keep = true

	c.mu.Lock()
	if old, ok := c.entries[key]; ok { // cannot happen while single-flight holds, but never count a copy twice
		c.total -= old.bytes
	}
	c.entries[key] = &entry{path: final, bytes: info.Size(), lastUsed: time.Now()}
	c.total += info.Size()
	c.evictLocked(key)
	c.mu.Unlock()
	return Derived{Path: final, SourceBytes: st.Size(), Bytes: info.Size()}, nil
}

// evictLocked removes the least recently used copies until the bounds hold, never the one just made (keep).
func (c *Cache) evictLocked(keep string) {
	if len(c.entries) <= c.lim.MaxFiles && c.total <= c.lim.MaxBytes {
		return
	}
	type kv struct {
		key string
		e   *entry
	}
	order := make([]kv, 0, len(c.entries))
	for k, e := range c.entries {
		if k != keep {
			order = append(order, kv{k, e})
		}
	}
	sort.Slice(order, func(i, j int) bool { return order[i].e.lastUsed.Before(order[j].e.lastUsed) })
	for _, o := range order {
		if len(c.entries) <= c.lim.MaxFiles && c.total <= c.lim.MaxBytes {
			return
		}
		_ = os.Remove(o.e.path)
		c.total -= o.e.bytes
		delete(c.entries, o.key)
	}
}

// Stats says how many copies are kept and how many bytes they take (for tests and for a status line).
func (c *Cache) Stats() (files int, bytes int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries), c.total
}

// Dir is the cache folder.
func (c *Cache) Dir() string { return c.dir }
