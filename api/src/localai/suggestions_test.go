package localai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/handler"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/huggingface"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// Plan 220, step 3: a tool suggests a model, Cinqo stores the suggestion, nothing is downloaded and nothing is added to the list.

const gigabyte = 1 << 30

// resolved is what the lookup would answer for the n-th file of a repository, without the network.
func resolved(n int) huggingface.Resolved {
	commit := fmt.Sprintf("%040x", n+1)
	file := fmt.Sprintf("model-%d-q4_k_m.gguf", n)
	return huggingface.Resolved{
		ID: entity.AddedID("someone/repo", file, commit), Repo: "someone/repo", File: file, Commit: commit,
		URL:    "https://huggingface.co/someone/repo/resolve/" + commit + "/" + file,
		SHA256: fmt.Sprintf("%064x", n+1), SizeBytes: 2 * gigabyte, License: "apache-2.0", Publisher: "someone", Name: fmt.Sprintf("model-%d-q4_k_m", n), Quantization: "Q4_K_M",
	}
}

type sugRig struct {
	t  *testing.T
	db *sql.DB
}

func newSugRig(t *testing.T, answer func(huggingface.Request) (huggingface.Resolved, error)) *sugRig {
	t.Helper()
	db := openMainDB(t)
	models.SetSource(models.FromDB(db, nil))
	restoreResolver := handler.SetResolver(func(_ context.Context, r huggingface.Request) (huggingface.Resolved, error) { return answer(r) })
	restoreMachine := handler.SetMachine(func() hardware.Info { return hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 * gigabyte} })
	t.Cleanup(func() { restoreResolver(); restoreMachine(); resetModels() })
	return &sugRig{t: t, db: db}
}

// newSugRigOn is a rig on a database that already has rows, with another answer for the lookup.
func newSugRigOn(t *testing.T, db *sql.DB, answer func(huggingface.Request) (huggingface.Resolved, error)) *sugRig {
	t.Helper()
	r := newSugRig(t, answer)
	r.db = db
	models.SetSource(models.FromDB(db, nil))
	return r
}

func (r *sugRig) suggest(body string) (int, map[string]any) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	handler.Handlers(canRun, &fakeInstalls{})["LocalAiSuggest"].ServeHTTP(fakeRequest{w: rec, di: fakeDI{db: r.db}, body: body})
	return rec.Code, message(rec)
}

func (r *sugRig) suggestUsing(installs *fakeInstalls, body string) (int, map[string]any) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	handler.Handlers(canRun, installs)["LocalAiSuggest"].ServeHTTP(fakeRequest{w: rec, di: fakeDI{db: r.db}, body: body})
	return rec.Code, message(rec)
}

func (r *sugRig) dismiss(id string) (int, map[string]any) {
	r.t.Helper()
	rec := httptest.NewRecorder()
	handler.Handlers(canRun, &fakeInstalls{})["LocalAiDismissSuggestion"].ServeHTTP(fakeRequest{w: rec, di: fakeDI{db: r.db}, vars: map[string]string{"id": id}})
	return rec.Code, message(rec)
}

func (r *sugRig) page() map[string]any {
	r.t.Helper()
	_, m := func() (int, map[string]any) {
		rec := call("LocalAiGet", canRun, r.db, "")
		return rec.Code, message(rec)
	}()
	return m
}

