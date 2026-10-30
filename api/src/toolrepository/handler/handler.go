// Package handler serves /api/v1/tool-repositories — see
// plan/ai/tools/repository/tool-repositories.md. Every route needs tool:manage
// (or cinqo:super:admin), including the reads: the list reveals which links an
// administrator trusts.
package handler

import (
	"errors"
	"io"
	"net/http"
	"runtime"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	"github.com/a-digi/cinqo/src/toolrepository/service"
	"github.com/a-digi/cinqo/src/toolrepository/status"

	tool_handler "github.com/a-digi/cinqo/src/tool/handler"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// diStore is the part of the DI container that holds boot-time values such as
// the app version.
type diStore interface {
	Get(key string) (any, bool)
}

// environment gathers what the status depends on: the installed tools, this
// machine and the running app's version (empty when unknown, which skips the
// minAppVersion check).
func environment(reqCtx request.RequestContext) (status.Env, error) {
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	tools, err := tool_query.NewToolQueryRepo(db).List()
	if err != nil {
		return status.Env{}, err
	}
	env := status.Env{OS: runtime.GOOS, Arch: runtime.GOARCH, Installed: make(map[string]string, len(tools))}
	for _, t := range tools {
		env.Installed[t.Slug] = t.Version
	}
	if st, ok := reqCtx.GetDI().(diStore); ok {
		if raw, ok := st.Get("app_version"); ok {
			if v, ok := raw.(string); ok {
				env.AppVersion = v
			}
		}
	}
	return env, nil
}

// downloadTimeout bounds a whole package download (up to 200 MB).
const downloadTimeout = 10 * time.Minute

func svc(reqCtx request.RequestContext) *service.Service {
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	return service.New(db, fetch.New()).WithDownloader(&fetch.Fetcher{Timeout: downloadTimeout})
}

func fail(w http.ResponseWriter, err error) {
	var se *service.Error
	if !errors.As(err, &se) {
		response.ErrorResponse(w, http.StatusInternalServerError, "request failed")
		return
	}
	status := http.StatusBadRequest
	switch {
	case errors.Is(se.Kind, service.ErrNotFound), errors.Is(se.Kind, service.ErrToolNotFound), errors.Is(se.Kind, service.ErrVersionMissing):
		status = http.StatusNotFound
	case errors.Is(se.Kind, service.ErrDuplicate):
		status = http.StatusConflict
	case errors.Is(se.Kind, service.ErrNativeRemoval):
		status = http.StatusForbidden
	case errors.Is(se.Kind, service.ErrDownload):
		status = http.StatusBadGateway
	}
	response.ErrorResponse(w, status, se.Message)
}

// ListHandler handles GET /api/v1/tool-repositories.
func ListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	env, err := environment(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read installed tools")
		return
	}
	repos, err := svc(reqCtx).Summaries(env)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list repositories")
		return
	}
	response.SuccessResponse(w, http.StatusOK, repos)
}

type addRequest struct {
	URL string `json:"url"`
}

// AddHandler handles POST /api/v1/tool-repositories: fetches and validates the
// file before anything is stored.
func AddHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body addRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repo, err := svc(reqCtx).Add(reqCtx.GetRequest().Context(), body.URL)
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusCreated, repo)
}

// DeleteHandler handles DELETE /api/v1/tool-repositories/{id}. Installed tools
// are never uninstalled.
func DeleteHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	if err := svc(reqCtx).Remove(reqCtx.GetURI().GetPathVariable("id")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RefreshHandler handles POST /api/v1/tool-repositories/{id}/refresh. A failed
// refresh answers 200 with the repository, which then shows lastStatus "error".
func RefreshHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	repo, err := svc(reqCtx).Refresh(reqCtx.GetRequest().Context(), reqCtx.GetURI().GetPathVariable("id"))
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, repo)
}

// GetHandler handles GET /api/v1/tool-repositories/{id}: the repository with
// each tool's status against what is installed.
func GetHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	env, err := environment(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read installed tools")
		return
	}
	detail, err := svc(reqCtx).Detail(reqCtx.GetURI().GetPathVariable("id"), env)
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, detail)
}

type installRequest struct {
	VersionID string `json:"versionId"`
}

// InstallHandler handles POST /api/v1/tool-repositories/{id}/tools/{toolId}/install:
// downloads the chosen version (default: the current one), verifies its
// checksum, and runs it through the same install pipeline as an upload, which
// also checks that the package's manifest is the tool and version the
// repository listed. Nothing installs or updates without this explicit call.
func InstallHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body installRequest
	if err := reqCtx.BindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	env, err := environment(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read installed tools")
		return
	}
	s := svc(reqCtx)
	plan, err := s.PlanInstall(reqCtx.GetURI().GetPathVariable("id"), reqCtx.GetURI().GetPathVariable("toolId"), body.VersionID, env)
	if err != nil {
		fail(w, err)
		return
	}
	data, err := s.Download(reqCtx.GetRequest().Context(), plan)
	if err != nil {
		fail(w, err)
		return
	}
	result, installErr := tool_handler.InstallPackage(reqCtx, data, &tool_handler.PackageExpectation{Slug: plan.ToolID, Version: plan.Version})
	if installErr != nil {
		response.ErrorResponse(w, installErr.Status, installErr.Message)
		return
	}
	code := http.StatusOK
	if result.Created {
		code = http.StatusCreated
	}
	response.SuccessResponse(w, code, result.Tool)
}
