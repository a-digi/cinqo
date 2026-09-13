// Command app is cinqo's single-executable, desktop-tool-style entry
// point: it starts the backend, starts Caddy in-process (serving the
// embedded frontend build and reverse-proxying /api and /auth to the
// backend), and opens a browser — see plan/ai/build/app.md and its
// step-by-step breakdown under plan/ai/build/app/.
package main

import (
	"context"
	_ "embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-server/server"

	caddycore "github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	_ "github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile" // registers the "caddyfile" adapter
	_ "github.com/caddyserver/caddy/v2/modules/standard"          // reverse_proxy, file_server, handle, try_files

	"github.com/a-digi/cinqo/cmd/app/webapp"
	"github.com/a-digi/cinqo/src/backendapp"
)

//go:embed embedded.Caddyfile
var embeddedCaddyfile []byte

const caddyAddr = "http://localhost:7030"

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	srv, cfg, log, err := backendapp.Start()
	if err != nil {
		return fmt.Errorf("backend: %w", err)
	}
	defer log.Close()

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
	if err := openBrowser(caddyAddr); err != nil {
		log.Warning("could not open a browser automatically: %v", err)
	}

	waitForShutdown(srv, cfg.PidFile, log, frontendDir)
	return nil
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
// this mode worse, not better, for that user. See
// plan/ai/build/app.md's "Opening the browser" section and
// plan/ai/build/app/step-04-combined-entrypoint.md's open question
// (flagged there, not yet confirmed) about whether this fallback is
// actually wanted versus a strict Chrome-or-fail requirement.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		if err := exec.Command("open", "-a", "Google Chrome", url).Start(); err == nil {
			return nil
		}
		return exec.Command("open", url).Start()
	case "linux":
		for _, bin := range []string{"google-chrome", "google-chrome-stable"} {
			if path, err := exec.LookPath(bin); err == nil {
				return exec.Command(path, url).Start()
			}
		}
		return exec.Command("xdg-open", url).Start()
	case "windows":
		for _, path := range []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		} {
			if _, err := os.Stat(path); err == nil {
				return exec.Command(path, url).Start()
			}
		}
		return exec.Command("cmd", "/c", "start", "", url).Start()
	default:
		return fmt.Errorf("unsupported OS %q for opening a browser", runtime.GOOS)
	}
}

// waitForShutdown blocks until SIGINT/SIGTERM, then stops Caddy before
// the backend — Caddy is the public-facing listener, so stopping the
// backend first would leave it still accepting connections it can no
// longer proxy anywhere.
func waitForShutdown(srv *http.Server, pidFile string, log logger.Logger, frontendDir string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	sig := <-ch
	log.Info("Received signal %s, shutting down...", sig)

	if err := caddycore.Stop(); err != nil {
		log.Warning("caddy stop: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
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
