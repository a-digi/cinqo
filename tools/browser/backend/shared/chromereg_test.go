package shared

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"browser-tool-backend/shared/chromeproc"
)

func registeredPID(pid int) (chromeproc.Entry, bool) {
	for _, e := range chromeproc.Entries() {
		if e.PID == pid {
			return e, true
		}
	}
	return chromeproc.Entry{}, false
}

func TestAChromedpWithoutABrowserHasNoProcessIdAndIsNotRegistered(t *testing.T) {
	if got := chromedpPID(context.Background()); got != 0 {
		t.Errorf("a context without chromedp: %d", got)
	}
	cancels := []context.CancelFunc{func() {}}
	out := registerChromedpChrome(chromeproc.KindHeaded, context.Background(), "", cancels)
	if len(out) != 1 {
		t.Errorf("nothing to register, nothing to release: %d cancel funcs", len(out))
	}
}

// The headless session's Chrome is registered with its process id, and the existing stop path (cancelAll) releases it (plan 224).
func TestTheSharedHeadlessChromeIsRegisteredAndReleasedByTheExistingStopPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real-browser test in -short mode")
	}
	if _, err := ChromePath(); err != nil {
		t.Skipf("no Chrome available: %v", err)
	}
	ctx, cancels, _, err := launchHeadlessBrowser("")
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	pid := chromedpPID(ctx)
	if pid <= 0 {
		cancelAll(cancels)
		t.Fatal("the launched Chrome must have a process id")
	}
	e, ok := registeredPID(pid)
	if !ok || e.Kind != chromeproc.KindSharedHeadless {
		cancelAll(cancels)
		t.Fatalf("%+v %v", e, ok)
	}
	if filepath.Dir(e.ProfileDir) != chromeproc.ProfileRoot() || !strings.HasPrefix(filepath.Base(e.ProfileDir), "shared-headless-") || !chromeproc.IsOursPID(pid) {
		t.Errorf("profile folder %q, ours=%v", e.ProfileDir, chromeproc.IsOursPID(pid))
	}
	cancelAll(cancels)
	if _, ok := registeredPID(pid); ok {
		t.Errorf("cancelAll must release pid %d", pid)
	}
	if _, err := os.Stat(e.ProfileDir); !os.IsNotExist(err) {
		t.Errorf("the profile folder must be removed when the browser is stopped: %v", err)
	}
}
