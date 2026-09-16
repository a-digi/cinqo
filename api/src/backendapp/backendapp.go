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
	"path/filepath"

	dbmanager "github.com/a-digi/coco-orm/orm"
	"github.com/a-digi/coco-server/server"

	"github.com/a-digi/cinqo/config"
	"github.com/a-digi/cinqo/config/di"
	"github.com/a-digi/cinqo/config/routes"
	auth_config "github.com/a-digi/cinqo/src/auth/config"
	auth_service "github.com/a-digi/cinqo/src/auth/service"
	"github.com/a-digi/cinqo/src/conversation"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	tool_manager "github.com/a-digi/cinqo/src/tool/manager"
	"github.com/a-digi/cinqo/src/tool/systemtools"

	"github.com/a-digi/coco-logger/logger"
)

// defaultDataDir is today's exact well-known location — every
// data/-relative literal that used to be hardcoded throughout this
// file now derives from whatever ResolveDataDir returns, and this is
// what it returns when the caller doesn't override it. "data" and the
// old "./data" literals resolve identically (a leading "./" is a
// no-op to every path/filepath and os function that touches these
// paths) — this fallback is provably behavior-preserving, not just
// close enough. See plan/ai/build/app/step-17-configurable-data-directory.md.
const defaultDataDir = "data"

// ResolveDataDir applies the fallback rule --data needs, in the one
// place every caller that needs it before/independent of Start itself
// (api/main.go's own shutdown-action logger, cmd/app/main.go's own
// apphome resolution) and Start itself all call, so the rule can't
// drift between them. fallback is step 18's own addition — an
// explicit --data always wins regardless; absent that, fallback wins
// if the caller has one (cmd/app/main.go passes its own apphome-based
// data/ directory when config.json wasn't found in CWD either), else
// defaultDataDir ("data", CWD-relative — api/main.go's own call always
// passes "" here, preserving its exact original behavior). See
// plan/ai/build/app/step-18-embedded-default-config-and-app-home.md.
func ResolveDataDir(flagValue, fallback string) string {
	if flagValue != "" {
		return flagValue
	}
	if fallback != "" {
		return fallback
	}
	return defaultDataDir
}

