// Package lifecycle lets a feature that owns something that must not outlive the application (a child process, a temporary
// file) say so once, without the application's shutdown code having to know the feature. A feature registers a function with
// OnShutdown at start-up; the application calls RunShutdownHooks once while it shuts down.
package lifecycle

import (
	"sync"
)

var (
	mu    sync.Mutex
	hooks []func()
)

// OnShutdown registers f to be called by RunShutdownHooks. Hooks run in the reverse order of registration, so what was
// started last is stopped first.
func OnShutdown(f func()) {
	if f == nil {
		return
	}
	mu.Lock()
	hooks = append(hooks, f)
	mu.Unlock()
}

// RunShutdownHooks calls every registered hook once and forgets them. A hook that panics does not keep the others from
// running; the panic is swallowed because the application is already shutting down and there is nothing left to report to.
func RunShutdownHooks() {
	mu.Lock()
	todo := hooks
	hooks = nil
	mu.Unlock()
	for i := len(todo) - 1; i >= 0; i-- {
		runOne(todo[i])
	}
}

func runOne(f func()) {
	defer func() { _ = recover() }()
	f()
}
