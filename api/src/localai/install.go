package localai

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/a-digi/cinqo/src/download"
	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
)

// DownloadScope is the scope needed to see and cancel the local AI's downloads, the same one that changes its settings.
const DownloadScope = "cinqo:platform:manage"

// Deps are what the module needs from the application at start-up.
type Deps struct {
	Downloads *download.Manager
	// DataDir is the application's data directory; everything the module downloads lives under <DataDir>/local-ai.
	DataDir string
}

// modelHost says whether a host may serve a model file: Hugging Face and the content hosts it redirects to (a.hf.co).
func modelHost(host string) bool {
	return host == "huggingface.co" || host == "hf.co" || strings.HasSuffix(host, ".hf.co")
}

// runtimeHost says whether a host may serve the runtime archive: GitHub and the host it redirects release downloads to
// (checked on 2026-11-03).
func runtimeHost(host string) bool {
	return host == "github.com" || host == "release-assets.githubusercontent.com"
}

// installer downloads, checks and removes the runtime and the models. It has no state of its own: the answer to "is it
// there" is always read from the disk and from the download manager.
type installer struct {
	downloads *download.Manager
	dataDir   string
	catalog   catalog.Catalog
	// source, when set, is the list of models now (shipped and added, package models); catalog is what is used without it.
	source func() (catalog.Catalog, error)

	// failures remembers why a download ended badly, seen while it was still listed by the manager (which forgets a finished job
	// after its retention time), so a page that is reloaded can still say why. It is cleared when a new attempt starts or succeeds,
	// is forgotten after failureMemory and is lost with a restart. It is the only state the installer keeps.
	failMu   sync.Mutex
	failures map[string]entity.Failure
}

// failureMemory is how long a seen failure is kept.
const failureMemory = 24 * time.Hour

// cat is the list of models and runtimes now.
func (in *installer) cat() catalog.Catalog {
	if in.source != nil {
		if c, err := in.source(); err == nil {
			return c
		}
	}
	return in.catalog
}

func (in *installer) root() string { return filepath.Join(in.dataDir, "local-ai") }

// ModelPath is where the finished model file lives: named after the catalog id, never after anything a server sent.
func (in *installer) ModelPath(m catalog.Model) string {
	return filepath.Join(in.root(), "models", m.ID+".gguf")
}

// RuntimeDir is the folder the runtime archive is unpacked into.
func (in *installer) RuntimeDir(r catalog.Runtime) string {
	return filepath.Join(in.root(), "runtime", r.ID)
}

// RuntimeBinary is the server program of an unpacked runtime.
func (in *installer) RuntimeBinary(r catalog.Runtime) string {
	return filepath.Join(in.RuntimeDir(r), filepath.FromSlash(r.Binary))
}

func (in *installer) archivePath(r catalog.Runtime) string {
	return filepath.Join(in.root(), "downloads", r.ID+".tar.gz")
}

