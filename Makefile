DEPLOY_GOARCH ?= arm64

ifeq ($(DEPLOY_GOARCH),amd64)
  ZIG_TARGET := x86_64-linux-gnu
else ifeq ($(DEPLOY_GOARCH),arm64)
  ZIG_TARGET := aarch64-linux-gnu
else
  $(error Unsupported DEPLOY_GOARCH "$(DEPLOY_GOARCH)" — use amd64 or arm64)
endif

BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null | sed 's/\./-/g')

# ── Application version ───────────────────────────────────────────────────────
# Single source of truth: the committed api/VERSION file (a bare dot-integer
# string, e.g. 0.0.1). It is embedded into the binary at build via ldflags
# (see VERSION_PKG below); the release build targets (build/build-linux) bump
# the patch first unless NO_BUMP=1. `run-dev` embeds the current value without
# bumping. Same scheme as coco-mda/coco-release.
VERSION_FILE := api/VERSION
VERSION_PKG  := github.com/a-digi/cinqo/src/version.Version

# api/ is this app's Go module root (its own go.mod) — every target below
# cds into api/ first, same convention as every sibling coco-aim app.

# Bump the patch segment of api/VERSION and rewrite the file (0.0.1 -> 0.0.2).
# Patch-only, matching the repo's plugin version scheme.
.PHONY: bump-version
bump-version:
	@cur=$$(tr -d ' \t\n\r' < $(VERSION_FILE)); \
	 maj=$${cur%%.*}; rest=$${cur#*.}; min=$${rest%%.*}; pat=$${rest#*.}; \
	 new="$$maj.$$min.$$((pat+1))"; \
	 printf '%s' "$$new" > $(VERSION_FILE); \
	 echo "cinqo version: $$cur -> $$new"

.PHONY: run-dev
run-dev:
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	v=$$(tr -d ' \t\n\r' < $(VERSION_FILE) 2>/dev/null || echo 0.0.1); \
	cd api && go run -ldflags "-X $(VERSION_PKG)=$$v" . start &

.PHONY: stop-dev
stop-dev:
	cd api && go run . shutdown

# Force-kill whatever is holding the backend dev port (7026), regardless of
# how it was started — use when `stop-dev` (graceful) can't reach it (stuck
# process, failed startup, etc.).
.PHONY: kill-backend
kill-backend:
	@pids=$$(lsof -ti tcp:7026 2>/dev/null); \
	if [ -n "$$pids" ]; then \
	  echo "Killing backend on :7026 (pids: $$pids)"; \
	  kill -9 $$pids 2>/dev/null || true; \
	else \
	  echo "Nothing listening on :7026"; \
	fi; \
	rm -f api/server.pid

.PHONY: build
build:
	@mkdir -p versions
ifndef NO_BUMP
	$(MAKE) bump-version
endif
	@v=$$(tr -d ' \t\n\r' < $(VERSION_FILE)); \
	 echo "building cinqo $$v"; \
	 cd api && go build -ldflags "-X $(VERSION_PKG)=$$v" -o ../versions/cinqo-$(BRANCH) .

.PHONY: build-linux
build-linux:
	@mkdir -p versions
	@command -v zig >/dev/null 2>&1 || { echo "zig not found. Install with: brew install zig"; exit 1; }
ifndef NO_BUMP
	$(MAKE) bump-version
endif
	@v=$$(tr -d ' \t\n\r' < $(VERSION_FILE)); \
	 echo "building cinqo $$v (linux/$(DEPLOY_GOARCH))"; \
	 cd api && CGO_ENABLED=1 GOOS=linux GOARCH=$(DEPLOY_GOARCH) \
	    CC="zig cc -target $(ZIG_TARGET)" \
	    CXX="zig c++ -target $(ZIG_TARGET)" \
	    go build -ldflags "-X $(VERSION_PKG)=$$v" -o ../versions/cinqo-deploy .

.PHONY: test
test:
	cd api && go test ./...

.PHONY: test-verbose
test-verbose:
	cd api && go test ./... -v

.PHONY: test-cover
test-cover:
	cd api && go test ./... -cover

.PHONY: mod-tidy
mod-tidy:
	cd api && go mod tidy

# Builds Caddy from source as part of this module's own build — no
# xcaddy, no system-wide Caddy install on any machine that builds this
# repo. api/cmd/caddy/main.go is a verbatim copy of Caddy's own
# documented "build without xcaddy" entry point; api/go.mod pins the
# exact caddy version, same as every other dependency here. Output is
# gitignored (a compiled binary, not source) and rebuilt on demand, not
# committed.
.PHONY: build-caddy
build-caddy:
	@mkdir -p api/bin
	cd api && go build -o bin/caddy ./cmd/caddy
	@echo "caddy built at api/bin/caddy"

# Runs Caddy in front of the built frontend (frontend/dist) and the
# already-running backend (:7026), unifying them on one local origin
# (:7030) — a "run it like production" preview, not the fast dev loop
# (that's run-dev-all / Vite's HMR server). Requires: `npm run build` has
# been run in frontend/, and the backend is already running (make
# run-dev). See plan/ai/deploy/local-caddy-plan.md.
.PHONY: run-caddy
run-caddy: build-caddy
	./api/bin/caddy run --config Caddyfile --adapter caddyfile

# Builds the frontend and copies its output under api/cmd/app/webapp/dist
# so //go:embed (which cannot reach outside the tree of the file
# containing the directive) can pull it into the single-executable
# build (make build-app). Output is a build artifact, gitignored, never
# hand-edited. See plan/ai/build/app/step-02-embed-frontend-build.md.
.PHONY: embed-frontend
embed-frontend:
	cd frontend && npm run build
	rm -rf api/cmd/app/webapp/dist
	cp -R frontend/dist api/cmd/app/webapp/dist

# Copies api/config/'s own runtime-data files (migrations SQL, route
# YAML, the auth config.json, iam.yaml, system-tools.yaml) into a
# location //go:embed in api/cmd/app/main.go can actually reach (embed
# patterns can't cross up out of their own package directory) —
# selectively, .go files (di.go, embed.go, routes.go,
# handlerfunc.go, scope_registry.go) excluded on purpose: copying them
# too would litter the module with duplicate, unimported Go packages
# that go build ./...//go vet ./... would then also compile. Same
# rm -rf + fresh-copy shape as embed-frontend above — a build-
# generated, gitignored directory, not something hand-edited. See
# plan/ai/build/app/step-19-embedded-config-directory.md.
.PHONY: embed-config
embed-config:
	rm -rf api/cmd/app/embeddedconfig
	mkdir -p api/cmd/app/embeddedconfig
	cd api/config && find . -type f \( -name '*.sql' -o -name '*.yaml' -o -name '*.json' \) \
	  -exec sh -c 'mkdir -p "../cmd/app/embeddedconfig/$$(dirname "$$1")" && cp "$$1" "../cmd/app/embeddedconfig/$$1"' _ {} \;

# app/VERSION tracks cinqo-app's own version, independently of
# api/VERSION (build/build-linux's, unrelated, unpadded M.N.P scheme).
# Format is M.N.PPP — patch always zero-padded to exactly 3 digits.
# Bumped unconditionally on every build-app run (no NO_BUMP escape,
# unlike bump-version below — intentional, per explicit instruction).
# See plan/ai/build/app/step-06-app-output-and-version-tracking.md.
APP_VERSION_FILE := app/VERSION

.PHONY: bump-app-version
bump-app-version:
	@mkdir -p app
	@cur=$$(cat $(APP_VERSION_FILE) 2>/dev/null || echo 0.0.000); \
	 maj=$${cur%%.*}; rest=$${cur#*.}; min=$${rest%%.*}; pat=$${rest#*.}; \
	 pat=$$((10#$$pat + 1)); \
	 if [ $$pat -gt 999 ]; then pat=0; min=$$((min + 1)); fi; \
	 new=$$(printf '%s.%s.%03d' "$$maj" "$$min" "$$pat"); \
	 printf '%s' "$$new" > $(APP_VERSION_FILE); \
	 echo "cinqo-app version: $$cur -> $$new"

# Builds the single, self-contained executable: backend + Caddy (as a
# library, not a second binary) + the embedded frontend build, all in
# one file. A distinct artifact from build/build-linux (which stay
# backend-only, the right shape for any future server-style remote
# deploy) — this is the desktop-tool-style "run it, it opens" mode.
# Output is a fixed filename (app/cinqo-app), overwritten every build —
# app/VERSION is the single source of truth for which version that
# binary currently is, not the filename itself. See
# plan/ai/build/app.md.
.PHONY: build-app
build-app: embed-frontend embed-config bump-app-version
	@mkdir -p app
	cd api && go build -o ../app/cinqo-app ./cmd/app
	@echo "cinqo-app $$(cat $(APP_VERSION_FILE)) built at app/cinqo-app"

# Builds (if needed) and runs the single executable in the foreground —
# unlike run-dev, deliberately NOT backgrounded with `&`: this is the
# "run it, it opens" desktop-tool mode, so Ctrl+C in this same terminal
# is the natural way to stop it. cinqo-app's own signal handler then
# stops Caddy and the backend together (they're one process, not two —
# see api/cmd/app/main.go's waitForShutdown) and cleans up the extracted
# frontend temp dir and PID file.
#
# Run with CWD=api/ — same convention run-dev already uses — NOT the
# repo root. cinqo-app's backend half still resolves its own
# config.json/config/ relative to its working directory (only the
# frontend build is actually embedded/portable today); running it from
# the repo root instead silently creates a fresh DEFAULT config.json
# there (coco-server's own port-2026 fallback, not this app's real
# 7026) rather than using the real api/config.json. Caught by testing
# this exact target, not assumed.
.PHONY: run-app
run-app: build-app
	cd api && ../app/cinqo-app

# Packages a tools/<name>/ source directory into the .zip the install
# endpoint (POST /api/v1/tools/install) expects — builds the backend
# (if present) first. See plan/ai/tools/step-09-skeleton-tool-templates.md.
.PHONY: package-tool
package-tool:
	@test -n "$(TOOL)" || (echo "Usage: make package-tool TOOL=<dir-name-under-tools/>"; exit 1)
	@test -d tools/$(TOOL) || (echo "tools/$(TOOL) does not exist"; exit 1)
	@rm -rf tools/$(TOOL)/build/generic
	@mkdir -p tools/$(TOOL)/build/generic
	cp tools/$(TOOL)/manifest.json tools/$(TOOL)/build/generic/
	@if [ -d tools/$(TOOL)/frontend ]; then cp -R tools/$(TOOL)/frontend tools/$(TOOL)/build/generic/frontend; fi
	@if [ -d tools/$(TOOL)/backend ]; then \
		mkdir -p tools/$(TOOL)/build/generic/backend && \
		cd tools/$(TOOL)/backend && go build -o ../build/generic/backend/tool . ; \
	fi
	@mkdir -p versions
	cd tools/$(TOOL)/build/generic && zip -r ../../../../versions/tool-$(TOOL).zip . >/dev/null
	@echo "packaged: versions/tool-$(TOOL).zip"
