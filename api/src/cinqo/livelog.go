package cinqo

import (
	"path/filepath"

	"github.com/a-digi/cinqo/src/livelog"
	livelog_handler "github.com/a-digi/cinqo/src/livelog/handler"
	livelog_providers "github.com/a-digi/cinqo/src/livelog/providers"
)

// configureLiveLogs is the composition root of the live log module (plan/ai/file-reader/live-follow.md): the one place that names the
// concrete providers, one Register line each. Adding a kind of log is a new file in src/livelog/providers and a line here; removing the
// module is deleting src/livelog, this file, the one call in Start, the route file route-livelog.yaml and the two executors in
// config/routes/routes.go (the module's boundary test keeps every other file from importing it).
func configureLiveLogs(dataDir string, logf func(format string, args ...any)) error {
	reg := livelog.NewRegistry()
	for _, p := range []livelog.Provider{
		livelog_providers.NewAITrace(filepath.Join(dataDir, "conversations")),
		livelog_providers.NewSystemLog(filepath.Join(dataDir, "logs")),
	} {
		if err := reg.Register(p); err != nil {
			return err
		}
	}
	livelog_handler.Configure(reg, logf)
	return nil
}
