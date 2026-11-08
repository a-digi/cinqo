package profileimage

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeMedia stands in for the core app's Media feature and records what it was asked.
type fakeMedia struct {
	mu    sync.Mutex
	calls []string // "METHOD path"
	next  int
}

func (m *fakeMedia) called() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

func startFakeMedia(t *testing.T) *fakeMedia {
	t.Helper()
	m := &fakeMedia{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.calls = append(m.calls, r.Method+" "+r.URL.Path)
		m.next++
		id := fmt.Sprintf("new-%d", m.next)
		m.mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/media/images":
			w.WriteHeader(http.StatusCreated)
			fmt.Fprintf(w, `{"message":{"fileId":%q}}`, id)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return m
}

func upload(t *testing.T, profileID string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("profileId", profileID)
	part, err := mw.CreateFormFile("file", "me.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("png-bytes"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/profile-image", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	Handler(w, req)
	return w
}

func del(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, target, nil)
	switch {
	case strings.HasPrefix(target, "/profile-photos"):
		ProfilePhotosHandler(w, req)
	case strings.HasPrefix(target, "/photos"):
		PhotosHandler(w, req)
	default:
		Handler(w, req)
	}
	return w
}

// photoIDs is the set of a profile's photos.
func photoIDs(t *testing.T, profileID string) map[string]bool {
	t.Helper()
	photos, err := listProfilePhotos(profileID)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, p := range photos {
		ids[p.MediaFileID] = true
	}
	return ids
}

func libraryIDs(t *testing.T) map[string]bool {
	t.Helper()
	photos, err := listPhotos()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, p := range photos {
		ids[p.MediaFileID] = true
	}
	return ids
}

// An upload makes a new photo and never replaces a file in place: it is added next to the
// profile's others, the default stays, another profile that has the old photo keeps it, and
// the old photo stays in the library.
func TestUpload_AddsANewPhotoNextToTheDefault(t *testing.T) {
	newLibraryDB(t)
	media := startFakeMedia(t)
	exec(t, `INSERT INTO profiles (id, image_media_file_id) VALUES ('p1', 'old'), ('p2', 'old')`)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('old')`)
	exec(t, `INSERT INTO profile_photos (profile_id, media_file_id) VALUES ('p1', 'old'), ('p2', 'old')`)

	w := upload(t, "p1")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), `"mediaFileId":"new-1"`) || !strings.Contains(w.Body.String(), `"isDefault":false`) {
		t.Fatalf("response = %s", w.Body)
	}
	if got := linkOf(t, "p1"); got != "old" {
		t.Fatalf("p1's default is %q, must stay old", got)
	}
	if photos := photoIDs(t, "p1"); len(photos) != 2 || !photos["old"] || !photos["new-1"] {
		t.Fatalf("p1's photos = %v, want old and new-1", photos)
	}
	if got := linkOf(t, "p2"); got != "old" {
		t.Fatalf("p2 links %q, must still link old", got)
	}
	ids := libraryIDs(t)
	if !ids["old"] || !ids["new-1"] {
		t.Fatalf("library = %v, want old and new-1", ids)
	}
	calls := media.called()
	if len(calls) != 1 || calls[0] != "POST /api/v1/media/images" {
		t.Fatalf("Media calls = %v, want one upload and no replace or delete", calls)
	}
}

// Removing a photo from a profile only takes it off that profile: the file is not deleted, the
// photo stays in the library, and a profile sharing it keeps it. Repeating it is fine.
func TestRemove_OnlyUnlinks(t *testing.T) {
	newLibraryDB(t)
	media := startFakeMedia(t)
	exec(t, `INSERT INTO profiles (id, image_media_file_id) VALUES ('p1', 'm1'), ('p2', 'm1')`)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1')`)
	exec(t, `INSERT INTO profile_photos (profile_id, media_file_id) VALUES ('p1', 'm1'), ('p2', 'm1')`)

	for i := 0; i < 2; i++ {
		if w := del(t, "/profile-photos?profileId=p1&mediaFileId=m1"); w.Code != http.StatusNoContent {
			t.Fatalf("run %d: status %d", i, w.Code)
		}
	}
	if linkOf(t, "p1") != "" || linkOf(t, "p2") != "m1" {
		t.Fatalf("p1=%q p2=%q", linkOf(t, "p1"), linkOf(t, "p2"))
	}
	if !libraryIDs(t)["m1"] {
		t.Fatal("the photo must stay in the library")
	}
	if calls := media.called(); len(calls) != 0 {
		t.Fatalf("Media was called: %v", calls)
	}
}

// Deleting a photo clears every profile that links it, drops it from the library and
// deletes the Media file once. Other photos are untouched.
func TestDeletePhoto_UnlinksEveryProfileAndDeletesTheFile(t *testing.T) {
	newLibraryDB(t)
	media := startFakeMedia(t)
	exec(t, `INSERT INTO profiles (id, image_media_file_id) VALUES ('p1', 'm1'), ('p2', 'm1'), ('p3', 'm2')`)
	exec(t, `INSERT INTO photos (media_file_id) VALUES ('m1'), ('m2')`)
	exec(t, `INSERT INTO profile_photos (profile_id, media_file_id) VALUES ('p1', 'm1'), ('p2', 'm1'), ('p3', 'm2')`)

	if w := del(t, "/photos?mediaFileId=m1"); w.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	if linkOf(t, "p1") != "" || linkOf(t, "p2") != "" || linkOf(t, "p3") != "m2" {
		t.Fatalf("links: %q %q %q", linkOf(t, "p1"), linkOf(t, "p2"), linkOf(t, "p3"))
	}
	if ids := libraryIDs(t); ids["m1"] || !ids["m2"] {
		t.Fatalf("library = %v", ids)
	}
	if calls := media.called(); len(calls) != 1 || calls[0] != "DELETE /api/v1/media/m1" {
		t.Fatalf("Media calls = %v", calls)
	}
}

// A photo that is not in the library is not deleted through here, and Media is not asked.
func TestDeletePhoto_UnknownIsANoOp(t *testing.T) {
	newLibraryDB(t)
	media := startFakeMedia(t)
	exec(t, `INSERT INTO profiles (id, image_media_file_id) VALUES ('p1', 'not-in-library')`)

	if w := del(t, "/photos?mediaFileId=not-in-library"); w.Code != http.StatusNoContent {
		t.Fatalf("status %d", w.Code)
	}
	if linkOf(t, "p1") != "not-in-library" {
		t.Fatal("an unknown photo must not touch profiles")
	}
	if calls := media.called(); len(calls) != 0 {
		t.Fatalf("Media was called: %v", calls)
	}
	if w := del(t, "/photos"); w.Code != http.StatusBadRequest {
		t.Fatalf("missing id: status %d", w.Code)
	}
}
