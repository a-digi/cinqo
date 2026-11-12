package system

import (
	"net/http"
	"strconv"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/system/runs/entity"
	"github.com/a-digi/cinqo/src/system/runs/repository/persistent"
	"github.com/a-digi/cinqo/src/system/runs/repository/query"
)

// maxEventID bounds the id a caller may name (ours are 36 characters).
const maxEventID = 64

// EventBody is one unexpected stop as the page reads it.
type EventBody struct {
	ID             string                `json:"id"`
	At             string                `json:"at"`
	Kind           string                `json:"kind"`
	Verdict        string                `json:"verdict"`
	Evidence       []entity.EvidenceItem `json:"evidence"`
	Acknowledged   bool                  `json:"acknowledged"`
	AcknowledgedAt string                `json:"acknowledgedAt,omitempty"`
}

// EventListItem is one stop as the table of the unexpected stops reads it: no evidence (the page of one stop has it).
type EventListItem struct {
	ID             string `json:"id"`
	At             string `json:"at"`
	Kind           string `json:"kind"`
	Verdict        string `json:"verdict"`
	Acknowledged   bool   `json:"acknowledged"`
	AcknowledgedAt string `json:"acknowledgedAt,omitempty"`
}

// EventsPageBody is the answer of GET /api/v1/system/events: one page, newest first. Never null; a page past the last is an empty list with the real total.
type EventsPageBody struct {
	Events   []EventListItem `json:"events"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
	Total    int             `json:"total"`
}

// SummaryLatest is the newest stop that is not seen yet.
type SummaryLatest struct {
	ID   string `json:"id"`
	At   string `json:"at"`
	Kind string `json:"kind"`
}

// SummaryBody is the answer of GET /api/v1/system/events/summary: what the bar and the dropdown need, without any list.
type SummaryBody struct {
	Total  int            `json:"total"`
	Unseen int            `json:"unseen"`
	Latest *SummaryLatest `json:"latest,omitempty"`
}

// MarkedBody is the answer of POST /api/v1/system/events/acknowledge-all.
type MarkedBody struct {
	Marked int64 `json:"marked"`
}

const (
	defaultPageSize = 20
	maxPage         = 1_000_000
)

// allowedPageSizes are the sizes of a page the table may ask for.
var allowedPageSizes = map[int]bool{10: true, 20: true, 50: true, 100: true}

// pagingOf reads ?page= and ?pageSize= (defaults 1 and 20). A value that is not a whole number, a page below 1 or past maxPage and a size that is not allowed
// give a sentence for the caller and ok false.
func pagingOf(uri *request.URI) (page, size int, problem string) {
	page, size = 1, defaultPageSize
	if v := uri.GetQuery("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPage {
			return 0, 0, "page must be a whole number from 1 to " + strconv.Itoa(maxPage)
		}
		page = n
	}
	if v := uri.GetQuery("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || !allowedPageSizes[n] {
			return 0, 0, "pageSize must be 10, 20, 50 or 100"
		}
		size = n
	}
	return page, size, ""
}

func eventBody(e entity.Event) EventBody {
	b := EventBody{ID: e.ID, At: e.At, Kind: e.Kind, Verdict: e.Verdict, Evidence: e.Evidence}
	if b.Evidence == nil {
		b.Evidence = []entity.EvidenceItem{}
	}
	if e.AcknowledgedAt != nil {
		b.Acknowledged, b.AcknowledgedAt = true, *e.AcknowledgedAt
	}
	return b
}

// EventsHandler handles GET /api/v1/system/events?page=&pageSize=: one page of the record of unexpected stops, newest first, without the evidence. Read only.
func EventsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	page, size, problem := pagingOf(reqCtx.GetURI())
	if problem != "" {
		response.ErrorResponse(w, http.StatusBadRequest, problem)
		return
	}
	repo := query.NewRunsQueryRepo(reqCtx.GetDI().GetDatabaseManager().Connector.DB)
	counts, err := repo.EventCounts()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the record of unexpected stops")
		return
	}
	events, err := repo.EventsPage((page-1)*size, size)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the record of unexpected stops")
		return
	}
	items := make([]EventListItem, 0, len(events))
	for _, e := range events {
		it := EventListItem{ID: e.ID, At: e.At, Kind: e.Kind, Verdict: e.Verdict}
		if e.AcknowledgedAt != nil {
			it.Acknowledged, it.AcknowledgedAt = true, *e.AcknowledgedAt
		}
		items = append(items, it)
	}
	response.SuccessResponse(w, http.StatusOK, EventsPageBody{Events: items, Page: page, PageSize: size, Total: counts.Total})
}

// EventsSummaryHandler handles GET /api/v1/system/events/summary: how many stops there are, how many are not seen yet and the newest of those. Read only.
func EventsSummaryHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	counts, err := query.NewRunsQueryRepo(reqCtx.GetDI().GetDatabaseManager().Connector.DB).EventCounts()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the record of unexpected stops")
		return
	}
	body := SummaryBody{Total: counts.Total, Unseen: counts.Unseen}
	if counts.Latest != nil {
		body.Latest = &SummaryLatest{ID: counts.Latest.ID, At: counts.Latest.At, Kind: counts.Latest.Kind}
	}
	response.SuccessResponse(w, http.StatusOK, body)
}

// EventHandler handles GET /api/v1/system/events/{id}: one stop with its evidence. 404 for an id that does not exist (it may have been trimmed away: only the
// newest stops are kept); 400 for an id that is empty or too long. Read only.
func EventHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" || len(id) > maxEventID {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid event id")
		return
	}
	e, found, err := query.NewRunsQueryRepo(reqCtx.GetDI().GetDatabaseManager().Connector.DB).FindEvent(id)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the unexpected stop")
		return
	}
	if !found {
		response.ErrorResponse(w, http.StatusNotFound, "no such stop")
		return
	}
	response.SuccessResponse(w, http.StatusOK, eventBody(e))
}

// AcknowledgeAllHandler handles POST /api/v1/system/events/acknowledge-all: the person has seen every stop that is not seen yet. It marks only those (the first
// time of the others is kept) and answers how many it marked, also 0.
func AcknowledgeAllHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	n, err := persistent.NewRunsPersistentRepo(reqCtx.GetDI().GetDatabaseManager().Connector.DB).AcknowledgeAll(time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to mark the stops as seen")
		return
	}
	response.SuccessResponse(w, http.StatusOK, MarkedBody{Marked: n})
}

// AcknowledgeEventHandler handles POST /api/v1/system/events/{id}/acknowledge: the person has seen this stop. 204 on success and when it was already
// seen; 404 for an id that does not exist; 400 for an id that is empty or too long. It changes only the "seen" time of that one event.
func AcknowledgeEventHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" || len(id) > maxEventID {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid event id")
		return
	}
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	found, err := persistent.NewRunsPersistentRepo(db).Acknowledge(id, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to mark the stop as seen")
		return
	}
	if !found {
		response.ErrorResponse(w, http.StatusNotFound, "no such stop")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
