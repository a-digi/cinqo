package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/localai/entity"
)

type unloadRequest struct {
	Force bool `json:"force"`
}

// unloadResponse is the answer of POST /api/v1/local-ai/unload.
type unloadResponse struct {
	State string `json:"state"`
}

// unload handles POST /api/v1/local-ai/unload (plan 225): stop the model program now. The body is optional ({"force": true} unloads even
// while a conversation is using the model). Nothing is stored and the "local AI is on" switch is untouched: the next conversation loads the model
// again. 200 {"state":"stopped"} also when nothing was running; 409 while a conversation is using the model and force is not set.
// The route takes no model, no path and no process id: it can only stop the module's own model program.
func (a api) unload(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body unloadRequest
	if err := reqCtx.BindJSON(&body); err != nil && !emptyBody(err) {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if a.runtime.Unload(body.Force) == entity.UnloadInUse {
		response.ErrorResponse(w, http.StatusConflict, "a conversation is using the model right now")
		return
	}
	response.SuccessResponse(w, http.StatusOK, unloadResponse{State: string(entity.UnloadStopped)})
}

// emptyBody says whether a decode error only means there was no body at all.
func emptyBody(err error) bool {
	var syn *json.SyntaxError
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || (errors.As(err, &syn) && err.Error() == "unexpected end of JSON input")
}
