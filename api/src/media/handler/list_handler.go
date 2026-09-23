package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	media_entity "github.com/a-digi/cinqo/src/media/entity"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// mediaFileResponse deliberately omits StoredPath — the raw filesystem
// path never needs to reach a browser client, and exposing it would
// leak deployment-layout details for no benefit. See
// plan/ai/media/step-03-admin-ui.md.
type mediaFileResponse struct {
	ID               string `json:"id"`
	ToolSlug         string `json:"tool_slug"`
	OriginalFilename string `json:"original_filename"`
	Extension        string `json:"extension"`
	ContentType      string `json:"content_type"`
	SizeBytes        int64  `json:"size_bytes"`
	UploadedByUserID string `json:"uploaded_by_user_id"`
	ConversationID   string `json:"conversation_id,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	CreatedAt        string `json:"created_at"`
	// omitempty on Width/Height matches the entity's own zero-means-null
	// convention (MediaFile's own doc comment) — a non-image row simply
	// never emits these fields, exactly like ConversationID/ExpiresAt
	// above for their own absent-value case.
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func toMediaFileResponse(m *media_entity.MediaFile) mediaFileResponse {
	return mediaFileResponse{
		ID:               m.ID,
		ToolSlug:         m.ToolSlug,
		OriginalFilename: m.OriginalFilename,
		Extension:        m.Extension,
		ContentType:      m.ContentType,
		SizeBytes:        m.SizeBytes,
		UploadedByUserID: m.UploadedByUserID,
		ConversationID:   m.ConversationID,
		ExpiresAt:        m.ExpiresAt,
		CreatedAt:        m.CreatedAt,
		Width:            m.Width,
		Height:           m.Height,
		UpdatedAt:        m.UpdatedAt,
	}
}

// ListHandler handles GET /api/v1/media?toolSlug=... — cinqo:super:admin
// only (route-media.yaml's own static scope declaration; unlike
// UploadHandler this is a genuinely global admin operation, not a
// per-tool dynamic one, so it doesn't need an in-handler scope check).
// Backs the admin Media page (MediaListPage.tsx). Returns every
// matching row regardless of expiry — an admin auditing/cleaning up
// media needs to see expired rows too.
func ListHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodGet {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	toolSlug := r.URL.Query().Get("toolSlug")

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	files, err := media_query.NewMediaQueryRepo(db).FindAll(toolSlug)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list media")
		return
	}

	out := make([]mediaFileResponse, 0, len(files))
	for _, f := range files {
		out = append(out, toMediaFileResponse(f))
	}

	// Bare array, matching ToolList/ListPlatformsHandler's own
	// established convention — response.SuccessResponse's own
	// {"success":true,"message":...} envelope is the one wrapping
	// layer, not a second named key on top of it.
	response.SuccessResponse(w, http.StatusOK, out)
}
