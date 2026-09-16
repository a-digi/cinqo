package handler

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	"github.com/a-digi/cinqo/src/tool/manager"
	"github.com/a-digi/cinqo/src/tool/manifest"
	tool_mcp "github.com/a-digi/cinqo/src/tool/mcp"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
	"github.com/a-digi/cinqo/src/tool/sandbox"
)

// discoverMCPToolsIfDeclared is a best-effort, non-fatal hook run after
// a tool's install/update/enable already succeeded: manifest.MCP is
// only an optimization gate (see manifest.go's own doc comment), so a
// failure here never fails the surrounding request — it just leaves
// tool_mcp_tools empty for this tool, same as any tool that never
// declared MCP support at all. Runs unconditionally regardless of
// whether the tool's own HTTP surface is currently enabled — MCP
// discovery spawns its own separate --mcp-mode process. See
// plan/ai/tools/pdf-generator/step-04-ai-model-invocation.md.
func discoverMCPToolsIfDeclared(reqCtx request.RequestContext, db *sql.DB, m manifest.Manifest, installDir, toolID string) {
	if !m.MCP {
		return
	}
	execPath, err := filepath.Abs(filepath.Join(installDir, backendExecutableFilename))
	if err != nil {
		reqCtx.GetDI().GetLogger().Warning("tool %q declared mcp support but its executable path could not be resolved: %v", m.Slug, err)
		return
	}
	// A real, reproduced failure without this: the spawned --mcp
	// process inherits a bare environment with none of
	// TOOL_DB_DIR/TOOL_UPLOADS_DIR/TOOL_TMP_DIR set, so its own
	// main() fails os.MkdirAll("", ...) at startup and exits before
	// ever completing the MCP handshake — surfaced only as "connection
	// closed: EOF" from Discover, not an obviously env-related error.
	port, portErr := corePort(reqCtx)
	if portErr != nil {
		reqCtx.GetDI().GetLogger().Warning("tool %q declared mcp support but its backend port could not be determined: %v", m.Slug, portErr)
		return
	}
	envVars, err := manager.ToolEnvVars(resolvedDataDir(reqCtx), m.Slug, port)
	if err != nil {
		reqCtx.GetDI().GetLogger().Warning("tool %q declared mcp support but its env vars could not be resolved: %v", m.Slug, err)
		return
	}
	// TOOL_OWN_PORT lets a tool's own --mcp subprocess reach its
	// already-running HTTP-mode sibling directly (127.0.0.1-only,
	// same host) — the mechanism a tool needing state to persist
	// across separate --mcp invocations (e.g. browser's own shared
	// session) relies on. Harmless, unused env var for every tool that
	// doesn't need it, same as CORE_API_URL already is for a tool that
	// never calls back into cinqo's own API. See
	// plan/ai/tools/browser/step-02-shared-browser-session.md.
	if port, ok := manager.Port(toolID); ok {
		envVars = append(envVars, fmt.Sprintf("TOOL_OWN_PORT=%d", port))
	}
	mcpTools, err := tool_mcp.Discover(reqCtx.GetRequest().Context(), execPath, envVars)
	if err != nil {
		reqCtx.GetDI().GetLogger().Warning("tool %q declared mcp support but discovery failed: %v", m.Slug, err)
		return
	}
	// MCP's own tools/list has no concept of a cinqo scope at all — the
	// manifest's own mcp_tools mapping is the only source of truth for
	// which scope gates each one. A discovered name with no matching
	// manifest entry is dropped, not cached with an empty scope (which
	// would make it callable by anyone) — logged so a manifest that
	// forgot to declare a real tool is a visible, not silent, gap.
	scopeByName := make(map[string]string, len(m.MCPTools))
	for _, mt := range m.MCPTools {
		scopeByName[mt.Name] = mt.RequiredScope
	}
	filtered := make([]tool_entity.ToolMCPTool, 0, len(mcpTools))
	for _, mt := range mcpTools {
		scope, declared := scopeByName[mt.Name]
		if !declared {
			reqCtx.GetDI().GetLogger().Warning("tool %q's mcp server declared a tool %q with no matching mcp_tools entry in its manifest — not cached", m.Slug, mt.Name)
			continue
		}
		mt.ToolID = toolID
		mt.RequiredScope = scope
		filtered = append(filtered, mt)
	}

	if err := tool_persistent.NewToolMCPToolPersistentRepo(db).ReplaceAll(toolID, filtered); err != nil {
		reqCtx.GetDI().GetLogger().Warning("tool %q mcp discovery succeeded but caching its tools failed: %v", m.Slug, err)
	}
}

