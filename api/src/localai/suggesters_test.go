package localai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/huggingface"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// Plan 220, step 9: a model suggested by many tools is one model: one card, one download. Every tool that suggested it is listed, in its own words.

func asTool(tool, why string) string {
	b, _ := json.Marshal(map[string]string{"repo": "someone/repo", "file": "model-1-q4_k_m.gguf", "suggestedBy": tool, "why": why})
	return string(b)
}

func toolsOf(m map[string]any) []string {
	var out []string
	for _, s := range m["suggesters"].([]any) {
		out = append(out, s.(map[string]any)["tool"].(string))
	}
	return out
}

func suggesterRows(t *testing.T, r *sugRig, id string) int {
	t.Helper()
	n := 0
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM local_ai_model_suggesters WHERE model_id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAModelSuggestedByManyToolsIsOneCardWithEveryToolListed(t *testing.T) {
	res := resolved(1)
	r := newConfirmRig(t)
	code, first := r.suggest(asTool("Career", "for crawl instructions"))
	if code != http.StatusCreated || first["created"] != true {
		t.Fatalf("%d %v", code, first)
	}
	code, second := r.suggest(asTool("Browser", "for reading pages"))
	if code != http.StatusOK || second["created"] != false || second["id"] != res.ID {
		t.Fatalf("a second tool answers the first suggestion: %d %v", code, second)
	}
	r.suggest(asTool("PDF Tools", ""))
	// the same tool again (any case) is listed once and its first words stay
	_, again := r.suggest(asTool("career", "other words"))
	got := toolsOf(again)
	if strings.Join(got, ",") != "Career,Browser,PDF Tools" {
		t.Fatalf("each tool once, first suggester first: %v", got)
	}
	list := again["suggesters"].([]any)
	if list[0].(map[string]any)["why"] != "for crawl instructions" || list[1].(map[string]any)["why"] != "for reading pages" || list[2].(map[string]any)["why"] != "" {
		t.Errorf("every tool in its own words, the first words of a tool stay: %v", list)
	}
	if again["suggestedBy"] != "Career" {
		t.Errorf("suggestedBy stays the first tool: %v", again["suggestedBy"])
	}
	// one row, one card
	if r.count("suggested") != 1 {
		t.Errorf("one model: %d rows", r.count("suggested"))
	}
	cards := r.page()["suggestions"].([]any)
	if len(cards) != 1 || strings.Join(toolsOf(cards[0].(map[string]any)), ",") != "Career,Browser,PDF Tools" {
		t.Fatalf("shown once with every tool: %v", cards)
	}
}

func TestAnEmptyOrWildToolNameIsCleanedAndANameCannotBeUsedToFloodTheList(t *testing.T) {
	r := newConfirmRig(t)
	r.suggest(asTool("", ""))
	_, got := r.suggest(asTool("  Brow\x00ser\n  tool  "+strings.Repeat("x", 100), "x"))
	tools := toolsOf(got)
	if tools[0] != "a tool" || !strings.HasPrefix(tools[1], "Brow ser tool xxx") || len([]rune(tools[1])) != 60 || len(tools) != 2 {
		t.Errorf("%q", tools)
	}
}

func TestConfirmingAModelSuggestedByManyToolsStartsOneDownloadAndTheListNamesEveryTool(t *testing.T) {
	res := resolved(1)
	r := newConfirmRig(t)
	for _, tool := range []string{"Career", "Browser", "PDF Tools"} {
		r.suggest(asTool(tool, "w"))
	}
	code, page := r.do("LocalAiInstallModel", map[string]string{"id": res.ID})
	if code != http.StatusAccepted || r.installs.starts != 1 {
		t.Fatalf("one confirmation, one download: %d starts=%d", code, r.installs.starts)
	}
	var tools []any
	for _, m := range page["catalog"].([]any) {
		if mm := m.(map[string]any); mm["id"] == res.ID {
			tools = mm["suggestedBy"].([]any)
			if mm["addedBy"] != "Career" {
				t.Errorf("addedBy is the first tool: %v", mm["addedBy"])
			}
		}
	}
	if len(tools) != 3 || tools[0] != "Career" || tools[1] != "Browser" || tools[2] != "PDF Tools" {
		t.Fatalf("the added model names every tool: %v", tools)
	}
	if len(page["suggestions"].([]any)) != 0 {
		t.Error("no card is left")
	}
	// a tool that suggests it after it was added is remembered too, and nothing is downloaded or listed again
	code, late := r.suggest(asTool("Notes", "late"))
	if code != http.StatusOK || late["status"] != "added" || late["created"] != false || strings.Join(toolsOf(late), ",") != "Career,Browser,PDF Tools,Notes" || r.installs.starts != 1 {
		t.Errorf("%d %v starts=%d", code, late, r.installs.starts)
	}
}

func TestAModelThatWasDismissedDoesNotGatherNewSuggestersAndRemovingAnAddedModelForgetsThem(t *testing.T) {
	res := resolved(1)
	r := newConfirmRig(t)
	r.suggest(asTool("Career", "w"))
	r.suggest(asTool("Browser", "w"))
	if _, dismissed := r.dismiss(res.ID); strings.Join(toolsOf(dismissed), ",") != "Career,Browser" {
		t.Errorf("the answer of a dismissal still says who suggested it: %v", dismissed)
	}
	_, got := r.suggest(asTool("Browser", "w"))
	if got["status"] != "dismissed" || strings.Join(toolsOf(got), ",") != "Career,Browser" {
		t.Errorf("a dismissed model stays as it is: %v", got)
	}
	// an added model: removing it forgets who suggested it
	res2 := resolved(2)
	r2 := newConfirmRig(t)
	added := res2.AddedModel("Career", nowForTest())
	added.Status = entity.AddedStatusAdded
	if err := persistent.NewAddedModelsPersistentRepo(r2.db).Insert(&added); err != nil {
		t.Fatal(err)
	}
	_ = persistent.NewAddedModelsPersistentRepo(r2.db).AddSuggester(added.ID, "Career", "w", "2026-11-09T10:00:00Z")
	_ = persistent.NewAddedModelsPersistentRepo(r2.db).AddSuggester(added.ID, "Browser", "w", "2026-11-09T10:01:00Z")
	if code, _ := r2.do("LocalAiRemoveModel", map[string]string{"id": added.ID}); code != http.StatusOK || suggesterRows(t, r2.sugRig, added.ID) != 0 {
		t.Errorf("%d rows %d", code, suggesterRows(t, r2.sugRig, added.ID))
	}
}

func TestOneCardPerModelAndTheCapCountsModelsNotTools(t *testing.T) {
	r := newConfirmRig(t)
	for i := 0; i < 30; i++ {
		if code, _ := r.suggest(asTool(fmt.Sprintf("Tool %d", i), "w")); code != http.StatusCreated && code != http.StatusOK {
			t.Fatalf("tool %d: %d", i, code)
		}
	}
	if r.count("suggested") != 1 || len(toolsOf(r.page()["suggestions"].([]any)[0].(map[string]any))) != 30 {
		t.Errorf("one model suggested by thirty tools is one card")
	}
	// the cap is about waiting models: with 19 more of them the next new one is refused
	for i := 2; i <= 20; i++ {
		r.put(resolved(i).AddedModel("Career", nowForTest()), entity.AddedStatusSuggested)
	}
	r.sugRig = newSugRigOn(t, r.db, func(huggingface.Request) (huggingface.Resolved, error) { return resolved(99), nil })
	if code, _ := r.suggest(asTool("Career", "w")); code != http.StatusTooManyRequests {
		t.Errorf("the 21st model: %d", code)
	}
	// a tool that suggests a model that is already waiting is not refused at the cap
	r.sugRig = newSugRigOn(t, r.db, func(huggingface.Request) (huggingface.Resolved, error) { return resolved(1), nil })
	if code, got := r.suggest(asTool("Newcomer", "w")); code != http.StatusOK || len(toolsOf(got)) != 31 {
		t.Errorf("%d %v", code, toolsOf(got))
	}
}

func TestAShippedModelSuggestedByTwoToolsIsOneCardAndOneDownloadAndTheSuggestionIsForgottenAfterwards(t *testing.T) {
	r := newShippedRig(t)
	r.suggestWith(`{"repo":"bartowski/Qwen2.5-7B-Instruct-GGUF","file":"Qwen2.5-7B-Instruct-Q4_K_M.gguf","suggestedBy":"Career","why":"crawls"}`)
	_, second := r.suggestWith(`{"repo":"bartowski/Qwen2.5-7B-Instruct-GGUF","file":"Qwen2.5-7B-Instruct-Q4_K_M.gguf","suggestedBy":"Browser","why":"reading"}`)
	if second["created"] != false || strings.Join(toolsOf(second), ",") != "Career,Browser" {
		t.Fatalf("%v", second)
	}
	cards := r.page()["suggestions"].([]any)
	if len(cards) != 1 || len(cards[0].(map[string]any)["suggesters"].([]any)) != 2 {
		t.Fatalf("one card, two tools: %v", cards)
	}
	if code, _ := r.do("LocalAiInstallModel", map[string]string{"id": shippedID}); code != http.StatusAccepted || r.installs.starts != 1 {
		t.Errorf("one download: %d starts=%d", code, r.installs.starts)
	}
	if suggesterRows(t, r.sugRig, shippedID) != 0 || r.status(shippedID) != "(none)" {
		t.Error("the suggestion and its tools are forgotten once the model is downloading")
	}
}

// Even when several confirmations arrive (two people, two tabs), the file is downloaded once.
func TestAModelIsDownloadedOnceHoweverManyTimesItIsConfirmed(t *testing.T) {
	r := newRig(t, nil)
	db := openMainDB(t)
	big := bytes.Repeat([]byte("m"), 2<<20)
	r.srv.files[modelURL] = big
	id := "added-oncedownload"
	add := addedModel(id, entity.AddedStatusAdded, int64(len(big)), sha(big))
	if err := persistent.NewAddedModelsPersistentRepo(db).Insert(&add); err != nil {
		t.Fatal(err)
	}
	src := models.FromDB(db, nil)
	r.in.source = src
	models.SetSource(src)
	t.Cleanup(func() { models.SetSource(nil) })

	r.srv.hold = make(chan struct{})
	for i := 0; i < 3; i++ {
		if err := r.in.Start(id, mac); err != nil {
			t.Fatal(err)
		}
	}
	r.wait(t, "the download to begin", func() bool { return r.srv.blocked.Load() > 0 })
	for i := 0; i < 2; i++ {
		if err := r.in.Start(id, mac); err != nil {
			t.Fatal(err)
		}
	}
	close(r.srv.hold)
	r.allDone(t)
	for i := 0; i < 2; i++ { // and once it is there, asking again downloads nothing
		if err := r.in.Start(id, mac); err != nil {
			t.Fatal(err)
		}
	}
	r.allDone(t)
	if hits := r.srv.hitsOn(modelURL); hits != 1 {
		t.Errorf("the model was requested %d times, want once", hits)
	}
	if _, err := os.Stat(filepath.Join(r.in.dataDir, "local-ai", "models", id+".gguf")); err != nil {
		t.Error(err)
	}
}

// A database from before this change keeps who suggested what: the migration copies it.
func TestTheMigrationKeepsWhoSuggestedTheModelsThatAlreadyExist(t *testing.T) {
	db := openMainDB(t)
	_, err := db.Exec(`INSERT INTO local_ai_models (id, repo, file, revision, url, sha256, size_bytes, name, suggested_by, why, status, created_at) VALUES
		('added-old1', 'o/r', 'a.gguf', 'c1', 'u', 's', 5, 'A', 'Career', 'because', 'suggested', '2026-11-08T10:00:00Z'),
		('added-old2', 'o/r', 'b.gguf', 'c1', 'u', 's', 5, 'B', '', '', 'added', '2026-11-08T11:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(migrationsDir, "31_10_2026_23_45_00.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		if strings.TrimSpace(stmt) != "" {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
	}
	who, err := query.NewAddedModelsQueryRepo(db).Suggesters("added-old1", "added-old2")
	if err != nil || len(who["added-old1"]) != 1 || who["added-old1"][0].Tool != "Career" || who["added-old1"][0].Why != "because" || who["added-old2"][0].Tool != "a tool" {
		t.Fatalf("%v %v", who, err)
	}
}

// A list is a list, never null: a waiting model nobody recorded a tool for (a row from before this change, written by hand) still answers an empty list.
func TestAModelWithNoRecordedToolAnswersAnEmptyListNotNull(t *testing.T) {
	r := newConfirmRig(t)
	r.put(resolved(5).AddedModel("", nowForTest()), entity.AddedStatusSuggested)
	cards := r.page()["suggestions"].([]any)
	list, ok := cards[0].(map[string]any)["suggesters"].([]any)
	if !ok || len(list) != 0 {
		t.Errorf("%v", cards[0])
	}
}
