package chromeproc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clock(t *time.Time) func() time.Time { return func() time.Time { return *t } }

func TestRegisterRecordsWhatTheLauncherSaidAndTheOperatingSystemsStartTime(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	r := New("", newFake(), clock(&now))
	r.Register(Launch{Kind: KindKatana, PID: 41, ProfileDir: "/p/k", Binary: "/bin/chrome"})
	got := r.Entries()
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	e := got[0]
	if e.PID != 41 || e.Kind != KindKatana || e.ProfileDir != "/p/k" || e.Binary != "/bin/chrome" || e.ProcessStart != "Mon Nov 9 14:00:00 2026" || !e.StartedAt.Equal(now) || !e.LastUsedAt.Equal(now) {
		t.Errorf("%+v", e)
	}
}

func TestAnIdThatCannotBeTrackedIsIgnored(t *testing.T) {
	r := New("", newFake(), nil)
	r.Register(Launch{Kind: KindRod, PID: 0})
	r.Register(Launch{Kind: KindRod, PID: -5})
	if len(r.Entries()) != 0 {
		t.Error("an id of 0 or less must not be registered")
	}
}

func TestReleaseForgetsAndAnUnknownIdIsHarmless(t *testing.T) {
	r := New("", newFake(), nil)
	r.Register(Launch{Kind: KindRod, PID: 41})
	r.Register(Launch{Kind: KindKatana, PID: 42})
	r.Release(41)
	r.Release(41)
	r.Release(9999)
	got := r.Entries()
	if len(got) != 1 || got[0].PID != 42 {
		t.Errorf("%+v", got)
	}
}

func TestRegisteringAnIdAgainReplacesTheEntry(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	fp := newFake()
	r := New("", fp, clock(&now))
	r.Register(Launch{Kind: KindRod, PID: 41})
	now = now.Add(time.Hour)
	fp.starts[41] = "Mon Nov 9 15:00:00 2026"
	r.Register(Launch{Kind: KindKatana, PID: 41})
	got := r.Entries()
	if len(got) != 1 || got[0].Kind != KindKatana || got[0].ProcessStart != "Mon Nov 9 15:00:00 2026" || !got[0].StartedAt.Equal(now) {
		t.Errorf("%+v", got)
	}
}

func TestTouchAndTouchKind(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	r := New("", newFake(), clock(&now))
	r.Register(Launch{Kind: KindRod, PID: 41})
	r.Register(Launch{Kind: KindKatana, PID: 42})
	now = now.Add(time.Minute)
	r.Touch(41)
	r.Touch(9999)
	byPID := map[int]Entry{}
	for _, e := range r.Entries() {
		byPID[e.PID] = e
	}
	if !byPID[41].LastUsedAt.Equal(now) || byPID[42].LastUsedAt.Equal(now) {
		t.Errorf("Touch must change only its own Chrome: %+v", byPID)
	}
	now = now.Add(time.Minute)
	r.TouchKind(KindKatana)
	byPID = map[int]Entry{}
	for _, e := range r.Entries() {
		byPID[e.PID] = e
	}
	if !byPID[42].LastUsedAt.Equal(now) || byPID[41].LastUsedAt.Equal(now) {
		t.Errorf("TouchKind must change only that kind: %+v", byPID)
	}
	if !byPID[41].StartedAt.Equal(time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)) {
		t.Error("touching must never change the start")
	}
}

func TestEntriesAreOldestFirstAndACopy(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	r := New("", newFake(), clock(&now))
	r.Register(Launch{Kind: KindRod, PID: 42})
	now = now.Add(time.Second)
	r.Register(Launch{Kind: KindKatana, PID: 41})
	got := r.Entries()
	if got[0].PID != 42 || got[1].PID != 41 {
		t.Errorf("oldest first: %+v", got)
	}
	got[0].PID = 7
	if r.Entries()[0].PID != 42 {
		t.Error("Entries must return a copy")
	}
	// same start second: ordered by id
	r2 := New("", newFake(), func() time.Time { return now })
	r2.Register(Launch{Kind: KindRod, PID: 42})
	r2.Register(Launch{Kind: KindRod, PID: 41})
	if e := r2.Entries(); e[0].PID != 41 {
		t.Errorf("ties by id: %+v", e)
	}
}

