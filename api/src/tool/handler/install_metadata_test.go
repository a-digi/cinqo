package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/coco-server/server/request"
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

// Plan 217: the list carries the description (inside metadata) and what each tool declares it needs.
func TestTheListCarriesTheDescriptionAndTheRequiredTools(t *testing.T) {
	di, _ := newUploadDI(t)
	pkg := func(manifest string) []byte {
		return zipOf(t, map[string]string{"manifest.json": manifest, "frontend/bundle.js": "x"})
	}
	for _, m := range []string{
		`{"slug":"browser","name":"Browser","version":"1.0.0","authors":[{"name":"A"}],"license":"MIT","description":"Opens and reads web pages."}`,
		`{"slug":"pdf_tools","name":"PDF Tools","version":"0.0.5","authors":[{"name":"A"}],"license":"MIT"}`,
		`{"slug":"career","name":"Career","version":"2.0.0","authors":[{"name":"A"}],"license":"MIT","description":"Your career workspace.","requires_tools":[{"slug":"pdf_tools","min_version":"0.0.1"},{"slug":"browser"}]}`,
	} {
		if code, out := upload(t, di, pkg(m)); code != http.StatusCreated {
			t.Fatalf("%d %v", code, out)
		}
	}

	rec := httptest.NewRecorder()
	ListHandler(request.NewContext(rec, httptest.NewRequest("GET", "/api/v1/tools", nil), di))
	var out struct {
		Message []struct {
			Slug     string `json:"slug"`
			Metadata *struct {
				Description string `json:"description"`
			} `json:"metadata"`
			RequiredTools []struct {
				Slug       string `json:"slug"`
				MinVersion string `json:"min_version"`
			} `json:"required_tools"`
		} `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	bySlug := map[string]int{}
	for i, m := range out.Message {
		bySlug[m.Slug] = i
	}
	career, browser, pdf := out.Message[bySlug["career"]], out.Message[bySlug["browser"]], out.Message[bySlug["pdf_tools"]]
	if len(career.RequiredTools) != 2 || career.RequiredTools[0].Slug != "pdf_tools" || career.RequiredTools[0].MinVersion != "0.0.1" ||
		career.RequiredTools[1].Slug != "browser" || career.RequiredTools[1].MinVersion != "" {
		t.Errorf("career needs two tools, in the order declared: %+v", career.RequiredTools)
	}
	if career.Metadata == nil || career.Metadata.Description != "Your career workspace." || browser.Metadata == nil || browser.Metadata.Description != "Opens and reads web pages." {
		t.Errorf("descriptions: %+v %+v", career.Metadata, browser.Metadata)
	}
	if pdf.Metadata != nil && pdf.Metadata.Description != "" {
		t.Errorf("a tool that declares none has none: %+v", pdf.Metadata)
	}
	// a tool that needs nothing has an empty list, never null
	if !strings.Contains(rec.Body.String(), `"slug":"browser"`) || strings.Count(rec.Body.String(), `"required_tools":[]`) != 2 {
		t.Errorf("two tools need nothing and must say [] (not null): %s", rec.Body)
	}

	// an update replaces what it declares, wholesale
	upd := pkg(`{"slug":"career","name":"Career","version":"2.0.1","authors":[{"name":"A"}],"license":"MIT","requires_tools":[{"slug":"browser"}]}`)
	code, o := upload(t, di, upd)
	if code != http.StatusOK {
		t.Fatalf("%d %v", code, o)
	}
	msg := o["message"].(map[string]any)
	if rt := msg["required_tools"].([]any); len(rt) != 1 || rt[0].(map[string]any)["slug"] != "browser" {
		t.Errorf("the update answer carries the new requirements: %v", msg["required_tools"])
	}
}

// Plan 217: the single-tool answers (enable, disable) carry the requirements too, in the order the tool declared them.
func TestEnableAndDisableAnswerWithTheRequiredTools(t *testing.T) {
	di, _ := newUploadDI(t)
	di.values["core_port"] = 7026 // enable starts the tool's backend and tells it where core is; a frontend-only tool has none to start
	pkg := func(manifest string) []byte {
		return zipOf(t, map[string]string{"manifest.json": manifest, "frontend/bundle.js": "x"})
	}
	for _, m := range []string{
		`{"slug":"browser","name":"Browser","version":"1.0.0","authors":[{"name":"A"}],"license":"MIT"}`,
		`{"slug":"pdf_tools","name":"PDF Tools","version":"0.0.5","authors":[{"name":"A"}],"license":"MIT"}`,
		`{"slug":"career","name":"Career","version":"2.0.0","authors":[{"name":"A"}],"license":"MIT","requires_tools":[{"slug":"pdf_tools","min_version":"0.0.1"},{"slug":"browser"}]}`,
	} {
		if code, out := upload(t, di, pkg(m)); code != http.StatusCreated {
			t.Fatalf("%d %v", code, out)
		}
	}
	answer := func(h func(request.RequestContext)) []any {
		t.Helper()
		rec := httptest.NewRecorder()
		ctx := request.NewContext(rec, httptest.NewRequest("POST", "/api/v1/tools/career/x", nil), di)
		ctx.GetURI().SetPathVariable("slug", "career")
		h(ctx)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		return out["message"].(map[string]any)["required_tools"].([]any)
	}
	for name, h := range map[string]func(request.RequestContext){"disable": DisableHandler, "enable": EnableHandler} {
		rt := answer(h)
		if len(rt) != 2 || rt[0].(map[string]any)["slug"] != "pdf_tools" || rt[0].(map[string]any)["min_version"] != "0.0.1" || rt[1].(map[string]any)["slug"] != "browser" {
			t.Errorf("%s answers with the requirements in the declared order: %v", name, rt)
		}
	}
	// the fresh-install answer carries them too (checked above through the list; here through the upload answer of a second tool)
	code, out := upload(t, di, pkg(`{"slug":"second","name":"Second","version":"1.0.0","authors":[{"name":"A"}],"license":"MIT","requires_tools":[{"slug":"browser"}]}`))
	if code != http.StatusCreated {
		t.Fatalf("%d %v", code, out)
	}
	if rt := out["message"].(map[string]any)["required_tools"].([]any); len(rt) != 1 || rt[0].(map[string]any)["slug"] != "browser" {
		t.Errorf("a fresh install answers with its requirements: %v", rt)
	}
}
