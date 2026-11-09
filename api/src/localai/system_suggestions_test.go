package localai

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// Plan 221, step 1: Phi-4-mini is a model Cinqo only offers on suggestion, Cinqo suggests it itself, and a model that is already on the computer is taken over.

type files map[string]entity.FileInstall

func (f files) ModelState(m catalog.Model) entity.FileInstall {
	if st, ok := f[m.ID]; ok {
		return st
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

var ready = entity.FileInstall{State: entity.InstallReady}

func clock() time.Time { return time.Now() }

func reconcile(t *testing.T, r *sugRig, f files) {
	t.Helper()
	known, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	reconcileSuggestOnly(r.db, f, known, clock())
}

func toolNames(t *testing.T, r *sugRig, id string) string {
	t.Helper()
	who, err := query.NewAddedModelsQueryRepo(r.db).Suggesters(id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range who[id] {
		out = append(out, s.Tool)
	}
	return strings.Join(out, ",")
}

func TestPhiIsOnlyOfferedOnSuggestionAndKeepsEverythingMeasuredAboutIt(t *testing.T) {
	known, _ := catalog.Default()
	phi, ok := known.Find(phiID)
	if !ok || !phi.SuggestOnly || !phi.SuggestedByCinqo || phi.SuggestWhy == "" || !phi.Tools || phi.ToolDialect != "phi-json" || phi.Driver != catalog.DriverCode || phi.KVBytesPerToken != 131072 {
		t.Fatalf("hidden, suggested by Cinqo, and what was measured is kept: %+v", phi)
	}
	// Cinqo suggests only Phi itself: not the Coder, not the others
	for _, m := range known.Models {
		if m.SuggestedByCinqo != (m.ID == phiID) {
			t.Errorf("%s: suggestedByCinqo = %v", m.ID, m.SuggestedByCinqo)
		}
	}
	// not listed by default, and not choosable
	r := newSugRig(t, fixed(resolved(1)))
	cat, _ := models.Current()
	if _, listed := cat.Find(phiID); listed {
		t.Error("hidden by default")
	}
	if _, err := entity.Validate(entity.Settings{ModelID: phiID, ContextTokens: 4096, IdleMinutes: 5}, cat, nil); err == nil {
		t.Error("a hidden model cannot be chosen")
	}
	_ = r
}

func TestCinqoSuggestsPhiItselfWhenItIsNotOnTheComputerAndNothingElse(t *testing.T) {
	r := newConfirmRig(t)
	reconcile(t, r.sugRig, files{})
	cards := r.page()["suggestions"].([]any)
	if len(cards) != 1 {
		t.Fatalf("one suggestion, Phi: %v", cards)
	}
	card := cards[0].(map[string]any)
	if card["id"] != phiID || card["shipped"] != true || card["status"] != "suggested" || strings.Join(toolsOf(card), ",") != "Cinqo" {
		t.Fatalf("%v", card)
	}
	who := card["suggesters"].([]any)[0].(map[string]any)
	if who["why"] != "The better answers of the local models, and the one used for the CV import. Needs the most memory." {
		t.Errorf("Cinqo's own reason: %v", who["why"])
	}
	if card["suggestedBy"] != "Cinqo" {
		t.Errorf("%v", card["suggestedBy"])
	}
	if inList(t, phiID) || r.count("suggested") != 1 || r.count("added") != 0 {
		t.Error("a suggestion is not in the list, and nothing else was added")
	}
	// nothing is downloaded by this
	if r.installs.starts != 0 {
		t.Error("nothing is downloaded")
	}
}

func TestTheReconciliationIsIdempotentAndLeavesWhatThePersonDecidedAlone(t *testing.T) {
	r := newConfirmRig(t)
	for i := 0; i < 3; i++ {
		reconcile(t, r.sugRig, files{})
	}
	if r.count("suggested") != 1 || toolNames(t, r.sugRig, phiID) != "Cinqo" {
		t.Fatalf("once however often it runs: %d %q", r.count("suggested"), toolNames(t, r.sugRig, phiID))
	}
	// dismissed: stays dismissed, is not suggested again and not taken over, even with the file there
	r.dismiss(phiID)
	reconcile(t, r.sugRig, files{})
	reconcile(t, r.sugRig, files{phiID: ready})
	if r.status(phiID) != "dismissed" || inList(t, phiID) {
		t.Errorf("the person's no stays: %s", r.status(phiID))
	}
	// confirmed: stays confirmed, nothing is added to it
	r2 := newConfirmRig(t)
	reconcile(t, r2.sugRig, files{})
	r2.do("LocalAiInstallModel", map[string]string{"id": phiID})
	reconcile(t, r2.sugRig, files{})
	if r2.status(phiID) != "added" || toolNames(t, r2.sugRig, phiID) != "Cinqo" || !inList(t, phiID) {
		t.Errorf("%s %q", r2.status(phiID), toolNames(t, r2.sugRig, phiID))
	}
}

func TestAModelAlreadyOnTheComputerIsTakenOverAndNothingIsDownloaded(t *testing.T) {
	r := newConfirmRig(t)
	// the models that are in the list anyway are never touched, even with their files there
	reconcile(t, r.sugRig, files{phiID: ready, coderID: ready, "qwen2.5-7b-instruct-q4_k_m": ready, "qwen2.5-1.5b-instruct-q4_k_m": ready})
	if r.count("added") != 2 || r.count("suggested") != 0 {
		t.Errorf("only the two models Cinqo offers on suggestion get a row: %d added", r.count("added"))
	}
	for _, id := range []string{phiID, coderID} {
		if r.status(id) != "added" || !inList(t, id) || toolNames(t, r.sugRig, id) != "Cinqo" {
			t.Errorf("%s: %s %q", id, r.status(id), toolNames(t, r.sugRig, id))
		}
	}
	if len(r.page()["suggestions"].([]any)) != 0 || r.installs.starts != 0 {
		t.Error("no card is left and nothing is downloaded")
	}
	// the entry in the list is the shipped one with everything measured: tools, dialect, driver
	cat, _ := models.Current()
	phi, _ := cat.Find(phiID)
	if !phi.Tools || phi.ToolDialect != "phi-json" || phi.Driver != catalog.DriverCode {
		t.Errorf("%+v", phi)
	}
	// the page lists it as added by Cinqo
	var entry map[string]any
	for _, m := range r.page()["catalog"].([]any) {
		if mm := m.(map[string]any); mm["id"] == phiID {
			entry = mm
		}
	}
	if entry == nil || entry["origin"] != "added" || entry["addedBy"] != "Cinqo" || strings.Join(strs(entry["suggestedBy"]), ",") != "Cinqo" {
		t.Errorf("%v", entry)
	}
}

func TestOnlyACompleteFileIsTakenOver(t *testing.T) {
	for state, st := range map[string]entity.FileInstall{
		"partial":     {State: entity.InstallPartial, PartialBytes: 5},
		"downloading": {State: entity.InstallDownloading, DownloadID: "d"},
		"nothing":     {State: entity.InstallNotDownloaded},
	} {
		r := newConfirmRig(t)
		reconcile(t, r.sugRig, files{coderID: st})
		if r.status(coderID) != "(none)" || inList(t, coderID) {
			t.Errorf("%s: %s", state, r.status(coderID))
		}
	}
}

func TestAToolThatSuggestedFirstAndTheFileOnDiskAreBothKept(t *testing.T) {
	// Career suggested Phi before the update, the file is there: taken over, with both
	r := newShippedRig(t)
	r.suggestWith(`{"repo":"unsloth/Phi-4-mini-instruct-GGUF","file":"Phi-4-mini-instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"for the CV import"}`)
	if r.status(phiID) != "suggested" {
		t.Fatalf("%s", r.status(phiID))
	}
	reconcile(t, r.sugRig, files{phiID: ready})
	if r.status(phiID) != "added" || toolNames(t, r.sugRig, phiID) != "Career,Cinqo" || !inList(t, phiID) {
		t.Errorf("%s %q", r.status(phiID), toolNames(t, r.sugRig, phiID))
	}
	// and when the file is not there: Cinqo joins Career on the one suggestion
	r2 := newShippedRig(t)
	r2.suggestWith(`{"repo":"unsloth/Phi-4-mini-instruct-GGUF","file":"Phi-4-mini-instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"for the CV import"}`)
	reconcile(t, r2.sugRig, files{})
	if toolNames(t, r2.sugRig, phiID) != "Career,Cinqo" || r2.count("suggested") != 1 {
		t.Errorf("%q %d", toolNames(t, r2.sugRig, phiID), r2.count("suggested"))
	}
	// the other way round: Cinqo first, then Career asks: one card, both
	r3 := newShippedRig(t)
	reconcile(t, r3.sugRig, files{})
	_, got := r3.suggestWith(`{"repo":"unsloth/Phi-4-mini-instruct-GGUF","file":"Phi-4-mini-instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"for the CV import"}`)
	if got["status"] != "suggested" || got["created"] != false || strings.Join(toolNames2(got), ",") != "Cinqo,Career" {
		t.Errorf("%v", got)
	}
}

func toolNames2(m map[string]any) []string { return toolsOf(m) }

func TestPhiIsSuggestedByCareerThroughTheSameRouteAndConfirmedLikeTheCoder(t *testing.T) {
	r := newShippedRig(t)
	code, got := r.suggestWith(`{"repo":"unsloth/Phi-4-mini-instruct-GGUF","file":"Phi-4-mini-instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"for the CV import"}`)
	if code != http.StatusCreated || got["id"] != phiID || got["shipped"] != true || got["name"] != "Phi-4-mini-instruct" || got["publisher"] != "unsloth" {
		t.Fatalf("%d %v", code, got)
	}
	// a file already on the disk does not make it "installed": it is asked about, and the confirmation takes the file over
	r.installs.states = map[string]entity.FileInstall{phiID: ready}
	if code, again := r.suggestWith(`{"repo":"unsloth/Phi-4-mini-instruct-GGUF","file":"Phi-4-mini-instruct-Q4_K_M.gguf","suggestedBy":"Browser"}`); code != http.StatusOK || again["status"] != "suggested" {
		t.Errorf("%d %v", code, again)
	}
	if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": phiID}); code != http.StatusAccepted || r.status(phiID) != "added" || !inList(t, phiID) || r.installs.starts != 1 {
		t.Errorf("%d %s", code, r.status(phiID))
	}
}

func TestRemovingPhiHidesItAgainAndCinqoSuggestsItAgainAtTheNextStart(t *testing.T) {
	r := newConfirmRig(t)
	reconcile(t, r.sugRig, files{})
	r.do("LocalAiInstallModel", map[string]string{"id": phiID})
	saveSettings(t, r.db, false, "qwen2.5-1.5b-instruct-q4_k_m")
	if code, _ := r.do("LocalAiRemoveModel", map[string]string{"id": phiID}); code != http.StatusOK || inList(t, phiID) || r.status(phiID) != "(none)" {
		t.Fatalf("%d", code)
	}
	reconcile(t, r.sugRig, files{})
	if r.status(phiID) != "suggested" || toolNames(t, r.sugRig, phiID) != "Cinqo" {
		t.Errorf("suggested again at the next start: %s %q", r.status(phiID), toolNames(t, r.sugRig, phiID))
	}
}

func TestACatalogEntryMustBeConsistentAboutWhoSuggestsIt(t *testing.T) {
	good := func() catalog.Model {
		known, _ := catalog.Default()
		m, _ := known.Find(phiID)
		return m
	}
	if err := catalog.ValidateModel(good()); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*catalog.Model){
		"suggested by Cinqo but not suggestOnly": func(m *catalog.Model) { m.SuggestOnly = false },
		"no reason":                              func(m *catalog.Model) { m.SuggestWhy = "  " },
		"a reason that is too long":              func(m *catalog.Model) { m.SuggestWhy = strings.Repeat("x", 301) },
		"a reason without being suggested":       func(m *catalog.Model) { m.SuggestedByCinqo = false },
	} {
		m := good()
		mutate(&m)
		if err := catalog.ValidateModel(m); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}

func TestTheRowOfAShippedModelComesFromTheFileAndNothingTheToolSays(t *testing.T) {
	known, _ := catalog.Default()
	phi, _ := known.Find(phiID)
	row := entity.FromCatalog(phi, entity.AddedStatusSuggested, "2026-11-09T12:00:00Z")
	if row.ID != phiID || row.Repo != "unsloth/Phi-4-mini-instruct-GGUF" || row.File != "Phi-4-mini-instruct-Q4_K_M.gguf" || row.Revision != "78eb92a46fc37e6b524df991ed9aca9bc6aa7b80" ||
		row.SHA256 != phi.SHA256 || row.SizeBytes != phi.SizeBytes || row.Publisher != "unsloth" || row.Name != "Phi-4-mini-instruct" || row.ContextTokens != 131072 || row.Status != "suggested" {
		t.Errorf("%+v", row)
	}
	_ = persistent.NewAddedModelsPersistentRepo
}

// A model a tool suggested and the person dismissed does not gather Cinqo as a suggester either.
func TestADismissedSuggestionDoesNotGatherCinqoAsASuggester(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(`{"repo":"unsloth/Phi-4-mini-instruct-GGUF","file":"Phi-4-mini-instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"for the CV import"}`)
	r.dismiss(phiID)
	reconcile(t, r.sugRig, files{})
	if toolNames(t, r.sugRig, phiID) != "Career" || r.status(phiID) != "dismissed" {
		t.Errorf("%q %s", toolNames(t, r.sugRig, phiID), r.status(phiID))
	}
}
