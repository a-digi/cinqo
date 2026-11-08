package dashboard

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	// ProviderTimeout bounds one provider call. A slow tool shows as
	// "unavailable" rather than holding the page.
	ProviderTimeout = 3 * time.Second
	// FailureTTL is how long an unavailable/error result is reused, so a broken
	// widget is not retried on every refresh of every open tab.
	FailureTTL = 5 * time.Second

	maxCacheEntries = 1024
)

// Engine turns providers into widget data: it decides who may see what, calls
// the provider with a timeout, validates the payload against its kind, maps the
// outcome to a state, and caches the result per (widget, user).
type Engine struct {
	sources []Source
	now     func() time.Time
	timeout time.Duration
	logf    func(format string, args ...any)

	mu       sync.Mutex
	cache    map[string]cacheEntry
	inflight map[string]*call
}

type cacheEntry struct {
	data    Data
	expires time.Time
}

type call struct {
	done chan struct{}
	data Data
}

// NewEngine builds an engine over the given sources, consulted in order.
func NewEngine(sources ...Source) *Engine {
	return &Engine{
		sources:  sources,
		now:      time.Now,
		timeout:  ProviderTimeout,
		logf:     func(string, ...any) {},
		cache:    map[string]cacheEntry{},
		inflight: map[string]*call{},
	}
}

// WithLogger sets where provider failures are logged. They are logged, never
// sent to the browser.
func (e *Engine) WithLogger(logf func(format string, args ...any)) *Engine {
	e.logf = logf
	return e
}

func (e *Engine) providers() []Provider {
	var all []Provider
	seen := map[string]struct{}{}
	for _, s := range e.sources {
		for _, p := range s.Providers() {
			id := p.Definition().ID()
			if _, dup := seen[id]; dup {
				continue // the first source wins; ids are unique
			}
			seen[id] = struct{}{}
			all = append(all, p)
		}
	}
	return all
}

func visible(def Definition, caller Caller) bool {
	return def.RequiredScope == "" || caller.HasScope(def.RequiredScope)
}

// Widgets returns the definitions the caller may see, in registration order.
func (e *Engine) Widgets(caller Caller) []Definition {
	out := []Definition{}
	for _, p := range e.providers() {
		if def := p.Definition(); visible(def, caller) {
			out = append(out, def)
		}
	}
	return out
}

func (e *Engine) find(caller Caller, id string) (Provider, bool) {
	for _, p := range e.providers() {
		if def := p.Definition(); def.ID() == id && visible(def, caller) {
			return p, true
		}
	}
	return nil, false
}

// Data returns the widget's current data for the caller. ok is false when the
// widget does not exist or the caller may not see it — the two are
// indistinguishable on purpose. Everything else, including a failing provider,
// is a Data with a state.
func (e *Engine) Data(ctx context.Context, caller Caller, id string) (data Data, ok bool) {
	p, ok := e.find(caller, id)
	if !ok {
		return Data{}, false
	}
	def := p.Definition()
	key := id + "\x00" + caller.UserID

	e.mu.Lock()
	if c, hit := e.cache[key]; hit && e.now().Before(c.expires) {
		e.mu.Unlock()
		return c.data, true
	}
	if f, running := e.inflight[key]; running {
		e.mu.Unlock()
		select {
		case <-f.done:
			return f.data, true
		case <-ctx.Done():
			return e.result(def, StateUnavailable, nil), true
		}
	}
	f := &call{done: make(chan struct{})}
	e.inflight[key] = f
	e.mu.Unlock()

	// Detached from the caller's context: the result is shared with whoever
	// joined this call, and cached, so one caller leaving must not cancel it.
	f.data = e.fetch(context.WithoutCancel(ctx), p, def, caller)

	e.mu.Lock()
	ttl := time.Duration(def.RefreshSeconds) * time.Second
	if f.data.State == StateUnavailable || f.data.State == StateError {
		ttl = FailureTTL
	}
	e.store(key, f.data, ttl)
	delete(e.inflight, key)
	e.mu.Unlock()
	close(f.done)
	return f.data, true
}

func (e *Engine) store(key string, d Data, ttl time.Duration) {
	if len(e.cache) >= maxCacheEntries {
		now := e.now()
		for k, c := range e.cache {
			if !now.Before(c.expires) {
				delete(e.cache, k)
			}
		}
		if len(e.cache) >= maxCacheEntries {
			e.cache = map[string]cacheEntry{}
		}
	}
	e.cache[key] = cacheEntry{data: d, expires: e.now().Add(ttl)}
}

func (e *Engine) result(def Definition, state State, payload []byte) Data {
	return Data{
		State:         state,
		Kind:          def.Kind,
		SchemaVersion: SchemaVersion,
		Data:          payload,
		FetchedAt:     e.now().UTC().Format(time.RFC3339),
	}
}

// fetch calls the provider, bounded by the timeout and shielded from a panic,
// and maps what came back to a state.
func (e *Engine) fetch(ctx context.Context, p Provider, def Definition, caller Caller) Data {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	type outcome struct {
		raw []byte
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- outcome{err: fmt.Errorf("provider panicked: %v", r)}
			}
		}()
		raw, err := p.Data(ctx, caller)
		done <- outcome{raw: raw, err: err}
	}()

	var out outcome
	select {
	case out = <-done:
	case <-ctx.Done():
		e.logf("dashboard: widget %s timed out after %s", def.ID(), e.timeout)
		return e.result(def, StateUnavailable, nil)
	}

	if out.err != nil {
		if errors.Is(out.err, ErrUnavailable) {
			return e.result(def, StateUnavailable, nil)
		}
		e.logf("dashboard: widget %s failed: %v", def.ID(), out.err)
		return e.result(def, StateError, nil)
	}
	clean, err := Validate(def.Kind, SchemaVersion, out.raw)
	if err != nil {
		e.logf("dashboard: widget %s returned invalid data: %v", def.ID(), err)
		return e.result(def, StateError, nil)
	}
	if IsEmpty(def.Kind, clean) {
		return e.result(def, StateEmpty, clean)
	}
	return e.result(def, StateOK, clean)
}

var (
	defaultMu     sync.RWMutex
	defaultEngine *Engine
)

// SetDefault installs the process-wide engine the HTTP handlers use. Called
// once at boot, the same convention as domainevent.SetDB.
func SetDefault(e *Engine) {
	defaultMu.Lock()
	defaultEngine = e
	defaultMu.Unlock()
}

// Default returns the engine installed by SetDefault, or nil.
func Default() *Engine {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultEngine
}
