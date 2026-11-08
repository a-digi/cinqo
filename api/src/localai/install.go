package localai

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

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
	m, ok := in.catalog.Find(modelID)
	if !ok {
		return entity.ErrUnknownModel
	}
	rt, ok := in.catalog.RuntimeFor(system.OS, system.Arch)
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

// Remove deletes everything the module downloaded (models, runtime, leftovers). It refuses while any download is running.
func (in *installer) Remove() error {
	for _, m := range in.catalog.Models {
		if _, ok := in.downloads.Active(in.ModelPath(m)); ok {
			return entity.ErrDownloadRunning
		}
	}
	for _, r := range in.catalog.Runtimes {
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
