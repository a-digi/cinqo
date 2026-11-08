package handler

import (
	"net/http"
	"runtime"
	"slices"
	"sync"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_handler "github.com/a-digi/cinqo/src/tool/handler"
	"github.com/a-digi/cinqo/src/toolrepository/resolver"
	"github.com/a-digi/cinqo/src/toolrepository/service"
)

// planLock lets one install-with-dependencies run at a time: two runs would each plan against tools the other is about to change.
var planLock sync.Mutex

// confirmKey names one step the way the person confirmed it: what is done, to which tool and version, from which repository.
func confirmKey(s resolver.Step) string {
	return string(s.Action) + ":" + s.ToolID + "@" + s.Version + ":" + s.RepositoryID
}

func confirmList(p *resolver.Plan) []string {
	out := make([]string, 0, len(p.Steps))
	for _, s := range p.Steps {
		out = append(out, confirmKey(s))
	}
	return out
}

type doneStep struct {
	ToolID  string `json:"toolId"`
	Action  string `json:"action"`
	Version string `json:"version"`
}

// runResult is the answer of an install with dependencies: what was done, what was not, and where it stopped. Nothing done is undone when a later step
// fails: an installed dependency stays installed, and the answer says exactly which steps are left.
type runResult struct {
	OK        bool          `json:"ok"`
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Installed []doneStep    `json:"installed"`
	NotDone   []doneStep    `json:"notDone"`
	Plan      *planResponse `json:"plan,omitempty"`
}

func stepOf(s resolver.Step) doneStep {
	return doneStep{ToolID: s.ToolID, Action: string(s.Action), Version: s.Version}
}

// installWithDependencies is the install route with withDependencies: it makes the plan, downloads and checks every package (nothing is installed until all
// of them are good), compares the plan with what the person confirmed, and only then does the steps in order.
func installWithDependencies(reqCtx request.RequestContext, body installRequest) {
	w := reqCtx.GetWriter()
	if !planLock.TryLock() {
		response.ErrorResponse(w, http.StatusConflict, "another installation is running: try again when it is done")
		return
	}
	defer planLock.Unlock()

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
	repoID := reqCtx.GetURI().GetPathVariable("id")
	s := svc(reqCtx)
	verified, err := s.VerifyPlan(reqCtx.GetRequest().Context(), repoID, reqCtx.GetURI().GetPathVariable("toolId"), body.VersionID, installed, env)
	if err != nil {
		fail(w, err)
		return
	}
	pr := toPlanResponse(&verified.Plan)
	if !verified.Plan.OK {
		response.SuccessResponse(w, http.StatusConflict, runResult{Code: "plan_blocked", Message: "this cannot be installed: see the problems", Installed: []doneStep{}, NotDone: []doneStep{}, Plan: &pr})
		return
	}
	if verified.Changed || !slices.Equal(body.Confirm, pr.Confirm) {
		response.SuccessResponse(w, http.StatusConflict, runResult{Code: "plan_changed", Message: "the plan is not the one that was confirmed: check it again", Installed: []doneStep{}, NotDone: []doneStep{}, Plan: &pr})
		return
	}

	res := runResult{Installed: []doneStep{}, NotDone: []doneStep{}}
	steps := verified.Plan.Steps
	for i, st := range steps {
		ierr := doStep(reqCtx, repoID, verified, st)
		if ierr != nil {
			res.Code = "step_failed"
			res.Message = st.ToolID + ": " + ierr.Message
			for _, rest := range steps[i:] {
				res.NotDone = append(res.NotDone, stepOf(rest))
			}
			response.SuccessResponse(w, http.StatusUnprocessableEntity, res)
			return
		}
		res.Installed = append(res.Installed, stepOf(st))
	}
	res.OK, res.Code = true, "done"
	response.SuccessResponse(w, http.StatusOK, res)
}

// doStep does one step of a verified plan: an install or update runs the package already downloaded and checked through the same pipeline as every other
// install, an enable step enables the installed tool; an update that keeps a disabled tool disabled enables it afterwards when the plan says so.
func doStep(reqCtx request.RequestContext, repoID string, v *service.Verified, st resolver.Step) *tool_handler.InstallError {
	if st.Action == resolver.ActionEnable {
		return tool_handler.EnableTool(reqCtx, st.ToolID)
	}
	vp := v.Packages[st.ToolID]
	if vp == nil {
		return &tool_handler.InstallError{Status: http.StatusInternalServerError, Message: "the package was not read"}
	}
	from := vp.RepositoryID
	if from == "" {
		from = repoID
	}
	if _, ierr := tool_handler.InstallPackage(reqCtx, vp.Data, &tool_handler.PackageExpectation{Slug: vp.Install.ToolID, Version: vp.Install.Version, Info: vp.Install.Info, SourceRepositoryID: from}); ierr != nil {
		return ierr
	}
	if st.EnableAfter {
		return tool_handler.EnableTool(reqCtx, st.ToolID)
	}
	return nil
}
