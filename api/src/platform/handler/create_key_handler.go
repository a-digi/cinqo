package handler

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/platform"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	platform_entity "github.com/a-digi/cinqo/src/platform/entity"
	platform_persistent "github.com/a-digi/cinqo/src/platform/repository/persistent"
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"
)

type createKeyRequest struct {
	Label    string `json:"label"`
	Platform string `json:"platform"`
	Key      string `json:"key"`
}

// CreateKeyHandler handles POST /api/v1/platforms/keys. See step 5's
// "POST /api/v1/platforms/keys, precisely":
//  1. validate platform is registered,
//  2. validate key is non-empty,
//  3. encrypt and insert (created_by = caller's own user ID),
//  4. respond with the masked view only — the raw key is never echoed
//     back, logged, or held any longer than this handler's own stack
//     frame.
func CreateKeyHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	var body createKeyRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !platform.IsValid(body.Platform) {
		response.ErrorResponse(w, http.StatusBadRequest, "platform is not a registered platform")
		return
	}
	if body.Key == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "key is required")
		return
	}
	if body.Label == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "label is required")
		return
	}

	encKey, err := encryptionKey(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "encryption key not configured")
		return
	}

	createdBy, err := callerUserID(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	encrypted, err := platform_crypto.Encrypt(body.Key, encKey)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to encrypt key")
		return
	}

	k := &platform_entity.Key{
		ID:           uuid.NewString(),
		Label:        body.Label,
		Platform:     body.Platform,
		EncryptedKey: encrypted,
		CreatedBy:    createdBy,
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	if err := platform_persistent.NewKeyPersistentRepo(db).Insert(k); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to create key")
		return
	}

	created, err := platform_query.NewKeyQueryRepo(db).FindByID(k.ID)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "key created but failed to reload")
		return
	}

	resp, err := toKeyResponse(created, encKey)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "key created but failed to mask")
		return
	}
	response.SuccessResponse(w, http.StatusCreated, resp)
}
