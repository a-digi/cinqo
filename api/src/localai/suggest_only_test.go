package localai

import (
	"database/sql"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// Qwen2.5-Coder-3B is a model Cinqo knows and measured (it uses tools through its dialect) but does not offer on its own: it is not in the list until a tool
// suggested it and the person confirmed it (plan 220). Nothing about its tools or its dialect changes: only whether it is listed.

const (
	coderID   = "qwen2.5-coder-3b-instruct-q4_k_m"
	coderBody = `{"repo":"Qwen/Qwen2.5-Coder-3B-Instruct-GGUF","file":"qwen2.5-coder-3b-instruct-q4_k_m.gguf","suggestedBy":"Career","why":"The best local model for crawl instructions.","name":"Whatever","contextTokens":512}`
)

// revealCoder confirms the model in a database, the way the person's confirmation does, and makes the list of models read that database.
func revealCoder(t *testing.T, db *sql.DB) {
	t.Helper()
	m := entity.AddedModel{ID: coderID, Repo: "Qwen/Qwen2.5-Coder-3B-Instruct-GGUF", File: "qwen2.5-coder-3b-instruct-q4_k_m.gguf", Revision: strings.Repeat("a", 40), URL: "u", SHA256: strings.Repeat("a", 64),
		SizeBytes: 5, Name: "x", Status: entity.AddedStatusAdded, CreatedAt: "2026-11-09T10:00:00Z"}
	if err := persistent.NewAddedModelsPersistentRepo(db).Insert(&m); err != nil {
		t.Fatal(err)
	}
	models.SetSource(models.FromDB(db, nil))
	t.Cleanup(func() { models.SetSource(nil) })
}

func TestAModelThatIsOnlyOfferedOnSuggestionIsNotInTheListByDefault(t *testing.T) {
	db := openMainDB(t)
	models.SetSource(models.FromDB(db, nil))
	t.Cleanup(func() { models.SetSource(nil) })
	cat, _ := models.Current()
	if _, ok := cat.Find(coderID); ok {
		t.Fatal("not listed by default")
	}
	known, _ := catalog.Default()
	if m, ok := known.Find(coderID); !ok || !m.SuggestOnly || !m.Tools || m.ToolDialect != "qwen-json" || m.KVBytesPerToken == 0 {
		t.Fatalf("Cinqo still knows the model and what was measured about it: %+v", m)
	}
	// not offered to the tools, even with its file on disk
	r := newEngineRig(t)
	r.in.source = models.FromDB(r.db, nil)
	if got := r.e.ReadyModels("tiny"); strings.Contains(strings.Join(got, ","), "coder") {
		t.Errorf("not offered: %v", got)
	}
	// and it cannot be chosen as the model of the settings
	if _, err := entity.Validate(entity.Settings{ModelID: coderID, ContextTokens: 4096, IdleMinutes: 5}, cat, nil); err == nil {
		t.Error("a model that is not in the list cannot be chosen")
	}
	// the page lists three models, without it
	rec := call("LocalAiGet", canRun, db, "")
	if strings.Contains(rec.Body.String(), coderID) {
		t.Error("the page does not list it")
	}
}

func TestAConfirmedModelHasEverythingTheFileSaysAboutIt(t *testing.T) {
	db := openMainDB(t)
	revealCoder(t, db)
	cat, _ := models.Current()
	m, ok := cat.Find(coderID)
	if !ok {
		t.Fatal("confirmed: it is in the list")
	}
	if _, problems := models.FromDB(db, func(error) {})(); problems != nil {
		t.Error("a confirmed model is not a problem")
	}
	var reported []error
	if _, err := models.FromDB(db, func(e error) { reported = append(reported, e) })(); err != nil || len(reported) != 0 {
		t.Errorf("a confirmed model that ships hidden reports nothing: %v %v", err, reported)
	}
	if !m.Tools || m.ToolDialect != "qwen-json" || m.KVBytesPerToken != 36864 || m.ContextTokens != 32768 || m.Name != "Qwen2.5-Coder-3B-Instruct" || m.SHA256 != "724fb256bec1ff062b2f65e4569e871ad2e95ab2a3989723d1769c54294730b7" {
		t.Errorf("the shipped entry is used, not a copy built from the row: %+v", m)
	}
	// a row that is only a suggestion (or dismissed) does not reveal it
	db2 := openMainDB(t)
	row := entity.AddedModel{ID: coderID, Repo: "r/r", File: "f.gguf", Revision: strings.Repeat("b", 40), URL: "u", SHA256: strings.Repeat("b", 64), SizeBytes: 5, Name: "x", Status: entity.AddedStatusSuggested, CreatedAt: "2026-11-09T10:00:00Z"}
	if err := persistent.NewAddedModelsPersistentRepo(db2).Insert(&row); err != nil {
		t.Fatal(err)
	}
	merged, problems := models.FromDB(db2, nil)()
	if _, ok := merged.Find(coderID); ok || problems != nil {
		t.Errorf("a suggestion does not reveal it: %v %v", ok, problems)
	}
}

func TestASuggestionOfTheModelIsAlwaysAskedAboutEvenWhenItsFileIsAlreadyThere(t *testing.T) {
	r := newShippedRig(t)
	r.installs.states = map[string]entity.FileInstall{coderID: {State: entity.InstallReady}}
	code, got := r.suggestWith(coderBody)
	if code != http.StatusCreated || got["id"] != coderID || got["status"] != "suggested" || got["shipped"] != true {
		t.Fatalf("a hidden model is never answered as installed: %d %v", code, got)
	}
	if got["name"] != "Qwen2.5-Coder-3B-Instruct" || got["contextTokens"].(float64) == 512 || got["license"] != "Qwen-Research" || got["publisher"] != "Qwen" {
		t.Errorf("what Cinqo knows wins over what the tool says: %v", got)
	}
	// still not in the list: the person has not said yes
	if inList(t, coderID) {
		t.Error("a suggestion is not in the list")
	}
	if n := len(r.page()["suggestions"].([]any)); n != 1 {
		t.Errorf("%d cards", n)
	}
	// a second tool adds itself to the one suggestion
	_, second := r.suggestWith(`{"repo":"Qwen/Qwen2.5-Coder-3B-Instruct-GGUF","file":"qwen2.5-coder-3b-instruct-q4_k_m.gguf","suggestedBy":"Browser"}`)
	if second["created"] != false || strings.Join(toolsOf(second), ",") != "Career,Browser" || r.count("suggested") != 1 {
		t.Errorf("%v", second)
	}
}

func TestConfirmingTheModelPutsItInTheListStartsItsDownloadOnceAndKeepsWhoSuggestedIt(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(coderBody)
	r.suggestWith(`{"repo":"Qwen/Qwen2.5-Coder-3B-Instruct-GGUF","file":"qwen2.5-coder-3b-instruct-q4_k_m.gguf","suggestedBy":"Browser"}`)
	code, page := r.do("LocalAiInstallModel", map[string]string{"id": coderID})
	if code != http.StatusAccepted || r.installs.startModel != coderID || r.installs.starts != 1 || r.status(coderID) != "added" {
		t.Fatalf("%d start=%q status=%s", code, r.installs.startModel, r.status(coderID))
	}
	if !inList(t, coderID) || len(page["suggestions"].([]any)) != 0 {
		t.Error("it is in the list now and no card is left")
	}
	var entry map[string]any
	for _, m := range page["catalog"].([]any) {
		if mm := m.(map[string]any); mm["id"] == coderID {
			entry = mm
		}
	}
	if entry == nil || entry["origin"] != "added" || entry["addedBy"] != "Career" || strings.Join(strs(entry["suggestedBy"]), ",") != "Career,Browser" || entry["note"] == "" {
		t.Fatalf("listed as added by both tools, with the note of the file: %v", entry)
	}
	// a tool that asks later is remembered, nothing is downloaded again
	_, late := r.suggestWith(`{"repo":"Qwen/Qwen2.5-Coder-3B-Instruct-GGUF","file":"qwen2.5-coder-3b-instruct-q4_k_m.gguf","suggestedBy":"Notes"}`)
	if late["status"] != "added" || strings.Join(toolsOf(late), ",") != "Career,Browser,Notes" || r.installs.starts != 1 {
		t.Errorf("%v starts=%d", late, r.installs.starts)
	}
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestAConfirmationThatCannotStartTheDownloadLeavesTheModelHiddenAsItWas(t *testing.T) {
	for name, failure := range map[string]struct {
		err  error
		want int
	}{"unsupported system": {entity.ErrUnsupportedSystem, http.StatusBadRequest}, "internal": {os.ErrPermission, http.StatusInternalServerError}} {
		r := newShippedRig(t)
		r.suggestWith(coderBody)
		r.installs.startErr = failure.err
		if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": coderID}); code != failure.want || r.status(coderID) != "suggested" || inList(t, coderID) {
			t.Errorf("%s: %d status=%s", name, code, r.status(coderID))
		}
	}
}

func TestADismissedSuggestionOfTheModelStaysDismissedAndItCanStillBeConfirmedLater(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(coderBody)
	if code, got := r.dismiss(coderID); code != http.StatusOK || got["status"] != "dismissed" {
		t.Fatalf("%d %v", code, got)
	}
	if code, got := r.suggestWith(coderBody); code != http.StatusOK || got["status"] != "dismissed" || got["created"] != false {
		t.Errorf("not offered again: %d %v", code, got)
	}
	if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": coderID}); code != http.StatusAccepted || !inList(t, coderID) {
		t.Errorf("the person can still say yes: %d", code)
	}
}

