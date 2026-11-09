package localai

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/huggingface"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
)

// A tool suggests a model that ships with Cinqo (here the 7B): it is not looked up and not added to the list again. It is stored as a
// suggestion under the model's own id so the person is asked, and confirming starts the download of the model that is already in the list.

const (
	shippedID   = "qwen2.5-7b-instruct-q4_k_m"
	shippedBody = `{"repo":"bartowski/Qwen2.5-7B-Instruct-GGUF","file":"Qwen2.5-7B-Instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"The best local model for crawl instructions.","name":"Whatever","contextTokens":512}`
)

func newShippedRig(t *testing.T) *confirmRig {
	t.Helper()
	lookedUp := false
	r := &confirmRig{sugRig: newSugRig(t, func(huggingface.Request) (huggingface.Resolved, error) {
		lookedUp = true
		return huggingface.Resolved{}, huggingface.ErrUnreachable
	}), installs: &fakeInstalls{}}
	t.Cleanup(func() {
		if lookedUp {
			t.Error("a shipped model must not be looked up on Hugging Face")
		}
	})
	return r
}

func (r *confirmRig) suggestWith(body string) (int, map[string]any) {
	r.t.Helper()
	return r.sugRig.suggestUsing(r.installs, body)
}

func TestASuggestedShippedModelIsStoredUnderItsOwnIdAndTheToolsWordsAboutItAreIgnored(t *testing.T) {
	r := newShippedRig(t)
	code, got := r.suggestWith(shippedBody)
	if code != http.StatusCreated || got["id"] != shippedID || got["status"] != "suggested" || got["created"] != true {
		t.Fatalf("%d %v", code, got)
	}
	if got["name"] != "Qwen2.5-7B-Instruct" || got["publisher"] != "bartowski" || got["license"] != "Apache-2.0" || got["contextTokens"].(float64) == 512 ||
		got["suggestedBy"] != "Career" || got["why"] != "The best local model for crawl instructions." || got["quantization"] != "Q4_K_M" {
		t.Fatalf("what Cinqo knows about the model wins over what the tool says: %v", got)
	}
	if got["shipped"] != true {
		t.Errorf("a suggestion of a shipped model says so: %v", got["shipped"])
	}
	if got["commit"] != "8911e8a47f92bac19d6f5c64a2e2095bd2f7d031" || got["repo"] != "bartowski/Qwen2.5-7B-Instruct-GGUF" {
		t.Fatalf("%v", got)
	}
	// it waits on the page, and the model list has the shipped model once (it was not added a second time)
	if n := len(r.page()["suggestions"].([]any)); n != 1 {
		t.Fatalf("%d suggestions", n)
	}
	cat, _ := models.Current()
	count := 0
	for _, m := range cat.Models {
		if m.ID == shippedID {
			count++
		}
	}
	if count != 1 || r.count("added") != 0 {
		t.Errorf("the model is in the list once: %d, added rows %d", count, r.count("added"))
	}
	// the same again answers the first one
	if code, again := r.suggestWith(`{"repo":"BARTOWSKI/qwen2.5-7b-instruct-gguf","file":"Qwen2.5-7B-Instruct-Q4_K_M.gguf","suggestedBy":"Browser"}`); code != http.StatusOK || again["created"] != false || again["suggestedBy"] != "Career" {
		t.Errorf("%d %v", code, again)
	}
	if r.count("suggested") != 1 {
		t.Errorf("stored once: %d", r.count("suggested"))
	}
}

func TestAShippedModelThatIsAlreadyDownloadedOrDownloadingIsAnsweredAsSuchAndNothingIsStored(t *testing.T) {
	for state, want := range map[entity.InstallState]string{entity.InstallReady: "installed", entity.InstallDownloading: "downloading"} {
		r := newShippedRig(t)
		r.installs.states = map[string]entity.FileInstall{shippedID: {State: state}}
		code, got := r.suggestWith(shippedBody)
		if code != http.StatusOK || got["status"] != want || got["created"] != false || got["id"] != shippedID {
			t.Errorf("%s: %d %v", want, code, got)
		}
		if r.count("suggested") != 0 {
			t.Errorf("%s: nothing is stored", want)
		}
	}
	// an interrupted download is not downloaded: the person is asked
	r := newShippedRig(t)
	r.installs.states = map[string]entity.FileInstall{shippedID: {State: entity.InstallPartial, PartialBytes: 5}}
	if code, got := r.suggestWith(shippedBody); code != http.StatusCreated || got["status"] != "suggested" {
		t.Errorf("a partial download: %d %v", code, got)
	}
}