// maxUploadedPackageBytes caps the raw upload before the multipart form
// is even parsed — rejects an oversized request before it's fully
// buffered in memory. Matches coco-mda's own cap.
const maxUploadedPackageBytes = 200 * 1024 * 1024

// InstallHandler handles POST /api/v1/tools/install — a multipart
// upload, single field "package". See
// plan/ai/tools/step-03-install-update-uninstall.md.
func InstallHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadedPackageBytes)
	if err := r.ParseMultipartForm(maxUploadedPackageBytes); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid or oversized upload: "+err.Error())
		return
	}

	file, _, err := r.FormFile("package")
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, `a "package" file is required`)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "failed to read the uploaded package")
		return
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "not a valid zip file")
		return
	}

	if err := os.MkdirAll(toolsRoot(reqCtx), 0o755); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to prepare the tools directory")
		return
	}

	stagingDir := filepath.Join(toolsRoot(reqCtx), ".staging", uuid.NewString())
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to prepare a staging directory")
		return
	}
	// Unconditional cleanup — nothing staged ever leaks on a rejected
	// upload, regardless of where the pipeline below fails.
	defer os.RemoveAll(stagingDir)

	if err := sandbox.ExtractZip(zr, stagingDir); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "package failed validation: "+err.Error())
		return
	}

	manifestBytes, err := os.ReadFile(filepath.Join(stagingDir, "manifest.json"))
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "package is missing manifest.json")
		return
	}
	m, err := manifest.Parse(manifestBytes)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	queryRepo := tool_query.NewToolQueryRepo(db)
	persistentRepo := tool_persistent.NewToolPersistentRepo(db)

	existing, findErr := queryRepo.FindBySlug(m.Slug)
	found := findErr == nil
	if findErr != nil && !errors.Is(findErr, sql.ErrNoRows) {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to check for an existing tool")
		return
	}

	excludeID := ""
	if found {
		excludeID = existing.ID
	}
	existingScopes, err := queryRepo.ExistingScopesExcludingTool(excludeID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to check for scope conflicts")
		return
	}

	installedTools, err := installedToolsMap(queryRepo)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to check installed tool dependencies")
		return
	}

	_, frontendErr := os.Stat(filepath.Join(stagingDir, frontendBundleFilename))
	_, backendErr := os.Stat(filepath.Join(stagingDir, backendExecutableFilename))

	currentAppVersion, err := appVersion(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to determine the running app version")
		return
	}

	kind, err := manifest.Validate(m, manifest.ValidationInput{
		HasFrontendBundle:    frontendErr == nil,
		HasBackendExecutable: backendErr == nil,
		ExistingScopes:       existingScopes,
		CurrentAppVersion:    currentAppVersion,
		InstalledTools:       installedTools,
	})
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	scopes, routes, required, requiredTools := childRowsFromManifest(m)
	finalDir := filepath.Join(toolsRoot(reqCtx), m.Slug)

	if !found {
		tool := &tool_entity.Tool{
			ID:      uuid.NewString(),
			Slug:    m.Slug,
			Name:    m.Name,
			Version: m.Version,
			Kind:    kind,
			Enabled: true, // auto-activates, matching the reference
			Status:  "installed",
		}
		if backendErr == nil {
			tool.BackendExecutableRelpath = backendExecutableFilename
		}
		if frontendErr == nil {
			tool.FrontendBundleRelpath = frontendBundleFilename
		}
		tool.MinAppVersion = m.MinAppVersion
		tool.MaxAppVersion = m.MaxAppVersion

		if err := os.Rename(stagingDir, finalDir); err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to install package: "+err.Error())
			return
		}
		tool.InstallPath = finalDir

		if err := chmodBackendExecutableIfPresent(finalDir); err != nil {
			os.RemoveAll(finalDir)
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to finalize install: "+err.Error())
			return
		}

		if err := persistentRepo.InsertFresh(tool, scopes, routes, required); err != nil {
			// DB insert failed after the directory was already moved —
			// undo the move so a retry isn't blocked by an orphaned
			// directory with no matching row.
			os.RemoveAll(finalDir)
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to record installed tool: "+err.Error())
			return
		}
		if err := tool_persistent.NewToolRequiredToolPersistentRepo(db).ReplaceAll(tool.ID, requiredTools); err != nil {
			os.RemoveAll(finalDir)
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to record tool dependencies: "+err.Error())
			return
		}

		// Best-effort start, matching the reference: a start failure
		// leaves status "error" (recorded by manager.Start itself),
		// doesn't fail the 201 — the tool is still successfully
		// installed, just not currently running.
		if port, portErr := corePort(reqCtx); portErr == nil {
			if startErr := manager.Start(db, resolvedDataDir(reqCtx), *tool, port); startErr != nil {
				reqCtx.GetDI().GetLogger().Warning("tool %q installed but failed to start: %v", tool.Slug, startErr)
			}
		} else {
			reqCtx.GetDI().GetLogger().Warning("tool %q installed but could not determine backend port to start it: %v", tool.Slug, portErr)
		}

		discoverMCPToolsIfDeclared(reqCtx, db, m, finalDir, tool.ID)

		created, err := queryRepo.FindByID(tool.ID)
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "tool installed but failed to reload")
			return
		}
		response.SuccessResponse(w, http.StatusCreated, toToolResponse(created, systemToolsConfigFrom(reqCtx)))
		return
	}

	// Update path: same slug, must be a strictly newer version.
	cmp, err := manifest.CompareVersions(m.Version, existing.Version)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if cmp <= 0 {
		response.ErrorResponse(w, http.StatusBadRequest,
			"version must be greater than the currently installed version ("+existing.Version+")")
		return
	}

	// Stop the tool's currently running backend before swapping its
	// code directory — can't safely replace a binary a process may
	// still hold open, and a clean stop/restart is simpler to reason
	// about than a live swap.
	if err := manager.Stop(db, existing.ID); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to stop the running tool before updating: "+err.Error())
		return
	}

	if err := replaceInstallDir(stagingDir, finalDir); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to update package: "+err.Error())
		return
	}

	if err := chmodBackendExecutableIfPresent(finalDir); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to finalize update: "+err.Error())
		return
	}

	updated := &tool_entity.Tool{
		ID:          existing.ID,
		Slug:        existing.Slug,
		Name:        m.Name,
		Version:     m.Version,
		Kind:        kind,
		Enabled:     existing.Enabled, // preserved — an update doesn't silently turn a disabled tool on
		Status:      "installed",
		InstallPath: finalDir,
	}
	if backendErr == nil {
		updated.BackendExecutableRelpath = backendExecutableFilename
	}
	if frontendErr == nil {
		updated.FrontendBundleRelpath = frontendBundleFilename
	}
	updated.MinAppVersion = m.MinAppVersion
	updated.MaxAppVersion = m.MaxAppVersion

	if err := persistentRepo.UpdateVersion(updated, scopes, routes, required); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to record updated tool: "+err.Error())
		return
	}
	if err := tool_persistent.NewToolRequiredToolPersistentRepo(db).ReplaceAll(updated.ID, requiredTools); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to record tool dependencies: "+err.Error())
		return
	}

	// Restart on the new code only if it was enabled before — an
	// update never silently turns a disabled tool on.
	if updated.Enabled {
		if port, portErr := corePort(reqCtx); portErr == nil {
			if startErr := manager.Start(db, resolvedDataDir(reqCtx), *updated, port); startErr != nil {
				reqCtx.GetDI().GetLogger().Warning("tool %q updated but failed to restart: %v", updated.Slug, startErr)
			}
		} else {
			reqCtx.GetDI().GetLogger().Warning("tool %q updated but could not determine backend port to restart it: %v", updated.Slug, portErr)
		}
	}

	// Unconditional — not gated on updated.Enabled, unlike the restart
	// above: MCP discovery spawns its own independent --mcp-mode
	// process regardless of the HTTP surface's enabled state.
	discoverMCPToolsIfDeclared(reqCtx, db, m, finalDir, updated.ID)

	reloaded, err := queryRepo.FindByID(updated.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "tool updated but failed to reload")
		return
	}
	response.SuccessResponse(w, http.StatusOK, toToolResponse(reloaded, systemToolsConfigFrom(reqCtx)))
}

