package profileimage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"career-tool-backend/db"
	"career-tool-backend/profile"
)

// The photos of one profile (step 111): any number, one of them the default.
// profiles.image_media_file_id is the default and is always one of the profile's photos
// (profile_photos). Adding a photo makes it the default only when there is none yet;
// removing the default promotes the most recently added remaining photo, or leaves none.
// See plan/ai/tools/career/step-111-many-photos-per-profile.md.
//
// career.db has one connection (SetMaxOpenConns(1)), so everything that runs inside a
// transaction uses only that transaction, never db.CareerDB, and closes its rows first.

type profilePhoto struct {
	MediaFileID string `json:"mediaFileId"`
	IsDefault   bool   `json:"isDefault"`
	AddedAt     string `json:"addedAt"`
}

// ProfilePhotosHandler serves /profile-photos: GET (a profile's photos), PUT (add a library
// photo to a profile) and DELETE (remove one from a profile).
func ProfilePhotosHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listProfilePhotosHandler(w, r)
	case http.MethodPut:
		addProfilePhotoHandler(w, r)
	case http.MethodDelete:
		removeProfilePhotoHandler(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// requireProfile writes the 400/500 for an unknown profile or a failed check and says
// whether the caller should stop.
func requireProfile(w http.ResponseWriter, profileID string) bool {
	if err := profile.RequireProfileExists(profileID); err != nil {
		if errors.Is(err, profile.ErrUnknownProfile) {
			http.Error(w, "unknown profile id", http.StatusBadRequest)
			return true
		}
		http.Error(w, "failed to validate profile: "+err.Error(), http.StatusInternalServerError)
		return true
	}
	return false
}

func listProfilePhotosHandler(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profileId")
	if profileID == "" {
		http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
		return
	}
	if requireProfile(w, profileID) {
		return
	}
	photos, err := listProfilePhotos(profileID)
	if err != nil {
		http.Error(w, "failed to load the profile's photos: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"photos": photos})
}

// The default first, then the newest.
func listProfilePhotos(profileID string) ([]profilePhoto, error) {
	rows, err := db.CareerDB.Query(`
		SELECT pp.media_file_id, pp.added_at, COALESCE(p.image_media_file_id = pp.media_file_id, 0)
		FROM profile_photos pp
		JOIN profiles p ON p.id = pp.profile_id
		WHERE pp.profile_id = ?
		ORDER BY 3 DESC, pp.added_at DESC, pp.rowid DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photos := []profilePhoto{}
	for rows.Next() {
		var ph profilePhoto
		if err := rows.Scan(&ph.MediaFileID, &ph.AddedAt, &ph.IsDefault); err != nil {
			return nil, err
		}
		photos = append(photos, ph)
	}
	return photos, rows.Err()
}

type profilePhotoBody struct {
	ProfileID   string `json:"profileId"`
	MediaFileID string `json:"mediaFileId"`
}

// readProfilePhotoBody decodes {profileId, mediaFileId} and checks the profile exists and
// the photo is in the library: only library photos can be attached, so a caller cannot
// attach an arbitrary Media file. It writes the error and returns false when it fails.
func readProfilePhotoBody(w http.ResponseWriter, r *http.Request) (profilePhotoBody, bool) {
	var body profilePhotoBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return body, false
	}
	if body.ProfileID == "" || body.MediaFileID == "" {
		http.Error(w, "profileId and mediaFileId are required", http.StatusBadRequest)
		return body, false
	}
	if requireProfile(w, body.ProfileID) {
		return body, false
	}
	inLibrary, err := photoInLibrary(body.MediaFileID)
	if err != nil {
		http.Error(w, "failed to look up the photo: "+err.Error(), http.StatusInternalServerError)
		return body, false
	}
	if !inLibrary {
		http.Error(w, "unknown photo", http.StatusBadRequest)
		return body, false
	}
	return body, true
}

// PUT /profile-photos: add a library photo to the profile. Idempotent; the photo becomes the
// default only when the profile has none yet.
func addProfilePhotoHandler(w http.ResponseWriter, r *http.Request) {
	body, ok := readProfilePhotoBody(w, r)
	if !ok {
		return
	}
	if err := addProfilePhoto(body.ProfileID, body.MediaFileID); err != nil {
		http.Error(w, "failed to add the photo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	photos, err := listProfilePhotos(body.ProfileID)
	if err != nil {
		http.Error(w, "photo added but failed to reload: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, map[string]any{"photos": photos})
}

// DELETE /profile-photos?profileId=&mediaFileId=: remove a photo from the profile (the photo
// stays in the library). 204, also when the profile does not have it.
func removeProfilePhotoHandler(w http.ResponseWriter, r *http.Request) {
	profileID := r.URL.Query().Get("profileId")
	mediaFileID := r.URL.Query().Get("mediaFileId")
	if profileID == "" || mediaFileID == "" {
		http.Error(w, "profileId and mediaFileId query parameters are required", http.StatusBadRequest)
		return
	}
	if err := removeProfilePhoto(profileID, mediaFileID); err != nil {
		http.Error(w, "failed to remove the photo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addProfilePhoto makes the photo one of the profile's, and its default if it has none.
func addProfilePhoto(profileID, mediaFileID string) error {
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := addMembership(tx, profileID, mediaFileID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE profiles SET image_media_file_id = ?, updated_at = datetime('now')
		 WHERE id = ? AND (image_media_file_id IS NULL OR image_media_file_id = '')`,
		mediaFileID, profileID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// makeDefault makes the photo the profile's default, adding it to the profile's photos first
// when it is not one of them.
func makeDefault(profileID, mediaFileID string) error {
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := addMembership(tx, profileID, mediaFileID); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE profiles SET image_media_file_id = ?, updated_at = datetime('now') WHERE id = ?`,
		mediaFileID, profileID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// removeProfilePhoto takes the photo off the profile; if it was the default, the most recently
// added remaining photo becomes the default, or there is none.
func removeProfilePhoto(profileID, mediaFileID string) error {
	tx, err := db.CareerDB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`DELETE FROM profile_photos WHERE profile_id = ? AND media_file_id = ?`, profileID, mediaFileID,
	); err != nil {
		return err
	}
	var isDefault int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM profiles WHERE id = ? AND image_media_file_id = ?`, profileID, mediaFileID,
	).Scan(&isDefault); err != nil {
		return err
	}
	if isDefault > 0 {
		if err := promoteDefault(tx, profileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func addMembership(tx *sql.Tx, profileID, mediaFileID string) error {
	_, err := tx.Exec(`INSERT OR IGNORE INTO profile_photos (profile_id, media_file_id) VALUES (?, ?)`, profileID, mediaFileID)
	return err
}

// promoteDefault sets the profile's default to its most recently added photo, or to none.
func promoteDefault(tx *sql.Tx, profileID string) error {
	_, err := tx.Exec(
		`UPDATE profiles SET updated_at = datetime('now'), image_media_file_id = (
			SELECT media_file_id FROM profile_photos WHERE profile_id = ?
			ORDER BY added_at DESC, rowid DESC LIMIT 1
		) WHERE id = ?`, profileID, profileID,
	)
	return err
}