func TestConfirmingASuggestedShippedModelStartsItsDownloadAndThenTheSuggestionIsGone(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(shippedBody)
	code, page := r.do("LocalAiInstallModel", map[string]string{"id": shippedID})
	if code != http.StatusAccepted || r.installs.startModel != shippedID || r.installs.starts != 1 {
		t.Fatalf("%d start=%q", code, r.installs.startModel)
	}
	if r.status(shippedID) != "(none)" || len(page["suggestions"].([]any)) != 0 {
		t.Errorf("the suggestion has done its job: %s", r.status(shippedID))
	}
	var found bool
	for _, m := range page["catalog"].([]any) {
		if mm := m.(map[string]any); mm["id"] == shippedID {
			found = mm["origin"] == "catalog"
		}
	}
	if !found {
		t.Error("the model is the shipped one, not an added copy")
	}
	// a download that cannot start keeps the suggestion
	r2 := newShippedRig(t)
	r2.suggestWith(shippedBody)
	r2.installs.startErr = entity.ErrUnsupportedSystem
	if code, _ := r2.do("LocalAiInstallModel", map[string]string{"id": shippedID}); code != http.StatusBadRequest || r2.status(shippedID) != "suggested" {
		t.Errorf("%d %s", code, r2.status(shippedID))
	}
	r2.installs.startErr = os.ErrPermission
	if code, _ := r2.do("LocalAiInstallModel", map[string]string{"id": shippedID}); code != http.StatusInternalServerError || r2.status(shippedID) != "suggested" {
		t.Errorf("%d %s", code, r2.status(shippedID))
	}
}

func TestADismissedSuggestionOfAShippedModelStaysDismissed(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(shippedBody)
	if code, got := r.dismiss(shippedID); code != http.StatusOK || got["status"] != "dismissed" {
		t.Fatalf("%d %v", code, got)
	}
	if n := len(r.page()["suggestions"].([]any)); n != 0 {
		t.Errorf("not listed: %d", n)
	}
	if code, got := r.suggestWith(shippedBody); code != http.StatusOK || got["status"] != "dismissed" || got["created"] != false {
		t.Errorf("not offered again: %d %v", code, got)
	}
	// a shipped model that nobody suggested is still not a suggestion
	if code, _ := newShippedRig(t).dismiss(shippedID); code != http.StatusNotFound {
		t.Errorf("%d", code)
	}
}

func TestAShippedSuggestionCountsTowardsTheCapAndAMatchIsByRepositoryAndFileOnly(t *testing.T) {
	r := newShippedRig(t)
	for i := 1; i <= 20; i++ {
		res := resolved(i)
		row := res.AddedModel("Career", nowForTest())
		r.put(row, entity.AddedStatusSuggested)
	}
	if code, _ := r.suggestWith(shippedBody); code != http.StatusTooManyRequests {
		t.Errorf("at the cap: %d", code)
	}
	// another file of the same repository is not the shipped one: it goes through the lookup
	r2 := newSugRig(t, func(huggingface.Request) (huggingface.Resolved, error) { return resolved(1), nil })
	code, _ := r2.suggest(`{"repo":"bartowski/Qwen2.5-7B-Instruct-GGUF","file":"Qwen2.5-7B-Instruct-Q8_0.gguf"}`)
	if code != http.StatusCreated {
		t.Errorf("another file of the repository is looked up: %d", code)
	}
	if strings.HasPrefix(shippedID, entity.AddedIDPrefix) {
		t.Fatal("the shipped id must not look like an added one")
	}
}

func TestOnlyTheExactFileOfAShippedModelMatchesAndAnAddedModelIsNeverTakenForAShippedOne(t *testing.T) {
	// a file name that is only the start of the shipped one is not that model: it is looked up (and refused as a name)
	r := newSugRig(t, func(req huggingface.Request) (huggingface.Resolved, error) {
		return huggingface.Resolved{}, huggingface.ErrInvalidRequest
	})
	if code, got := r.suggestUsing(&fakeInstalls{}, `{"repo":"bartowski/Qwen2.5-7B-Instruct-GGUF","file":"Qwen2.5-7B-Instruct-Q4_K_M"}`); code != http.StatusBadRequest {
		t.Errorf("a prefix of the file name: %d %v", code, got)
	}
	// a model that was added earlier is found by the lookup like any other file, not by the list of shipped models
	res := resolved(1)
	looked := 0
	r2 := newSugRig(t, func(huggingface.Request) (huggingface.Resolved, error) { looked++; return res, nil })
	added := res.AddedModel("Career", nowForTest())
	added.Status = entity.AddedStatusAdded
	if err := persistent.NewAddedModelsPersistentRepo(r2.db).Insert(&added); err != nil {
		t.Fatal(err)
	}
	code, got := r2.suggestUsing(&fakeInstalls{}, `{"repo":"someone/repo","file":"model-1-q4_k_m.gguf"}`)
	if code != http.StatusOK || got["status"] != "added" || got["id"] != res.ID || looked != 1 {
		t.Errorf("%d %v looked up %d times", code, got, looked)
	}
}
