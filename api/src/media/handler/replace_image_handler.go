package handler

import (
	"bytes"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/media/imageprocessing"
	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
	media_query "github.com/a-digi/cinqo/src/media/repository/query"
)

// ReplaceImageHandler handles PUT /api/v1/media/images/{id} — step 1's
// own "overwrite the old one" capability: replaces an EXISTING image
// row's own bytes in place, keeping the same id, so anything that
// already references this id (a future avatar/logo feature, or simply
// a durable link a user has bookmarked) keeps working unchanged.
//
// Ownership check mirrors DownloadHandler/DeleteHandler exactly
// (confirmed by reading both directly, not re-derived independently):
// cinqo:super:admin OR the row's own uploaded_by_user_id — never a
// toolSlug-scope check (replacing your own already-uploaded image is
// about who uploaded it, not which tool's scope set covers it).
//
// Refuses to replace a row that isn't already an image
// (width/height both zero — see MediaFile's own doc comment): this
// route can never be used to clobber, say, a tool's own uploaded PDF
// with image bytes. Same decode/resize/re-encode pipeline as
// UploadImageHandler. See
// plan/ai/media/step-03-image-upload-and-replace-endpoints.md.
func ReplaceImageHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodPut {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	userID, scopes, err := callerIdentity(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	m, err := media_query.NewMediaQueryRepo(db).FindByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "media not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up media")
		return
	}

	if !hasScope(scopes, "cinqo:super:admin") && userID != m.UploadedByUserID {
		response.ErrorResponse(w, http.StatusForbidden, "not authorized to replace this media file")
		return
	}

	if m.Width == 0 || m.Height == 0 {
		response.ErrorResponse(w, http.StatusBadRequest, "this media file is not an image")
		return
	}

	if err := r.ParseMultipartForm(maxMediaUploadBytes); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is required and must be under the size limit")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	img, sourceFormat, err := imageprocessing.DecodeImage(file)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is not a valid image")
		return
	}
	resized := imageprocessing.ResizeToMax(img, imageResizeMaxDim)
	outFormat := imageprocessing.OutputFormat(sourceFormat)

	var buf bytes.Buffer
	if err := imageprocessing.EncodeImage(&buf, resized, outFormat, imageJPEGQuality); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to process image")
		return
	}

	extension := extensionForFormat(outFormat)
	contentType := "image/" + outFormat

	// New path, never reusing the exact old filename — sidesteps any
	// same-path write-while-serving race. Cleanup order below is
	// deliberate: write new, update the row, THEN delete old — a
	// mid-failure at any point before the row update leaves the OLD
	// file (and the row pointing at it) intact; a failure to delete the
	// old file after a successful row update just leaves one harmless
	// orphaned file on disk, never a row pointing at nothing.
	toolDir := filepath.Dir(m.StoredPath)
	newStoredPath := filepath.Join(toolDir, uuid.NewString()+extension)
	if err := os.WriteFile(newStoredPath, buf.Bytes(), 0o644); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to store file")
		return
	}

	b := resized.Bounds()
	oldStoredPath := m.StoredPath
	if err := media_persistent.NewMediaPersistentRepo(db).UpdateImage(
		id, newStoredPath, contentType, extension, int64(buf.Len()), b.Dx(), b.Dy(),
	); err != nil {
		_ = os.Remove(newStoredPath)
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to record replacement")
		return
	}
	_ = os.Remove(oldStoredPath)

	response.SuccessResponse(w, http.StatusOK, uploadImageResponse{FileID: id, Width: b.Dx(), Height: b.Dy()})
}
