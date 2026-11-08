package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeProvider struct {
	def   Definition
	calls atomic.Int32
	data  func(ctx context.Context, caller Caller) ([]byte, error)
}

func (f *fakeProvider) Definition() Definition { return f.def }
func (f *fakeProvider) Data(ctx context.Context, caller Caller) ([]byte, error) {
	f.calls.Add(1)
	return f.data(ctx, caller)
}

func statDef(key, scope string) Definition {
	return Definition{
		Provider: "test", Key: key, Title: "T " + key, Kind: KindStat,
		Size: Size{W: 1, H: 1}, RefreshSeconds: 60, RequiredScope: scope,
	}
}

func okStat(context.Context, Caller) ([]byte, error) {
	return []byte(`{"label":"Jobs","value":3,"extra":"x"}`), nil
}

func provider(key, scope string, data func(context.Context, Caller) ([]byte, error)) *fakeProvider {
	return &fakeProvider{def: statDef(key, scope), data: data}
}

func engineOf(t *testing.T, ps ...Provider) *Engine {
	t.Helper()
	r := NewRegistry()
	for _, p := range ps {
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	return NewEngine(r)
}

var alice = Caller{UserID: "alice", Scopes: []string{"tool:test:read"}}

func TestDefinitionValidate(t *testing.T) {
	if err := statDef("ok", "").Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Definition){
		"bad provider": func(d *Definition) { d.Provider = "Bad-Name" },
		"bad key":      func(d *Definition) { d.Key = "1x" },
		"no title":     func(d *Definition) { d.Title = " " },
		"bad kind":     func(d *Definition) { d.Kind = "table" },
		"too wide":     func(d *Definition) { d.Size.W = MaxWidth + 1 },
		"too tall":     func(d *Definition) { d.Size.H = 0 },
		"fast refresh": func(d *Definition) { d.RefreshSeconds = MinRefreshSeconds - 1 },
		"slow refresh": func(d *Definition) { d.RefreshSeconds = MaxRefreshSeconds + 1 },
	} {
		d := statDef("ok", "")
		mutate(&d)
		if err := d.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if got := statDef("jobs", "").ID(); got != "test.jobs" {
		t.Errorf("ID = %q", got)
	}
}

func TestRegistryRejectsDuplicatesAndInvalid(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(provider("a", "", okStat)); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(provider("a", "", okStat)); err == nil {
		t.Error("duplicate id accepted")
	}
	bad := provider("b", "", okStat)
	bad.def.Kind = "nope"
	if err := r.Register(bad); err == nil {
		t.Error("invalid definition accepted")
	}
	if got := len(r.Providers()); got != 1 {
		t.Errorf("providers = %d, want 1", got)
	}
}

func TestWidgetsAreFilteredByScope(t *testing.T) {
	e := engineOf(t,
		provider("open", "", okStat),
		provider("scoped", "tool:test:read", okStat),
		provider("secret", "tool:test:admin", okStat),
	)
	ids := func(c Caller) []string {
		var out []string
		for _, d := range e.Widgets(c) {
			out = append(out, d.ID())
		}
		return out
	}
	if got := ids(Caller{UserID: "x"}); len(got) != 1 || got[0] != "test.open" {
		t.Errorf("no scopes: %v", got)
	}
	if got := ids(alice); len(got) != 2 || got[0] != "test.open" || got[1] != "test.scoped" {
		t.Errorf("alice: %v (registration order, own scope only)", got)
	}
	if got := ids(Caller{UserID: "root", Scopes: []string{AdminScope}}); len(got) != 3 {
		t.Errorf("super admin should see all: %v", got)
	}
}

func TestDataOfForbiddenOrUnknownWidgetIsNotFound(t *testing.T) {
	p := provider("secret", "tool:test:admin", okStat)
	e := engineOf(t, p)
	for _, id := range []string{"test.secret", "test.missing", "", "nodot"} {
		if _, ok := e.Data(context.Background(), alice, id); ok {
			t.Errorf("%q: found", id)
		}
	}
	if p.calls.Load() != 0 {
		t.Error("a forbidden widget's provider must never be called")
	}
}

func TestDataOKIsValidatedAndNormalised(t *testing.T) {
	e := engineOf(t, provider("jobs", "", okStat))
	d, ok := e.Data(context.Background(), alice, "test.jobs")
	if !ok || d.State != StateOK || d.Kind != KindStat || d.SchemaVersion != SchemaVersion {
		t.Fatalf("got %+v ok=%v", d, ok)
	}
	var got map[string]any
	if err := json.Unmarshal(d.Data, &got); err != nil || got["extra"] != nil || got["label"] != "Jobs" {
		t.Errorf("payload not normalised: %s", d.Data)
	}
	if d.FetchedAt == "" {
		t.Error("fetchedAt missing")
	}
}

func TestStateMapping(t *testing.T) {
	for name, tc := range map[string]struct {
		data func(context.Context, Caller) ([]byte, error)
		want State
	}{
		"unavailable": {func(context.Context, Caller) ([]byte, error) { return nil, ErrUnavailable }, StateUnavailable},
		"wrapped unavailable": {func(context.Context, Caller) ([]byte, error) {
			return nil, errors.Join(errors.New("dial"), ErrUnavailable)
		}, StateUnavailable},
		"provider error":  {func(context.Context, Caller) ([]byte, error) { return nil, errors.New("boom") }, StateError},
		"invalid payload": {func(context.Context, Caller) ([]byte, error) { return []byte(`{"label":"x"}`), nil }, StateError},
		"not json":        {func(context.Context, Caller) ([]byte, error) { return []byte(`nope`), nil }, StateError},
		"panic":           {func(context.Context, Caller) ([]byte, error) { panic("kaboom") }, StateError},
	} {
		e := engineOf(t, provider("w", "", tc.data))
		d, ok := e.Data(context.Background(), alice, "test.w")
		if !ok || d.State != tc.want {
			t.Errorf("%s: state = %q, want %q", name, d.State, tc.want)
		}
		if len(d.Data) != 0 {
			t.Errorf("%s: a failed widget must carry no data, got %s", name, d.Data)
		}
	}
}

func TestEmptyState(t *testing.T) {
	p := &fakeProvider{
		def:  Definition{Provider: "test", Key: "l", Title: "L", Kind: KindList, Size: Size{W: 1, H: 1}, RefreshSeconds: 60},
		data: func(context.Context, Caller) ([]byte, error) { return []byte(`{"items":[]}`), nil },
	}
	d, _ := engineOf(t, p).Data(context.Background(), alice, "test.l")
	if d.State != StateEmpty {
		t.Errorf("state = %q, want empty", d.State)
	}
}

func TestSlowProviderTimesOutAsUnavailable(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	p := provider("slow", "", func(ctx context.Context, _ Caller) ([]byte, error) {
		<-release // ignores ctx on purpose: the engine must not depend on the provider's manners
		return okStat(ctx, Caller{})
	})
	e := engineOf(t, p)
	e.timeout = 30 * time.Millisecond
	start := time.Now()
	d, _ := e.Data(context.Background(), alice, "test.slow")
	if d.State != StateUnavailable {
		t.Errorf("state = %q, want unavailable", d.State)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("the timeout did not bound the call")
	}
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func TestCacheIsPerWidgetAndUserAndExpires(t *testing.T) {
	p := provider("jobs", "", okStat)
	e := engineOf(t, p)
	clk := &clock{t: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)}
	e.now = clk.now
	bob := Caller{UserID: "bob"}

	e.Data(context.Background(), alice, "test.jobs")
	e.Data(context.Background(), alice, "test.jobs")
	if p.calls.Load() != 1 {
		t.Errorf("same user within TTL: %d calls, want 1", p.calls.Load())
	}
	e.Data(context.Background(), bob, "test.jobs")
	if p.calls.Load() != 2 {
		t.Errorf("another user must not share the cache: %d calls, want 2", p.calls.Load())
	}
	clk.advance(59 * time.Second)
	e.Data(context.Background(), alice, "test.jobs")
	if p.calls.Load() != 2 {
		t.Errorf("still within refreshSeconds: %d calls, want 2", p.calls.Load())
	}
	clk.advance(2 * time.Second)
	e.Data(context.Background(), alice, "test.jobs")
	if p.calls.Load() != 3 {
		t.Errorf("after refreshSeconds: %d calls, want 3", p.calls.Load())
	}
}

func TestFailuresAreCachedBrieflyOnly(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	p := provider("flaky", "", func(ctx context.Context, c Caller) ([]byte, error) {
		if fail.Load() {
			return nil, ErrUnavailable
		}
		return okStat(ctx, c)
	})
	e := engineOf(t, p)
	clk := &clock{t: time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)}
	e.now = clk.now

	e.Data(context.Background(), alice, "test.flaky")
	e.Data(context.Background(), alice, "test.flaky")
	if p.calls.Load() != 1 {
		t.Errorf("a failure should not be retried at once: %d calls", p.calls.Load())
	}
	fail.Store(false)
	clk.advance(FailureTTL + time.Second)
	d, _ := e.Data(context.Background(), alice, "test.flaky")
	if d.State != StateOK {
		t.Errorf("after the failure TTL the provider is asked again: state = %q", d.State)
	}
}

