package profile

import (
	"slices"
	"testing"

	"career-tool-backend/comments/commentstest"
	"career-tool-backend/db"
)

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := db.CareerDB.Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func TestDeleteProfile_PurgesItselfItsPersonasAndTheirCvs(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := db.InitDatabases(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.JobsDB.Close()
		db.CareerDB.Close()
	})
	purged := commentstest.Start(t)
	mine, err := CreateProfile("Ada", "Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := CreateProfile("Bob", "Other")
	for _, r := range [][2]string{{"p1", mine}, {"p2", mine}, {"p3", other}} {
		exec(t, `INSERT INTO personas (id, profile_id, name) VALUES (?, ?, ?)`, r[0], r[1], r[0])
	}
	for _, r := range [][2]string{{"c1", "p1"}, {"c2", "p2"}, {"c3", "p3"}} {
		exec(t, `INSERT INTO cv_documents (id, persona_id, template_id, title, data_json, media_file_id) VALUES (?, ?, 'classic', 'T', '{}', 'm')`, r[0], r[1])
	}

	if err := DeleteProfile(mine); err != nil {
		t.Fatal(err)
	}
	want := []string{"cv:c1", "cv:c2", "persona:p1", "persona:p2", "profile:" + mine}
	slices.Sort(want)
	if got := purged(); !slices.Equal(got, want) {
		t.Fatalf("purged %v, want %v", got, want)
	}
	if err := DeleteProfile("no-such-profile"); err != nil {
		t.Fatal(err)
	}
	if got := purged(); len(got) != len(want) {
		t.Fatalf("an unknown id purged more: %v", got)
	}
}
