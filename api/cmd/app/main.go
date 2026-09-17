// Command app is cinqo's single-executable, desktop-tool-style entry
// point: it starts the backend, starts Caddy in-process (serving the
// embedded frontend build and reverse-proxying /api and /auth to the
// backend), and opens a browser — see plan/ai/build/app.md and its
// step-by-step breakdown under plan/ai/build/app/.
package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-server/server"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	_ "github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile" // registers the "caddyfile" adapter
	_ "github.com/caddyserver/caddy/v2/modules/standard"          // reverse_proxy, file_server, handle, try_files

	"github.com/a-digi/cinqo/cmd/app/webapp"
	"github.com/a-digi/cinqo/config"
	"github.com/a-digi/cinqo/config/di"
	auth_config "github.com/a-digi/cinqo/src/auth/config"
	"github.com/a-digi/cinqo/src/backendapp"
	tool_manager "github.com/a-digi/cinqo/src/tool/manager"
)

//go:embed embedded.Caddyfile
var embeddedCaddyfile []byte

// embeddedConfigJSON is the default root config.json (port/pid_file)
// this app falls back to when launched from a directory that doesn't
// already have one — the actual fix for "load config: could not open
// config file: open config.json: no such file or directory" when
// opened from somewhere other than api/. Its own pid_file value is a
// template only: ensureConfigFile overrides it to an absolute,
// apphome-relative path before ever writing the file to disk. See
// plan/ai/build/app/step-18-embedded-default-config-and-app-home.md.
//
//go:embed embedded-config.json
var embeddedConfigJSON []byte

// embeddedConfigDir is api/config/'s own runtime-data files (migration
// SQL, route YAML, the auth config.json, iam.yaml, system-tools.yaml)
// — Go source files (di.go, embed.go, routes.go, handlerfunc.go,
// scope_registry.go) deliberately excluded by `make embed-config`'s
// own selective copy, not embedded here. Extracted (always
// overwritten, unlike embeddedConfigJSON's own once-only
// ensureConfigFile) to <home>/config and pointed at via
// CINQO_CONFIG_DIR when launched from a directory with no api/config/
// tree of its own. See
// plan/ai/build/app/step-19-embedded-config-directory.md.
//
//go:embed all:embeddedconfig
var embeddedConfigDir embed.FS

// embeddedAppVersion is a copy of api/VERSION's own content — the fix
// for "failed to determine the running app version" on a fresh tool
// install when this binary is opened from a directory with no
// api/VERSION beside it. Always present, compiled in — unlike
// api/main.go's own equivalent (a real, possibly-failing file read),
// this can never fail to resolve regardless of launch location. See
// plan/ai/build/app/step-20-app-version-via-di.md.
//
//go:embed embedded-version
var embeddedAppVersion string

// caddyPort is the single source of truth for the port embedded.Caddyfile
// itself hardcodes (its site address can't be an {env.*} placeholder —
// see plan/ai/build/app/step-03-embedded-caddy-config.md). Kept here too
// so stopStaleInstance and caddyAddr don't each carry their own copy of
// the literal.
const caddyPort = 7030