func TestSnapshotSaysRunningOrGoneAndHasAgeAndIdle(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	fp := newFake()
	r := New("", fp, clock(&now))
	r.Register(Launch{Kind: KindKatana, PID: 41})
	r.Register(Launch{Kind: KindRod, PID: 42})
	now = now.Add(90 * time.Second)
	r.Touch(42)
	now = now.Add(30 * time.Second)
	fp.alive[41] = false
	st := r.Snapshot()
	if len(st.Chromes) != 2 {
		t.Fatalf("%+v", st)
	}
	a, b := st.Chromes[0], st.Chromes[1]
	if a.PID != 41 || a.State != "gone" || a.AgeSeconds != 120 || a.IdleSeconds != 120 || a.Kind != "katana" {
		t.Errorf("%+v", a)
	}
	if b.PID != 42 || b.State != "running" || b.AgeSeconds != 120 || b.IdleSeconds != 30 {
		t.Errorf("%+v", b)
	}
	now = now.Add(-time.Hour)
	if got := r.Snapshot().Chromes[0].AgeSeconds; got != 0 {
		t.Errorf("a clock that went back gives 0, got %d", got)
	}
}

func TestSnapshotOfNothingIsAnEmptyListNotNull(t *testing.T) {
	raw, _ := json.Marshal(New("", newFake(), nil).Snapshot())
	if string(raw) != `{"chromes":[]}` {
		t.Errorf("%s", raw)
	}
}

func TestTheStatusRouteIsReadOnlyAndShowsNoFolderNoCommand(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)
	old := std
	t.Cleanup(func() { std = old })
	std = New("", newFake(), clock(&now))
	std.Register(Launch{Kind: KindKatana, PID: 41, ProfileDir: "/secret/profile", Binary: "/secret/chrome"})

	rec := httptest.NewRecorder()
	StatusHandler(rec, httptest.NewRequest(http.MethodGet, "/chrome-processes", nil))
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%d %v", rec.Code, rec.Header())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"pid":41`) || !strings.Contains(body, `"kind":"katana"`) {
		t.Errorf("%s", body)
	}
	for _, secret := range []string{"secret", "profile", "binary", "processStart"} {
		if strings.Contains(body, secret) {
			t.Errorf("%q must not leave the tool: %s", secret, body)
		}
	}
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		StatusHandler(rec, httptest.NewRequest(m, "/chrome-processes", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
}

func TestTheLedgerFollowsEveryChangeAndIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ledgerName)
	r := New(path, newFake(), nil)
	r.Register(Launch{Kind: KindKatana, PID: 41, ProfileDir: "/p/k"})
	r.Register(Launch{Kind: KindRod, PID: 42})
	got, err := ReadLedger(path)
	if err != nil || len(got) != 2 || got[0].PID != 41 || got[0].ProfileDir != "/p/k" || got[0].ProcessStart == "" {
		t.Fatalf("%+v %v", got, err)
	}
	r.Release(41)
	got, _ = ReadLedger(path)
	if len(got) != 1 || got[0].PID != 42 {
		t.Errorf("after release: %+v", got)
	}
	r.Release(42)
	got, err = ReadLedger(path)
	if err != nil || len(got) != 0 {
		t.Errorf("empty ledger: %+v %v", got, err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, ".chrome-pids-*.tmp"))
	if len(left) != 0 {
		t.Errorf("temporary files left behind: %v", left)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm()&0o077 == 0o077 {
		t.Logf("mode %v", info.Mode())
	}
}

func TestAReleaseOfAnUnknownIdDoesNotRewriteTheLedger(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ledgerName)
	r := New(path, newFake(), nil)
	r.Release(41)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("no change, no file: %v", err)
	}
}

