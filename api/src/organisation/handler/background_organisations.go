package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/organisation"
	"github.com/a-digi/cinqo/src/organisation/repository/query"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// serviceScheme is the Authorization scheme of a tool's service token (the same one the unattended proxy route takes).
const serviceScheme = "ToolService "

// BackgroundOrganisationsHandler handles GET /api/v1/background/organisations: a tool asks which organisations it should work for without a signed-in person
// (plan/ai/tools/namako/saas/s7-unattended-monitoring.md, step 6). It is public in route-organisation.yaml because the caller is a tool: the whole check is
// here, with the tool's own service token.
func BackgroundOrganisationsHandler(reqCtx request.RequestContext) {
	backgroundOrganisations(dbOf(reqCtx), reqCtx)
}

// backgroundOrganisations answers the ids of the active organisations that switched unattended use on, to a tool that is enabled and holds at least one approval.
// To anyone else (a tool without an approval, a disabled tool, or while the kill switch is on) it answers an empty list, never an error that would tell more.
// The caller must still pass every check of the unattended route on each call: this list only says where it is worth trying.
func backgroundOrganisations(db *sql.DB, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	token := strings.TrimSpace(strings.TrimPrefix(reqCtx.GetRequest().Header.Get("Authorization"), serviceScheme))
	if !strings.HasPrefix(reqCtx.GetRequest().Header.Get("Authorization"), serviceScheme) || token == "" {
		response.ErrorResponse(w, http.StatusUnauthorized, "missing or malformed service token")
		return
	}
	tool, err := tool_query.NewToolQueryRepo(db).FindByServiceToken(token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusUnauthorized, "invalid service token")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to authenticate")
		return
	}
	items := []string{}
	if tool.Enabled && !organisation.BackgroundKillSwitchOn() {
		grants, err := tool_query.NewToolBackgroundQueryRepo(db).Grants(tool.ID)
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the approvals")
			return
		}
		if len(grants) > 0 {
			if items, err = query.NewOrganisationQueryRepo(db).BackgroundOrganisations(); err != nil {
				response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the organisations")
				return
			}
		}
	}
	response.SuccessResponse(w, http.StatusOK, map[string]any{"items": items})
}
