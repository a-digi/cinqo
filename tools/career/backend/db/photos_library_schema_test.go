package db

import (
	"testing"

	_ "modernc.org/sqlite"
)

// Photos already linked by profiles become library entries, once each, and the
// backfill can run again without changing anything.
func TestMigratePhotosLibrary_BackfillsLinkedPhotos(t *testing.T) {
	t.Setenv("TOOL_DB_DIR", t.TempDir())
	if err := InitDatabases(); err != nil {
		t.Fatal(err)
	}
	defer JobsDB.Close()
	defer CareerDB.Close()

	for _, row := range [][]any{
		{"a", "media-1"}, // two profiles share one file
		{"b", "media-1"},
		{"c", "media-2"},
		{"d", nil}, // no photo
		{"e", ""},  // an empty id is not a photo
	} {
		if _, err := CareerDB.Exec(`INSERT INTO profiles (id, image_media_file_id) VALUES (?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := migratePhotosLibrary(CareerDB); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	rows, err := CareerDB.Query(`SELECT media_file_id FROM photos ORDER BY media_file_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if len(got) != 2 || got[0] != "media-1" || got[1] != "media-2" {
		t.Fatalf("library = %v, want [media-1 media-2]", got)
	}
}
