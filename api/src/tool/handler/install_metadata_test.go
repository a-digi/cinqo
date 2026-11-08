package handler

import (
	"net/http"
	"strings"
	"testing"
)

func TestAnUploadRecordsWhoMadeTheToolAndRefusesAPackageWithoutAuthors(t *testing.T) {
	di, _ := newUploadDI(t)

	// a package that does not say who made it is refused, naming the fields, and nothing is installed
	bare := zipOf(t, map[string]string{
		"manifest.json":      `{"slug":"demo","name":"Demo","version":"1.0.0"}`,
		"frontend/bundle.js": "x",
	})
	code, out := upload(t, di, bare)
	msg, _ := out["message"].(string)
	if code != http.StatusBadRequest || !strings.Contains(msg, "authors") || !strings.Contains(msg, "license") {
		t.Fatalf("%d %v", code, out)
	}
	var n int
	di.db.QueryRow(`SELECT COUNT(*) FROM tools`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d tools installed from a refused package", n)
	}

	// one that does is recorded, and the list shows it
	full := zipOf(t, map[string]string{
		"manifest.json": `{"slug":"demo","name":"Demo","version":"1.0.0",
		  "authors":[{"name":"Ada Lovelace","email":"ada@example.com"},{"name":"Grace Hopper"}],
		  "company":{"name":"Acme GmbH","url":"https://acme.example"},"license":"MIT",
		  "links":{"homepage":"https://acme.example"},"copyright":"© 2026 Acme","category":"browser","keywords":["crawl"]}`,
		"frontend/bundle.js": "x",
	})
	code, out = upload(t, di, full)
	if code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	meta := out["message"].(map[string]any)["metadata"].(map[string]any)
	if len(meta["authors"].([]any)) != 2 || meta["license"] != "MIT" || meta["company"].(map[string]any)["name"] != "Acme GmbH" ||
		meta["copyright"] != "© 2026 Acme" || meta["category"] != "browser" || meta["keywords"].([]any)[0] != "crawl" {
		t.Fatalf("%v", meta)
	}

	// a newer upload replaces it
	next := zipOf(t, map[string]string{
		"manifest.json":      `{"slug":"demo","name":"Demo","version":"1.0.1","authors":[{"name":"Someone New"}],"license":"Apache-2.0"}`,
		"frontend/bundle.js": "y",
	})
	code, out = upload(t, di, next)
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, out)
	}
	meta = out["message"].(map[string]any)["metadata"].(map[string]any)
	if meta["license"] != "Apache-2.0" || meta["authors"].([]any)[0].(map[string]any)["name"] != "Someone New" || meta["company"] != nil {
		t.Fatalf("an update replaces the metadata wholesale: %v", meta)
	}
}
