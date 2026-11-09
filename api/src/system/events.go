package system

import (
	"database/sql"
	"net/http"
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

// EventsBody is the answer of GET /api/v1/system/events: newest first, at most the 50 the recorder keeps. Never null.
type EventsBody struct {
	Events []EventBody `json:"events"`
}

func eventBodies(db *sql.DB) ([]EventBody, error) {
	events, err := query.NewRunsQueryRepo(db).Events()
	if err != nil {
		return nil, err
	}
	out := make([]EventBody, 0, len(events))
	for _, e := range events {
		b := EventBody{ID: e.ID, At: e.At, Kind: e.Kind, Verdict: e.Verdict, Evidence: e.Evidence}
		if b.Evidence == nil {
			b.Evidence = []entity.EvidenceItem{}
		}
		if e.AcknowledgedAt != nil {
			b.Acknowledged, b.AcknowledgedAt = true, *e.AcknowledgedAt
		}
		out = append(out, b)
	}
	return out, nil
}

// EventsHandler handles GET /api/v1/system/events: the record of unexpected stops. Read only; nothing the caller sends is used.
func EventsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	events, err := eventBodies(db)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the record of unexpected stops")
		return
	}
	response.SuccessResponse(w, http.StatusOK, EventsBody{Events: events})
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
