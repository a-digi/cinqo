package localai

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/handler"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/huggingface"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// Plan 220, step 4: a person confirms a suggestion (it is added to the list of models and its download starts) or removes one added model.

type confirmRig struct {
	*sugRig
	installs *fakeInstalls
}

func newConfirmRig(t *testing.T) *confirmRig {
	t.Helper()
	return &confirmRig{sugRig: newSugRig(t, func(huggingface.Request) (huggingface.Resolved, error) { return resolved(1), nil }), installs: &fakeInstalls{}}
}

func (r *confirmRig) do(name string, vars map[string]string) (int, map[string]any) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	handler.Handlers(canRun, r.installs)[name].ServeHTTP(fakeRequest{w: rec, di: fakeDI{db: r.db}, vars: vars})
	return rec.Code, message(rec)
}

func (r *confirmRig) status(id string) string {
	r.t.Helper()
	m, err := query.NewAddedModelsQueryRepo(r.db).Find(id)
	if err != nil {
		return "(none)"
	}
	return m.Status
}

func (r *confirmRig) put(m entity.AddedModel, status string) entity.AddedModel {
	r.t.Helper()
	m.Status = status
	if err := persistent.NewAddedModelsPersistentRepo(r.db).Insert(&m); err != nil {
		r.t.Fatal(err)
	}
	return m
}

func inList(t *testing.T, id string) bool {
	t.Helper()
	cat, err := models.Current()
	if err != nil {
		t.Fatal(err)
	}
	_, ok := cat.Find(id)
	return ok
}

func TestConfirmingASuggestionAddsTheModelToTheListAndStartsItsDownload(t *testing.T) {
	r := newConfirmRig(t)
	res := resolved(1)
	if code, _ := r.suggest(valid); code != http.StatusCreated {
		t.Fatal(code)
	}
	if inList(t, res.ID) {
		t.Fatal("a suggestion is not in the list yet")
	}
	code, page := r.do("LocalAiInstallModel", map[string]string{"id": res.ID})
	if code != http.StatusAccepted || r.status(res.ID) != "added" || r.installs.startModel != res.ID || r.installs.starts != 1 {
		t.Fatalf("%d status=%s start=%q %v", code, r.status(res.ID), r.installs.startModel, page)
	}
	if r.installs.startSystem.OS != "darwin" {
		t.Errorf("the download is for this computer: %+v", r.installs.startSystem)
	}
	if !inList(t, res.ID) {
		t.Error("the confirmed model is in the list of models")
	}
	if n := len(page["suggestions"].([]any)); n != 0 {
		t.Errorf("it no longer waits as a suggestion: %d", n)
	}
	var found bool
	for _, m := range page["catalog"].([]any) {
		if mm := m.(map[string]any); mm["id"] == res.ID {
			found = mm["origin"] == "added" && mm["addedBy"] == "Career"
		}
	}
	if !found {
		t.Error("the answer lists it as an added model")
	}
}

func TestADismissedSuggestionCanStillBeConfirmedAndAnAddedOneIsDownloadedAgain(t *testing.T) {
	r := newConfirmRig(t)
	res := resolved(1)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusDismissed)
	if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": res.ID}); code != http.StatusAccepted || r.status(res.ID) != "added" {
		t.Fatalf("a dismissed suggestion: %d %s", code, r.status(res.ID))
	}
	if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": res.ID}); code != http.StatusAccepted || r.installs.starts != 2 || r.status(res.ID) != "added" {
		t.Errorf("an added model is downloaded again: %d starts=%d", code, r.installs.starts)
	}
}

func TestAConfirmationThatCannotStartTheDownloadLeavesTheModelAsItWas(t *testing.T) {
	for name, failure := range map[string]struct {
		err  error
		want int
	}{"unsupported system": {entity.ErrUnsupportedSystem, http.StatusBadRequest}, "unknown": {entity.ErrUnknownModel, http.StatusBadRequest}, "internal": {os.ErrPermission, http.StatusInternalServerError}} {
		r := newConfirmRig(t)
		res := resolved(1)
		r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusSuggested)
		r.installs.startErr = failure.err
		code, _ := r.do("LocalAiInstallModel", map[string]string{"id": res.ID})
		if code != failure.want || r.status(res.ID) != "suggested" || inList(t, res.ID) {
			t.Errorf("%s: %d status=%s", name, code, r.status(res.ID))
		}
	}
	// a model that was already added stays added when the download cannot start
	r := newConfirmRig(t)
	res := resolved(2)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	r.installs.startErr = os.ErrPermission
	if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": res.ID}); code != http.StatusInternalServerError || r.status(res.ID) != "added" {
		t.Errorf("%d %s", code, r.status(res.ID))
	}
}