func childRowsFromManifest(m manifest.Manifest) ([]tool_entity.ToolScope, []tool_entity.ToolRoute, []tool_entity.ToolRequiredScope, []tool_entity.ToolRequiredTool) {
	scopes := make([]tool_entity.ToolScope, 0, len(m.Scopes))
	for _, s := range m.Scopes {
		scopes = append(scopes, tool_entity.ToolScope{Scope: s.Scope, Description: s.Description})
	}
	routes := make([]tool_entity.ToolRoute, 0, len(m.Routes))
	for _, rt := range m.Routes {
		routes = append(routes, tool_entity.ToolRoute{Method: rt.Method, PathSuffix: rt.PathSuffix, RequiredScope: rt.RequiredScope})
	}
	required := make([]tool_entity.ToolRequiredScope, 0, len(m.RequiredScopes))
	for _, s := range m.RequiredScopes {
		required = append(required, tool_entity.ToolRequiredScope{Scope: s})
	}
	requiredTools := make([]tool_entity.ToolRequiredTool, 0, len(m.RequiresTools))
	for _, rt := range m.RequiresTools {
		requiredTools = append(requiredTools, tool_entity.ToolRequiredTool{RequiredSlug: rt.Slug, MinVersion: rt.MinVersion})
	}
	return scopes, routes, required, requiredTools
}

