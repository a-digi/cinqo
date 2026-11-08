package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
)

// Plan dependency-install, step 6: an update that cannot be applied leaves the old version as it was: files, version, requirements, status, and its
// backend started again when it was enabled.
func TestAFailedUpdateLeavesTheOldVersionInPlace(t *testing.T) {
	base := `"authors":[{"name":"A"}],"license":"MIT"`
	pkg := func(t *testing.T, version, requires string) []byte {
		return zipOf(t, map[string]string{
			"manifest.json":      `{"slug":"career","name":"Career","version":"` + version + `",` + base + `,"requires_tools":` + requires + `}`,
			"frontend/bundle.js": "bundle-" + version,
		})
	}
	cases := []struct {
		name   string
		break_ func(t *testing.T, di *uploadDI)
	}{
		{"the second rename fails", func(t *testing.T, di *uploadDI) {
			orig := renameDir
			n := 0
			renameDir = func(a, b string) error {
				n++
				if n == 2 {
					return errors.New("boom")
				}
				return orig(a, b)
			}
			t.Cleanup(func() { renameDir = orig })
		}},
		{"the executable cannot be made runnable", func(t *testing.T, di *uploadDI) {
			orig := chmodBackend
			chmodBackend = func(string) error { return errors.New("boom") }
			t.Cleanup(func() { chmodBackend = orig })
		}},
		{"the database refuses the new version", func(t *testing.T, di *uploadDI) {
			if _, err := di.db.Exec(`CREATE TRIGGER no_update BEFORE UPDATE ON tools WHEN NEW.version = '1.0.1' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
				t.Fatal(err)
			}
		}},
		{"the database refuses the new requirements", func(t *testing.T, di *uploadDI) {
			if _, err := di.db.Exec(`CREATE TRIGGER no_req BEFORE INSERT ON tool_required_tools WHEN NEW.max_version = '5.0.0' BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			di, dir := newUploadDI(t)
			di.values["core_port"] = 7030
			if code, out := upload(t, di, zipOf(t, map[string]string{"manifest.json": `{"slug":"browser","name":"Browser","version":"1.0.0",` + base + `}`, "frontend/bundle.js": "x"})); code != http.StatusCreated {
				t.Fatalf("%d %v", code, out)
			}
			if code, out := upload(t, di, pkg(t, "1.0.0", `[{"slug":"browser","min_version":"1.0.0"}]`)); code != http.StatusCreated {
				t.Fatalf("%d %v", code, out)
			}
			var stopped, started int
			origStop, origStart := stopTool, startTool
			stopTool = func(_ *sql.DB, _ string) error { stopped++; return nil }
			startTool = func(_ *sql.DB, _ string, _ tool_entity.Tool, _ int) error { started++; return nil }
			t.Cleanup(func() { stopTool, startTool = origStop, origStart })
			if _, err := di.db.Exec(`UPDATE tools SET enabled = 1, status = 'running' WHERE slug = 'career'`); err != nil {
				t.Fatal(err)
			}
			c.break_(t, di)

			update := pkg(t, "1.0.1", `[{"slug":"browser","max_version":"5.0.0"}]`)
			if c.name != "the database refuses the new requirements" {
				update = pkg(t, "1.0.1", `[{"slug":"browser"}]`)
			}
			if code, out := upload(t, di, update); code == http.StatusOK {
				t.Fatalf("the update must fail: %v", out)
			}

			var version, status, min string
			di.db.QueryRow(`SELECT version, status FROM tools WHERE slug = 'career'`).Scan(&version, &status)
			di.db.QueryRow(`SELECT min_version FROM tool_required_tools WHERE tool_id = (SELECT id FROM tools WHERE slug='career')`).Scan(&min)
			if version != "1.0.0" || status != "running" || min != "1.0.0" {
				t.Errorf("the record must be the old one: version=%q status=%q min=%q", version, status, min)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "tools", "career", "frontend", "bundle.js"))
			if err != nil || !strings.Contains(string(raw), "bundle-1.0.0") {
				t.Errorf("the old files must be back: %q %v", raw, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "tools", "career.prev")); err == nil {
				t.Error("no .prev may be left behind")
			}
			if stopped != 1 || started != 1 {
				t.Errorf("the old backend is stopped once and started again once: stop=%d start=%d", stopped, started)
			}
		})
	}
}
