package profileimage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"
)

// seed makes a profile with the given photos, oldest first (added_at one minute apart), and
// the first of them as its default unless def is "".
func seed(t *testing.T, profileID, def string, photos ...string) {
	t.Helper()
	exec(t, `INSERT INTO profiles (id, first_name) VALUES (?, ?)`, profileID, profileID)
	for i, ph := range photos {
		exec(t, `INSERT OR IGNORE INTO photos (media_file_id) VALUES (?)`, ph)
		exec(t, `INSERT INTO profile_photos (profile_id, media_file_id, added_at) VALUES (?, ?, datetime('2026-01-01 10:00:00', ?))`,
			profileID, ph, "+"+string(rune('0'+i))+" minutes")
	}
	if def != "" {
		exec(t, `UPDATE profiles SET image_media_file_id = ? WHERE id = ?`, def, profileID)
	}
}

func call(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if strings.HasPrefix(target, "/profile-photos") {
		ProfilePhotosHandler(w, req)
	} else if strings.HasPrefix(target, "/photos") {
		PhotosHandler(w, req)
	} else {
		Handler(w, req)
	}
	return w
}

// invariant: a profile's default, when set, is always one of its photos.
func checkInvariant(t *testing.T) {
	t.Helper()
	var n int
	err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM profiles p
		WHERE p.image_media_file_id IS NOT NULL AND p.image_media_file_id != ''
		AND NOT EXISTS (SELECT 1 FROM profile_photos pp WHERE pp.profile_id = p.id AND pp.media_file_id = p.image_media_file_id)`).Scan(&n)
	if err != nil || n != 0 {
		t.Fatalf("%d profile(s) have a default that is not one of their photos (err=%v)", n, err)
	}
}

func order(t *testing.T, profileID string) []string {
	t.Helper()
	photos, err := listProfilePhotos(profileID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, p := range photos {
		ids = append(ids, p.MediaFileID)
	}
	return ids
}

func same(a, b []string) bool { return strings.Join(a, ",") == strings.Join(b, ",") }

func TestProfilePhotos_ListsTheDefaultFirstThenTheNewest(t *testing.T) {
	newLibraryDB(t)
	seed(t, "p1", "a", "a", "b", "c") // a oldest and default, c newest
	w := call(t, http.MethodGet, "/profile-photos?profileId=p1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var got struct {
		Photos []profilePhoto `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Photos) != 3 || got.Photos[0].MediaFileID != "a" || !got.Photos[0].IsDefault ||
		got.Photos[1].MediaFileID != "c" || got.Photos[1].IsDefault || got.Photos[2].MediaFileID != "b" {
		t.Fatalf("photos = %+v", got.Photos)
	}
	if w := call(t, http.MethodGet, "/profile-photos", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("missing profileId: %d", w.Code)
	}
	if w := call(t, http.MethodGet, "/profile-photos?profileId=nope", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown profile: %d", w.Code)
	}
	if w := call(t, http.MethodPost, "/profile-photos", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
}

func TestProfilePhotos_AddIsIdempotentAndDefaultsOnlyWhenThereIsNone(t *testing.T) {
	newLibraryDB(t)
	seed(t, "empty", "")
	seed(t, "has", "x", "x")
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1'), ('m2')`)

	for i := 0; i < 2; i++ {
		if w := call(t, http.MethodPut, "/profile-photos", `{"profileId":"empty","mediaFileId":"m1"}`); w.Code != http.StatusOK {
			t.Fatalf("run %d: %d %s", i, w.Code, w.Body)
		}
	}
	if linkOf(t, "empty") != "m1" || !same(order(t, "empty"), []string{"m1"}) {
		t.Fatalf("empty: default=%q photos=%v", linkOf(t, "empty"), order(t, "empty"))
	}
	if w := call(t, http.MethodPut, "/profile-photos", `{"profileId":"empty","mediaFileId":"m2"}`); w.Code != http.StatusOK {
		t.Fatalf("%d", w.Code)
	}
	if linkOf(t, "empty") != "m1" {
		t.Fatalf("a second photo must not change the default: %q", linkOf(t, "empty"))
	}
	call(t, http.MethodPut, "/profile-photos", `{"profileId":"has","mediaFileId":"m1"}`)
	if linkOf(t, "has") != "x" || !photoIDs(t, "has")["m1"] {
		t.Fatalf("has: default=%q", linkOf(t, "has"))
	}
	checkInvariant(t)
}

func TestProfilePhotos_AddRefusesWhatItCannotAdd(t *testing.T) {
	newLibraryDB(t)
	seed(t, "p1", "a", "a")
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1')`)
	cases := map[string]string{
		"photo not in the library": `{"profileId":"p1","mediaFileId":"someone-elses-file"}`,
		"unknown profile":          `{"profileId":"nope","mediaFileId":"m1"}`,
		"missing mediaFileId":      `{"profileId":"p1"}`,
		"missing profileId":        `{"mediaFileId":"m1"}`,
		"not json":                 `nope`,
	}
	for name, body := range cases {
		if w := call(t, http.MethodPut, "/profile-photos", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", name, w.Code)
		}
	}
	if !same(order(t, "p1"), []string{"a"}) {
		t.Fatalf("a refused add must not change the profile: %v", order(t, "p1"))
	}
}

