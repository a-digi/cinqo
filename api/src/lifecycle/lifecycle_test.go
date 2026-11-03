package lifecycle

import (
	"reflect"
	"sync"
	"testing"
)

func reset() {
	mu.Lock()
	hooks = nil
	mu.Unlock()
}

func TestHooksRunInReverseOrderOfRegistration(t *testing.T) {
	reset()
	var order []int
	for i := 1; i <= 3; i++ {
		i := i
		OnShutdown(func() { order = append(order, i) })
	}
	RunShutdownHooks()
	if !reflect.DeepEqual(order, []int{3, 2, 1}) {
		t.Errorf("order = %v, want [3 2 1]: what was started last stops first", order)
	}
}

func TestHooksRunOnlyOnce(t *testing.T) {
	reset()
	n := 0
	OnShutdown(func() { n++ })
	RunShutdownHooks()
	RunShutdownHooks()
	if n != 1 {
		t.Errorf("hook ran %d times, want 1", n)
	}
}

func TestAPanickingHookDoesNotStopTheOthers(t *testing.T) {
	reset()
	var ran []string
	OnShutdown(func() { ran = append(ran, "first") })
	OnShutdown(func() { panic("boom") })
	OnShutdown(func() { ran = append(ran, "last") })
	RunShutdownHooks()
	if !reflect.DeepEqual(ran, []string{"last", "first"}) {
		t.Errorf("ran = %v, want both outer hooks", ran)
	}
}

func TestANilHookIsIgnoredAndRunWithNothingRegisteredIsFine(t *testing.T) {
	reset()
	OnShutdown(nil)
	RunShutdownHooks()
	mu.Lock()
	defer mu.Unlock()
	if len(hooks) != 0 {
		t.Error("a nil hook must not be kept")
	}
}

func TestRegisteringWhileRunningIsSafe(t *testing.T) {
	reset()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); OnShutdown(func() {}) }()
	}
	wg.Wait()
	RunShutdownHooks()
}
