package handler

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	tool_query "github.com/a-digi/cinqo/src/tool/repository/query"

	media_entity "github.com/a-digi/cinqo/src/media/entity"
	"github.com/a-digi/cinqo/src/media/imageprocessing"
	media_persistent "github.com/a-digi/cinqo/src/media/repository/persistent"
)

// imageResizeMaxDim/imageJPEGQuality are step 1's own approved
// defaults (Open Question 5) — plain constants, matching this
// package's own "constants over premature configurability" convention
// (maxMediaUploadBytes, upload_handler.go).
const imageResizeMaxDim = 1600
const imageJPEGQuality = 85

// cinqoSystemToolSlug is a reserved toolSlug value representing the
// core app itself, not an installed plugin Tool — there is no
// `tools` row for "Cinqo" to look up scopes from (installed Tools and
// the core app are different concepts). An image uploaded under this
// slug (e.g. a system-wide logo/illustration, not owned by any
// specific Tool) requires cinqo:super:admin directly instead of the
// normal dynamic per-Tool scope check below. See
// plan/ai/media/step-08-admin-media-upload.md.
const cinqoSystemToolSlug = "cinqo"

type uploadImageResponse struct {
	FileID string `json:"fileId"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// UploadImageHandler handles POST /api/v1/media/images — a second,
// image-specific sibling of UploadHandler (upload_handler.go), never a
// replacement for it: that route stays untouched and content-type
// agnostic for every existing/future non-image caller. Multipart,
// fields "file" (required) and "toolSlug" (required) — same
// toolSlug-scope authorization UploadHandler itself already does (a
// caller must hold a scope toolSlug itself declares, or
// cinqo:super:admin).
//
// The uploaded bytes are decoded as an image, resized down to
// imageResizeMaxDim (imageprocessing.ResizeToMax — never upscales),
// and RE-ENCODED (PNG stays PNG, everything else becomes JPEG — see
// imageprocessing.OutputFormat) before being stored: the file actually
// written to disk, and the size_bytes/content_type/extension recorded
// for it, always describe the RESIZED/RE-ENCODED version, never the
// original upload's own bytes. See
// plan/ai/media/step-03-image-upload-and-replace-endpoints.md.
func UploadImageHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()

	if r.Method != http.MethodPost {
		response.ErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	userID, callerScopes, err := callerIdentity(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := r.ParseMultipartForm(maxMediaUploadBytes); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is required and must be under the size limit")
		return
	}

	toolSlug := strings.TrimSpace(r.FormValue("toolSlug"))
	if toolSlug == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "toolSlug is required")
		return
	}

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB

	if toolSlug == cinqoSystemToolSlug {
		if !hasScope(callerScopes, "cinqo:super:admin") {
			response.ErrorResponse(w, http.StatusForbidden, "missing required scope")
			return
		}
	} else {
		tool, err := tool_query.NewToolQueryRepo(db).FindBySlug(toolSlug)
		if err != nil {
			response.ErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("unknown tool %q", toolSlug))
			return
		}

		// Same dynamic scope check as UploadHandler's own doc comment
		// describes: the caller must hold at least one scope toolSlug
		// itself declared (or cinqo:super:admin).
		toolScopes, err := tool_query.NewToolQueryRepo(db).ScopesForTool(tool.ID)
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to resolve tool scopes")
			return
		}
		if !hasScope(callerScopes, "cinqo:super:admin") && !hasAnyScope(callerScopes, toolScopes) {
			response.ErrorResponse(w, http.StatusForbidden, "missing required scope")
			return
		}
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	// filepath.Base strips any directory component a hostile or buggy
	// client filename could carry — same reasoning as
	// upload_handler.go's own identical guard. This is metadata only
	// (OriginalFilename, for display) — it plays no part in the actual
	// on-disk stored path below, which is id-based regardless.
	originalFilename := filepath.Base(header.Filename)
	if originalFilename == "." || originalFilename == string(filepath.Separator) || originalFilename == "" {
		originalFilename = "image"
	}

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

	toolDir := mediaRoot(reqCtx, toolSlug)
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to prepare media storage")
		return
	}

	id := uuid.NewString()
	storedPath := filepath.Join(toolDir, id+extension)
	if err := os.WriteFile(storedPath, buf.Bytes(), 0o644); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to store file")
		return
	}

	b := resized.Bounds()
	m := &media_entity.MediaFile{
		ID:               id,
		ToolSlug:         toolSlug,
		OriginalFilename: originalFilename,
		Extension:        extension,
		StoredPath:       storedPath,
		ContentType:      contentType,
		SizeBytes:        int64(buf.Len()),
		UploadedByUserID: userID,
		Width:            b.Dx(),
		Height:           b.Dy(),
	}
	if err := media_persistent.NewMediaPersistentRepo(db).Insert(m); err != nil {
		_ = os.Remove(storedPath)
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to record upload")
		return
	}

	response.SuccessResponse(w, http.StatusCreated, uploadImageResponse{FileID: id, Width: b.Dx(), Height: b.Dy()})
}

// extensionForFormat maps an imageprocessing.OutputFormat result
// ("jpeg" or "png") to the file extension actually written to disk —
// ".jpg", not ".jpeg", matching the far more common convention.
func extensionForFormat(format string) string {
	if format == "png" {
		return ".png"
	}
	return ".jpg"
}
