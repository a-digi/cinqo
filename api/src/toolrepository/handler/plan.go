package handler

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"runtime"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
	"github.com/a-digi/cinqo/src/toolmeta"
	"github.com/a-digi/cinqo/src/toolrepository/resolver"
)

type installPlanRequest struct {
	VersionID string `json:"versionId"`
}

type planStepResponse struct {
	ToolID           string   `json:"toolId"`
	Name             string   `json:"name"`
	Action           string   `json:"action"`
	VersionID        string   `json:"versionId"`
	Version          string   `json:"version"`
	InstalledVersion string   `json:"installedVersion"`
	RepositoryID     string   `json:"repositoryId"`
	RepositoryName   string   `json:"repositoryName"`
	SizeBytes        int64    `json:"sizeBytes"`
	Authors          []string `json:"authors"`
	License          string   `json:"license"`
	RequiredBy       []string `json:"requiredBy"`
	Marks            []string `json:"marks"`
	Dependents       []string `json:"dependents"`
	EnableAfter      bool     `json:"enableAfter"`
}

type planProblemResponse struct {
	ToolID     string   `json:"toolId"`
	RequiredBy []string `json:"requiredBy"`
	Kind       string   `json:"kind"`
	Message    string   `json:"message"`
}

type planSearchedResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Stale       bool   `json:"stale"`
	RefreshedAt string `json:"refreshedAt"`
}

// planResponse is what the preview answers. It carries what the confirmation shows and nothing a client could use to install without asking: no
// download address and no checksum.
type planResponse struct {
	OK         bool                   `json:"ok"`
	Steps      []planStepResponse     `json:"steps"`
	TotalBytes int64                  `json:"totalBytes"`
	Problems   []planProblemResponse  `json:"problems"`
	Searched   []planSearchedResponse `json:"searched"`
	// Confirm lists the steps as the install route must be given them back ("action:tool@version:repository"): the install does only what was confirmed.
	Confirm []string `json:"confirm"`
}

// InstallPlanHandler handles POST /api/v1/tool-repositories/{id}/tools/{toolId}/install-plan: the read-only preview of installing or updating a tool
// together with the tools it needs, in the order they would be done (plan plan/ai/tools/repository/dependency-install.md, step 4). It installs and
// downloads nothing and contacts no repository: it reads the stored repository files and the installed tools. Same scopes as the install.
func InstallPlanHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body installPlanRequest
	if err := reqCtx.BindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	installed, err := installedForPlan(db)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read installed tools")
		return
	}
	statusEnv, err := environment(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read installed tools")
		return
	}
	env := resolver.Env{OS: runtime.GOOS, Arch: runtime.GOARCH, AppVersion: statusEnv.AppVersion}
	plan, err := svc(reqCtx).PlanDependencies(reqCtx.GetURI().GetPathVariable("id"), reqCtx.GetURI().GetPathVariable("toolId"), body.VersionID, installed, env)
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, toPlanResponse(plan))
}

// installedForPlan describes every installed tool the way the resolver needs it: its version, whether it is enabled and running, where it came from,
// who made it, and what it declared it needs.
func installedForPlan(db *sql.DB) ([]resolver.InstalledTool, error) {
	tools, err := tool_query.NewToolQueryRepo(db).List()
	if err != nil {
		return nil, err
	}
	rows, err := tool_query.NewToolRequiredToolQueryRepo(db).FindAll()
	if err != nil {
		return nil, err
	}
	requires := map[string][]resolver.Requirement{}
	for _, r := range rows {
		requires[r.ToolID] = append(requires[r.ToolID], resolver.Requirement{ID: r.RequiredSlug, Min: r.MinVersion, Max: r.MaxVersion})
	}
	out := make([]resolver.InstalledTool, 0, len(tools))
	for _, t := range tools {
		it := resolver.InstalledTool{
			ID: t.Slug, Name: t.Name, Version: t.Version, Enabled: t.Enabled, Running: t.Status == "running",
			SourceRepositoryID: t.SourceRepositoryID, Requires: requires[t.ID],
		}
		if meta := toolmeta.Decode(t.Metadata); meta != nil {
			for _, a := range meta.Authors {
				it.Authors = append(it.Authors, a.Name)
			}
		}
		out = append(out, it)
	}
	return out, nil
}

func toPlanResponse(p *resolver.Plan) planResponse {
	out := planResponse{OK: p.OK, Steps: []planStepResponse{}, Problems: []planProblemResponse{}, Searched: []planSearchedResponse{}, Confirm: confirmList(p)}
	for _, s := range p.Steps {
		marks := make([]string, 0, len(s.Marks))
		for _, m := range s.Marks {
			marks = append(marks, string(m))
		}
		out.Steps = append(out.Steps, planStepResponse{
			ToolID: s.ToolID, Name: s.Name, Action: string(s.Action), VersionID: s.VersionID, Version: s.Version, InstalledVersion: s.InstalledVersion,
			RepositoryID: s.RepositoryID, RepositoryName: s.RepositoryName, SizeBytes: s.SizeBytes, Authors: nonNil(s.Authors), License: s.License,
			RequiredBy: nonNil(s.RequiredBy), Marks: marks, Dependents: nonNil(s.Dependents), EnableAfter: s.EnableAfter,
		})
		out.TotalBytes += s.SizeBytes
	}
	for _, pr := range p.Problems {
		out.Problems = append(out.Problems, planProblemResponse{ToolID: pr.ToolID, RequiredBy: nonNil(pr.RequiredBy), Kind: string(pr.Kind), Message: pr.Message})
	}
	for _, s := range p.Searched {
		out.Searched = append(out.Searched, planSearchedResponse{ID: s.ID, Name: s.Name, Stale: s.Stale, RefreshedAt: s.RefreshedAt})
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