func TestReadLedgerRefusesDamageAndUnknownVersions(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadLedger(filepath.Join(dir, "missing.json")); !os.IsNotExist(err) {
		t.Errorf("missing: %v", err)
	}
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{not json"), 0o600)
	if _, err := ReadLedger(bad); err == nil {
		t.Error("damaged must be an error")
	}
	v2 := filepath.Join(dir, "v2.json")
	os.WriteFile(v2, []byte(`{"version":2,"chromes":[{"pid":5}]}`), 0o600)
	if _, err := ReadLedger(v2); err == nil {
		t.Error("an unknown version must be an error")
	}
	ok := filepath.Join(dir, "ok.json")
	os.WriteFile(ok, []byte(`{"version":1,"chromes":[{"pid":5,"kind":"rod"},{"pid":0},{"pid":-2}]}`), 0o600)
	got, err := ReadLedger(ok)
	if err != nil || len(got) != 1 || got[0].PID != 5 {
		t.Errorf("entries without a usable id are dropped: %+v %v", got, err)
	}
}

func TestConfigureKeepsAnEarlierRunsLedgerAsideAndStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	old := std
	t.Cleanup(func() { std = old })
	// an earlier run left one Chrome in its ledger
	prev := New(filepath.Join(dir, ledgerName), newFake(), nil)
	prev.Register(Launch{Kind: KindKatana, PID: 41})
	std = New("", newFake(), nil)
	if err := Configure(dir); err != nil {
		t.Fatal(err)
	}
	left, err := ReadLedger(filepath.Join(dir, previousLedgerName))
	if err != nil || len(left) != 1 || left[0].PID != 41 {
		t.Fatalf("the earlier run's entry must be kept for the sweep: %+v %v", left, err)
	}
	now, err := ReadLedger(filepath.Join(dir, ledgerName))
	if err != nil || len(now) != 0 {
		t.Errorf("the new ledger starts empty: %+v %v", now, err)
	}
	Register(Launch{Kind: KindRod, PID: 42})
	now, _ = ReadLedger(filepath.Join(dir, ledgerName))
	if len(now) != 1 || now[0].PID != 42 {
		t.Errorf("the package functions write the configured ledger: %+v", now)
	}
}

func TestConfigureWithAnEmptyEarlierLedgerKeepsNothingAside(t *testing.T) {
	dir := t.TempDir()
	old := std
	t.Cleanup(func() { std = old })
	New(filepath.Join(dir, ledgerName), newFake(), nil).Register(Launch{PID: 41})
	New(filepath.Join(dir, ledgerName), newFake(), nil).Release(41) // never registered here: still lists 41
	os.WriteFile(filepath.Join(dir, ledgerName), []byte(`{"version":1,"chromes":[]}`), 0o600)
	std = New("", newFake(), nil)
	if err := Configure(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, previousLedgerName)); !os.IsNotExist(err) {
		t.Errorf("an empty ledger is not worth keeping: %v", err)
	}
}

func TestConfigureKeepsADamagedLedgerAsideAndNeverReadsItAsTruth(t *testing.T) {
	dir := t.TempDir()
	old := std
	t.Cleanup(func() { std = old })
	os.WriteFile(filepath.Join(dir, ledgerName), []byte("{broken"), 0o600)
	std = New("", newFake(), nil)
	if err := Configure(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, previousLedgerName+".damaged")); err != nil {
		t.Errorf("the damaged file must be kept aside: %v", err)
	}
	if got, err := ReadLedger(filepath.Join(dir, ledgerName)); err != nil || len(got) != 0 {
		t.Errorf("%+v %v", got, err)
	}
}

func TestConfigureNeedsAFolder(t *testing.T) {
	old := std
	t.Cleanup(func() { std = old })
	if err := Configure(""); err == nil {
		t.Error("no folder must be an error")
	}
	if err := Configure(filepath.Join(t.TempDir(), "a", "b")); err != nil {
		t.Errorf("a missing folder is made: %v", err)
	}
}
