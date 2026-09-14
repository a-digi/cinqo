package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/a-digi/coco-server/server"

	"github.com/a-digi/cinqo/src/backendapp"

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
	resolvedDataDir := backendapp.ResolveDataDir(*dataDir, "")

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

	srv, cfg, _, log, err := backendapp.Start(resolvedDataDir, "config.json")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer log.Close()

	server.GracefulShutdown(srv, cfg.PidFile, log)
}
