package localai

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/download"
	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// scripted serves bytes by address, counting the requests, and can hold a request until released.
type scripted struct {
	mu      sync.Mutex
	files   map[string][]byte
	hits    map[string]int
	hold    chan struct{}
	blocked atomic.Int32
}

func (s *scripted) Stream(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error) {
	if opts.AllowURL != nil {
		u, _ := url.Parse(rawURL)
		if err := opts.AllowURL(u); err != nil {
			return nil, err
		}
	}
	s.mu.Lock()
	data, ok := s.files[rawURL]
	s.hits[rawURL]++
	hold := s.hold
	s.mu.Unlock()
	if !ok {
		return &fetch.Stream{Status: 404, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	if hold != nil {
		s.blocked.Add(1)
		select {
		case <-hold:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &fetch.Stream{Status: 200, ContentLength: int64(len(data)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (s *scripted) hitsOn(u string) int { s.mu.Lock(); defer s.mu.Unlock(); return s.hits[u] }

const (
	modelURL   = "https://huggingface.co/o/r/resolve/78eb92a46fc37e6b524df991ed9aca9bc6aa7b80/m.gguf"
	runtimeURL = "https://github.com/ggml-org/llama.cpp/releases/download/b1/llama-b1-bin-macos-arm64.tar.gz"
)

var mac = hardware.Info{OS: "darwin", Arch: "arm64"}

type rig struct {
	in      *installer
	srv     *scripted
	model   catalog.Model
	rt      catalog.Runtime
	data    []byte
	archive []byte
}

func goodArchive(t *testing.T, server string) []byte {
	t.Helper()
	p := buildArchive(t,
		entry{name: "llama-b1/", typ: tar.TypeDir, mode: 0o755},
		entry{name: "llama-b1/" + server, body: "#!server", mode: 0o755},
		entry{name: "llama-b1/libllama.dylib", typ: tar.TypeSymlink, link: "libllama.0.dylib"},
		entry{name: "llama-b1/libllama.0.dylib", body: "lib", mode: 0o755},
	)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newRig(t *testing.T, archive []byte) *rig {
	t.Helper()
	data := bytes.Repeat([]byte("model"), 4000)
	if archive == nil {
		archive = goodArchive(t, "llama-server")
	}
	r := &rig{
		data: data, archive: archive,
		model: catalog.Model{ID: "tiny", Name: "Tiny model", SizeBytes: int64(len(data)), SHA256: sha(data), URL: modelURL},
		rt:    catalog.Runtime{ID: "rt-mac", Name: "llama.cpp b1", OS: "darwin", Arch: "arm64", SizeBytes: int64(len(archive)), SHA256: sha(archive), URL: runtimeURL, Binary: "llama-b1/llama-server"},
		srv:   &scripted{files: map[string][]byte{modelURL: data, runtimeURL: archive}, hits: map[string]int{}},
	}
	mgr := download.New(download.Options{Fetcher: r.srv, StallAfter: time.Minute, StallFailAfter: time.Minute})
	r.in = &installer{downloads: mgr, dataDir: t.TempDir(), catalog: catalog.Catalog{Models: []catalog.Model{r.model}, Runtimes: []catalog.Runtime{r.rt}}}
	return r
}

func (r *rig) wait(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (r *rig) allDone(t *testing.T) {
	t.Helper()
	r.wait(t, "the downloads to finish", func() bool {
		for _, v := range r.in.downloads.List() {
			if v.State.Active() {
				return false
			}
		}
		return true
	})
}

func TestOnlyTheExpectedHostsMayServeDownloads(t *testing.T) {
	for host, want := range map[string]bool{
		"huggingface.co": true, "hf.co": true, "us.aws.cdn.hf.co": true, "cas-bridge.xethub.hf.co": true,
		"evilhf.co": false, "hf.co.evil.example": false, "huggingface.co.evil.example": false, "evil.example": false, "": false, "github.com": false, "xhuggingface.co": false,
	} {
		if got := modelHost(host); got != want {
			t.Errorf("modelHost(%q) = %v, want %v", host, got, want)
		}
	}
	for host, want := range map[string]bool{
		"github.com": true, "release-assets.githubusercontent.com": true,
		"objects.githubusercontent.com": false, "githubusercontent.com": false, "evilgithub.com": false, "github.com.evil.example": false, "huggingface.co": false, "": false,
	} {
		if got := runtimeHost(host); got != want {
			t.Errorf("runtimeHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestPathsAreNamedAfterTheCatalogNeverAfterAServer(t *testing.T) {
	r := newRig(t, nil)
	root := filepath.Join(r.in.dataDir, "local-ai")
	if got := r.in.ModelPath(r.model); got != filepath.Join(root, "models", "tiny.gguf") {
		t.Errorf("model path = %s", got)
	}
	if got := r.in.RuntimeDir(r.rt); got != filepath.Join(root, "runtime", "rt-mac") {
		t.Errorf("runtime dir = %s", got)
	}
	if got := r.in.RuntimeBinary(r.rt); got != filepath.Join(root, "runtime", "rt-mac", "llama-b1", "llama-server") {
		t.Errorf("runtime binary = %s", got)
	}
}

func TestStartDownloadsAndUnpacksTheRuntimeAndDownloadsTheModel(t *testing.T) {
	r := newRig(t, nil)
	if got := r.in.ModelState(r.model).State; got != entity.InstallNotDownloaded {
		t.Fatalf("before: %v", got)
	}
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)

	if st := r.in.ModelState(r.model); st.State != entity.InstallReady {
		t.Errorf("model = %+v", st)
	}
	if st := r.in.RuntimeState(r.rt); st.State != entity.InstallReady {
		t.Errorf("runtime = %+v", st)
	}
	got, err := os.ReadFile(r.in.ModelPath(r.model))
	if err != nil || !bytes.Equal(got, r.data) {
		t.Errorf("model file wrong (%v)", err)
	}
	if st, err := os.Stat(r.in.RuntimeBinary(r.rt)); err != nil || st.Mode().Perm() != 0o755 {
		t.Errorf("runtime binary: %v %v", st, err)
	}
	if b, err := os.ReadFile(filepath.Join(r.in.RuntimeDir(r.rt), "llama-b1", "libllama.dylib")); err != nil || string(b) != "lib" {
		t.Errorf("the library link must resolve: %q %v", b, err)
	}
	if _, err := os.Stat(r.in.archivePath(r.rt)); err == nil {
		t.Error("the archive must be deleted once it is unpacked")
	}
	for _, v := range r.in.downloads.List() {
		if v.Scope != DownloadScope || v.State != download.StateDone {
			t.Errorf("job %q: scope %q state %s", v.Label, v.Scope, v.State)
		}
	}

	// Everything is there: a second start downloads nothing.
	hits := r.srv.hitsOn(modelURL) + r.srv.hitsOn(runtimeURL)
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
	if now := r.srv.hitsOn(modelURL) + r.srv.hitsOn(runtimeURL); now != hits {
		t.Errorf("requests went from %d to %d, want no new ones", hits, now)
	}
}

func TestStartRefusesWhatCannotWork(t *testing.T) {
	r := newRig(t, nil)
	if err := r.in.Start("no-such-model", mac); !errors.Is(err, entity.ErrUnknownModel) {
		t.Errorf("unknown model: err = %v", err)
	}
	if err := r.in.Start("", mac); !errors.Is(err, entity.ErrUnknownModel) {
		t.Errorf("empty model: err = %v", err)
	}
	for _, sys := range []hardware.Info{{OS: "windows", Arch: "amd64"}, {OS: "linux", Arch: "arm64"}, {}} {
		if err := r.in.Start("tiny", sys); !errors.Is(err, entity.ErrUnsupportedSystem) {
			t.Errorf("%v: err = %v, want ErrUnsupportedSystem", sys, err)
		}
	}
	if len(r.in.downloads.List()) != 0 {
		t.Error("a refused start must not begin any download")
	}
}

func TestARunningDownloadIsJoinedAndFoundAgain(t *testing.T) {
	r := newRig(t, nil)
	r.srv.hold = make(chan struct{})
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "both requests to arrive", func() bool { return r.srv.blocked.Load() == 2 })
	m := r.in.ModelState(r.model)
	rt := r.in.RuntimeState(r.rt)
	if m.State != entity.InstallDownloading || m.DownloadID == "" || rt.State != entity.InstallDownloading || rt.DownloadID == "" {
		t.Fatalf("model %+v runtime %+v", m, rt)
	}
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	if again := r.in.ModelState(r.model); again.DownloadID != m.DownloadID {
		t.Error("a second start must join the running download")
	}
	if len(r.in.downloads.List()) != 2 {
		t.Errorf("%d jobs, want 2", len(r.in.downloads.List()))
	}
	close(r.srv.hold)
	r.allDone(t)
	if r.srv.hitsOn(modelURL) != 1 || r.srv.hitsOn(runtimeURL) != 1 {
		t.Errorf("hits: model %d runtime %d, want 1 each", r.srv.hitsOn(modelURL), r.srv.hitsOn(runtimeURL))
	}
}

func TestAnInterruptedDownloadShowsAsPartial(t *testing.T) {
	r := newRig(t, nil)
	dest := r.in.ModelPath(r.model)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest+".part", r.data[:1234], 0o644); err != nil {
		t.Fatal(err)
	}
	if st := r.in.ModelState(r.model); st.State != entity.InstallPartial || st.PartialBytes != 1234 {
		t.Errorf("model = %+v", st)
	}
	// A file of the wrong size in place is not "ready".
	if err := os.WriteFile(dest, r.data[:10], 0o644); err != nil {
		t.Fatal(err)
	}
	if st := r.in.ModelState(r.model); st.State == entity.InstallReady {
		t.Error("a file of the wrong size must not count as ready")
	}
	// A runtime folder without the server program is not ready either.
	if err := os.MkdirAll(filepath.Join(r.in.RuntimeDir(r.rt), "llama-b1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if st := r.in.RuntimeState(r.rt); st.State == entity.InstallReady {
		t.Error("a runtime without the server program must not count as ready")
	}
	if err := os.WriteFile(r.in.RuntimeBinary(r.rt), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := r.in.RuntimeState(r.rt); st.State == entity.InstallReady {
		t.Error("a server program that cannot be executed must not count as ready")
	}
}

func TestAnArchiveWithoutTheServerProgramFailsAndLeavesNothingUnpacked(t *testing.T) {
	r := newRig(t, goodArchive(t, "llama-cli")) // a server program of the wrong name
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
	var msg string
	for _, v := range r.in.downloads.List() {
		if v.Label == r.rt.Name {
			msg = v.Error
			if v.State != download.StateFailed {
				t.Errorf("runtime job state = %s", v.State)
			}
		}
	}
	if !strings.Contains(msg, "does not contain the server program") {
		t.Errorf("error = %q", msg)
	}
	if _, err := os.Stat(r.in.RuntimeDir(r.rt)); err == nil {
		t.Error("an unusable runtime must not be left unpacked")
	}
	if st := r.in.RuntimeState(r.rt); st.State == entity.InstallReady {
		t.Error("the runtime must not be ready")
	}
}

func TestAHostileArchiveFailsTheRuntimeDownloadWithAPlainMessage(t *testing.T) {
	p := buildArchive(t, entry{name: "llama-b1/llama-server", body: "x", mode: 0o755}, entry{name: "../../escape", body: "x"})
	hostile, _ := os.ReadFile(p)
	r := newRig(t, hostile)
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
	for _, v := range r.in.downloads.List() {
		if v.Label == r.rt.Name && (v.State != download.StateFailed || !strings.Contains(v.Error, "not allowed")) {
			t.Errorf("runtime job: %s %q", v.State, v.Error)
		}
	}
	if _, err := os.Stat(filepath.Join(r.in.dataDir, "escape")); err == nil {
		t.Error("nothing may be written outside the folder")
	}
}

func TestARuntimeThatDoesNotMatchItsChecksumIsNeverUnpacked(t *testing.T) {
	r := newRig(t, nil)
	r.rt.SHA256 = sha([]byte("something else"))
	r.in.catalog.Runtimes = []catalog.Runtime{r.rt}
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
	if _, err := os.Stat(r.in.RuntimeDir(r.rt)); err == nil {
		t.Error("a runtime that fails its checksum must not be unpacked")
	}
	if st := r.in.RuntimeState(r.rt); st.State != entity.InstallNotDownloaded {
		t.Errorf("runtime = %+v", st)
	}
}

func TestRemoveDeletesEverythingAndRefusesWhileDownloading(t *testing.T) {
	r := newRig(t, nil)
	r.srv.hold = make(chan struct{})
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "requests", func() bool { return r.srv.blocked.Load() == 2 })
	if err := r.in.Remove(); !errors.Is(err, entity.ErrDownloadRunning) {
		t.Errorf("while downloading: err = %v", err)
	}
	close(r.srv.hold)
	r.allDone(t)
	if r.in.ModelState(r.model).State != entity.InstallReady {
		t.Fatal("setup: the model must be downloaded")
	}
	if err := r.in.Remove(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{r.in.ModelPath(r.model), r.in.RuntimeDir(r.rt), filepath.Join(r.in.root(), "models"), filepath.Join(r.in.root(), "runtime"), filepath.Join(r.in.root(), "downloads")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s must be gone", p)
		}
	}
	if r.in.ModelState(r.model).State != entity.InstallNotDownloaded || r.in.RuntimeState(r.rt).State != entity.InstallNotDownloaded {
		t.Error("everything must be back to not downloaded")
	}
	if err := r.in.Remove(); err != nil {
		t.Errorf("removing when nothing is there must work: %v", err)
	}
}

func TestRegisterNeedsTheDownloadManagerAndTheDataDirectory(t *testing.T) {
	db := openMainDB(t)
	if err := Register(db, Deps{}); err == nil {
		t.Error("a missing manager and data directory must be refused")
	}
	if err := Register(db, Deps{Downloads: download.New(download.Options{})}); err == nil {
		t.Error("a missing data directory must be refused")
	}
	if err := Register(db, Deps{DataDir: t.TempDir()}); err == nil {
		t.Error("a missing manager must be refused")
	}
}

func TestTheHandlersWorkBeforeTheInstallerExists(t *testing.T) {
	setInstaller(nil)
	l := lazyInstalls{}
	if st := l.ModelState(catalog.Model{ID: "x"}); st.State != entity.InstallNotDownloaded {
		t.Errorf("model state = %+v", st)
	}
	if st := l.RuntimeState(catalog.Runtime{ID: "x"}); st.State != entity.InstallNotDownloaded {
		t.Errorf("runtime state = %+v", st)
	}
	if err := l.Start("x", mac); err == nil {
		t.Error("start without an installer must fail")
	}
	if err := l.Remove(); err == nil {
		t.Error("remove without an installer must fail")
	}
}

// The scope of the downloads must be the one that changes the local AI's settings: whoever may start a download may also
// see and cancel it, and nobody else may.
func TestTheDownloadScopeIsTheSettingsManageScope(t *testing.T) {
	if DownloadScope != "cinqo:platform:manage" {
		t.Errorf("DownloadScope = %q, want cinqo:platform:manage", DownloadScope)
	}
	raw, err := os.ReadFile(routeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "- "+DownloadScope) {
		t.Errorf("the route file must require %s for the routes that start and remove downloads", DownloadScope)
	}
}

func TestRemoveIsRefusedWhileEitherOneDownloadIsRunning(t *testing.T) {
	for name, hold := range map[string]string{"only the model": modelURL, "only the runtime": runtimeURL} {
		r := newRig(t, nil)
		if err := r.in.Start("tiny", mac); err != nil {
			t.Fatal(err)
		}
		r.allDone(t)
		// Delete one of the two and download it again, held back, so that exactly one download is running.
		if hold == modelURL {
			os.Remove(r.in.ModelPath(r.model))
		} else {
			os.RemoveAll(r.in.RuntimeDir(r.rt))
		}
		r.srv.mu.Lock()
		r.srv.hold = make(chan struct{})
		r.srv.mu.Unlock()
		if err := r.in.Start("tiny", mac); err != nil {
			t.Fatal(err)
		}
		r.wait(t, "the one download to start", func() bool { return r.srv.blocked.Load() == 1 })
		if err := r.in.Remove(); !errors.Is(err, entity.ErrDownloadRunning) {
			t.Errorf("%s: err = %v, want ErrDownloadRunning", name, err)
		}
		close(r.srv.hold)
		r.allDone(t)
		if err := r.in.Remove(); err != nil {
			t.Errorf("%s: once it is over the removal must work: %v", name, err)
		}
	}
}

// --- plan 205, step 1 ---

func TestCancelDownloadsStopsTheRunningOnesAndLeavesNoPartialFile(t *testing.T) {
	r := newRig(t, nil)
	r.srv.hold = make(chan struct{})
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "both requests to arrive", func() bool { return r.srv.blocked.Load() == 2 })
	r.in.CancelDownloads()
	r.wait(t, "both downloads to stop", func() bool {
		return r.in.ModelState(r.model).State != entity.InstallDownloading && r.in.RuntimeState(r.rt).State != entity.InstallDownloading
	})
	if r.in.ModelState(r.model).State != entity.InstallNotDownloaded || r.in.RuntimeState(r.rt).State != entity.InstallNotDownloaded {
		t.Errorf("model %+v runtime %+v, want not downloaded", r.in.ModelState(r.model), r.in.RuntimeState(r.rt))
	}
	r.in.CancelDownloads() // nothing running: a no-op, never an error
	// and a later start works from scratch
	r.srv.mu.Lock()
	r.srv.hold = nil
	r.srv.mu.Unlock()
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
}

// The state the user was in on 2026-11-08: the model is complete and the program's download ended at 0 bytes (an empty .part).
// A new start must fetch the program from the beginning and must not touch the model.
func TestAnEmptyPartialProgramFileIsStartedAgainAndTheFinishedModelIsKept(t *testing.T) {
	r := newRig(t, nil)
	if err := os.MkdirAll(filepath.Dir(r.in.ModelPath(r.model)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.in.ModelPath(r.model), r.data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(r.in.archivePath(r.rt)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.in.archivePath(r.rt)+".part", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if r.in.RuntimeState(r.rt).State != entity.InstallNotDownloaded {
		t.Fatalf("an empty partial file is not a partial download: %+v", r.in.RuntimeState(r.rt))
	}
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
	if r.in.RuntimeState(r.rt).State != entity.InstallReady || r.in.ModelState(r.model).State != entity.InstallReady {
		t.Errorf("runtime %+v model %+v, want both ready", r.in.RuntimeState(r.rt), r.in.ModelState(r.model))
	}
	if r.srv.hitsOn(modelURL) != 0 {
		t.Errorf("the finished model must not be fetched again (%d requests)", r.srv.hitsOn(modelURL))
	}
}

// A release download that is redirected to a host the module does not allow fails with a message that names the host.
type redirectingTo struct{ host string }

func (rd redirectingTo) Stream(ctx context.Context, rawURL string, opts fetch.StreamOptions) (*fetch.Stream, error) {
	u, _ := url.Parse("https://" + rd.host + "/file")
	if err := opts.AllowURL(u); err != nil {
		return nil, err
	}
	return nil, errors.New("not reached")
}

func TestARedirectToAnotherHostFailsTheProgramDownloadAndNamesTheHost(t *testing.T) {
	r := newRig(t, nil)
	r.in.downloads = download.New(download.Options{Fetcher: redirectingTo{host: "cdn.somewhere-else.example"}, StallAfter: time.Minute, StallFailAfter: time.Minute})
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	var msg string
	r.wait(t, "the failure", func() bool {
		for _, v := range r.in.downloads.List() {
			if v.State == download.StateFailed && strings.Contains(v.Error, "cdn.somewhere-else.example") {
				msg = v.Error
				return true
			}
		}
		return false
	})
	if !strings.Contains(msg, "not on the list of allowed download hosts") {
		t.Errorf("the failure: %q", msg)
	}
}

// --- plan 205, step 2: the installer remembers why a download failed ---

func TestWhyTheProgramDownloadFailedIsRememberedAfterTheManagerForgetsTheJob(t *testing.T) {
	r := newRig(t, nil)
	r.in.downloads = download.New(download.Options{Fetcher: redirectingTo{host: "cdn.somewhere-else.example"}, Retention: 30 * time.Millisecond, StallAfter: time.Minute, StallFailAfter: time.Minute})
	if f := r.in.RuntimeFailure(r.rt); f != nil {
		t.Fatalf("nothing happened yet: %+v", f)
	}
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	var seen *entity.Failure
	r.wait(t, "the failure to be seen", func() bool { seen = r.in.RuntimeFailure(r.rt); return seen != nil })
	if seen.What != "program" || !strings.Contains(seen.Message, "cdn.somewhere-else.example") || seen.At == "" {
		t.Fatalf("%+v", seen)
	}
	time.Sleep(120 * time.Millisecond) // past the manager's retention: the job is forgotten
	if _, ok := r.in.downloads.Latest(r.in.archivePath(r.rt)); ok {
		t.Fatal("setup: the manager must have forgotten the job")
	}
	if again := r.in.RuntimeFailure(r.rt); again == nil || again.Message != seen.Message {
		t.Errorf("the page reloaded after the job was forgotten must still be told why: %+v", again)
	}
}

func TestANewAttemptOrASuccessClearsTheRememberedFailure(t *testing.T) {
	r := newRig(t, nil)
	archive := r.srv.files[runtimeURL]
	delete(r.srv.files, runtimeURL) // the address answers 404: the program download fails
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "the failure", func() bool { return r.in.RuntimeFailure(r.rt) != nil })
	r.srv.mu.Lock()
	r.srv.files[runtimeURL] = archive // the address works again
	r.srv.hold = make(chan struct{})
	r.srv.mu.Unlock()
	if err := r.in.Start("tiny", mac); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "the new attempt to run", func() bool { return r.srv.blocked.Load() >= 1 })
	if f := r.in.RuntimeFailure(r.rt); f != nil {
		t.Errorf("a new attempt is running: the old failure must not show: %+v", f)
	}
	close(r.srv.hold)
	r.allDone(t)
	if f := r.in.RuntimeFailure(r.rt); f != nil {
		t.Errorf("after a success: %+v", f)
	}
}

func TestARememberedFailureIsForgottenAfterADay(t *testing.T) {
	r := newRig(t, nil)
	old := time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339)
	r.in.failures = map[string]entity.Failure{r.in.ModelPath(r.model): {What: "model", Message: "old", At: old}}
	if f := r.in.ModelFailure(r.model); f != nil {
		t.Errorf("a failure older than a day: %+v", f)
	}
	fresh := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	r.in.failures = map[string]entity.Failure{r.in.ModelPath(r.model): {What: "model", Message: "recent", At: fresh}}
	if f := r.in.ModelFailure(r.model); f == nil || f.Message != "recent" {
		t.Errorf("a recent failure: %+v", f)
	}
}
