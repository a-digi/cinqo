package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"
)

// ListKeysHandler handles GET /api/v1/platforms/keys — never the
// ciphertext or plaintext, only a server-computed masked value (step 2's
// Mask, recomputed from a fresh decrypt on every response). See
// plan/ai/platform/step-05-platform-and-key-api.md.
func ListKeysHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	encKey, err := encryptionKey(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "encryption key not configured")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	keys, err := platform_query.NewKeyQueryRepo(db).List()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list keys")
		return
	}

	out := make([]keyResponse, 0, len(keys))
	for _, k := range keys {
		resp, err := toKeyResponse(k, encKey)
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to decrypt a stored key")
			return
		}
		out = append(out, resp)
	}
	response.SuccessResponse(w, http.StatusOK, out)
}
