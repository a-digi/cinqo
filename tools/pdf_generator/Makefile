# tools/pdf_generator/Makefile — cross-compiles and packages this
# tool's backend for macOS (Apple Silicon), Linux, and Windows, each a
# complete, independently-installable .zip dropped into this tool's own
# tools/pdf_generator/versions/ — not the shared root /versions/ (which
# stays cinqo's own binary's and every other tool's generic-package
# drop point). See
# plan/ai/tools/pdf-generator/step-06-multi-os-packaging.md.
#
# pdf_generator's own backend has zero cgo dependencies (chromedp,
# google/uuid, the MCP SDK are all pure Go) — plain CGO_ENABLED=0
# cross-compilation is sufficient; no C cross-toolchain needed here,
# unlike cinqo's own cgo-dependent (mattn/go-sqlite3) build. A future
# cgo-dependent tool would need a heavier Makefile than this one.
#
# The Windows binary is deliberately named "tool", not "tool.exe" —
# matches the host's fixed backendExecutableFilename constant
# (api/src/tool/handler/paths.go) so this zip stays installable through
# the unmodified install pipeline. Relies on Windows' CreateProcess
# executing a full-path PE binary regardless of extension — a real,
# stated assumption, not verified on an actual Windows host from here.
#
# Linux ships both amd64 and arm64 — checked coco-iam's own real
# plugins/coco-observe/Makefile directly (a genuine sibling-project
# reference for this exact "cross-compile a Go binary per OS/arch"
# problem): it builds linux/amd64 AND linux/arm64 together as its own
# real convention (CGO_ENABLED=0, same as here), and has no Windows
# target at all. Its own distribution mechanism is otherwise not a
# direct precedent — it go:embeds the cross-compiled binary straight
# into the coco-iam server binary at build time, a fundamentally
# different model from cinqo's own tool system (an independently
# zipped, separately-installed package — verified repeatedly
# elsewhere in this plan tree) — so only the CGO_ENABLED=0 /
# per-arch-naming convention carries over here, not the embed
# mechanism itself. Windows is kept despite the reference having none,
# since it was explicitly asked for here.

BUILD_DIR := build

.PHONY: build-darwin
build-darwin:
	@mkdir -p $(BUILD_DIR)/darwin-arm64/backend
	cp manifest.json $(BUILD_DIR)/darwin-arm64/
	cd backend && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ../$(BUILD_DIR)/darwin-arm64/backend/tool .
	@echo "built: $(BUILD_DIR)/darwin-arm64/backend/tool"

.PHONY: build-linux
build-linux:
	@mkdir -p $(BUILD_DIR)/linux-amd64/backend $(BUILD_DIR)/linux-arm64/backend
	cp manifest.json $(BUILD_DIR)/linux-amd64/
	cp manifest.json $(BUILD_DIR)/linux-arm64/
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../$(BUILD_DIR)/linux-amd64/backend/tool .
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../$(BUILD_DIR)/linux-arm64/backend/tool .
	@echo "built: $(BUILD_DIR)/linux-amd64/backend/tool"
	@echo "built: $(BUILD_DIR)/linux-arm64/backend/tool"

.PHONY: build-windows
build-windows:
	@mkdir -p $(BUILD_DIR)/windows-amd64/backend
	cp manifest.json $(BUILD_DIR)/windows-amd64/
	cd backend && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ../$(BUILD_DIR)/windows-amd64/backend/tool .
	@echo "built: $(BUILD_DIR)/windows-amd64/backend/tool"

.PHONY: build-all
build-all: build-darwin build-linux build-windows


# VERSIONS_DIR is this tool's own output folder — deliberately not the
# shared root versions/ (which the root Makefile's build/build-linux/
# generic package-tool still use for cinqo's own binary and every other
# tool's single-OS package): a tool with its own multi-OS Makefile
# keeps its own finished .zips alongside itself, under
# tools/pdf_generator/versions/.
VERSIONS_DIR := versions

.PHONY: zip
zip: build-all
	@mkdir -p $(VERSIONS_DIR)
	cd $(BUILD_DIR)/darwin-arm64  && zip -r ../../$(VERSIONS_DIR)/tool-pdf_generator-darwin-arm64.zip  . >/dev/null
	cd $(BUILD_DIR)/linux-amd64   && zip -r ../../$(VERSIONS_DIR)/tool-pdf_generator-linux-amd64.zip   . >/dev/null
	cd $(BUILD_DIR)/linux-arm64   && zip -r ../../$(VERSIONS_DIR)/tool-pdf_generator-linux-arm64.zip   . >/dev/null
	cd $(BUILD_DIR)/windows-amd64 && zip -r ../../$(VERSIONS_DIR)/tool-pdf_generator-windows-amd64.zip . >/dev/null
	@echo "packaged:"
	@echo "  tools/pdf_generator/$(VERSIONS_DIR)/tool-pdf_generator-darwin-arm64.zip"
	@echo "  tools/pdf_generator/$(VERSIONS_DIR)/tool-pdf_generator-linux-amd64.zip"
	@echo "  tools/pdf_generator/$(VERSIONS_DIR)/tool-pdf_generator-linux-arm64.zip"
	@echo "  tools/pdf_generator/$(VERSIONS_DIR)/tool-pdf_generator-windows-amd64.zip"

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