func TestOnlyASuggestedModelCanBeConfirmed(t *testing.T) {
	r := newConfirmRig(t)
	cat, _ := models.Current()
	for _, id := range []string{"added-nothere", cat.Models[0].ID, ""} {
		if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": id}); code != http.StatusNotFound || r.installs.starts != 0 {
			t.Errorf("%q: %d", id, code)
		}
	}
}

func TestRemovingAnAddedModelRemovesItsFileAndItsRow(t *testing.T) {
	r := newConfirmRig(t)
	res := resolved(1)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	code, page := r.do("LocalAiRemoveModel", map[string]string{"id": res.ID})
	if code != http.StatusOK || r.status(res.ID) != "(none)" || inList(t, res.ID) {
		t.Fatalf("%d status=%s", code, r.status(res.ID))
	}
	if len(r.installs.removedModels) != 1 || r.installs.removedModels[0].ID != res.ID || r.installs.removedModels[0].SizeBytes != res.SizeBytes {
		t.Errorf("the file of that model is removed: %+v", r.installs.removedModels)
	}
	for _, m := range page["catalog"].([]any) {
		if m.(map[string]any)["id"] == res.ID {
			t.Error("the answer no longer lists it")
		}
	}
}

func TestAModelThatCannotBeRemovedStaysWithItsRow(t *testing.T) {
	res := resolved(1)
	// the model chosen on the page
	r := newConfirmRig(t)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	saveSettings(t, r.db, false, res.ID)
	if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": res.ID}); code != http.StatusConflict || r.status(res.ID) != "added" || len(r.installs.removedModels) != 0 {
		t.Errorf("the chosen model: %d", code)
	}
	// being downloaded
	r = newConfirmRig(t)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	r.installs.removeModelErr = entity.ErrDownloadRunning
	if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": res.ID}); code != http.StatusConflict || r.status(res.ID) != "added" {
		t.Errorf("downloading: %d", code)
	}
	// the file cannot be removed
	r = newConfirmRig(t)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	r.installs.removeModelErr = os.ErrPermission
	if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": res.ID}); code != http.StatusInternalServerError || r.status(res.ID) != "added" {
		t.Errorf("file error: %d", code)
	}
	// not added: dismiss it instead; unknown and shipped ids are not found
	r = newConfirmRig(t)
	r.put(res.AddedModel("Career", nowForTest()), entity.AddedStatusSuggested)
	if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": res.ID}); code != http.StatusConflict || r.status(res.ID) != "suggested" {
		t.Errorf("a suggestion: %d", code)
	}
	cat, _ := models.Current()
	for _, id := range []string{"added-nothere", cat.Models[0].ID} {
		if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": id}); code != http.StatusNotFound {
			t.Errorf("%q: %d", id, code)
		}
	}
}

