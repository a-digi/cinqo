package localai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
)

// Plan 220, step 1: the list of models is the shipped catalog plus the models a person added after a tool suggested them, and every place that needs
// "the models" asks that one list.

const (
	addedRepo     = "someone/tiny-gguf"
	addedFile     = "m.gguf"
	addedRevision = "78eb92a46fc37e6b524df991ed9aca9bc6aa7b80"
)

func addedModel(id, status string, size int64, sum string) entity.AddedModel {
	return entity.AddedModel{
		ID: id, Repo: addedRepo, File: addedFile, Revision: addedRevision, URL: modelURL, SHA256: sum, SizeBytes: size, Quantization: "Q4_K_M",
		License: "MIT", Name: "Tiny added", Publisher: "someone", ContextTokens: 8192, Why: "good for crawl instructions", SuggestedBy: "Career",
		Status: status, CreatedAt: "2026-11-09T10:00:00Z",
	}
}

// listed is how many shipped models are in the list by default: the ones Cinqo only offers when a tool suggested them (suggestOnly) are not.
func listed(t *testing.T) int {
	t.Helper()
	n := 0
	for _, m := range shipped(t).Models {
		if !m.SuggestOnly {
			n++
		}
	}
	return n
}

func shipped(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMergeKeepsTheShippedListAndAppendsValidAddedModels(t *testing.T) {
	base := shipped(t)
	good := addedModel("added-aaa111", entity.AddedStatusAdded, 2<<20, strings.Repeat("a", 64))
	merged, problems := models.Merge(base, []entity.AddedModel{good})
	if len(problems) != 0 {
		t.Fatalf("%v", problems)
	}
	if len(merged.Models) != listed(t)+1 || merged.Models[len(merged.Models)-1].ID != "added-aaa111" {
		t.Fatalf("the added model is appended: %d models", len(merged.Models))
	}
	i := 0
	for _, m := range base.Models {
		if m.SuggestOnly {
			continue
		}
		if merged.Models[i].ID != m.ID {
			t.Fatalf("the shipped models keep their place and order")
		}
		i++
	}
	if len(merged.Runtimes) != len(base.Runtimes) {
		t.Error("the runtimes are the shipped ones")
	}
	got, _ := merged.Find("added-aaa111")
	if got.Tools || got.ToolDialect != "" || got.Driver != "" || len(got.UnsuitableFor) != 0 || got.KVBytesPerToken != 0 {
		t.Errorf("what a tool says about abilities is never trusted: %+v", got)
	}
	if got.ContextTokens != 8192 || got.Name != "Tiny added" || got.Note != "good for crawl instructions" || !strings.Contains(got.Source, "added by Career") {
		t.Errorf("%+v", got)
	}
	if len(shipped(t).Models) != len(base.Models) {
		t.Error("the shipped catalog value must not be changed")
	}
}

func TestMergeLeavesOutAnAddedModelThatBreaksARuleTakesAShippedIdOrIsRepeated(t *testing.T) {
	base := shipped(t)
	shippedID := base.Models[0].ID
	bad := func(id string, mutate func(*entity.AddedModel)) entity.AddedModel {
		m := addedModel(id, entity.AddedStatusAdded, 2<<20, strings.Repeat("a", 64))
		mutate(&m)
		return m
	}
	cases := map[string]entity.AddedModel{
		"no prefix":         bad("tiny-1", func(*entity.AddedModel) {}),
		"a shipped id":      bad(shippedID, func(*entity.AddedModel) {}),
		"too small":         bad("added-small", func(m *entity.AddedModel) { m.SizeBytes = 10 }),
		"not a pinned file": bad("added-url", func(m *entity.AddedModel) { m.URL = "https://example.com/m.gguf" }),
		"bad hash":          bad("added-hash", func(m *entity.AddedModel) { m.SHA256 = "xyz" }),
		"no licence":        bad("added-lic", func(m *entity.AddedModel) { m.License = "" }),
		"context too small": bad("added-ctx", func(m *entity.AddedModel) { m.ContextTokens = 5 }),
	}
	for name, m := range cases {
		merged, problems := models.Merge(base, []entity.AddedModel{m})
		if len(problems) != 1 || len(merged.Models) != listed(t) {
			t.Errorf("%s: problems=%v models=%d", name, problems, len(merged.Models))
		}
	}
	// the same id twice: the first one stays
	one := addedModel("added-dup", entity.AddedStatusAdded, 2<<20, strings.Repeat("a", 64))
	merged, problems := models.Merge(base, []entity.AddedModel{one, one})
	if len(problems) != 1 || len(merged.Models) != listed(t)+1 {
		t.Errorf("a repeated id: problems=%v models=%d", problems, len(merged.Models))
	}
}

func TestFromDBMergesOnlyTheAddedStatusAndNeverFailsTheShippedList(t *testing.T) {
	db := openMainDB(t)
	repo := persistent.NewAddedModelsPersistentRepo(db)
	mk := func(id, rev, status string) entity.AddedModel {
		m := addedModel(id, status, 2<<20, strings.Repeat("b", 64))
		m.Revision = rev
		m.URL = "https://huggingface.co/" + addedRepo + "/resolve/" + rev + "/" + addedFile
		return m
	}
	for _, m := range []entity.AddedModel{
		mk("added-ok", strings.Repeat("1", 40), entity.AddedStatusAdded),
		mk("added-sug", strings.Repeat("2", 40), entity.AddedStatusSuggested),
		mk("added-dis", strings.Repeat("3", 40), entity.AddedStatusDismissed),
	} {
		if err := repo.Insert(&m); err != nil {
			t.Fatal(err)
		}
	}
	// the same file at the same commit twice is refused by the table
	dup := mk("added-ok2", strings.Repeat("1", 40), entity.AddedStatusAdded)
	if err := repo.Insert(&dup); err == nil {
		t.Error("the same repository, file and commit must be stored once")
	}
	var seen []error
	cat, err := models.FromDB(db, func(e error) { seen = append(seen, e) })()
	if err != nil || len(seen) != 0 {
		t.Fatalf("%v %v", err, seen)
	}
	if _, ok := cat.Find("added-ok"); !ok {
		t.Error("an added model is in the list")
	}
	for _, id := range []string{"added-sug", "added-dis"} {
		if _, ok := cat.Find(id); ok {
			t.Errorf("%s is not part of the list until it is added", id)
		}
	}
	// a table that cannot be read still answers the shipped models, and says why
	if _, err := db.Exec(`DROP TABLE local_ai_models`); err != nil {
		t.Fatal(err)
	}
	seen = nil
	cat, err = models.FromDB(db, func(e error) { seen = append(seen, e) })()
	if err != nil || len(cat.Models) != listed(t) || len(seen) != 1 {
		t.Errorf("shipped models must still be answered: err=%v models=%d seen=%v", err, len(cat.Models), seen)
	}
}

// With the list wired in, an added model is found by the code that downloads, checks and offers models, exactly like a shipped one.
func TestAnAddedModelIsFoundWhereverTheShippedOnesAreAndIsDownloadedIntoTheSameFolder(t *testing.T) {
	r := newRig(t, nil)
	db := openMainDB(t)
	id := "added-77aa88bb"
	// a model must be at least 1 MiB to be a model at all, like a shipped one
	big := bytes.Repeat([]byte("m"), 2<<20)
	r.srv.files[modelURL] = big
	add := addedModel(id, entity.AddedStatusAdded, int64(len(big)), sha(big))
	if err := persistent.NewAddedModelsPersistentRepo(db).Insert(&add); err != nil {
		t.Fatal(err)
	}
	src := models.FromDB(db, nil)
	r.in.source = src
	models.SetSource(src)
	t.Cleanup(func() { models.SetSource(nil) })

	if _, ok := r.in.cat().Find("qwen2.5-1.5b-instruct-q4_k_m"); !ok {
		t.Error("the shipped models are still in the list")
	}
	m, ok := r.in.cat().Find(id)
	if !ok {
		t.Fatal("the installer does not know the added model")
	}
	if got := r.in.ModelState(m).State; got != entity.InstallNotDownloaded {
		t.Fatalf("before the download: %v", got)
	}
	if err := r.in.Start(id, mac); err != nil {
		t.Fatal(err)
	}
	r.allDone(t)
	if got := r.in.ModelState(m).State; got != entity.InstallReady {
		t.Fatalf("after the download: %v", got)
	}
	want := filepath.Join(r.in.dataDir, "local-ai", "models", id+".gguf")
	if st, err := os.Stat(want); err != nil || st.Size() != m.SizeBytes {
		t.Fatalf("the file must be in the folder every model lives in: %v", err)
	}
	if r.srv.hitsOn(modelURL) != 1 {
		t.Errorf("one download of the model: %d", r.srv.hitsOn(modelURL))
	}
	// no tools: only a shipped model that was measured gets any
	if toolsUsable(id) || dialectOf(id) != nil {
		t.Error("an added model gets no tools")
	}
	// removing everything deletes its file too (the rows stay: they are only metadata)
	if err := r.in.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Error("the downloaded file must be removed")
	}
	// the settings can choose it, because the settings are checked against the merged list
	cat, err := models.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entity.Validate(entity.Settings{ModelID: id, ContextTokens: 4096, IdleMinutes: 5}, cat, nil); err != nil {
		t.Errorf("an added model can be chosen as the model: %v", err)
	}
	if _, err := entity.Validate(entity.Settings{ModelID: "added-unknown", ContextTokens: 4096, IdleMinutes: 5}, cat, nil); err == nil {
		t.Error("an id that is nowhere in the list is still refused")
	}
}

