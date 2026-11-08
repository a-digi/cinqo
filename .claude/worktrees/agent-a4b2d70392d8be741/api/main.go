package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/a-digi/coco-server/server"

	"github.com/a-digi/cinqo/src/cinqo"

	"github.com/a-digi/coco-logger/logger"
)

func main() {
	// --data must come before the start/shutdown action — Go's own
	// flag package stops parsing at the first non-flag argument, so
	// `cinqo-app --data /path start` works but `cinqo-app start --data
	// /path` does not (the "start" positional halts flag parsing
	// before --data is ever seen). See
	// plan/ai/build/app/step-17-configurable-data-directory.md.
	dataDir := flag.String("data", "", `path to the data directory (default: "data", relative to the working directory)`)
	flag.Parse()

	action := "start"
	if args := flag.Args(); len(args) > 0 {
		action = args[0]
	}

	// This dev binary always assumes cd api && ... (the Makefile's own
	// convention) — no apphome fallback here, unlike cmd/app/main.go
	// (step 18): a missing api/config.json in this workflow is a real
	// setup problem worth failing loudly on, not smoothing over. Both
	// calls below pass the literal "config.json" and an empty data-dir
	// fallback ("") for exactly that reason.
	resolvedDataDir := cinqo.ResolveDataDir(*dataDir, "")

	if action == "shutdown" {
		log, err := logger.NewLogger(server.LogFileName("cinqo"), filepath.Join(resolvedDataDir, "logs"))
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer log.Close()
		if err := server.ShutdownServer("config.json", log); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}

	srv, cfg, _, log, err := cinqo.Start(cinqo.Options{
		DataDir:    resolvedDataDir,
		ConfigPath: "config.json",
		AppVersion: readAppVersionFile(),
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer log.Close()

	server.GracefulShutdown(srv, cfg.PidFile, log)
}

// readAppVersionFile reads api/VERSION, unchanged CWD-relative
// behavior — this dev binary always assumes cd api && ..., same
// philosophy as the "config.json" literal above. Deliberately
// non-fatal on failure (returns "", not an error) — today, a missing
// VERSION file only breaks tool installation specifically, not
// startup; appVersion(reqCtx) (tool/handler/paths.go) is what turns
// an empty value back into that same, narrowly-scoped error, only
// if/when a tool install is actually attempted. See
// plan/ai/build/app/step-20-app-version-via-di.md.
func readAppVersionFile() string {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