var caddyAddr = fmt.Sprintf("http://localhost:%d", caddyPort)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	// --data overrides where this app's own local state (db/, logs/,
	// keys/, tools/) lives — omitted, falls back to resolveAppHome's
	// own data/ default, next to the running executable. See
	// plan/ai/build/app/step-17-configurable-data-directory.md and
	// plan/ai/build/app/step-22-data-dir-always-executable-relative.md.
	dataDir := flag.String("data", "", `path to the data directory (default: "data", next to the executable)`)
	flag.Parse()

	// Armed before anything else starts (backend, Caddy, or the
	// browser) so a self-sent SIGTERM from launchPrivateChrome's exit
	// watcher can never race ahead of signal.Notify — an unhandled
	// SIGTERM's default disposition is immediate, non-graceful process
	// termination. See
	// plan/ai/build/app/step-10-shutdown-when-browser-closes.md.
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	// Resolved first, before anything else touches config.json — always
	// the running executable's own directory, regardless of the
	// process's CWD or launch method, so every launch from anywhere
	// finds the same persistent state instead of starting fresh each
	// time or landing in an OS-user-scoped home directory the user
	// never asked for. See
	// plan/ai/build/app/step-18-embedded-default-config-and-app-home.md
	// and plan/ai/build/app/step-22-data-dir-always-executable-relative.md.
	home, err := resolveAppHome()
	if err != nil {
		return fmt.Errorf("resolve app home: %w", err)
	}
	configPath := filepath.Join(home, "config.json")
	if err := ensureConfigFile(configPath, home); err != nil {
		return fmt.Errorf("create default config: %w", err)
	}
	dataDefault := filepath.Join(home, "data")
	chromePidPath := filepath.Join(home, "chrome.pid")

	// migrations/route YAML/the auth config.json/iam.yaml/
	// system-tools.yaml all live under api/config/ today, resolved via
	// candidateDefaults()'s own executable-relative/CWD-relative
	// search — which finds nothing when there's no api/config/ tree
	// shipped next to this binary at all. Extracting the embedded copy
	// and pointing CINQO_CONFIG_DIR (the override candidateDefaults()
	// already respects) at it fixes every one of those in one shot —
	// no changes needed to backendapp.Start or the config package
	// itself. See plan/ai/build/app/step-19-embedded-config-directory.md.
	configDir := filepath.Join(home, "config")
	if err := extractEmbeddedConfig(configDir); err != nil {
		return fmt.Errorf("extract embedded config: %w", err)
	}
	if err := os.Setenv(config.EnvVarConfigDir, configDir); err != nil {
		return fmt.Errorf("set %s: %w", config.EnvVarConfigDir, err)
	}

	cfg, err := server.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	stoppedPID, err := stopStaleInstance(cfg)
	if err != nil {
		return err
	}
	closeStaleChromeInstance(chromePidPath)

	srv, cfg, ctx, log, err := backendapp.Start(backendapp.Options{
		DataDir:    backendapp.ResolveDataDir(*dataDir, dataDefault),
		ConfigPath: configPath,
		AppVersion: strings.TrimSpace(embeddedAppVersion),
	})
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}
	defer log.Close()

	if stoppedPID != 0 {
		log.Info("stopped previous instance (pid %d) before starting", stoppedPID)
	}

	overrideFrontendCallbackURL(ctx, log)

	frontendDir, err := extractFrontend()
	if err != nil {
		return fmt.Errorf("extract embedded frontend: %w", err)
	}
	defer os.RemoveAll(frontendDir)

	if err := startCaddy(cfg.Port, frontendDir); err != nil {
		return fmt.Errorf("start caddy: %w", err)
	}

	if err := waitUntilUp(caddyAddr, 5*time.Second); err != nil {
		log.Warning("caddy did not become reachable in time: %v", err)
	}
	if pid, err := openBrowser(caddyAddr); err != nil {
		log.Warning("could not open a browser automatically: %v", err)
	} else if pid != 0 {
		if err := os.WriteFile(chromePidPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
			log.Warning("could not record chrome pid: %v", err)
		}
	}

	waitForShutdown(shutdownCh, srv, cfg.PidFile, log, frontendDir, ctx.GetDatabaseManager().Connector.DB)
	return nil
}

// resolveAppHome decides where this run's own config.json — and,
// absent an explicit --data, its data/ directory and chrome.pid too —
// live: always the running executable's own directory, never the
// process's CWD and never an OS-user-scoped home directory. This is
// deliberately one unified tree, matching this app's own existing
// convention of config.json sitting next to data/ rather than
// splitting config/data/cache across OS-specific locations, and
// guarantees every path this app writes to (databases, its own logs,
// every tool's own database/uploads/tmp directories — see
// plan/ai/build/app/step-22-data-dir-always-executable-relative.md)
// resolves under the same one directory regardless of how or from
// where the binary was launched. See
// plan/ai/build/app/step-18-embedded-default-config-and-app-home.md.
func resolveAppHome() (home string, err error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	// Resolve symlinks so launching via a symlink (a Homebrew-style
	// /usr/local/bin -> Cellar link, or a desktop shortcut) still
	// resolves to the real binary's own directory, not the symlink's —
	// os.Executable()'s own doc comment calls this caveat out
	// explicitly.
	if resolved, evalErr := filepath.EvalSymlinks(exePath); evalErr == nil {
		exePath = resolved
	}
	return filepath.Dir(exePath), nil
}