func TestMakeDefault_AddsTheMembershipAndKeepsThePreviousDefault(t *testing.T) {
	newLibraryDB(t)
	seed(t, "p1", "a", "a", "b")
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('new')`)

	if w := call(t, http.MethodPut, "/profile-image", `{"profileId":"p1","mediaFileId":"b"}`); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if linkOf(t, "p1") != "b" {
		t.Fatalf("default = %q", linkOf(t, "p1"))
	}
	// A photo that is in the library but not yet the profile's becomes one of its photos.
	if w := call(t, http.MethodPut, "/profile-image", `{"profileId":"p1","mediaFileId":"new"}`); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if linkOf(t, "p1") != "new" || len(photoIDs(t, "p1")) != 3 || !photoIDs(t, "p1")["a"] {
		t.Fatalf("default=%q photos=%v", linkOf(t, "p1"), photoIDs(t, "p1"))
	}
	checkInvariant(t)
}

func TestRemoveFromProfile_PromotesTheNewestWhenTheDefaultGoes(t *testing.T) {
	newLibraryDB(t)
	seed(t, "p1", "b", "a", "b", "c", "d") // d newest, a oldest, b the default
	seed(t, "p2", "a", "a")                // shares a

	remove := func(profile, photo string) int {
		return call(t, http.MethodDelete, "/profile-photos?profileId="+profile+"&mediaFileId="+photo, "").Code
	}
	// A non-default photo goes: nothing else changes.
	if remove("p1", "a") != http.StatusNoContent || linkOf(t, "p1") != "b" || !same(order(t, "p1"), []string{"b", "d", "c"}) {
		t.Fatalf("after removing a: default=%q order=%v", linkOf(t, "p1"), order(t, "p1"))
	}
	// Repeating it is fine.
	if remove("p1", "a") != http.StatusNoContent {
		t.Fatal("not idempotent")
	}
	// The default goes with c and d left: the NEWEST (d) takes over, not the oldest.
	if remove("p1", "b") != http.StatusNoContent || linkOf(t, "p1") != "d" {
		t.Fatalf("after removing the default: %q, want d", linkOf(t, "p1"))
	}
	if remove("p1", "d") != http.StatusNoContent || linkOf(t, "p1") != "c" {
		t.Fatalf("after removing d: %q, want c", linkOf(t, "p1"))
	}
	// The last one goes: no default.
	if remove("p1", "c") != http.StatusNoContent || linkOf(t, "p1") != "" || len(order(t, "p1")) != 0 {
		t.Fatalf("after removing the last: default=%q", linkOf(t, "p1"))
	}
	// Another profile and the library are untouched.
	lib := libraryIDs(t)
	if linkOf(t, "p2") != "a" || !lib["a"] || !lib["b"] || !lib["c"] || !lib["d"] {
		t.Fatal("p2 or the library changed")
	}
	if call(t, http.MethodDelete, "/profile-photos?profileId=p1", "").Code != http.StatusBadRequest {
		t.Fatal("missing mediaFileId should be 400")
	}
	checkInvariant(t)
}

// DELETE /profile-image is gone (step 111): a profile's photos are removed through /profile-photos.
func TestProfileImage_NoLongerDeletes(t *testing.T) {
	newLibraryDB(t)
	seed(t, "p1", "a", "a")
	if w := call(t, http.MethodDelete, "/profile-image?profileId=p1", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, want 405", w.Code)
	}
	if linkOf(t, "p1") != "a" {
		t.Fatal("the photo must be untouched")
	}
}

func TestDeletePhoto_RemovesItFromEveryProfileAndPromotes(t *testing.T) {
	newLibraryDB(t)
	media := startFakeMedia(t)
	seed(t, "p1", "x", "y", "x") // x is the default, y is older
	seed(t, "p2", "y", "y", "x") // y is the default, x is newer: x is not its default
	seed(t, "p3", "z", "z")

	if w := call(t, http.MethodDelete, "/photos?mediaFileId=x", ""); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if linkOf(t, "p1") != "y" || !same(order(t, "p1"), []string{"y"}) {
		t.Fatalf("p1: default=%q order=%v", linkOf(t, "p1"), order(t, "p1"))
	}
	if linkOf(t, "p2") != "y" || !same(order(t, "p2"), []string{"y"}) {
		t.Fatalf("p2: default=%q order=%v", linkOf(t, "p2"), order(t, "p2"))
	}
	if linkOf(t, "p3") != "z" || libraryIDs(t)["x"] {
		t.Fatal("p3 changed or x is still in the library")
	}
	if calls := media.called(); len(calls) != 1 || calls[0] != "DELETE /api/v1/media/x" {
		t.Fatalf("Media calls = %v", calls)
	}
	// Deleting y leaves p1 and p2 with nothing.
	call(t, http.MethodDelete, "/photos?mediaFileId=y", "")
	if linkOf(t, "p1") != "" || linkOf(t, "p2") != "" || len(order(t, "p1")) != 0 {
		t.Fatalf("p1=%q p2=%q", linkOf(t, "p1"), linkOf(t, "p2"))
	}
	checkInvariant(t)
}

func TestUpload_BecomesTheDefaultOnlyWhenThereIsNone(t *testing.T) {
	newLibraryDB(t)
	startFakeMedia(t)
	seed(t, "p1", "")
	w := upload(t, "p1")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"isDefault":true`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if linkOf(t, "p1") != "new-1" {
		t.Fatalf("default = %q", linkOf(t, "p1"))
	}
	if !libraryIDs(t)["new-1"] || !photoIDs(t, "p1")["new-1"] {
		t.Fatal("the upload must be in the library and the profile's photos")
	}
	checkInvariant(t)
}