func TestAConfirmedModelCanBeRemovedAndIsHiddenAgainAndNotWhileItIsTheChosenOne(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(coderBody)
	r.do("LocalAiInstallModel", map[string]string{"id": coderID})
	saveSettings(t, r.db, false, coderID)
	if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": coderID}); code != http.StatusConflict || len(r.installs.removedModels) != 0 {
		t.Fatalf("the chosen model: %d", code)
	}
	saveSettings(t, r.db, false, "qwen2.5-1.5b-instruct-q4_k_m")
	code, page := r.do("LocalAiRemoveModel", map[string]string{"id": coderID})
	if code != http.StatusOK || len(r.installs.removedModels) != 1 || r.installs.removedModels[0].ID != coderID {
		t.Fatalf("%d removed=%v", code, r.installs.removedModels)
	}
	if inList(t, coderID) || r.status(coderID) != "(none)" || suggesterRows(t, r.sugRig, coderID) != 0 {
		t.Error("hidden again, its row and its tools forgotten")
	}
	for _, m := range page["catalog"].([]any) {
		if m.(map[string]any)["id"] == coderID {
			t.Error("the answer no longer lists it")
		}
	}
	// and a tool can suggest it again
	if code, got := r.suggestWith(coderBody); code != http.StatusCreated || got["status"] != "suggested" {
		t.Errorf("%d %v", code, got)
	}
}

