package cvbuilder

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"career-tool-backend/cvbuilder/templates"
	"career-tool-backend/db"
)

// The templates that show a photo (ats-clean among them) embed the data URI in the XHTML.
const photoTemplate = "ats-clean"

// fakeCore stands in for the core app's Media feature and pdf_tools: it serves photo bytes
// ("img:<id>"), records the XHTML it is asked to render, and stores a CV as "cv-media-1".
type fakeCore struct {
	mu         sync.Mutex
	xhtml      []string
	downloaded []string
}

func (f *fakeCore) rendered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.xhtml...)
}

func startFakeCore(t *testing.T) *fakeCore {
	t.Helper()
	f := &fakeCore{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/download"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/media/"), "/download")
			f.mu.Lock()
			f.downloaded = append(f.downloaded, id)
			f.mu.Unlock()
			w.Header().Set("Content-Type", "image/png")
			fmt.Fprint(w, "img:"+id)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/proxy/generate"):
			var body struct {
				Xhtml string `json:"xhtml"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.xhtml = append(f.xhtml, body.Xhtml)
			f.mu.Unlock()
			fmt.Fprint(w, `{"id":"res1","bytes":4}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/proxy/files"):
			fmt.Fprint(w, "%PDF")
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/media/upload":
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"message":{"fileId":"cv-media-1"}}`)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("CORE_API_URL", srv.URL)
	return f
}

func imgURI(id string) string {
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("img:"+id))
}

// profileOf returns the profile id of a persona.
func profileOf(t *testing.T, personaID string) string {
	t.Helper()
	var id string
	if err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// givePhotos puts the photos on the persona's profile (oldest first), the first as the default.
func givePhotos(t *testing.T, personaID string, photos ...string) {
	t.Helper()
	profileID := profileOf(t, personaID)
	for i, ph := range photos {
		if _, err := db.CareerDB.Exec(`INSERT OR IGNORE INTO photos (media_file_id) VALUES (?)`, ph); err != nil {
			t.Fatal(err)
		}
		if _, err := db.CareerDB.Exec(
			`INSERT INTO profile_photos (profile_id, media_file_id, added_at) VALUES (?, ?, datetime('2026-01-01 10:00:00', ?))`,
			profileID, ph, fmt.Sprintf("+%d minutes", i),
		); err != nil {
			t.Fatal(err)
		}
	}
	if len(photos) > 0 {
		if _, err := db.CareerDB.Exec(`UPDATE profiles SET image_media_file_id = ? WHERE id = ?`, photos[0], profileID); err != nil {
			t.Fatal(err)
		}
	}
}

func post(t *testing.T, h http.HandlerFunc, target string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(raw)))
	return w
}

func cvBody(personaID string, extra map[string]any) map[string]any {
	b := map[string]any{
		"personaId":  personaID,
		"templateId": photoTemplate,
		"title":      "My CV",
		"data":       templates.CvData{FullName: "Ada Lovelace"},
	}
	for k, v := range extra {
		b[k] = v
	}
	return b
}

func TestPreview_UsesTheChosenPhoto_OrTheDefault(t *testing.T) {
	personaID := setupCvDB(t)
	core := startFakeCore(t)
	givePhotos(t, personaID, "A", "B") // A is the default

	// Nothing named: the default.
	if w := post(t, PreviewHandler, "/cv-documents/preview", cvBody(personaID, map[string]any{"attachPhoto": true})); w.Code != http.StatusOK {
		t.Fatalf("default: %d %s", w.Code, w.Body)
	}
	// B named: B.
	if w := post(t, PreviewHandler, "/cv-documents/preview", cvBody(personaID, map[string]any{"attachPhoto": true, "photoMediaFileId": "B"})); w.Code != http.StatusOK {
		t.Fatalf("chosen: %d %s", w.Code, w.Body)
	}
	got := core.rendered()
	if len(got) != 2 || !strings.Contains(got[0], imgURI("A")) || strings.Contains(got[0], imgURI("B")) ||
		!strings.Contains(got[1], imgURI("B")) || strings.Contains(got[1], imgURI("A")) {
		t.Fatalf("rendered %d documents; the default one has A? %v, the chosen one has B? %v",
			len(got), len(got) > 0 && strings.Contains(got[0], imgURI("A")), len(got) > 1 && strings.Contains(got[1], imgURI("B")))
	}
}

func TestPreview_NoPhotoWhenNotAttached_AndAChosenOneIsIgnored(t *testing.T) {
	personaID := setupCvDB(t)
	core := startFakeCore(t)
	givePhotos(t, personaID, "A", "B")

	w := post(t, PreviewHandler, "/cv-documents/preview", cvBody(personaID, map[string]any{"attachPhoto": false, "photoMediaFileId": "B"}))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := core.rendered()
	if len(got) != 1 || strings.Contains(got[0], "base64") {
		t.Fatal("a CV without attachPhoto must carry no photo")
	}
	if len(core.downloaded) != 0 {
		t.Fatalf("no photo should be downloaded: %v", core.downloaded)
	}
}

func TestCreate_RefusesAPhotoThatIsNotTheProfiles(t *testing.T) {
	personaID := setupCvDB(t)
	core := startFakeCore(t)
	givePhotos(t, personaID, "A")
	// Another profile's photo, and one only in the library.
	other := setupOtherProfilePhoto(t, "Z")
	if _, err := db.CareerDB.Exec(`INSERT INTO photos (media_file_id) VALUES ('lib-only')`); err != nil {
		t.Fatal(err)
	}
	_ = other

	for _, id := range []string{"Z", "lib-only", "never-heard-of"} {
		for name, h := range map[string]http.HandlerFunc{"preview": PreviewHandler, "create": DocumentsHandler} {
			w := post(t, h, "/cv-documents", cvBody(personaID, map[string]any{"attachPhoto": true, "photoMediaFileId": id}))
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s with %q: status %d, want 400 (%s)", name, id, w.Code, w.Body)
			}
		}
	}
	if len(core.rendered()) != 0 {
		t.Fatal("a refused photo must not render a CV")
	}
	var n int
	_ = db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d CV(s) were recorded", n)
	}
}

// setupOtherProfilePhoto gives another profile (with its own persona) a photo.
func setupOtherProfilePhoto(t *testing.T, photo string) string {
	t.Helper()
	if _, err := db.CareerDB.Exec(`INSERT INTO profiles (id, first_name) VALUES ('other', 'Other')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CareerDB.Exec(`INSERT INTO photos (media_file_id) VALUES (?)`, photo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CareerDB.Exec(`INSERT INTO profile_photos (profile_id, media_file_id) VALUES ('other', ?)`, photo); err != nil {
		t.Fatal(err)
	}
	return "other"
}