// Options bundles Start's own input parameters — switched from
// positional args to a struct once a third one (AppVersion) showed
// up, per step 18's own "revisit once a third override parameter is
// needed" note. DataDir and ConfigPath are both expected to already
// be fully resolved by the caller (ResolveDataDir for the former; the
// latter has no equivalent helper since its own fallback — apphome —
// is a cmd/app/main.go-specific concept Start itself has no business
// knowing about). AppVersion may be empty (api/main.go's own
// non-fatal read failure, step 20) — Start registers whatever it's
// given as-is; appVersion(reqCtx)'s own downstream empty-string check
// (tool/handler/paths.go) is what turns that into a real error, only
// if/when a tool install is actually attempted. See
// plan/ai/build/app/step-18-embedded-default-config-and-app-home.md
// and plan/ai/build/app/step-20-app-version-via-di.md.
type Options struct {
	DataDir    string
	ConfigPath string
	AppVersion string
}

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
func Start(opts Options) (srv *http.Server, cfg *server.Config, ctx *di.ContextBag, log logger.Logger, err error) {
	dataDir := opts.DataDir
	configPath := opts.ConfigPath
	logsDir := filepath.Join(dataDir, "logs")
	dbDir := filepath.Join(dataDir, "db")
	keyPath := filepath.Join(dataDir, "keys", "platform-encryption.key")

	log, err = logger.NewLogger(server.LogFileName("cinqo"), logsDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	migrationsPath, err := config.MigrationsPath()
	if err != nil {
		return nil, nil, nil, log, err
	}

	manager, err := dbmanager.NewDatabaseManager("cinqo.db", dbDir, []string{migrationsPath})
	if err != nil {
		return nil, nil, nil, log, err
	}

	if err := manager.SyncMigrations(); err != nil {
		return nil, nil, nil, log, err
	}

	// Reconcile every tool the database says is already enabled — this
	// package's own tool manager otherwise starts every restart with a
	// completely empty in-memory "what's running" map, leaving every
	// tool's proxy route 503ing until someone manually disables and
	// re-enables it by hand. See
	// plan/ai/tools/step-11-restart-enabled-tools-on-boot.md.
	corePortCfg, err := server.LoadConfig(configPath)
	if err != nil {
		return nil, nil, nil, log, err
	}
	tool_manager.StartAllEnabled(manager.Connector.DB, dataDir, corePortCfg.Port, func(format string, args ...any) { log.Warning(format, args...) })

	ctx = di.NewContextBag(manager, log)

	// Conversation feature's own, separate SQLite database (its own
	// file, its own migrations folder) — a deliberate second
	// DatabaseManager, not a table in cinqo.db, per
	// plan/ai/conversation/step-01-data-model-and-separate-database.md.
	// Registered into DI under its own key rather than replacing
	// ContextBag.DatabaseManager, so existing handlers resolving the
	// main database are unaffected.
	conversationMigrationsPath, err := config.ConversationMigrationsPath()
	if err != nil {
		return nil, nil, nil, log, err
	}
	conversationManager, err := dbmanager.NewDatabaseManager("conversation.db", dbDir, []string{conversationMigrationsPath})
	if err != nil {
		return nil, nil, nil, log, err
	}
	if err := conversationManager.SyncMigrations(); err != nil {
		return nil, nil, nil, log, err
	}
	ctx.Set("conversation_db_manager", conversationManager)

	// A turn run's own execution lives in a goroutine, not a supervised
	// OS process — it has no PID to reattach to after a restart the way
	// tool_manager.StartAllEnabled reconciles tools. Every turn_runs row
	// still "running" at this point in boot is therefore, by
	// definition, dead; mark it failed and record a matching error turn
	// so a reopened page explains what happened instead of the turn
	// simply vanishing. See
	// plan/ai/conversation/step-23-detach-turn-execution-from-request.md.
	conversation.ReconcileOrphanedTurnRuns(conversationManager.Connector.DB, func(format string, args ...any) { log.Warning(format, args...) })

	// Platform API-key encryption key — loaded/generated once at
	// bootstrap, deliberately never sourced from config.json (step 2),
	// registered into DI so every platform handler resolves the same
	// in-memory key rather than each re-reading the file itself.
	platformEncryptionKey, err := platform_crypto.LoadOrGenerateKey(keyPath)
	if err != nil {
		return nil, nil, nil, log, err
	}
	ctx.Set("platform_encryption_key", platformEncryptionKey)

	// The resolved data directory itself — registered so tool/handler
	// (installed-tool directories) and conversation/handler (per-
	// conversation log files) can each derive their own subdirectory
	// from the same root --data overrode, instead of each hardcoding
	// their own "data/..." literal. See
	// plan/ai/build/app/step-17-configurable-data-directory.md.
	ctx.Set("data_dir", dataDir)

	// The running app's own version — resolved once here from
	// whatever the caller supplied (embedded, for cmd/app; read fresh
	// from api/VERSION, for the dev binary) rather than tool/handler
	// re-reading a bare "VERSION" file itself on every install
	// request. May be empty (api/main.go's own non-fatal read
	// failure) — appVersion(reqCtx) (tool/handler/paths.go) is what
	// turns that into a real, user-facing error, only if/when a tool
	// install is actually attempted. See
	// plan/ai/build/app/step-20-app-version-via-di.md.
	ctx.Set("app_version", opts.AppVersion)

	// The backend's own listening port — reuses corePortCfg (already
	// loaded above, via the correctly-resolved configPath, for
	// tool_manager.StartAllEnabled) rather than tool/handler's own
	// corePort(reqCtx) re-reading config.json a second time via a bare
	// CWD-relative literal, the exact same class of bug app_version
	// itself had. See plan/ai/build/app/step-20-app-version-via-di.md.
	ctx.Set("core_port", corePortCfg.Port)

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

	// system-tools.yaml is the sole source of truth for which installed
	// tools are protected from deletion — optional, same convention as
	// iam.yaml above: absent or unparseable is logged and falls back to
	// "nothing protected" rather than failing startup. See
	// plan/ai/tools/step-10-system-tools.md.
	systemToolsCfg := systemtools.Config{}
	if data, err := config.ReadConfigFile("system-tools.yaml"); err == nil {
		if cfg, parseErr := systemtools.Load(data); parseErr != nil {
			log.Warning("system-tools.yaml present but could not be parsed, ignoring: %v", parseErr)
		} else {
			systemToolsCfg = cfg
		}
	}
	ctx.Set("system_tools_config", systemToolsCfg)

	routes.Init(ctx)

	srv, cfg, err = server.StartServer(configPath, log)
	return srv, cfg, ctx, log, err
}
