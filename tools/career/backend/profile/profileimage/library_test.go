package profileimage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"career-tool-backend/db"

	_ "modernc.org/sqlite"
)

func newLibraryDB(t *testing.T) {
	t.Helper()
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(q, args...); err != nil {
		t.Fatal(err)
	}
}

func put(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	Handler(w, httptest.NewRequest(http.MethodPut, "/profile-image", strings.NewReader(body)))
	return w
}

func linkOf(t *testing.T, profileID string) string {
	t.Helper()
	id, err := currentImage(profileID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPhotos_ListsEachPhotoWithTheProfilesUsingIt(t *testing.T) {
	newLibraryDB(t)
	exec(t, `INSERT INTO profiles (id, first_name, last_name, created_at, image_media_file_id) VALUES
		('p1', 'Ada', 'Lovelace', '2026-01-01 10:00:00', 'm1'),
		('p2', 'Alan', '',        '2026-01-02 10:00:00', 'm1'),
		('p3', 'Grace', 'Hopper', '2026-01-03 10:00:00', NULL)`)
	exec(t, `INSERT INTO photos (media_file_id, created_at) VALUES ('m1', '2026-02-01 00:00:00'), ('m2', '2026-03-01 00:00:00')`)
	// p1 has m1 as its default; p2 has m1 among its photos but another default; p3 has none.
	exec(t, `UPDATE profiles SET image_media_file_id = 'm2' WHERE id = 'p2'`)
	exec(t, `INSERT INTO profile_photos (profile_id, media_file_id) VALUES ('p1', 'm1'), ('p2', 'm1'), ('p2', 'm2')`)

	w := httptest.NewRecorder()
	PhotosHandler(w, httptest.NewRequest(http.MethodGet, "/photos", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var got struct {
		Photos []photoEntry `json:"photos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Photos) != 2 {
		t.Fatalf("got %d photos: %s", len(got.Photos), w.Body)
	}
	// Newest first; "used by" means the profile has it among its photos, and says whether it
	// is that profile's default.
	m2 := got.Photos[0]
	if m2.MediaFileID != "m2" || len(m2.UsedBy) != 1 || m2.UsedBy[0] != (photoUser{ProfileID: "p2", Name: "Alan", IsDefault: true}) {
		t.Fatalf("m2 = %+v", m2)
	}
	m1 := got.Photos[1]
	if m1.MediaFileID != "m1" || len(m1.UsedBy) != 2 ||
		m1.UsedBy[0] != (photoUser{ProfileID: "p1", Name: "Ada Lovelace", IsDefault: true}) ||
		m1.UsedBy[1] != (photoUser{ProfileID: "p2", Name: "Alan", IsDefault: false}) {
		t.Fatalf("m1 = %+v", m1)
	}
}

func TestPhotos_UnusedPhotoHasAnEmptyUsedBy(t *testing.T) {
	newLibraryDB(t)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1')`)
	photos, err := listPhotos()
	if err != nil || len(photos) != 1 || photos[0].UsedBy == nil || len(photos[0].UsedBy) != 0 {
		t.Fatalf("photos=%+v err=%v", photos, err)
	}
}

func TestPhotos_OnlyGetAndDelete(t *testing.T) {
	w := httptest.NewRecorder()
	PhotosHandler(w, httptest.NewRequest(http.MethodPost, "/photos", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", w.Code)
	}
}

func TestMakeDefault_SharesOnePhotoBetweenProfiles(t *testing.T) {
	newLibraryDB(t)
	exec(t, `INSERT INTO profiles (id) VALUES ('p1'), ('p2')`)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1')`)

	for _, id := range []string{"p1", "p2"} {
		w := put(t, `{"profileId":"`+id+`","mediaFileId":"m1"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("link %s: status %d: %s", id, w.Code, w.Body)
		}
	}
	if linkOf(t, "p1") != "m1" || linkOf(t, "p2") != "m1" {
		t.Fatal("both profiles should link m1")
	}
}

func TestLink_RefusesWhatItCannotLink(t *testing.T) {
	newLibraryDB(t)
	exec(t, `INSERT INTO profiles (id, image_media_file_id) VALUES ('p1', 'old')`)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1')`)

	cases := map[string]string{
		"photo not in the library": `{"profileId":"p1","mediaFileId":"someone-elses-file"}`,
		"unknown profile":          `{"profileId":"nope","mediaFileId":"m1"}`,
		"missing mediaFileId":      `{"profileId":"p1"}`,
		"missing profileId":        `{"mediaFileId":"m1"}`,
		"not json":                 `not json`,
	}
	for name, body := range cases {
		if w := put(t, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, w.Code, w.Body)
		}
	}
	if linkOf(t, "p1") != "old" {
		t.Fatal("a refused link must not change the profile")
	}
}

func TestLink_ReplacesThePreviousLinkWithoutDeletingIt(t *testing.T) {
	newLibraryDB(t)
	exec(t, `INSERT INTO profiles (id, image_media_file_id) VALUES ('p1', 'm1')`)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1'), ('m2')`)

	if w := put(t, `{"profileId":"p1","mediaFileId":"m2"}`); w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if linkOf(t, "p1") != "m2" {
		t.Fatal("p1 should link m2")
	}
	ok, err := photoInLibrary("m1")
	if err != nil || !ok {
		t.Fatalf("the previous photo should stay in the library (ok=%v err=%v)", ok, err)
	}
}

func TestAddToLibrary_IsIdempotent(t *testing.T) {
	newLibraryDB(t)
	for i := 0; i < 2; i++ {
		if err := addToLibrary("m1"); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM photos`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
