package main

import (
	"fmt"
	"os"

	"github.com/a-digi/coco-server/server"

	"github.com/a-digi/cinqo/src/backendapp"

	"github.com/a-digi/coco-logger/logger"
)

func main() {
	action := "start"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}

	if action == "shutdown" {
		log, err := logger.NewLogger(server.LogFileName("cinqo"), "data/logs")
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

	srv, cfg, log, err := backendapp.Start()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer log.Close()

	server.GracefulShutdown(srv, cfg.PidFile, log)
}
