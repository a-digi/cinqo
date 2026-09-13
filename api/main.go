package main

import (
	"context"
	"fmt"
	"os"

	dbmanager "github.com/a-digi/coco-orm/orm"
	"github.com/a-digi/coco-server/server"

	"github.com/a-digi/cinqo/config"
	"github.com/a-digi/cinqo/config/di"
	"github.com/a-digi/cinqo/config/routes"
	auth_config "github.com/a-digi/cinqo/src/auth/config"
	auth_service "github.com/a-digi/cinqo/src/auth/service"

	"github.com/a-digi/coco-logger/logger"
)

func main() {
	action := "start"
	if len(os.Args) > 1 {
		action = os.Args[1]
	}

	log, err := logger.NewLogger(server.LogFileName("cinqo"), "data/logs")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer log.Close()

	if action == "shutdown" {
		if err := server.ShutdownServer("config.json", log); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return
	}

	migrationsPath, err := config.MigrationsPath()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	manager, err := dbmanager.NewDatabaseManager("cinqo.db", "./data/db", []string{migrationsPath})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if err := manager.SyncMigrations(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	ctx := di.NewContextBag(manager, log)

	// Auth bootstrap: config.json's "auth" block is always present (dev
	// HS256 secret); iam.yaml does not exist on disk yet (see
	// plan/ai/backend/auth/step-01-iam-config-and-registration.md's open
	// question — cinqo has no coco-iam application registration yet), so
	// its read failure is expected and handled as "not registered yet",
	// not a startup error.
	authCfgBytes, err := config.ReadConfigFile("config.json")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	authCfg, err := auth_config.Load(authCfgBytes)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	if iamBytes, iamErr := config.ReadConfigFile("iam.yaml"); iamErr == nil {
		iamCfg, iamParseErr := auth_config.LoadIamConfig(iamBytes)
		if iamParseErr != nil {
			log.Warning("iam.yaml present but could not be parsed, ignoring: %v", iamParseErr)
		} else {
			iamCfg.Apply(&authCfg)
			log.Info("IAM config loaded from iam.yaml (issuer: %s)", authCfg.Issuer)
		}
	}

	jwksCtx, cancelJwks := context.WithCancel(context.Background())
	defer cancelJwks()

	jwksSvc := auth_service.NewJwksService(authCfg.JwksURL, authCfg.HS256Secret, authCfg.Issuer, authCfg.Audience)
	jwksSvc.Start(jwksCtx, func(format string, args ...any) { log.Warning(format, args...) })

	ctx.Set("jwks_service", jwksSvc)
	ctx.Set("auth_config", authCfg)

	routes.Init(ctx)

	serv, cfg, err := server.StartServer("config.json", log)
	if err != nil {
		log.Error("failed to start server: %v", err)
		os.Exit(1)
	}

	server.GracefulShutdown(serv, cfg.PidFile, log)
}
