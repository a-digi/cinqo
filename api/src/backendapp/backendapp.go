// Package backendapp holds cinqo's backend bootstrap, extracted out of
// api/main.go so it can be called from more than one entrypoint —
// api/main.go itself (unchanged behavior) and, from
// plan/ai/build/app/step-04-combined-entrypoint.md on, the combined
// single-executable's api/cmd/app/main.go. A package main file cannot be
// imported by another package main, which is why this exists as its own
// importable package rather than staying inline in main().
package backendapp

import (
	"context"
	"net/http"

	dbmanager "github.com/a-digi/coco-orm/orm"
	"github.com/a-digi/coco-server/server"

	"github.com/a-digi/cinqo/config"
	"github.com/a-digi/cinqo/config/di"
	"github.com/a-digi/cinqo/config/routes"
	auth_config "github.com/a-digi/cinqo/src/auth/config"
	auth_service "github.com/a-digi/cinqo/src/auth/service"

	"github.com/a-digi/coco-logger/logger"
)

// Start performs the full backend bootstrap (config, DB manager, DI,
// auth, routes.Init) and starts the HTTP server, returning it
// un-blocked. The caller decides how to wait for shutdown:
// api/main.go calls server.GracefulShutdown right after (today's exact
// behavior); api/cmd/app/main.go instead coordinates shutdown together
// with an in-process Caddy instance. The returned ContextBag lets a
// caller override a DI-registered value after bootstrap — e.g.
// cmd/app/main.go overriding "auth_config"'s FrontendCallbackURL to
// match the port its own embedded Caddy actually serves the frontend
// on, since request handlers (auth_handler.getAuthConfig) read
// "auth_config" fresh from the ContextBag on every request rather than
// having it baked into a closure at routes.Init time — a later Set()
// here takes effect immediately, no restart needed.
func Start() (srv *http.Server, cfg *server.Config, ctx *di.ContextBag, log logger.Logger, err error) {
	log, err = logger.NewLogger(server.LogFileName("cinqo"), "data/logs")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	migrationsPath, err := config.MigrationsPath()
	if err != nil {
		return nil, nil, nil, log, err
	}

	manager, err := dbmanager.NewDatabaseManager("cinqo.db", "./data/db", []string{migrationsPath})
	if err != nil {
		return nil, nil, nil, log, err
	}

	if err := manager.SyncMigrations(); err != nil {
		return nil, nil, nil, log, err
	}

	ctx = di.NewContextBag(manager, log)

	// Auth bootstrap: config.json's "auth" block is always present (dev
	// HS256 secret); iam.yaml does not exist on disk yet (see
	// plan/ai/backend/auth/step-01-iam-config-and-registration.md's open
	// question — cinqo has no coco-iam application registration yet), so
	// its read failure is expected and handled as "not registered yet",
	// not a startup error.
	authCfgBytes, err := config.ReadConfigFile("config.json")
	if err != nil {
		return nil, nil, nil, log, err
	}
	authCfg, err := auth_config.Load(authCfgBytes)
	if err != nil {
		return nil, nil, nil, log, err
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

	// jwksCtx is intentionally never cancelled: the original main()'s
	// `defer cancelJwks()` only ever ran at process exit anyway (right
	// after GracefulShutdown's blocking wait returned), so dropping it
	// here changes nothing observable — the refresh goroutine dies with
	// the process either way. See
	// plan/ai/build/app/step-01-backend-bootstrap-refactor.md's "Open
	// question" section.
	jwksCtx := context.Background()
	jwksSvc := auth_service.NewJwksService(authCfg.JwksURL, authCfg.HS256Secret, authCfg.Issuer, authCfg.Audience)
	jwksSvc.Start(jwksCtx, func(format string, args ...any) { log.Warning(format, args...) })

	ctx.Set("jwks_service", jwksSvc)
	ctx.Set("auth_config", authCfg)

	routes.Init(ctx)

	srv, cfg, err = server.StartServer("config.json", log)
	return srv, cfg, ctx, log, err
}