func TestRemovingEverythingDownloadedPutsAddedModelsBackToSuggestionsAndUnchoosesTheirs(t *testing.T) {
	r := newConfirmRig(t)
	a := r.put(resolved(1).AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	b := r.put(resolved(2).AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	if _, err := r.db.Exec(`UPDATE local_ai_settings SET enabled = 0, model_id = ?, context_tokens = 8192, idle_minutes = 7 WHERE id = 1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := r.do("LocalAiRemoveDownloads", nil); code != http.StatusNoContent || !r.installs.removed {
		t.Fatalf("%d", code)
	}
	if r.status(a.ID) != "suggested" || r.status(b.ID) != "suggested" || inList(t, a.ID) {
		t.Errorf("added models are suggestions again: %s %s", r.status(a.ID), r.status(b.ID))
	}
	s, _ := query.NewSettingsQueryRepo(r.db).Load()
	if s.ModelID != "" || s.ContextTokens != 8192 || s.IdleMinutes != 7 || s.Enabled {
		t.Errorf("the chosen model is cleared and nothing else changes: %+v", s)
	}
	// a shipped model chosen on the page stays chosen
	r2 := newConfirmRig(t)
	r2.put(resolved(3).AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	saveSettings(t, r2.db, false, "qwen2.5-1.5b-instruct-q4_k_m")
	r2.do("LocalAiRemoveDownloads", nil)
	if s, _ := query.NewSettingsQueryRepo(r2.db).Load(); s.ModelID != "qwen2.5-1.5b-instruct-q4_k_m" {
		t.Errorf("a shipped model stays chosen: %q", s.ModelID)
	}
	// when the files could not be removed nothing changes
	r3 := newConfirmRig(t)
	c := r3.put(resolved(4).AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	r3.installs.removeErr = os.ErrPermission
	if code, _ := r3.do("LocalAiRemoveDownloads", nil); code == http.StatusNoContent || r3.status(c.ID) != "added" {
		t.Errorf("%d %s", code, r3.status(c.ID))
	}
	// and while the local AI is on it is refused, with nothing demoted
	r4 := newConfirmRig(t)
	d := r4.put(resolved(5).AddedModel("Career", nowForTest()), entity.AddedStatusAdded)
	saveSettings(t, r4.db, true, "qwen2.5-1.5b-instruct-q4_k_m")
	if code, _ := r4.do("LocalAiRemoveDownloads", nil); code != http.StatusConflict || r4.status(d.ID) != "added" {
		t.Errorf("%d %s", code, r4.status(d.ID))
	}
}

// The installer removes one model's file and its unfinished download, and refuses while that model is being downloaded.
func TestTheInstallerRemovesOneModelsFileAndRefusesWhileItIsBeingDownloaded(t *testing.T) {
	r := newRig(t, nil)
	big := strings.Repeat("m", 2<<20)
	r.srv.files[modelURL] = []byte(big)
	m := addedModel("added-removeme", entity.AddedStatusAdded, int64(len(big)), sha([]byte(big))).CatalogModel()
	r.in.catalog.Models = append(r.in.catalog.Models, m)
	other := r.model
	dest := r.in.ModelPath(m)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dest, dest + ".part", r.in.ModelPath(other)} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.in.RemoveModel(m); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must be gone", p)
		}
	}
	if _, err := os.Stat(r.in.ModelPath(other)); err != nil {
		t.Error("another model's file is left alone")
	}
	if err := r.in.RemoveModel(m); err != nil {
		t.Errorf("removing what is not there is fine: %v", err)
	}
	// while it is being downloaded
	r.srv.hold = make(chan struct{})
	if err := r.in.Start(m.ID, mac); err != nil {
		t.Fatal(err)
	}
	r.wait(t, "the download to begin", func() bool { return r.srv.blocked.Load() > 0 })
	if err := r.in.RemoveModel(m); err != entity.ErrDownloadRunning {
		t.Errorf("a running download: %v", err)
	}
	close(r.srv.hold)
	r.allDone(t)
}

// A downloaded added model is offered to every picker like a shipped one, and only while it is added.
func TestADownloadedAddedModelIsOfferedToThePickersAndASuggestionIsNot(t *testing.T) {
	r := newEngineRig(t)
	macOnly := hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 64 << 30}
	r.e.system = func() hardware.Info { return macOnly }
	add := addedModel("added-offered", entity.AddedStatusAdded, 2<<20, strings.Repeat("e", 64))
	sug := addedModel("added-waiting", entity.AddedStatusSuggested, 2<<20, strings.Repeat("f", 64))
	sug.Revision = strings.Repeat("9", 40)
	sug.URL = "https://huggingface.co/" + addedRepo + "/resolve/" + sug.Revision + "/" + addedFile
	for _, m := range []entity.AddedModel{add, sug} {
		m := m
		if err := persistent.NewAddedModelsPersistentRepo(r.db).Insert(&m); err != nil {
			t.Fatal(err)
		}
	}
	r.in.source = models.FromDB(r.db, nil)
	// the merged list has the shipped runtime of this system: put its program in place
	rt, ok := shipped(t).RuntimeFor("darwin", "arm64")
	if !ok {
		t.Fatal("no shipped runtime for this system")
	}
	if err := os.MkdirAll(filepath.Dir(r.in.RuntimeBinary(rt)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.in.RuntimeBinary(rt), []byte("#!fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := r.e.ReadyModels("tiny"); strings.Contains(strings.Join(got, ","), "added-") {
		t.Fatalf("not downloaded yet: %v", got)
	}
	r.install(t, add.CatalogModel())
	r.install(t, sug.CatalogModel())
	got := r.e.ReadyModels("tiny")
	if !strings.Contains(strings.Join(got, ","), "added-offered") || strings.Contains(strings.Join(got, ","), "added-waiting") {
		t.Errorf("a downloaded added model is offered, a suggestion is not: %v", got)
	}
}