func message(rec *httptest.ResponseRecorder) map[string]any {
	var out struct {
		Message any `json:"message"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if m, ok := out.Message.(map[string]any); ok {
		return m
	}
	return map[string]any{"text": out.Message}
}

func (r *sugRig) count(status string) int {
	r.t.Helper()
	n, err := query.NewAddedModelsQueryRepo(r.db).CountByStatus(status)
	if err != nil {
		r.t.Fatal(err)
	}
	return n
}

const valid = `{"repo":"someone/repo","file":"model-1-q4_k_m.gguf","suggestedBy":"Career","why":"the best small model for crawl instructions"}`

func fixed(res huggingface.Resolved) func(huggingface.Request) (huggingface.Resolved, error) {
	return func(huggingface.Request) (huggingface.Resolved, error) { return res, nil }
}

func TestASuggestionIsStoredWithWhatCinqoFoundAndNothingElseHappens(t *testing.T) {
	res := resolved(1)
	r := newSugRig(t, fixed(res))
	code, got := r.suggest(valid)
	if code != http.StatusCreated || got["created"] != true || got["id"] != res.ID || got["status"] != "suggested" {
		t.Fatalf("%d %v", code, got)
	}
	if got["shipped"] != false {
		t.Errorf("an added model is not shipped: %v", got["shipped"])
	}
	if got["commit"] != res.Commit || got["publisher"] != "someone" || got["license"] != "apache-2.0" || got["sizeBytes"].(float64) != 2*gigabyte ||
		got["suggestedBy"] != "Career" || got["why"] != "the best small model for crawl instructions" || got["contextTokens"].(float64) != 4096 || got["fits"] != true {
		t.Fatalf("%v", got)
	}
	// the address and the hash are not sent
	for _, secret := range []string{"url", "sha256"} {
		if _, ok := got[secret]; ok {
			t.Errorf("%s must not be sent", secret)
		}
	}
	// it is a suggestion only: not in the list of models, nothing downloaded, and the page lists it as waiting
	cat, _ := models.Current()
	if _, ok := cat.Find(res.ID); ok {
		t.Error("a suggestion is not part of the list of models")
	}
	page := r.page()
	waiting := page["suggestions"].([]any)
	if len(waiting) != 1 || waiting[0].(map[string]any)["id"] != res.ID {
		t.Fatalf("%v", waiting)
	}
	for _, m := range page["catalog"].([]any) {
		if m.(map[string]any)["id"] == res.ID {
			t.Error("the page's list of models must not include a suggestion")
		}
	}
}

func TestTheSameFileIsStoredOnceAndASecondSuggestionAnswersTheFirst(t *testing.T) {
	r := newSugRig(t, fixed(resolved(1)))
	if code, _ := r.suggest(valid); code != http.StatusCreated {
		t.Fatal(code)
	}
	code, again := r.suggest(`{"repo":"someone/repo","file":"model-1-q4_k_m.gguf","suggestedBy":"Browser","why":"another reason"}`)
	if code != http.StatusOK || again["created"] != false || again["suggestedBy"] != "Career" {
		t.Fatalf("%d %v", code, again)
	}
	if r.count("suggested") != 1 {
		t.Errorf("stored once, found %d", r.count("suggested"))
	}
}

func TestWhatAToolWritesIsCleanedAndBounded(t *testing.T) {
	r := newSugRig(t, fixed(resolved(1)))
	body, _ := json.Marshal(map[string]any{
		"repo": "someone/repo", "file": "model-1-q4_k_m.gguf", "suggestedBy": "  Car\x00eer\n\n  tool  " + strings.Repeat("x", 200), "name": "My\tnice\nmodel", "why": strings.Repeat("é", 400), "contextTokens": 8192,
	})
	code, got := r.suggest(string(body))
	if code != http.StatusCreated {
		t.Fatalf("%d %v", code, got)
	}
	by, name, why := got["suggestedBy"].(string), got["name"].(string), got["why"].(string)
	if len([]rune(by)) != 60 || !strings.HasPrefix(by, "Car eer tool xxx") || strings.ContainsAny(by, "\x00\n\t") {
		t.Errorf("suggestedBy: %q", by)
	}
	if name != "My nice model" {
		t.Errorf("name: %q", name)
	}
	if len([]rune(why)) != 300 {
		t.Errorf("why: %d characters", len([]rune(why)))
	}
	if got["contextTokens"].(float64) != 8192 {
		t.Errorf("the context size the tool stated within the bounds is kept: %v", got["contextTokens"])
	}
}

func TestTheContextSizeMustBeInsideTheBoundsOfTheSettingsPage(t *testing.T) {
	r := newSugRig(t, fixed(resolved(1)))
	for _, ctx := range []int{1, 511, 131073, -5} {
		code, _ := r.suggest(fmt.Sprintf(`{"repo":"someone/repo","file":"model-1-q4_k_m.gguf","contextTokens":%d}`, ctx))
		if code != http.StatusBadRequest {
			t.Errorf("context %d: %d", ctx, code)
		}
	}
	if r.count("suggested") != 0 {
		t.Error("a refused suggestion stores nothing")
	}
	for _, ctx := range []int{512, 131072} {
		r2 := newSugRig(t, fixed(resolved(ctx)))
		if code, _ := r2.suggest(fmt.Sprintf(`{"repo":"someone/repo","file":"model-1-q4_k_m.gguf","contextTokens":%d}`, ctx)); code != http.StatusCreated {
			t.Errorf("context %d: %d", ctx, code)
		}
	}
	if code, _ := r.suggest(`{not json`); code != http.StatusBadRequest {
		t.Errorf("bad body: %d", code)
	}
	if code, _ := r.suggest(`{"repo":"someone/repo","file":"x.gguf","sizeBytes":-1}`); code != http.StatusBadRequest {
		t.Errorf("negative size: %d", code)
	}
}

func TestAtMostTwentySuggestionsWaitAndAKnownFileStillAnswers(t *testing.T) {
	r := newSugRig(t, func(req huggingface.Request) (huggingface.Resolved, error) {
		var n int
		fmt.Sscanf(req.File, "model-%d-q4_k_m.gguf", &n)
		return resolved(n), nil
	})
	for i := 1; i <= handler.MaxPendingSuggestions; i++ {
		if code, got := r.suggest(fmt.Sprintf(`{"repo":"someone/repo","file":"model-%d-q4_k_m.gguf"}`, i)); code != http.StatusCreated {
			t.Fatalf("suggestion %d: %d %v", i, code, got)
		}
	}
	if code, _ := r.suggest(`{"repo":"someone/repo","file":"model-99-q4_k_m.gguf"}`); code != http.StatusTooManyRequests {
		t.Errorf("the 21st: %d", code)
	}
	if r.count("suggested") != handler.MaxPendingSuggestions {
		t.Errorf("%d stored", r.count("suggested"))
	}
	if code, got := r.suggest(`{"repo":"someone/repo","file":"model-3-q4_k_m.gguf"}`); code != http.StatusOK || got["created"] != false {
		t.Errorf("a file that is already known still answers at the cap: %d %v", code, got)
	}
	// dismissing one makes room
	if code, _ := r.dismiss(resolved(1).ID); code != http.StatusOK {
		t.Fatal(code)
	}
	if code, _ := r.suggest(`{"repo":"someone/repo","file":"model-99-q4_k_m.gguf"}`); code != http.StatusCreated {
		t.Errorf("after a dismissal: %d", code)
	}
}

func TestWhyALookupFailedDecidesTheAnswerAndNothingIsStored(t *testing.T) {
	cases := map[error]int{
		huggingface.ErrInvalidRequest: http.StatusBadRequest,
		huggingface.ErrNotFound:       http.StatusNotFound,
		huggingface.ErrNotAvailable:   http.StatusUnprocessableEntity,
		huggingface.ErrNoChecksum:     http.StatusUnprocessableEntity,
		huggingface.ErrNoLicense:      http.StatusUnprocessableEntity,
		huggingface.ErrMismatch:       http.StatusUnprocessableEntity,
		huggingface.ErrInvalid:        http.StatusUnprocessableEntity,
		huggingface.ErrUnreachable:    http.StatusBadGateway,
	}
	for failure, want := range cases {
		r := newSugRig(t, func(huggingface.Request) (huggingface.Resolved, error) { return huggingface.Resolved{}, failure })
		code, got := r.suggest(valid)
		if code != want || got["text"] != failure.Error() {
			t.Errorf("%v: %d %v", failure, code, got)
		}
		if r.count("suggested") != 0 {
			t.Errorf("%v stored something", failure)
		}
	}
}

func TestWhatTheToolStatesIsHandedToTheLookupToBeCheckedNeverStored(t *testing.T) {
	var seen huggingface.Request
	r := newSugRig(t, func(req huggingface.Request) (huggingface.Resolved, error) { seen = req; return resolved(1), nil })
	sum := strings.Repeat("a", 64)
	code, _ := r.suggest(fmt.Sprintf(`{"repo":" someone/repo ","file":"model-1-q4_k_m.gguf","revision":" main ","sha256":"%s","sizeBytes":5}`, sum))
	if code != http.StatusCreated || seen.Repo != "someone/repo" || seen.Revision != "main" || seen.ExpectSHA256 != sum || seen.ExpectSizeBytes != 5 {
		t.Fatalf("%d %+v", code, seen)
	}
	row, err := query.NewAddedModelsQueryRepo(r.db).Find(resolved(1).ID)
	if err != nil || row.SHA256 != resolved(1).SHA256 || row.SizeBytes != 2*gigabyte {
		t.Fatalf("the stored hash and size are the ones found, never the tool's: %+v %v", row, err)
	}
}

func TestAShippedModelIsNotSuggestedAgain(t *testing.T) {
	cat, _ := models.Current()
	shipped := cat.Models[0]
	res := resolved(1)
	res.SHA256 = shipped.SHA256
	r := newSugRig(t, fixed(res))
	if code, _ := r.suggest(valid); code != http.StatusConflict || r.count("suggested") != 0 {
		t.Errorf("%d", code)
	}
}

func TestADismissedSuggestionStaysDismissedAndAToolIsNotAnsweredWithANewOne(t *testing.T) {
	res := resolved(1)
	r := newSugRig(t, fixed(res))
	r.suggest(valid)
	code, got := r.dismiss(res.ID)
	if code != http.StatusOK || got["status"] != "dismissed" {
		t.Fatalf("%d %v", code, got)
	}
	if code, got := r.dismiss(res.ID); code != http.StatusOK || got["status"] != "dismissed" {
		t.Errorf("dismissing twice is fine: %d %v", code, got)
	}
	if n := len(r.page()["suggestions"].([]any)); n != 0 {
		t.Errorf("a dismissed suggestion is not listed: %d", n)
	}
	code, again := r.suggest(valid)
	if code != http.StatusOK || again["created"] != false || again["status"] != "dismissed" || r.count("suggested") != 0 {
		t.Errorf("the same file is not offered again: %d %v", code, again)
	}
}

func TestOnlyASuggestionCanBeDismissed(t *testing.T) {
	r := newSugRig(t, fixed(resolved(1)))
	if code, _ := r.dismiss("added-doesnotexist"); code != http.StatusNotFound {
		t.Errorf("unknown: %d", code)
	}
	cat, _ := models.Current()
	if code, _ := r.dismiss(cat.Models[0].ID); code != http.StatusNotFound {
		t.Errorf("a shipped model is not a suggestion: %d", code)
	}
	added := resolved(2).AddedModel("Career", nowForTest())
	added.Status = entity.AddedStatusAdded
	if err := persistent.NewAddedModelsPersistentRepo(r.db).Insert(&added); err != nil {
		t.Fatal(err)
	}
	if code, _ := r.dismiss(added.ID); code != http.StatusConflict {
		t.Errorf("an added model: %d", code)
	}
	row, _ := query.NewAddedModelsQueryRepo(r.db).Find(added.ID)
	if row.Status != entity.AddedStatusAdded {
		t.Errorf("an added model stays added: %s", row.Status)
	}
}

func TestAnAddedModelIsMarkedAsAddedWithWhoSuggestedItAndTheCatalogOnesAreNot(t *testing.T) {
	r := newSugRig(t, fixed(resolved(1)))
	added := resolved(2).AddedModel("Career", nowForTest())
	added.Status, added.Why = entity.AddedStatusAdded, "for crawl instructions"
	if err := persistent.NewAddedModelsPersistentRepo(r.db).Insert(&added); err != nil {
		t.Fatal(err)
	}
	var sawAdded, sawCatalog bool
	for _, m := range r.page()["catalog"].([]any) {
		mm := m.(map[string]any)
		switch mm["id"] {
		case added.ID:
			sawAdded = mm["origin"] == "added" && mm["addedBy"] == "Career" && mm["publisher"] == "someone" && mm["repo"] == "someone/repo" && mm["why"] == "for crawl instructions" && mm["tools"] == false
		default:
			if mm["origin"] == "catalog" && mm["addedBy"] == nil {
				sawCatalog = true
			} else {
				t.Errorf("a shipped model must say catalog and nothing else: %v", mm["id"])
			}
		}
	}
	if !sawAdded || !sawCatalog {
		t.Errorf("added=%v catalog=%v", sawAdded, sawCatalog)
	}
}

func nowForTest() time.Time { return time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC) }