// ensureConfigFile writes the embedded default config.json to path if
// nothing is there yet — never overwrites an existing file, so a
// user's own later edits (e.g. a different port) persist across
// restarts, the same expectation the checked-in dev config.json
// already has. pid_file is overridden to an absolute, home-relative
// path before writing — parsed and re-marshaled, not string-
// templated, since verified directly (coco-server's own pid.go)
// that PidFile is used as a genuinely opaque path (os.Create/
// os.ReadFile/os.Remove, no joining with any other base directory),
// so an absolute value here is safe and resolves correctly regardless
// of process CWD at any future launch. See
// plan/ai/build/app/step-18-embedded-default-config-and-app-home.md.
func ensureConfigFile(path, home string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(embeddedConfigJSON, &cfg); err != nil {
		return fmt.Errorf("parse embedded default config: %w", err)
	}
	cfg["pid_file"] = filepath.Join(home, "server.pid")

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// extractEmbeddedConfig always overwrites dest with the embedded
// config tree's current content — deliberately different from
// ensureConfigFile's own never-overwrite policy just above: this is
// app-owned, versioned data (migrations, route YAML, the auth
// config.json, iam.yaml, system-tools.yaml), not something a user is
// expected to hand-edit, so a later app version's own new migration
// must actually reach a user whose ~/.cinqo/config/ already exists
// from an earlier install — "extract once" would silently prevent
// that. Same fs.WalkDir + os.MkdirAll/os.WriteFile shape
// extractFrontend already uses for the embedded frontend build, just
// writing to a stable, persistent destination instead of a fresh temp
// directory removed on shutdown. See
// plan/ai/build/app/step-19-embedded-config-directory.md.
func extractEmbeddedConfig(dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return fs.WalkDir(embeddedConfigDir, "embeddedconfig", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("embeddedconfig", path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(embeddedConfigDir, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// stopStaleInstance detects whether a previous cinqo-app instance — or a
// stray `make run-dev` backend, since it writes the exact same PID file
// via the same server.StartServer call — is still running, signals it
// to stop, and waits until this app's own ports are actually free before
// returning. Runs before backendapp.Start() creates the structured
// logger, so progress is reported directly to stdout: the interactive,
// "watch it happen" signal this is meant to be, not something to bury in
// a log file nobody's tailing live. Returns the stopped PID (0 if there
// was nothing to stop) so the caller can log a permanent record of it
// once the real logger exists. See
// plan/ai/build/app/step-07-stale-instance-shutdown-on-startup.md.
func stopStaleInstance(cfg *server.Config) (stoppedPID int, err error) {
	pid, err := server.ReadPID(cfg.PidFile)
	if err != nil {
		return 0, nil // no PID file — nothing to stop
	}

	process, findErr := os.FindProcess(pid)
	alive := findErr == nil && process.Signal(syscall.Signal(0)) == nil
	if !alive {
		// Stale leftover file from an unclean shutdown/crash — nothing
		// is actually running, just clear it and move on.
		_ = server.RemovePID(cfg.PidFile)
		return 0, nil
	}

	fmt.Printf("Shutting down previous cinqo instance (pid %d)", pid)
	_ = server.SendSIGTERM(pid)

	ports := []int{cfg.Port, caddyPort}

	if waitForPortsFree(ports, 10*time.Second) {
		fmt.Println(" done.")
		_ = server.RemovePID(cfg.PidFile)
		return pid, nil
	}

	fmt.Println()
	fmt.Printf("Previous instance (pid %d) did not stop within 10s — forcing it to stop...\n", pid)
	_ = process.Signal(syscall.SIGKILL)

	if !waitForPortsFree(ports, 3*time.Second) {
		return 0, fmt.Errorf(
			"port %d or %d is still in use after force-stopping the previous instance (pid %d) — "+
				"something else may be using it", cfg.Port, caddyPort, pid)
	}

	_ = server.RemovePID(cfg.PidFile)
	return pid, nil
}

// waitForPortsFree polls until every port in ports can be bound, or
// timeout elapses. Prints one "." per poll tick (no newline) so the
// caller's in-progress line grows visibly.
func waitForPortsFree(ports []int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if allPortsFree(ports) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		fmt.Print(".")
		time.Sleep(300 * time.Millisecond)
	}
}

func allPortsFree(ports []int) bool {
	for _, p := range ports {
		if !portFree(p) {
			return false
		}
	}
	return true
}

func portFree(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// closeStaleChromeInstance closes a private Chrome instance left behind
// by a previous cinqo-app run (tracked via chromePidPath — resolveAppHome's
// own executable-relative path), if one is still open, before this run
// opens its own fresh one. Much simpler
// than stopStaleInstance: there's no port/listening contract to wait
// on, so a short fixed grace period is enough instead of a polling
// loop, and no stdout progress output — closing a leftover browser
// window is expected to be near-instant. See
// plan/ai/build/app/step-09-private-chrome-instance-and-pid-tracking.md
// and plan/ai/build/app/step-18-embedded-default-config-and-app-home.md.
func closeStaleChromeInstance(chromePidPath string) {
	data, err := os.ReadFile(chromePidPath)
	if err != nil {
		return // no PID file — nothing to close
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(chromePidPath)
		return
	}

	process, findErr := os.FindProcess(pid)
	alive := findErr == nil && process.Signal(syscall.Signal(0)) == nil
	if !alive {
		// Stale leftover file — the browser window (or whole machine
		// session) was already closed some other way.
		_ = os.Remove(chromePidPath)
		return
	}

	_ = process.Signal(syscall.SIGTERM)
	time.Sleep(2 * time.Second)
	if process.Signal(syscall.Signal(0)) == nil {
		_ = process.Signal(syscall.SIGKILL)
	}

	_ = os.Remove(chromePidPath)
}

// overrideFrontendCallbackURL points the OAuth login flow's frontend
// redirect at this mode's actual frontend origin (Caddy on caddyPort),
// instead of config.json's committed dev value (Vite's :5173 — correct
// only for the separate `npm run dev` + `make run-dev` loop, unreachable
// here since neither Vite nor a bare backend-only setup is running).
// config.json itself is intentionally left unchanged — same convention
// every coco-aim reference app already follows (its committed config
// targets Vite's dev port; nothing there is tuned for a Caddy-fronted
// mode). Safe to override post-Start: auth_handler.getAuthConfig reads
// "auth_config" fresh from the ContextBag on every request rather than
// having it baked into a closure at routes.Init time, so this takes
// effect for every request from here on, and no code path in cmd/app
// (or the config file on disk) needs to know two ports at once. See
// plan/ai/build/app/step-13-frontend-callback-url-override.md.
func overrideFrontendCallbackURL(ctx *di.ContextBag, log logger.Logger) {
	raw, ok := ctx.Get("auth_config")
	if !ok {
		return
	}
	authCfg, ok := raw.(auth_config.AppAuthConfig)
	if !ok {
		return
	}
	authCfg.FrontendCallbackURL = fmt.Sprintf("%s/login/callback", caddyAddr)
	ctx.Set("auth_config", authCfg)
	log.Info("overrode frontend_callback_url for app mode: %s", authCfg.FrontendCallbackURL)
}

// extractFrontend writes the embedded frontend build out to a real temp
// directory — Caddy's stock file_server module serves from a real
// filesystem path, not an in-process fs.FS; writing a custom Caddy
// module to bridge embed.FS directly would be more code for no benefit
// at this scope, so extraction is the deliberate, simpler choice.
func extractFrontend() (string, error) {
	dir, err := os.MkdirTemp("", "cinqo-frontend-*")
	if err != nil {
		return "", err
	}

	err = fs.WalkDir(webapp.FS, "dist", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel("dist", path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(webapp.FS, path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// startCaddy adapts the embedded Caddyfile (backendAddr/frontendDir
// filled in via {env.*} placeholders — see
// plan/ai/build/app/step-03-embedded-caddy-config.md) and loads it
// in-process. The site's own port is a literal :7030 baked into
// embedded.Caddyfile, not a placeholder — verified in step 3 that a
// Caddyfile site address can't be one.
func startCaddy(backendPort int, frontendDir string) error {
	if err := os.Setenv("CINQO_BACKEND_ADDR", fmt.Sprintf("127.0.0.1:%d", backendPort)); err != nil {
		return err
	}
	if err := os.Setenv("CINQO_FRONTEND_DIR", frontendDir); err != nil {
		return err
	}

	adapter := caddyconfig.GetAdapter("caddyfile")
	if adapter == nil {
		return fmt.Errorf("caddyfile adapter not registered")
	}
	jsonConfig, _, err := adapter.Adapt(embeddedCaddyfile, nil)
	if err != nil {
		return fmt.Errorf("adapt embedded Caddyfile: %w", err)
	}

	return caddycore.Load(jsonConfig, true)
}

// waitUntilUp polls url until it responds (any status code — reachability
// is all that's being checked) or timeout elapses. Same reasoning as
// coco-mda's own run-dev-all Makefile target polling its backend before
// starting its frontend: gives Caddy's listener a moment to actually
// bind before a browser tab is opened against it.
func waitUntilUp(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for time.Now().Before(deadline) {
		if resp, err := client.Get(url); err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", url)
}

// openBrowser tries Google Chrome specifically first (the requirement),
// falling back to the OS's generic "open a URL" mechanism if Chrome
// isn't found — a hard failure when Chrome is merely absent would make
// this mode worse, not better, for that user. Returns the launched
// Chrome process's PID (0 for the generic-fallback path, which hands
// off to the OS and exits immediately — there is no stable PID to learn
// there on any platform) so the caller can record it in chromePidFile.
// See plan/ai/build/app.md's "Opening the browser" section and
// plan/ai/build/app/step-09-private-chrome-instance-and-pid-tracking.md.
func openBrowser(url string) (int, error) {
	switch runtime.GOOS {
	case "darwin":
		chromePath := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(chromePath); err == nil {
			return launchPrivateChrome(chromePath, url)
		}
		return 0, exec.Command("open", url).Start()
	case "linux":
		for _, bin := range []string{"google-chrome", "google-chrome-stable"} {
			if path, err := exec.LookPath(bin); err == nil {
				return launchPrivateChrome(path, url)
			}
		}
		return 0, exec.Command("xdg-open", url).Start()
	case "windows":
		for _, path := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		} {
			if _, err := os.Stat(path); err == nil {
				return launchPrivateChrome(path, url)
			}
		}
		return 0, exec.Command("cmd", "/c", "start", "", url).Start()
	default:
		return 0, fmt.Errorf("unsupported OS %q for opening a browser", runtime.GOOS)
	}
}

// launchPrivateChrome spawns a genuinely separate Chrome instance — not
// a new tab in whatever Chrome window already happens to be open.
// Verified directly: invoking the real binary with a fresh
// --user-data-dir forces Chrome's own single-instance check to treat
// this as an independent instance (its own PID, own process tree),
// which is also what makes returning a trackable PID possible at all —
// the OS-handoff commands (`open`, `xdg-open`, `cmd /c start`) used
// elsewhere in this file never give one back. A normal window (tab
// strip, address bar, reload button) is opened, not Chrome's chromeless
// "app mode" — that was tried (step 11) and reverted (step 15) once
// real usage showed two concrete costs: no download-shelf indicator at
// all (a successful download gives no visible confirmation), and no
// visible reload control on a single-page app that only picks up new
// code on an actual reload. See
// plan/ai/build/app/step-15-revert-to-normal-private-chrome-window.md.
//
// No longer passes --incognito, and no longer passes
// --no-startup-window (both tried and reverted in the same debugging
// session): profileDir is a brand-new os.MkdirTemp dir on every single
// launch, so Chrome always sees this as a first-ever-used default
// profile. --incognito on the command line against a fresh profile
// caused some Chrome versions to open a SECOND, empty, tab-less window
// for the automatic default-profile startup in addition to the
// requested incognito one. --no-startup-window, tried to suppress that
// automatic window, instead suppressed the ENTIRE launch — no window
// opened at all — because this is a brand-new process every time, with
// no already-running Chrome instance for the trailing `url` argument to
// be handed off to (the mechanism --no-startup-window actually assumes,
// e.g. an OS "open with Chrome" handler talking to an already-launched
// background instance). Neither flag survives here. A fresh, ephemeral
// --user-data-dir alone already gives the same practical isolation from
// the user's own regular Chrome profile (no persisted history/cookies
// survive between launches either way, incognito or not) without
// either of those two Incognito-adjacent quirks.
func launchPrivateChrome(chromePath, url string) (int, error) {
	profileDir, err := os.MkdirTemp("", "cinqo-chrome-profile-*")
	if err != nil {
		return 0, err
	}

	debugPort, err := findFreePort()
	if err != nil {
		_ = os.RemoveAll(profileDir)
		return 0, err
	}

	cmd := exec.Command(chromePath,
		"--user-data-dir="+profileDir,
		"--no-first-run",
		"--no-default-browser-check",
		fmt.Sprintf("--remote-debugging-port=%d", debugPort),
		url, // a normal window/tab navigated to url, not --app=<url>
	)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(profileDir)
		return 0, err
	}

	// Verified directly (see plan/ai/build/app/step-10-shutdown-when-browser-closes.md):
	// cmd.Wait() reliably unblocks in real time when this exact spawned
	// Chrome process exits, whether that's the user closing the last
	// window, quitting the app, or a crash. Self-signaling SIGTERM
	// reuses waitForShutdown's already-tested cleanup sequence — the
	// same path SIGINT/SIGTERM already take — instead of duplicating
	// it. Only fires while some part of this specific instance is
	// still alive; additional windows opened within it keep it running
	// and correctly don't trigger this.
	go func() {
		_ = cmd.Wait()
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()

	// Closing the app-mode window alone does NOT make the Chrome
	// process exit — verified directly (see
	// plan/ai/build/app/step-14-fully-quit-chrome-on-window-close.md):
	// macOS (and every platform) leaves a GUI app's process running
	// with zero windows after its last window closes, same as any
	// other app. cmd.Wait() above never fires on its own in that case.
	// This goroutine finishes the job Chrome won't do itself: poll this
	// specific instance's own CDP debug port (addressed by a private
	// TCP port, not the ambiguous, shared "Google Chrome" bundle
	// identity AppleScript would use) for its visible window/tab count,
	// and terminate the process once it's genuinely gone — which then
	// drives the cmd.Wait() goroutine above, same as a direct
	// Cmd+Q/SIGTERM already does.
	go watchForWindowClose(cmd, debugPort)

	return cmd.Process.Pid, nil
}

// findFreePort asks the OS for an available TCP port by briefly binding
// to :0 and reading back what it picked, then releasing it immediately
// for Chrome's own debug server to bind instead. Same find-a-free-port
// idiom this file already relies on via portFree, just inverted (find
// one that's free, rather than confirm one already is).
func findFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// watchForWindowClose polls the Chrome instance's own CDP debug port
// (see findFreePort/launchPrivateChrome) until its visible window is
// gone, then terminates the process. A short debounce (two consecutive
// zero-readings, ~1s) guards against a same-origin SPA navigation or
// reload transiently reporting zero visible pages — not a proven issue
// for this app's own React Router SPA (client-side navigation never
// destroys/recreates the page target), but a cheap safety margin. See
// plan/ai/build/app/step-14-fully-quit-chrome-on-window-close.md.
func watchForWindowClose(cmd *exec.Cmd, debugPort int) {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	consecutiveZero := 0
	for {
		time.Sleep(500 * time.Millisecond)

		if process, err := os.FindProcess(cmd.Process.Pid); err != nil || process.Signal(syscall.Signal(0)) != nil {
			return // process already gone (e.g. via cmd.Wait()'s own path) — nothing left to watch
		}

		if hasVisiblePage(client, debugPort) {
			consecutiveZero = 0
			continue
		}

		consecutiveZero++
		if consecutiveZero >= 2 {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			return
		}
	}
}

// cdpTarget mirrors just the fields needed from a Chrome DevTools
// Protocol /json/list entry — the endpoint returns several always-
// present internal targets (background_page, browser_ui,
// service_worker) regardless of window state; only "page" is a real,
// visible window/tab. Verified directly against a real instance before
// relying on this filter.
type cdpTarget struct {
	Type string `json:"type"`
}

func hasVisiblePage(client *http.Client, debugPort int) bool {
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", debugPort))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var targets []cdpTarget
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return false
	}

	for _, t := range targets {
		if t.Type == "page" {
			return true
		}
	}
	return false
}

// waitForShutdown blocks until SIGINT/SIGTERM arrives on ch — armed by
// the caller (run) before anything is started, and also the target of
// launchPrivateChrome's own self-signal when the user closes the
// spawned browser — then stops Caddy before the backend, since Caddy is
// the public-facing listener and stopping the backend first would leave
// it still accepting connections it can no longer proxy anywhere.
func waitForShutdown(ch <-chan os.Signal, srv *http.Server, pidFile string, log logger.Logger, frontendDir string, toolDB *sql.DB) {
	sig := <-ch
	log.Info("Received signal %s, shutting down...", sig)

	if err := caddycore.Stop(); err != nil {
		log.Warning("caddy stop: %v", err)
	}

	// Stopped before the backend's own HTTP shutdown below: a tool's
	// subprocess is a child of THIS process, never of Caddy or the
	// backend's http.Server, so nothing else here would ever reap it —
	// left unstopped, it (and anything it in turn spawned, e.g. a
	// headless Chrome instance a crawl started) becomes a permanently
	// orphaned process once this one exits. Reproduced and confirmed
	// live before this fix.
	tool_manager.StopAll(toolDB, func(format string, args ...any) {
		log.Warning(format, args...)
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if srv != nil {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Error("backend graceful shutdown failed: %v", err)
		}
	}

	if err := os.RemoveAll(frontendDir); err != nil {
		log.Warning("could not remove extracted frontend dir: %v", err)
	}
	if err := server.RemovePID(pidFile); err != nil {
		log.Warning("could not remove PID file: %v", err)
	}
}
