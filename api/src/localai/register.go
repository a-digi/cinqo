package localai

import (
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/a-digi/coco-server/server/routing"

	"github.com/a-digi/cinqo/src/lifecycle"
	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/handler"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform"
)

// Register adds the local platform to the platform registry. It is the module's only start-up hook: call it once at
// boot, after the main database is open. Removing the module means deleting this package and this one call.
func Register(mainDB *sql.DB, deps Deps) error {
	// A broken shipped catalog refuses the whole module at start, with the reason, instead of failing later on a request.
	cat, err := catalog.Default()
	if err != nil {
		return err
	}
	if deps.Downloads == nil || deps.DataDir == "" {
		return errors.New("the local AI needs the download manager and the data directory")
	}
	// the models now are the shipped ones and the ones a person added after a tool suggested them (plan 220)
	models.SetSource(models.FromDB(mainDB, nil))
	in := &installer{downloads: deps.Downloads, dataDir: deps.DataDir, catalog: cat, source: models.Current}
	setInstaller(in)
	// a model Cinqo only offers on suggestion: one that is already on this computer is taken over, one that is missing and that Cinqo suggests itself is
	// suggested (plan 221). Nothing is downloaded here.
	reconcileSuggestOnly(mainDB, in, cat, time.Now())

	// A model program left over from an earlier run that did not shut down cleanly is stopped before anything else.
	killStale(in.root(), processAlive, commandOf, 3*time.Second)
	engineForApp := newManagedEngine(in, newSupervisor(supervisorOptions{}), query.NewSettingsQueryRepo(mainDB))
	lifecycle.OnShutdown(engineForApp.Stop) // and the one this run starts is stopped with the application
	setManaged(engineForApp)
	setDialects(defaultDialects())
	return platform.Register(Entry(mainDB))
}

var (
	installerMu sync.RWMutex
	current     *installer
)

func setInstaller(in *installer) {
	installerMu.Lock()
	current = in
	installerMu.Unlock()
}

// lazyInstalls hands the handlers the installer created by Register, which runs before the routes are built.
type lazyInstalls struct{}

func (lazyInstalls) get() (*installer, bool) {
	installerMu.RLock()
	defer installerMu.RUnlock()
	return current, current != nil
}

func (l lazyInstalls) ModelState(m catalog.Model) entity.FileInstall {
	if in, ok := l.get(); ok {
		return in.ModelState(m)
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

func (l lazyInstalls) RuntimeState(r catalog.Runtime) entity.FileInstall {
	if in, ok := l.get(); ok {
		return in.RuntimeState(r)
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

func (l lazyInstalls) ModelFailure(m catalog.Model) *entity.Failure {
	if in, ok := l.get(); ok {
		return in.ModelFailure(m)
	}
	return nil
}

func (l lazyInstalls) RuntimeFailure(r catalog.Runtime) *entity.Failure {
	if in, ok := l.get(); ok {
		return in.RuntimeFailure(r)
	}
	return nil
}

func (l lazyInstalls) Start(modelID string, system hardware.Info) error {
	in, ok := l.get()
	if !ok {
		return errors.New("downloads are not available")
	}
	return in.Start(modelID, system)
}

func (l lazyInstalls) CancelDownloads() {
	if in, ok := l.get(); ok {
		in.CancelDownloads()
	}
}

func (l lazyInstalls) RemoveModel(m catalog.Model) error {
	in, ok := l.get()
	if !ok {
		return errors.New("downloads are not available")
	}
	return in.RemoveModel(m)
}

func (l lazyInstalls) Remove() error {
	in, ok := l.get()
	if !ok {
		return errors.New("downloads are not available")
	}
	return in.Remove()
}

// Handlers returns the module's HTTP handlers under the executor names its route file (config/routes/route-localai.yaml)
// uses. routes.Init merges them into its handler map.
func Handlers() map[string]routing.HandlerInterface {
	return handler.Handlers(managedRuntime{}, lazyInstalls{})
}
