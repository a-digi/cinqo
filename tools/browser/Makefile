# tools/browser/Makefile — cross-compiles and packages this tool's
# backend for macOS (Apple Silicon), Linux, and Windows, each a
# complete, independently-installable .zip dropped into this tool's
# own tools/browser/versions/ — not the shared root /versions/ (which
# stays cinqo's own binary's and every other tool's generic-package
# drop point). Adapted line-for-line from
# tools/pdf_generator/Makefile (already proven working) — see
# plan/ai/tools/browser/step-06-multi-os-packaging.md.
#
# Verified directly (not assumed) that this tool's own dependencies —
# chromedp/cdproto (pure Go, same as pdf_generator's) AND
# modernc.org/sqlite (the login allowlist's own pure-Go SQLite driver,
# step 5) — all cross-compile cleanly under plain CGO_ENABLED=0 for
# every target below; no C cross-toolchain needed here, unlike
# cinqo's own cgo-dependent (mattn/go-sqlite3) build.
#
# The Windows binary is deliberately named "tool", not "tool.exe" —
# matches the host's fixed backendExecutableFilename constant
# (api/src/tool/handler/paths.go) so this zip stays installable
# through the unmodified install pipeline. Relies on Windows'
# CreateProcess executing a full-path PE binary regardless of
# extension — a real, stated assumption, not verified on an actual
# Windows host from here (same caveat pdf_generator's own Makefile
# already carries).
#
# Linux ships both amd64 and arm64, matching pdf_generator's own
# already-verified convention.
#
# Step 10 added frontend/bundle.js — plain JS text, not compiled,
# identical across every OS/arch, so it's copied verbatim into each
# OS build dir alongside manifest.json (same reasoning already applied
# to manifest.json itself). Its presence is what promotes this
# package's own derived "kind" from backend_only to
# frontend_and_backend at install time (api/src/tool/manifest/manifest.go)
# — nothing here declares kind explicitly. See
# plan/ai/tools/browser/step-10-credential-management-frontend.md.

BUILD_DIR := build

.PHONY: build-darwin
build-darwin:
	@mkdir -p $(BUILD_DIR)/darwin-arm64/backend
	cp manifest.json $(BUILD_DIR)/darwin-arm64/
	cp -r frontend $(BUILD_DIR)/darwin-arm64/
	cd backend && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ../$(BUILD_DIR)/darwin-arm64/backend/tool .
	@echo "built: $(BUILD_DIR)/darwin-arm64/backend/tool"

.PHONY: build-linux
build-linux:
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
build-windows:
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
# tools/browser/versions/.
VERSIONS_DIR := versions

.PHONY: zip
zip: build-all
	@mkdir -p $(VERSIONS_DIR)
	cd $(BUILD_DIR)/darwin-arm64  && zip -r ../../$(VERSIONS_DIR)/tool-browser-darwin-arm64.zip  . >/dev/null
	cd $(BUILD_DIR)/linux-amd64   && zip -r ../../$(VERSIONS_DIR)/tool-browser-linux-amd64.zip   . >/dev/null
	cd $(BUILD_DIR)/linux-arm64   && zip -r ../../$(VERSIONS_DIR)/tool-browser-linux-arm64.zip   . >/dev/null
	cd $(BUILD_DIR)/windows-amd64 && zip -r ../../$(VERSIONS_DIR)/tool-browser-windows-amd64.zip . >/dev/null
	@echo "packaged:"
	@echo "  tools/browser/$(VERSIONS_DIR)/tool-browser-darwin-arm64.zip"
	@echo "  tools/browser/$(VERSIONS_DIR)/tool-browser-linux-amd64.zip"
	@echo "  tools/browser/$(VERSIONS_DIR)/tool-browser-linux-arm64.zip"
	@echo "  tools/browser/$(VERSIONS_DIR)/tool-browser-windows-amd64.zip"

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