func TestCreate_RemembersTheChosenPhoto(t *testing.T) {
	personaID := setupCvDB(t)
	core := startFakeCore(t)
	givePhotos(t, personaID, "A", "B")

	w := post(t, DocumentsHandler, "/cv-documents", cvBody(personaID, map[string]any{"attachPhoto": true, "photoMediaFileId": "B"}))
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var doc CvDocument
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.PhotoMediaFileID != "B" {
		t.Fatalf("response photo = %q", doc.PhotoMediaFileID)
	}
	if got := core.rendered(); len(got) != 1 || !strings.Contains(got[0], imgURI("B")) {
		t.Fatal("the CV was not rendered with B")
	}
	stored, err := GetCvDocument(doc.ID)
	if err != nil || stored.PhotoMediaFileID != "B" {
		t.Fatalf("stored = %q err=%v", stored.PhotoMediaFileID, err)
	}
	// It comes back in the lists too.
	list, err := ListCvDocuments(personaID)
	if err != nil || len(list) != 1 || list[0].PhotoMediaFileID != "B" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	all, err := ListAllCvDocuments()
	if err != nil || len(all) != 1 || all[0].PhotoMediaFileID != "B" {
		t.Fatalf("all = %+v err=%v", all, err)
	}
}

// Nothing named, or not attached: NULL is stored (the default, or no photo) and the JSON has no field.
func TestCreate_StoresNothingWhenNoPhotoWasChosen(t *testing.T) {
	personaID := setupCvDB(t)
	startFakeCore(t)
	givePhotos(t, personaID, "A", "B")

	for name, extra := range map[string]map[string]any{
		"default":      {"attachPhoto": true},
		"not attached": {"attachPhoto": false, "photoMediaFileId": "B"},
	} {
		w := post(t, DocumentsHandler, "/cv-documents", cvBody(personaID, extra))
		if w.Code != http.StatusCreated {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body)
		}
		var raw map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &raw)
		if _, has := raw["photoMediaFileId"]; has {
			t.Errorf("%s: the response carries photoMediaFileId: %v", name, raw["photoMediaFileId"])
		}
		var stored any
		if err := db.CareerDB.QueryRow(`SELECT photo_media_file_id FROM cv_documents WHERE id = ?`, raw["id"]).Scan(&stored); err != nil || stored != nil {
			t.Errorf("%s: stored %v (err=%v), want NULL", name, stored, err)
		}
	}
}

func TestPhotoStatus_ListsThePhotosDefaultFirst(t *testing.T) {
	personaID := setupCvDB(t)
	givePhotos(t, personaID, "A", "B", "C") // C newest, A the default

	w := httptest.NewRecorder()
	PhotoStatusHandler(w, httptest.NewRequest(http.MethodGet, "/cv-documents/photo-status?personaId="+personaID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var got photoStatusResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasPhoto || len(got.Photos) != 3 ||
		got.Photos[0] != (cvPhoto{MediaFileID: "A", IsDefault: true}) ||
		got.Photos[1] != (cvPhoto{MediaFileID: "C"}) || got.Photos[2] != (cvPhoto{MediaFileID: "B"}) {
		t.Fatalf("status = %+v", got)
	}
}

func TestPhotoStatus_NoPhotoAtAllHasAnEmptyList(t *testing.T) {
	personaID := setupCvDB(t)
	w := httptest.NewRecorder()
	PhotoStatusHandler(w, httptest.NewRequest(http.MethodGet, "/cv-documents/photo-status?personaId="+personaID, nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"photos":[]`) || !strings.Contains(w.Body.String(), `"hasPhoto":false`) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	PhotoStatusHandler(w, httptest.NewRequest(http.MethodGet, "/cv-documents/photo-status?personaId=nope", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown persona: %d", w.Code)
	}
}

// A profile with a single photo works exactly as before: the default is used, no choice needed.
func TestCreate_SinglePhotoStillWorksWithoutAChoice(t *testing.T) {
	personaID := setupCvDB(t)
	core := startFakeCore(t)
	givePhotos(t, personaID, "A")
	w := post(t, DocumentsHandler, "/cv-documents", cvBody(personaID, map[string]any{"attachPhoto": true}))
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := core.rendered(); len(got) != 1 || !strings.Contains(got[0], imgURI("A")) {
		t.Fatal("the single photo was not used")
	}
}