func TestRemovingEverythingDownloadedHidesTheConfirmedModelAgainAndUnchoosesIt(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(coderBody)
	r.do("LocalAiInstallModel", map[string]string{"id": coderID})
	if _, err := r.db.Exec(`UPDATE local_ai_settings SET enabled = 0, model_id = ?, context_tokens = 8192 WHERE id = 1`, coderID); err != nil {
		t.Fatal(err)
	}
	if code, _ := r.do("LocalAiRemoveDownloads", nil); code != http.StatusNoContent {
		t.Fatalf("%d", code)
	}
	if inList(t, coderID) || r.status(coderID) != "suggested" {
		t.Errorf("a suggestion again, hidden: %s", r.status(coderID))
	}
	if s, _ := query.NewSettingsQueryRepo(r.db).Load(); s.ModelID != "" || s.ContextTokens != 8192 {
		t.Errorf("the chosen model is cleared, nothing else: %+v", s)
	}
}

func TestAnOrdinaryModelThatIsNotMarkedSuggestOnlyIsUnchanged(t *testing.T) {
	// the 7B is in the list by default, and a row with its id (a suggestion of it) never changes that
	db := openMainDB(t)
	models.SetSource(models.FromDB(db, nil))
	t.Cleanup(func() { models.SetSource(nil) })
	if !inList(t, "qwen2.5-7b-instruct-q4_k_m") || !inList(t, "phi-4-mini-instruct-q4_k_m") || !inList(t, "qwen2.5-1.5b-instruct-q4_k_m") {
		t.Error("the three default models are listed")
	}
	if code, _ := newShippedRig(t).suggestWith(shippedBody); code != http.StatusCreated {
		t.Errorf("%d", code)
	}
}

// The same bytes under another repository are the model Cinqo already knows, hidden or not: refused, not added a second time.
func TestTheSameFileUnderAnotherRepositoryIsTheKnownModelEvenWhenItIsHidden(t *testing.T) {
	known, _ := catalog.Default()
	coder, _ := known.Find(coderID)
	res := resolved(1)
	res.SHA256 = coder.SHA256
	r := newSugRig(t, fixed(res))
	if code, got := r.suggest(`{"repo":"someone/repo","file":"model-1-q4_k_m.gguf"}`); code != http.StatusConflict || r.count("suggested") != 0 {
		t.Errorf("%d %v", code, got)
	}
}
