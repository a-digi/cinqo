package ping

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	ping_entity "github.com/a-digi/cinqo/src/ping/entity"
	ping_persistent "github.com/a-digi/cinqo/src/ping/repository/persistent"
	ping_query "github.com/a-digi/cinqo/src/ping/repository/query"
)

type createRequest struct {
	Message string `json:"message"`
}

// ListHandler handles GET /api/v1/pings.
func ListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB

	pings, err := ping_query.NewPingQueryRepo(db).List()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list pings")
		return
	}
	response.SuccessResponse(w, http.StatusOK, pings)
}

// CreateHandler handles POST /api/v1/pings.
func CreateHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	var body createRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Message == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "message is required")
		return
	}

	p := &ping_entity.Ping{
		ID:      uuid.NewString(),
		Message: body.Message,
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	if err := ping_persistent.NewPingPersistentRepo(db).Insert(p); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to create ping")
		return
	}

	created, err := ping_query.NewPingQueryRepo(db).FindByID(p.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "ping created but failed to reload")
		return
	}
	response.SuccessResponse(w, http.StatusCreated, created)
}
