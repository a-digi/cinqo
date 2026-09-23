// Package profileimage lets a Profile have exactly one uploaded photo,
// usable from the CV Builder templates — stored in the core app's own
// Media feature, not in this tool's own database.
// profiles.image_media_file_id (db.go) is a plain string reference to
// that Media file id, the same "not a real FK, Media lives in cinqo's
// own database, not this one" convention cv_documents.media_file_id
// and cv_import_runs.media_file_id already established.
//
// This mirrors cv/cv_import.go's own forwardToMedia shape almost
// exactly, not cvbuilder/handler.go's — the browser is the one that
// already has the image bytes (an incoming multipart request), so this
// handler re-forwards that request rather than generating anything
// server-side (contrast cvbuilder, which uploads a PDF it generated
// itself). See plan/ai/career/profile-image/step-01-overview.md.
package profileimage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

type profileImageResponse struct {
	ProfileID   string `json:"profileId"`
	MediaFileID string `json:"mediaFileId"`
}

// Handler dispatches GET (fetch the current image, if any), POST
// (upload a new one, replacing any existing one in place), and DELETE
// (remove it) — all addressed by ?profileId=, since there is at most
// one row per profile (no separate id needed, unlike cv_documents'
// own ?id=/?personaId= split).
func Handler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getHandler(w, r)
	case http.MethodPost:
		uploadHandler(w, r)
	case http.MethodDelete:
		deleteHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profileId")
	if profileID == "" {
		http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
		return
	}
	mediaFileID, err := currentImage(profileID)
	if err != nil {
		http.Error(w, "failed to load profile image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if mediaFileID == "" {
		http.Error(w, "no image set for this profile", http.StatusNotFound)
		return
	}
	db.WriteJSON(w, profileImageResponse{ProfileID: profileID, MediaFileID: mediaFileID})
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		http.Error(w, "invalid multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}
	profileID := r.FormValue("profileId")
	if profileID == "" {
		http.Error(w, "profileId is required", http.StatusBadRequest)
		return
	}
	if err := profile.RequireProfileExists(profileID); err != nil {
		if errors.Is(err, profile.ErrUnknownProfile) {
			http.Error(w, "unknown profile id", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to validate profile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	existingMediaFileID, err := currentImage(profileID)
	if err != nil {
		http.Error(w, "failed to load existing profile image: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var mediaFileID string
	if existingMediaFileID != "" {
		mediaFileID, err = forwardReplaceImage(r, existingMediaFileID, file, header.Filename)
	} else {
		mediaFileID, err = forwardUploadImage(r, file, header.Filename)
	}
	if err != nil {
		http.Error(w, "failed to upload image: "+err.Error(), http.StatusBadGateway)
		return
	}

	if err := setImage(profileID, mediaFileID); err != nil {
		http.Error(w, "image uploaded but failed to save: "+err.Error(), http.StatusInternalServerError)
		return
	}

	db.WriteJSON(w, profileImageResponse{ProfileID: profileID, MediaFileID: mediaFileID})
}

func deleteHandler(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profileId")
	if profileID == "" {
		http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
		return
	}
	mediaFileID, err := currentImage(profileID)
	if err != nil {
		http.Error(w, "failed to load profile image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if mediaFileID == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := clearImage(profileID); err != nil {
		http.Error(w, "failed to remove profile image: "+err.Error(), http.StatusInternalServerError)
		return
	}
	forwardDeleteToMedia(r, mediaFileID)
	w.WriteHeader(http.StatusNoContent)
}

// DeleteBestEffort is called by routeHandler.go's own profilesHandler
// DELETE case (package main) before a whole Profile is deleted — the
// same "capture the media id first, best-effort forward-delete, never
// block the parent delete on Media's own availability" pattern
// cvbuilder's DocumentsHandler DELETE case already established for
// cv_documents. Deliberately swallows every error: a Profile delete
// must never fail just because Media's own cleanup call did.
func DeleteBestEffort(r *http.Request, profileID string) {
	mediaFileID, err := currentImage(profileID)
	if err != nil || mediaFileID == "" {
		return
	}
	forwardDeleteToMedia(r, mediaFileID)
}

func currentImage(profileID string) (string, error) {
	var mediaFileID sql.NullString
	err := db.CareerDB.QueryRow(`SELECT image_media_file_id FROM profiles WHERE id = ?`, profileID).Scan(&mediaFileID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return mediaFileID.String, nil
}

func setImage(profileID, mediaFileID string) error {
	_, err := db.CareerDB.Exec(
		`UPDATE profiles SET image_media_file_id = ?, updated_at = datetime('now') WHERE id = ?`,
		mediaFileID, profileID,
	)
	return err
}

func clearImage(profileID string) error {
	_, err := db.CareerDB.Exec(
		`UPDATE profiles SET image_media_file_id = NULL, updated_at = datetime('now') WHERE id = ?`,
		profileID,
	)
	return err
}

// --- Media forwarding ---
//
// All three mirror cv/cv_import.go's own forwardToMedia/
// forwardDeleteToMedia shape: re-attach the ORIGINAL caller's own
// Authorization/Cookie headers (already sitting in r.Header, since
// ProxyHandler forwards the browser's own request through to this
// backend unmodified) onto a second outbound call, rather than
// inventing a new credential.

const profileImageToolSlug = "career"

type mediaEnvelope struct {
	Message struct {
		FileID string `json:"fileId"`
	} `json:"message"`
}

// forwardUploadImage re-multiparts the browser's own already-open file
// reader into a brand-new outbound request — targeting Media's own
// image-specific upload route (POST /api/v1/media/images), not the
// generic one, so the image gets resized/re-encoded server-side and a
// stable file id comes back that forwardReplaceImage can later reuse.
func forwardUploadImage(originalReq *http.Request, file multipart.File, filename string) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.WriteField("toolSlug", profileImageToolSlug); err != nil {
		return "", err
	}
	if err := writer.WriteField("title", "Profile Photo"); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	uploadURL := os.Getenv("CORE_API_URL") + "/api/v1/media/images"
	req, err := http.NewRequest(http.MethodPost, uploadURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	forwardAuthHeaders(originalReq, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed: %s: %s", resp.Status, string(respBody))
	}
	var envelope mediaEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", err
	}
	return envelope.Message.FileID, nil
}

// forwardReplaceImage calls Media's own PUT /api/v1/media/images/{id}
// route — keeps the SAME media file id stable across a re-upload,
// exactly the "one image per profile, replace in place" behavior
// wanted here (no orphaned previous file left behind to separately
// clean up).
func forwardReplaceImage(originalReq *http.Request, mediaFileID string, file multipart.File, filename string) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	replaceURL := os.Getenv("CORE_API_URL") + "/api/v1/media/images/" + mediaFileID
	req, err := http.NewRequest(http.MethodPut, replaceURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	forwardAuthHeaders(originalReq, req)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("replace failed: %s: %s", resp.Status, string(respBody))
	}
	var envelope mediaEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil || envelope.Message.FileID == "" {
		return mediaFileID, nil
	}
	return envelope.Message.FileID, nil
}

// forwardDeleteToMedia is best-effort — log-and-continue, matching
// cv/cv_import.go's own forwardDeleteToMedia and cvbuilder's
// DocumentsHandler DELETE case: a Media-side failure must never block
// removing this tool's own link to it.
func forwardDeleteToMedia(originalReq *http.Request, mediaFileID string) {
	deleteURL := os.Getenv("CORE_API_URL") + "/api/v1/media/" + mediaFileID
	req, err := http.NewRequest(http.MethodDelete, deleteURL, nil)
	if err != nil {
		return
	}
	forwardAuthHeaders(originalReq, req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func forwardAuthHeaders(originalReq, req *http.Request) {
	if auth := originalReq.Header.Get("Authorization"); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	if cookie := originalReq.Header.Get("Cookie"); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}
