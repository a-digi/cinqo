# tools/career/Makefile — cross-compiles and packages this tool's
# backend for macOS (Apple Silicon), Linux, and Windows, each a
# complete, independently-installable .zip dropped into this tool's
# own tools/career/versions/ — not the shared root /versions/. Adapted
# line-for-line from tools/browser/Makefile (its own post-step-15
# shape, already proven working) — see
# plan/ai/tools/career/step-06-multi-os-packaging.md.
#
# Verified directly (not assumed) that this tool's own dependencies —
# modernc.org/sqlite (steps 2/3/4, backing both career.db and jobs.db)
# and github.com/google/uuid — all cross-compile cleanly under plain
# CGO_ENABLED=0 for every target below; no C cross-toolchain needed
# here, unlike cinqo's own cgo-dependent (mattn/go-sqlite3) build.
# Having two SQLite databases instead of browser's one introduces no
# new cross-compilation risk — same driver, same pure-Go properties.
#
# The Windows binary is deliberately named "tool", not "tool.exe" —
# matches the host's fixed backendExecutableFilename constant
# (api/src/tool/handler/paths.go) so this zip stays installable
# through the unmodified install pipeline. Relies on Windows'
# CreateProcess executing a full-path PE binary regardless of
# extension — a real, stated assumption, not verified on an actual
# Windows host from here (same caveat browser's/pdf_generator's own
# Makefiles already carry).
#
# Linux ships both amd64 and arm64, matching browser's own convention.
#
# frontend/bundle.js is copied verbatim into each OS build dir
# alongside manifest.json (same reasoning already applied to
# manifest.json itself: identical across every OS/arch). Its presence
# is what promotes this package's own derived "kind" from
# backend_only to frontend_and_backend at install time
# (api/src/tool/manifest/manifest.go) — nothing here declares kind
# explicitly. build-frontend builds it fresh each time from
# frontend-app/'s own React+Tailwind source (tracked); Vite's own
# outDir writes straight to ../frontend/bundle.js — no separate copy
# step needed for the frontend BUILD itself, only for shipping the
# already-built file into each OS's own package dir. See
# plan/ai/tools/career/step-05-react-tailwind-frontend.md.

BUILD_DIR := build

.PHONY: build-frontend
build-frontend:
	cd frontend-app && npm install && npm run build
	@echo "built: frontend/bundle.js (from frontend-app/)"

.PHONY: build-darwin
build-darwin: build-frontend
	@mkdir -p $(BUILD_DIR)/darwin-arm64/backend
	cp manifest.json $(BUILD_DIR)/darwin-arm64/
	cp -r frontend $(BUILD_DIR)/darwin-arm64/
	cd backend && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ../$(BUILD_DIR)/darwin-arm64/backend/tool .
	@echo "built: $(BUILD_DIR)/darwin-arm64/backend/tool"

.PHONY: build-linux
build-linux: build-frontend
	@mkdir -p $(BUILD_DIR)/linux-amd64/backend $(BUILD_DIR)/linux-arm64/backend
	cp manifest.json $(BUILD_DIR)/linux-amd64/
	cp manifest.json $(BUILD_DIR)/linux-arm64/
	cp -r frontend $(BUILD_DIR)/linux-amd64/
	cp -r frontend $(BUILD_DIR)/linux-arm64/
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../$(BUILD_DIR)/linux-amd64/backend/tool .
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../$(BUILD_DIR)/linux-arm64/backend/tool .
	@echo "built: $(BUILD_DIR)/linux-amd64/backend/tool"
	@echo "built: $(BUILD_DIR)/linux-arm64/backend/tool"

.PHONY: build-windows
build-windows: build-frontend
	@mkdir -p $(BUILD_DIR)/windows-amd64/backend
	cp manifest.json $(BUILD_DIR)/windows-amd64/
	cp -r frontend $(BUILD_DIR)/windows-amd64/
	cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ../$(BUILD_DIR)/windows-amd64/backend/tool .
	@echo "built: $(BUILD_DIR)/windows-amd64/backend/tool"

.PHONY: build-all
build-all: build-darwin build-linux build-windows


# VERSIONS_DIR is this tool's own output folder — deliberately not the
# shared root versions/ (which the root Makefile's generic
# package-tool target still uses for cinqo's own binary and every
# other single-OS tool): a tool with its own multi-OS Makefile keeps
# its own finished .zips alongside itself, under
# tools/career/versions/.
VERSIONS_DIR := versions

.PHONY: zip
zip: build-all
	@mkdir -p $(VERSIONS_DIR)
	cd $(BUILD_DIR)/darwin-arm64  && zip -r ../../$(VERSIONS_DIR)/tool-career-darwin-arm64.zip  . >/dev/null
	cd $(BUILD_DIR)/linux-amd64   && zip -r ../../$(VERSIONS_DIR)/tool-career-linux-amd64.zip   . >/dev/null
	cd $(BUILD_DIR)/linux-arm64   && zip -r ../../$(VERSIONS_DIR)/tool-career-linux-arm64.zip   . >/dev/null
	cd $(BUILD_DIR)/windows-amd64 && zip -r ../../$(VERSIONS_DIR)/tool-career-windows-amd64.zip . >/dev/null
	@echo "packaged:"
	@echo "  tools/career/$(VERSIONS_DIR)/tool-career-darwin-arm64.zip"
	@echo "  tools/career/$(VERSIONS_DIR)/tool-career-linux-amd64.zip"
	@echo "  tools/career/$(VERSIONS_DIR)/tool-career-linux-arm64.zip"
	@echo "  tools/career/$(VERSIONS_DIR)/tool-career-windows-amd64.zip"

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
	rm -rf frontend-app/node_modules frontend-app/dist
