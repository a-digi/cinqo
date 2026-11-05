package profileimage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

// The photo library (step 110): a photo is one Media file, kept in the photos table
// on its own, and any number of profiles can link it through
// profiles.image_media_file_id. See plan/ai/tools/career/step-110-shared-photo-library.md.

type photoUser struct {
	ProfileID string `json:"profileId"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type photoEntry struct {
	MediaFileID string      `json:"mediaFileId"`
	CreatedAt   string      `json:"createdAt"`
	UsedBy      []photoUser `json:"usedBy"`
}

// PhotosHandler serves GET /photos (every photo in the library, newest first, with
// the profiles that link it) and DELETE /photos?mediaFileId= (delete a photo for good).
func PhotosHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listHandler(w)
	case http.MethodDelete:
		deletePhotoHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func listHandler(w http.ResponseWriter) {
	photos, err := listPhotos()
	if err != nil {
		http.Error(w, "failed to load photos: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"photos": photos})
}

func listPhotos() ([]photoEntry, error) {
	rows, err := db.CareerDB.Query(`
		SELECT ph.media_file_id, ph.created_at, p.id, p.first_name, p.last_name, p.image_media_file_id = ph.media_file_id
		FROM photos ph
		LEFT JOIN profile_photos pp ON pp.media_file_id = ph.media_file_id
		LEFT JOIN profiles p ON p.id = pp.profile_id
		ORDER BY ph.created_at DESC, ph.rowid DESC, p.created_at ASC, p.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	photos := []photoEntry{}
	index := map[string]int{}
	for rows.Next() {
		var mediaFileID, createdAt string
		var profileID, first, last *string
		var isDefault *bool
		if err := rows.Scan(&mediaFileID, &createdAt, &profileID, &first, &last, &isDefault); err != nil {
			return nil, err
		}
		i, seen := index[mediaFileID]
		if !seen {
			i = len(photos)
			index[mediaFileID] = i
			photos = append(photos, photoEntry{MediaFileID: mediaFileID, CreatedAt: createdAt, UsedBy: []photoUser{}})
		}
		if profileID != nil {
			photos[i].UsedBy = append(photos[i].UsedBy, photoUser{
				ProfileID: *profileID, Name: fullName(first, last), IsDefault: isDefault != nil && *isDefault,
			})
		}
	}
	return photos, rows.Err()
}

func fullName(first, last *string) string {
	var parts []string
	for _, p := range []*string{first, last} {
		if p != nil && strings.TrimSpace(*p) != "" {
			parts = append(parts, strings.TrimSpace(*p))
		}
	}
	return strings.Join(parts, " ")
}

// linkHandler serves PUT /profile-image: make a photo that is already in the library the
// profile's default (adding it to the profile's photos first if it is not one of them).
// Only library photos can be used, so a caller cannot attach an arbitrary Media file. The
// profile's previous default stays one of its photos.
func linkHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID   string `json:"profileId"`
		MediaFileID string `json:"mediaFileId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.ProfileID == "" || body.MediaFileID == "" {
		http.Error(w, "profileId and mediaFileId are required", http.StatusBadRequest)
		return
	}
	if err := profile.RequireProfileExists(body.ProfileID); err != nil {
		if errors.Is(err, profile.ErrUnknownProfile) {
			http.Error(w, "unknown profile id", http.StatusBadRequest)
			return
		}
		http.Error(w, "failed to validate profile: "+err.Error(), http.StatusInternalServerError)
		return
	}
	inLibrary, err := photoInLibrary(body.MediaFileID)
	if err != nil {
		http.Error(w, "failed to look up the photo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !inLibrary {
		http.Error(w, "unknown photo", http.StatusBadRequest)
		return
	}
	if err := makeDefault(body.ProfileID, body.MediaFileID); err != nil {
		http.Error(w, "failed to set the default photo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, profileImageResponse{ProfileID: body.ProfileID, MediaFileID: body.MediaFileID})
}

func photoInLibrary(mediaFileID string) (bool, error) {
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM photos WHERE media_file_id = ?`, mediaFileID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func addToLibrary(mediaFileID string) error {
	_, err := db.CareerDB.Exec(`INSERT OR IGNORE INTO photos (media_file_id) VALUES (?)`, mediaFileID)
	return err
}

// deletePhotoHandler deletes a photo for good: every profile that has it loses it first, then the library row goes, then the Media file (best effort, like
// every other Media delete here: Media allows only the uploader or an admin, and a
// Media-side failure must not keep the photo in the library). 204 also when the
// photo is not in the library, and then Media is not called: only library photos
// can be deleted through here.
func deletePhotoHandler(w http.ResponseWriter, r *http.Request) {
	mediaFileID := r.URL.Query().Get("mediaFileId")
	if mediaFileID == "" {
		http.Error(w, "mediaFileId query parameter is required", http.StatusBadRequest)
		return
	}
	deleted, err := deletePhoto(mediaFileID)
	if err != nil {
		http.Error(w, "failed to delete the photo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if deleted {
		forwardDeleteToMedia(r, mediaFileID)
	}
	w.WriteHeader(http.StatusNoContent)
}

// deletePhoto removes the photo from every profile's photos and from the library in one
// transaction; a profile whose default it was gets the next one (see removeProfilePhoto's
// rule). False if it was not in the library.
func deletePhoto(mediaFileID string) (bool, error) {
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM photos WHERE media_file_id = ?`, mediaFileID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil
	}
	defaultOf, err := profilesWithDefault(tx, mediaFileID)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM profile_photos WHERE media_file_id = ?`, mediaFileID); err != nil {
		return false, err
	}
	for _, profileID := range defaultOf {
		if err := promoteDefault(tx, profileID); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

func profilesWithDefault(tx *sql.Tx, mediaFileID string) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM profiles WHERE image_media_file_id = ?`, mediaFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
