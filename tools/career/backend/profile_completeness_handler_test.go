package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"career-tool-backend/db"
	"career-tool-backend/persona"
	"career-tool-backend/profile"
)

type completenessJSON struct {
	Score int    `json:"score"`
	Band  string `json:"band"`
	Items []struct {
		Key    string `json:"key"`
		Label  string `json:"label"`
		Weight int    `json:"weight"`
		Done   bool   `json:"done"`
	} `json:"items"`
}

func listProfilesJSON(t *testing.T) map[string]completenessJSON {
	t.Helper()
	rec := httptest.NewRecorder()
	profilesHandler(rec, httptest.NewRequest(http.MethodGet, "/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Profiles []struct {
			ID           string           `json:"id"`
			FirstName    string           `json:"firstName"`
			Completeness completenessJSON `json:"completeness"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	byName := map[string]completenessJSON{}
	for _, p := range resp.Profiles {
		byName[p.FirstName] = p.Completeness
	}
	return byName
}

func mustExec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// The list carries each profile's completeness, worked out from its own links, image and personas.
func TestProfilesList_CarriesCompleteness(t *testing.T) {
	setupHandlerDB(t)

	// Empty: a name and nothing else (10).
	if _, err := profile.CreateProfile("Empty", ""); err != nil {
		t.Fatal(err)
	}

	// Full: every item (100). Skills and experience sit on one persona; a second, bare persona must not lower it.
	fullID, err := profile.CreateProfile("Full", "Profile")
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.UpsertProfileExternalLink(fullID, "linkedin", "https://linkedin.com/in/full"); err != nil {
		t.Fatal(err)
	}
	mustExec(t, `UPDATE profiles SET image_media_file_id = 'media-1' WHERE id = ?`, fullID)
	pid, err := persona.CreatePersona(fullID, "Backend", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := persona.CreatePersona(fullID, "Bare", ""); err != nil {
		t.Fatal(err)
	}
	mustExec(t, `INSERT INTO persona_details (persona_id, headline, summary) VALUES (?, 'Go dev', 'Builds APIs')`, pid)
	for i, skill := range []string{"go", "sql", "docker"} {
		mustExec(t, `INSERT INTO career_skills (id, persona_id, skill) VALUES (?, ?, ?)`, "s"+string(rune('a'+i)), pid, skill)
	}
	mustExec(t, `INSERT INTO career_experience (id, persona_id, company, title) VALUES ('e1', ?, 'Acme', 'Engineer')`, pid)

	// Spread: skills split over two personas never reach the threshold on either (name + persona = 25).
	spreadID, err := profile.CreateProfile("Spread", "")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := persona.CreatePersona(spreadID, "A", "")
	b, _ := persona.CreatePersona(spreadID, "B", "")
	mustExec(t, `INSERT INTO career_skills (id, persona_id, skill) VALUES ('x1', ?, 'go'), ('x2', ?, 'sql'), ('x3', ?, 'docker')`, a, a, b)

	got := listProfilesJSON(t)
	if got["Empty"].Score != 10 || got["Empty"].Band != "just-started" {
		t.Errorf("Empty = %+v", got["Empty"])
	}
	if got["Full"].Score != 100 || got["Full"].Band != "complete" {
		t.Errorf("Full = %+v", got["Full"])
	}
	if got["Spread"].Score != 25 {
		t.Errorf("Spread score = %d, want 25", got["Spread"].Score)
	}
	if len(got["Full"].Items) != 7 || got["Full"].Items[1].Key != "links" || !got["Full"].Items[1].Done {
		t.Errorf("Full items = %+v", got["Full"].Items)
	}
	if got["Empty"].Items[1].Done {
		t.Error("Empty has no links but links item is done")
	}
}

// Deleting a profile never deletes its photo: another profile may link the same one
// (step 110), and the photo stays in the library.
func TestProfileDelete_KeepsTheSharedPhoto(t *testing.T) {
	setupHandlerDB(t)
	var mediaCalls int
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaCalls++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer media.Close()
	t.Setenv("CORE_API_URL", media.URL)

	mustExec(t, `INSERT INTO profiles (id, first_name, image_media_file_id) VALUES ('p1', 'A', 'm1'), ('p2', 'B', 'm1')`)
	mustExec(t, `INSERT INTO photos (media_file_id) VALUES ('m1')`)

	rec := httptest.NewRecorder()
	profilesHandler(rec, httptest.NewRequest(http.MethodDelete, "/profiles?id=p1", nil))
	if rec.Code >= 300 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var link string
	if err := db.CareerDB.QueryRow(`SELECT image_media_file_id FROM profiles WHERE id = 'p2'`).Scan(&link); err != nil || link != "m1" {
		t.Fatalf("p2 link=%q err=%v", link, err)
	}
	var inLibrary int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM photos WHERE media_file_id = 'm1'`).Scan(&inLibrary); err != nil || inLibrary != 1 {
		t.Fatalf("library count=%d err=%v", inLibrary, err)
	}
	if mediaCalls != 0 {
		t.Fatalf("Media was called %d times", mediaCalls)
	}
}

// The list carries the Media file id of a profile's photo so the rows can show it; a
// profile without one has no such field at all (omitempty), never null or "".
func TestProfilesList_CarriesThePhotoId(t *testing.T) {
	setupHandlerDB(t)
	mustExec(t, `INSERT INTO profiles (id, first_name, image_media_file_id) VALUES ('p1', 'With', 'm1'), ('p2', 'Without', NULL), ('p3', 'Empty', '')`)

	rec := httptest.NewRecorder()
	profilesHandler(rec, httptest.NewRequest(http.MethodGet, "/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Profiles []map[string]any `json:"profiles"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	got := map[string]any{}
	has := map[string]bool{}
	for _, p := range resp.Profiles {
		name := p["firstName"].(string)
		got[name], has[name] = p["imageMediaFileId"]
	}
	if got["With"] != "m1" {
		t.Fatalf("With = %v", got["With"])
	}
	if has["Without"] || has["Empty"] {
		t.Fatalf("a profile without a photo must not carry the field: %v", has)
	}
}
