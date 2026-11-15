package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/organisation"
	"github.com/a-digi/cinqo/src/organisation/repository/persistent"
	"github.com/a-digi/cinqo/src/organisation/repository/query"
	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"
)

// The operator's side of unattended tool use (plan/ai/tools/namako/saas/s7-unattended-monitoring.md, step 4): switch it on or off per organisation, and see which
// tools hold approvals and what they used. The approvals themselves are given on the Tools page.

// Days of usage the overview can show.
const (
	DefaultUsageDays = 7
	MaxUsageDays     = 90
)

type backgroundSwitch struct {
	Enabled *bool `json:"enabled"`
}

// SetBackgroundHandler handles PUT /api/v1/organisations/{id}/background.
func SetBackgroundHandler(reqCtx request.RequestContext) {
	setBackground(dbOf(reqCtx), callerUserID(reqCtx), reqCtx)
}

// setBackground switches unattended use on or off for an organisation and answers its list entry. It needs a known operator (the switch records who made it).
func setBackground(db *sql.DB, caller string, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	if caller == "" {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var body backgroundSwitch
	if err := reqCtx.BindJSON(&body); err != nil || body.Enabled == nil {
		response.ErrorResponse(w, http.StatusBadRequest, "enabled must be true or false")
		return
	}
	id := reqCtx.GetURI().GetPathVariable("id")
	if err := persistent.NewOrganisationPersistentRepo(db).SetBackground(id, *body.Enabled, caller); err != nil {
		fail(w, err)
		return
	}
	q := query.NewOrganisationQueryRepo(db)
	o, err := q.FindByID(id)
	if err != nil {
		fail(w, err)
		return
	}
	counts, err := q.MemberCounts()
	if err != nil {
		fail(w, err)
		return
	}
	response.SuccessResponse(w, http.StatusOK, listItem{Organisation: *o, MemberCount: counts[id], BackgroundEnabled: *body.Enabled})
}

type overviewTool struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Requested int    `json:"requested"`
	Approved  int    `json:"approved"`
}

type overviewUsage struct {
	ToolSlug string `json:"toolSlug"`
	ToolName string `json:"toolName"`
	OrgID    string `json:"orgId"`
	OrgName  string `json:"orgName"`
	Day      string `json:"day"`
	Calls    int    `json:"calls"`
	Denied   int    `json:"denied"`
}

type overview struct {
	KillSwitch bool            `json:"killSwitch"`
	Days       int             `json:"days"`
	Since      string          `json:"since"`
	Tools      []overviewTool  `json:"tools"`
	Usage      []overviewUsage `json:"usage"`
}

// BackgroundOverviewHandler handles GET /api/v1/background/overview?days=N.
func BackgroundOverviewHandler(reqCtx request.RequestContext) {
	backgroundOverview(dbOf(reqCtx), time.Now, reqCtx)
}

// backgroundOverview answers the kill switch, the tools that ask to be used unattended (how many routes they ask for and how many are approved), and the daily
// counters of the last days days (counted in UTC days, today included).
func backgroundOverview(db *sql.DB, now func() time.Time, reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	days := DefaultUsageDays
	if v := reqCtx.GetRequest().URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxUsageDays {
			response.ErrorResponse(w, http.StatusBadRequest, "days must be a whole number from 1 to 90")
			return
		}
		days = n
	}
	since := now().UTC().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	out := overview{KillSwitch: organisation.BackgroundKillSwitchOn(), Days: days, Since: since, Tools: []overviewTool{}, Usage: []overviewUsage{}}

	tools, err := tool_query.NewToolQueryRepo(db).List()
	if err != nil {
		fail(w, err)
		return
	}
	bq := tool_query.NewToolBackgroundQueryRepo(db)
	sums, err := bq.Summaries()
	if err != nil {
		fail(w, err)
		return
	}
	for _, t := range tools {
		if s, ok := sums[t.ID]; ok {
			out.Tools = append(out.Tools, overviewTool{Slug: t.Slug, Name: t.Name, Requested: s.Requested, Approved: s.Approved})
		}
	}
	orgs, err := query.NewOrganisationQueryRepo(db).List()
	if err != nil {
		fail(w, err)
		return
	}
	names := map[string]string{}
	for _, o := range orgs {
		names[o.ID] = o.Name
	}
	rows, err := bq.UsageSince(since)
	if err != nil {
		fail(w, err)
		return
	}
	for _, u := range rows {
		name, ok := names[u.OrgID]
		if !ok {
			name = u.OrgID // an organisation that is gone, or none was named
		}
		out.Usage = append(out.Usage, overviewUsage{ToolSlug: u.ToolSlug, ToolName: u.ToolName, OrgID: u.OrgID, OrgName: name, Day: u.Day, Calls: u.Calls, Denied: u.Denied})
	}
	response.SuccessResponse(w, http.StatusOK, out)
}
