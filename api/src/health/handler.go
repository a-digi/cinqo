// Package health exposes a minimal liveness probe for cinqo's core API.
package health

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
)

// healthzResponse is the shape of GET /healthz.
type healthzResponse struct {
	Status string `json:"status"`
}

// GetHandler handles GET /healthz — a public liveness probe. Deliberately
// minimal (no version, no build host, no dependency/DB status): anything
// more here is public information disclosure for zero operational
// benefit at this stage.
func GetHandler(reqCtx request.RequestContext) {
	reqCtx.JSON(http.StatusOK, healthzResponse{Status: "ok"})
}