func TestConcurrentRequestsShareOneProviderCall(t *testing.T) {
	gate := make(chan struct{})
	p := provider("jobs", "", func(ctx context.Context, c Caller) ([]byte, error) {
		<-gate
		return okStat(ctx, c)
	})
	e := engineOf(t, p)

	const n = 8
	var wg sync.WaitGroup
	states := make([]State, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d, _ := e.Data(context.Background(), alice, "test.jobs")
			states[i] = d.State
		}(i)
	}
	deadline := time.Now().Add(2 * time.Second)
	for p.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // let the rest queue up behind the first
	close(gate)
	wg.Wait()

	if p.calls.Load() != 1 {
		t.Errorf("provider called %d times, want 1", p.calls.Load())
	}
	for i, s := range states {
		if s != StateOK {
			t.Errorf("caller %d: state %q", i, s)
		}
	}
}

func TestEarlierSourceWinsOnDuplicateID(t *testing.T) {
	first := provider("w", "", okStat)
	second := provider("w", "", func(context.Context, Caller) ([]byte, error) { return nil, errors.New("must not run") })
	r1, r2 := NewRegistry(), NewRegistry()
	_ = r1.Register(first)
	_ = r2.Register(second)
	e := NewEngine(r1, r2)
	if got := len(e.Widgets(alice)); got != 1 {
		t.Errorf("widgets = %d, want 1", got)
	}
	if d, _ := e.Data(context.Background(), alice, "test.w"); d.State != StateOK {
		t.Errorf("state = %q", d.State)
	}
}

func TestProvidersAreAskedAfreshEachRequest(t *testing.T) {
	r := NewRegistry()
	e := NewEngine(r)
	if len(e.Widgets(alice)) != 0 {
		t.Fatal("expected no widgets")
	}
	_ = r.Register(provider("late", "", okStat))
	if len(e.Widgets(alice)) != 1 {
		t.Error("a provider added after the engine was built must appear")
	}
}