// installedToolsMap builds manifest.ValidationInput's own
// InstalledTools lookup from every currently-installed tool — the
// same "convert tool_entity rows into the DB-free shape manifest.go
// itself needs" pattern ExistingScopesExcludingTool's own caller-side
// map-building already established for scopes. See
// plan/ai/tools/step-14-tool-dependencies.md.
func installedToolsMap(queryRepo *tool_query.ToolQueryRepo) (map[string]manifest.InstalledToolInfo, error) {
	all, err := queryRepo.List()
	if err != nil {
		return nil, err
	}
	out := make(map[string]manifest.InstalledToolInfo, len(all))
	for _, t := range all {
		out[t.Slug] = manifest.InstalledToolInfo{Version: t.Version, Enabled: t.Enabled}
	}
	return out, nil
}

// replaceInstallDir swaps finalDir for stagingDir's contents, keeping a
// ".prev" backup during the swap so a mid-failure doesn't leave the
// tool with no code directory at all — matches the reference's own
// update-path safety exactly. If the second rename fails, it
// best-effort restores ".prev" back to finalDir.
func replaceInstallDir(stagingDir, finalDir string) error {
	prevDir := finalDir + ".prev"
	_ = os.RemoveAll(prevDir) // clear any leftover from a previously failed update

	if err := os.Rename(finalDir, prevDir); err != nil {
		return err
	}

	if err := os.Rename(stagingDir, finalDir); err != nil {
		_ = os.Rename(prevDir, finalDir) // best-effort restore
		return err
	}

	_ = os.RemoveAll(prevDir)
	return nil
}
