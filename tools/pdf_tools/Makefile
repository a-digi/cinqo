# tools/pdf_tools/Makefile — cross-compiles and packages this tool's
# backend for macOS (Apple Silicon), Linux, and Windows, each a
# complete, independently-installable .zip dropped into this tool's own
# tools/pdf_tools/versions/ — not the shared root /versions/ (which
# stays cinqo's own binary's and every other tool's generic-package
# drop point). Mirrors tools/pdf_generator/Makefile almost verbatim —
# see plan/ai/tools/pdf-tools/step-04-packaging-and-deploy.md.
#
# pdf_tools's own backend has zero cgo dependencies (github.com/ledongthuc/pdf,
# the MCP SDK are both pure Go — the design deliberately avoided any
# dependency on the pdfcpu CLI binary, see
# plan/ai/tools/pdf-tools/step-02-pdf-to-markdown-core.md's own
# Open Question 2) — plain CGO_ENABLED=0 cross-compilation is
# sufficient, no C cross-toolchain needed, and no separate host-level
# binary needs to be installed alongside this one on any target
# platform.
#
# The Windows binary is deliberately named "tool", not "tool.exe" —
# matches the host's fixed backendExecutableFilename constant
# (api/src/tool/handler/paths.go) so this zip stays installable through
# the unmodified install pipeline.

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
# tools/pdf_tools/versions/.
VERSIONS_DIR := versions

.PHONY: zip
zip: build-all
	@mkdir -p $(VERSIONS_DIR)
	cd $(BUILD_DIR)/darwin-arm64  && zip -r ../../$(VERSIONS_DIR)/tool-pdf_tools-darwin-arm64.zip  . >/dev/null
	cd $(BUILD_DIR)/linux-amd64   && zip -r ../../$(VERSIONS_DIR)/tool-pdf_tools-linux-amd64.zip   . >/dev/null
	cd $(BUILD_DIR)/linux-arm64   && zip -r ../../$(VERSIONS_DIR)/tool-pdf_tools-linux-arm64.zip   . >/dev/null
	cd $(BUILD_DIR)/windows-amd64 && zip -r ../../$(VERSIONS_DIR)/tool-pdf_tools-windows-amd64.zip . >/dev/null
	@echo "packaged:"
	@echo "  tools/pdf_tools/$(VERSIONS_DIR)/tool-pdf_tools-darwin-arm64.zip"
	@echo "  tools/pdf_tools/$(VERSIONS_DIR)/tool-pdf_tools-linux-amd64.zip"
	@echo "  tools/pdf_tools/$(VERSIONS_DIR)/tool-pdf_tools-linux-arm64.zip"
	@echo "  tools/pdf_tools/$(VERSIONS_DIR)/tool-pdf_tools-windows-amd64.zip"

.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)
