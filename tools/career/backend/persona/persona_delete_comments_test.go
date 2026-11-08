package persona

import (
	"slices"
	"testing"

	"career-tool-backend/comments/commentstest"
	"career-tool-backend/db"
)

func insertCv(t *testing.T, id, personaID string) {
	t.Helper()
	exec(t, `INSERT INTO cv_documents (id, persona_id, template_id, title, data_json, media_file_id) VALUES (?, ?, 'classic', 'T', '{}', 'm')`, id, personaID)
	exec(t, `INSERT INTO cv_document_personas (cv_document_id, persona_id) VALUES (?, ?)`, id, personaID)
}

func TestDeletePersona_PurgesItsCommentsAndThoseOfItsCvs(t *testing.T) {
	profileID := setupRunsDB(t)
	purged := commentstest.Start(t)
	a, err := CreatePersona(profileID, "A", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := CreatePersona(profileID, "B", "")
	insertCv(t, "cv-a1", a)
	insertCv(t, "cv-a2", a)
	insertCv(t, "cv-b1", b)
	// A CV of B that is merely attached to A is not deleted with A, so its comments stay.
	exec(t, `INSERT INTO cv_document_personas (cv_document_id, persona_id) VALUES ('cv-b1', ?)`, a)

	if err := DeletePersona(a); err != nil {
		t.Fatal(err)
	}
	want := []string{"cv:cv-a1", "cv:cv-a2", "persona:" + a}
	slices.Sort(want)
	if got := purged(); !slices.Equal(got, want) {
		t.Fatalf("purged %v, want %v", got, want)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM cv_documents`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cv rows left = %d (%v), want 1 (cv-b1)", n, err)
	}

	// An unknown id deletes nothing and purges nothing.
	if err := DeletePersona("no-such-persona"); err != nil {
		t.Fatal(err)
	}
	if got := purged(); len(got) != len(want) {
		t.Fatalf("an unknown id purged more: %v", got)
	}
}

func TestDeletePersona_StillDeletesWhenCoreIsDown(t *testing.T) {
	profileID := setupRunsDB(t)
	t.Setenv("CORE_API_URL", "http://127.0.0.1:1")
	t.Setenv("TOOL_SERVICE_TOKEN", "x")
	a, _ := CreatePersona(profileID, "A", "")
	insertCv(t, "cv-a1", a)
	if err := DeletePersona(a); err != nil {
		t.Fatalf("a failing purge failed the delete: %v", err)
	}
	var n int
	if err := db.CareerDB.QueryRow(`SELECT COUNT(*) FROM personas WHERE id = ?`, a).Scan(&n); err != nil || n != 0 {
		t.Fatalf("persona still there (%d, %v)", n, err)
	}
}
