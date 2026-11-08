package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/toolrepository/updatecheck"
)

// DIKeyUpdateChecker is where cinqo.Start registers the *updatecheck.Checker.
const DIKeyUpdateChecker = "tool_update_checker"

type updatesResponse struct {
	Check         updatecheck.Check    `json:"check"`
	LastCheckedAt string               `json:"lastCheckedAt"`
	Updates       []updatecheck.Update `json:"updates"`
}

func checkerFrom(reqCtx request.RequestContext) (*updatecheck.Checker, bool) {
	st, ok := reqCtx.GetDI().(diStore)
	if !ok {
		return nil, false
	}
	raw, ok := st.Get(DIKeyUpdateChecker)
	if !ok {
		return nil, false
	}
	c, ok := raw.(*updatecheck.Checker)
	return c, ok && c != nil
}

// respondWithUpdates answers with the run state and the updates, which are always
// computed from the stored catalogs (no network): correct mid-run and after a restart.
func respondWithUpdates(reqCtx request.RequestContext, status int, check updatecheck.Check) {
	w := reqCtx.GetWriter()
	env, err := environment(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read installed tools")
		return
	}
	repos, err := svc(reqCtx).List()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list repositories")
		return
	}
	response.SuccessResponse(w, status, updatesResponse{
		Check:         check,
		LastCheckedAt: updatecheck.LastCheckedAt(repos),
		Updates:       updatecheck.Updates(repos, env),
	})
}

// UpdatesHandler handles GET /api/v1/tool-updates: the latest check's state and
// the installed tools that have an update.
func UpdatesHandler(reqCtx request.RequestContext) {
	checker, ok := checkerFrom(reqCtx)
	if !ok {
		response.ErrorResponse(reqCtx.GetWriter(), http.StatusServiceUnavailable, "the update check is not available")
		return
	}
	respondWithUpdates(reqCtx, http.StatusOK, checker.Snapshot())
}

// CheckUpdatesHandler handles POST /api/v1/tool-updates/check: starts a check in
// the background (202), or answers 200 with the running or just-finished one.
// "Already running" is not an error.
func CheckUpdatesHandler(reqCtx request.RequestContext) {
	checker, ok := checkerFrom(reqCtx)
	if !ok {
		response.ErrorResponse(reqCtx.GetWriter(), http.StatusServiceUnavailable, "the update check is not available")
		return
	}
	check, started := checker.Start(updatecheck.TriggerManual)
	status := http.StatusOK
	if started {
		status = http.StatusAccepted
	}
	respondWithUpdates(reqCtx, status, check)
}