// The page's own answer lists an added model next to the shipped ones.
func TestThePagesAnswerListsAnAddedModelNextToTheShippedOnes(t *testing.T) {
	db := openMainDB(t)
	add := addedModel("added-pagetest", entity.AddedStatusAdded, 2<<20, strings.Repeat("c", 64))
	if err := persistent.NewAddedModelsPersistentRepo(db).Insert(&add); err != nil {
		t.Fatal(err)
	}
	models.SetSource(models.FromDB(db, nil))
	t.Cleanup(func() { models.SetSource(nil) })
	rec := call("LocalAiGet", canRun, db, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Message struct {
			Catalog []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"catalog"`
		} `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, m := range out.Message.Catalog {
		ids[m.ID] = true
	}
	if !ids["added-pagetest"] || !ids["qwen2.5-1.5b-instruct-q4_k_m"] {
		t.Errorf("the page lists shipped and added models: %v", ids)
	}
}

// The readable name of a model says its size in MiB when it is under one GiB (a very small added model must not read "0.0 GiB"), and in GiB otherwise,
// exactly as before for the shipped models.
func TestTheReadableNameOfAModelSaysItsSizeInTheRightUnit(t *testing.T) {
	for bytes, want := range map[int64]string{
		1364064:        "1.3 MiB",
		1 << 20:        "1.0 MiB",
		(1 << 30) - 1:  "1024.0 MiB",
		1 << 30:        "1.0 GiB",
		2104932800:     "2.0 GiB",
		2491874272:     "2.3 GiB",
		16 * (1 << 30): "16.0 GiB",
	} {
		if got := downloadSize(bytes); got != want {
			t.Errorf("%d: %q, want %q", bytes, got, want)
		}
	}
	names := modelNames()
	if names["qwen2.5-7b-instruct-q4_k_m"] != "Qwen2.5-7B-Instruct, 4.4 GiB" || names["phi-4-mini-instruct-q4_k_m"] != "Phi-4-mini-instruct, 2.3 GiB" {
		t.Errorf("the shipped names do not change: %v", names)
	}
	// an added tiny model reads in MiB, wired through the list of models
	db := openMainDB(t)
	tiny := addedModel("added-tinyname", entity.AddedStatusAdded, 1364064, strings.Repeat("a", 64))
	if err := persistent.NewAddedModelsPersistentRepo(db).Insert(&tiny); err != nil {
		t.Fatal(err)
	}
	models.SetSource(models.FromDB(db, nil))
	t.Cleanup(func() { models.SetSource(nil) })
	if got := modelNames()["added-tinyname"]; got != "Tiny added, 1.3 MiB" {
		t.Errorf("%q", got)
	}
}
