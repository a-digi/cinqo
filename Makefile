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