func (in *installer) ModelState(m catalog.Model) entity.FileInstall {
	dest := in.ModelPath(m)
	if id, ok := in.downloads.Active(dest); ok {
		return entity.FileInstall{State: entity.InstallDownloading, DownloadID: id, PartialBytes: download.PartialBytes(dest)}
	}
	if st, err := os.Stat(dest); err == nil && st.Mode().IsRegular() && st.Size() == m.SizeBytes {
		return entity.FileInstall{State: entity.InstallReady}
	}
	if p := download.PartialBytes(dest); p > 0 {
		return entity.FileInstall{State: entity.InstallPartial, PartialBytes: p}
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

func (in *installer) RuntimeState(r catalog.Runtime) entity.FileInstall {
	archive := in.archivePath(r)
	if id, ok := in.downloads.Active(archive); ok {
		return entity.FileInstall{State: entity.InstallDownloading, DownloadID: id, PartialBytes: download.PartialBytes(archive)}
	}
	if st, err := os.Stat(in.RuntimeBinary(r)); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 {
		return entity.FileInstall{State: entity.InstallReady}
	}
	if p := download.PartialBytes(archive); p > 0 {
		return entity.FileInstall{State: entity.InstallPartial, PartialBytes: p}
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

// Start begins the downloads a model needs: the runtime for this system (unless it is there) and the model file (unless it
// is there). A download that is already running is joined, not started twice.
func (in *installer) Start(modelID string, system hardware.Info) error {
	m, ok := in.cat().Find(modelID)
	if !ok {
		return entity.ErrUnknownModel
	}
	rt, ok := in.cat().RuntimeFor(system.OS, system.Arch)
	if !ok {
		return entity.ErrUnsupportedSystem
	}
	if in.RuntimeState(rt).State != entity.InstallReady {
		if _, err := in.downloads.Start(download.Spec{
			Label:     rt.Name,
			URL:       rt.URL,
			Dest:      in.archivePath(rt),
			SizeBytes: rt.SizeBytes,
			SHA256:    rt.SHA256,
			AllowHost: runtimeHost,
			Scope:     DownloadScope,
			Install: func(archive string) error {
				if err := extractTarGz(archive, in.RuntimeDir(rt)); err != nil {
					return err
				}
				st, err := os.Stat(in.RuntimeBinary(rt))
				if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o111 == 0 {
					_ = os.RemoveAll(in.RuntimeDir(rt))
					return errors.New("the archive does not contain the server program")
				}
				_ = os.Remove(archive)
				return nil
			},
		}); err != nil {
			return err
		}
	}
	if in.ModelState(m).State != entity.InstallReady {
		if _, err := in.downloads.Start(download.Spec{
			Label:     m.Name,
			URL:       m.URL,
			Dest:      in.ModelPath(m),
			SizeBytes: m.SizeBytes,
			SHA256:    m.SHA256,
			AllowHost: modelHost,
			Scope:     DownloadScope,
		}); err != nil {
			return err
		}
	}
	return nil
}

// failureOf reports why the last download for key (a destination path) failed, or nil: a running or finished-well job clears it.
func (in *installer) failureOf(key, what string) *entity.Failure {
	in.failMu.Lock()
	defer in.failMu.Unlock()
	if v, ok := in.downloads.Latest(key); ok {
		switch {
		case v.State == download.StateFailed:
			if in.failures == nil {
				in.failures = map[string]entity.Failure{}
			}
			if old, seen := in.failures[key]; !seen || old.At != v.FinishedAt {
				in.failures[key] = entity.Failure{What: what, Message: v.Error, At: v.FinishedAt}
			}
		default: // queued, running, installing, done or cancelled: whatever failed before is history
			delete(in.failures, key)
		}
	}
	f, ok := in.failures[key]
	if !ok {
		return nil
	}
	if at, err := time.Parse(time.RFC3339, f.At); err == nil && time.Since(at) > failureMemory {
		delete(in.failures, key)
		return nil
	}
	return &f
}

// ModelFailure and RuntimeFailure say why the last download of the model or of the program failed (nil when none did).
func (in *installer) ModelFailure(m catalog.Model) *entity.Failure {
	return in.failureOf(in.ModelPath(m), "model")
}

func (in *installer) RuntimeFailure(r catalog.Runtime) *entity.Failure {
	return in.failureOf(in.archivePath(r), "program")
}

// CancelDownloads stops every download of this module that is running (the program and the models) and removes their unfinished
// files. It is what switching the local AI off does while it is still being set up (plan 205).
func (in *installer) CancelDownloads() {
	for _, m := range in.cat().Models {
		if id, ok := in.downloads.Active(in.ModelPath(m)); ok {
			in.downloads.Cancel(id)
		}
	}
	for _, r := range in.cat().Runtimes {
		if id, ok := in.downloads.Active(in.archivePath(r)); ok {
			in.downloads.Cancel(id)
		}
	}
}

// RemoveModel deletes one model's file and any unfinished download of it. It refuses while that model is being downloaded.
func (in *installer) RemoveModel(m catalog.Model) error {
	dest := in.ModelPath(m)
	if _, ok := in.downloads.Active(dest); ok {
		return entity.ErrDownloadRunning
	}
	for _, p := range []string{dest, dest + ".part"} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return errors.New("the model file could not be removed")
		}
	}
	return nil
}

// Remove deletes everything the module downloaded (models, runtime, leftovers). It refuses while any download is running.
func (in *installer) Remove() error {
	for _, m := range in.cat().Models {
		if _, ok := in.downloads.Active(in.ModelPath(m)); ok {
			return entity.ErrDownloadRunning
		}
	}
	for _, r := range in.cat().Runtimes {
		if _, ok := in.downloads.Active(in.archivePath(r)); ok {
			return entity.ErrDownloadRunning
		}
	}
	for _, sub := range []string{"models", "runtime", "downloads"} {
		if err := os.RemoveAll(filepath.Join(in.root(), sub)); err != nil {
			return errors.New("the downloaded files could not be removed")
		}
	}
	return nil
}
