package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	tool_entity "github.com/a-digi/cinqo/src/tool/entity"
	tool_persistent "github.com/a-digi/cinqo/src/tool/repository/persistent"
	repo_persistent "github.com/a-digi/cinqo/src/toolrepository/repository/persistent"

	"github.com/a-digi/coco-server/server/request"
)

// planTool is one tool entry of a repository file with one version that may carry `requires` and a size.
func planTool(id, version, requires string, size int) string {
	if requires != "" {
		requires = `"requires":` + requires + ","
	}
	return fmt.Sprintf(`{"id":%q,"currentVersionId":"v1","versions":[{"versionId":"v1","name":%q,"version":%q,"authors":[{"name":"Test Author"}],"license":"MIT",%s
	  "packages":[{"os":"any","arch":"any","url":"files/%s.zip","sha256":%q,"sizeBytes":%d}]}]}`,
		id, strings.ToUpper(id[:1])+id[1:], version, requires, id, strings.Repeat("a", 64), size)
}

func planFile(name string, tools ...string) []byte {
	return []byte(fmt.Sprintf(`{"formatVersion":2,"name":%q,"authors":[{"name":"R"}],"tools":[%s]}`, name, strings.Join(tools, ",")))
}

// countingServer serves repository files and counts every request, so a test can tell what was fetched.
type countingServer struct {
	files map[string][]byte
	hits  atomic.Int64
	srv   *httptest.Server
}

func newCountingServer(t *testing.T) *countingServer {
	c := &countingServer{files: map[string][]byte{}}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.hits.Add(1)
		b, ok := c.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (e *env) installRow(t *testing.T, slug, version string, enabled bool, status, source string, requires ...tool_entity.ToolRequiredTool) {
	t.Helper()
	tool := &tool_entity.Tool{ID: "id-" + slug, Slug: slug, Name: strings.ToUpper(slug[:1]) + slug[1:], Version: version, Kind: "frontend_only", Enabled: enabled,
		Status: status, InstallPath: "/x", ServiceToken: "tok-" + slug, SourceRepositoryID: source,
		Metadata: `{"authors":[{"name":"Test Author"}],"license":"MIT"}`}
	if err := tool_persistent.NewToolPersistentRepo(e.di.db).InsertFresh(tool, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := tool_persistent.NewToolRequiredToolPersistentRepo(e.di.db).ReplaceAll(tool.ID, requires); err != nil {
		t.Fatal(err)
	}
}

func (e *env) plan(repoID, toolID, body string) (int, map[string]any) {
	return e.call(InstallPlanHandler, "POST", "/x", body, map[string]string{"id": repoID, "toolId": toolID})
}

func planOf(out map[string]any) map[string]any { return out["message"].(map[string]any) }

func stepsOf(out map[string]any) []map[string]any {
	var steps []map[string]any
	for _, s := range planOf(out)["steps"].([]any) {
		steps = append(steps, s.(map[string]any))
	}
	return steps
}

func stepIDs(out map[string]any) string {
	var ids []string
	for _, s := range stepsOf(out) {
		ids = append(ids, fmt.Sprintf("%s:%s@%s", s["toolId"], s["action"], s["version"]))
	}
	return strings.Join(ids, " ")
}

func careerFile() []byte {
	return planFile("Main",
		planTool("career", "0.0.806", `[{"id":"browser"},{"id":"pdf_tools","minVersion":"0.0.1"}]`, 3000),
		planTool("browser", "0.0.245", "", 1000),
		planTool("pdf_tools", "0.0.124", "", 2000),
	)
}

func TestThePreviewListsWhatCareerNeedsInTheOrderItIsDone(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")

	code, out := e.plan(id, "career", "")
	if code != http.StatusOK || planOf(out)["ok"] != true {
		t.Fatalf("%d %v", code, out)
	}
	if got := stepIDs(out); got != "browser:install@0.0.245 pdf_tools:install@0.0.124 career:install@0.0.806" {
		t.Fatalf("%s", got)
	}
	if planOf(out)["totalBytes"].(float64) != 6000 {
		t.Fatalf("the total is the sum of the sizes: %v", planOf(out)["totalBytes"])
	}
	first := stepsOf(out)[0]
	if first["name"] != "Browser" || first["repositoryId"] != id || first["repositoryName"] != "Main" || first["license"] != "MIT" ||
		first["authors"].([]any)[0] != "Test Author" || first["requiredBy"].([]any)[0] != "career" || first["installedVersion"] != "" ||
		first["versionId"] != "v1" || first["sizeBytes"].(float64) != 1000 {
		t.Fatalf("%v", first)
	}
	// lists are lists, never null
	for _, s := range stepsOf(out) {
		for _, k := range []string{"authors", "requiredBy", "marks", "dependents"} {
			if _, ok := s[k].([]any); !ok {
				t.Errorf("%s of %v must be a list: %v", k, s["toolId"], s[k])
			}
		}
	}
	if len(planOf(out)["problems"].([]any)) != 0 || planOf(out)["searched"].([]any)[0].(map[string]any)["name"] != "Main" {
		t.Fatalf("%v", planOf(out))
	}
}

// The preview changes nothing and contacts nothing: no request to any repository, no package, no row, and nothing a client could install with.
func TestThePreviewInstallsAndDownloadsNothingAndShowsNoAddress(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	hits := srv.hits.Load()
	var before int
	e.di.db.QueryRow(`SELECT COUNT(*) FROM tools`).Scan(&before)

	rec := httptest.NewRecorder()
	ctx := newContext(e, rec, "POST", `{"versionId":""}`, map[string]string{"id": id, "toolId": "career"})
	InstallPlanHandler(ctx)
	body := rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "http") || strings.Contains(body, "sha256") || strings.Contains(body, strings.Repeat("a", 64)) || strings.Contains(body, ".zip") {
		t.Fatalf("no download address or checksum may leave the server: %d %s", rec.Code, body)
	}
	if srv.hits.Load() != hits {
		t.Fatal("the preview must not contact any repository: it reads the stored files")
	}
	var after int
	e.di.db.QueryRow(`SELECT COUNT(*) FROM tools`).Scan(&after)
	if after != before || len(leftovers(t, e)) != 0 {
		t.Fatalf("nothing is installed: %d -> %d, %v", before, after, leftovers(t, e))
	}
}

func TestInstalledToolsDecideWhatIsStillNeeded(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	// browser is installed and fine; pdf_tools is too old and running, and Notes uses it
	e.installRow(t, "browser", "0.0.245", true, "running", id)
	e.installRow(t, "pdf_tools", "0.0.0", true, "running", id)
	e.installRow(t, "notes", "1.0.0", true, "running", id, tool_entity.ToolRequiredTool{RequiredSlug: "pdf_tools", MinVersion: "0.0.1"})
	code, out := e.plan(id, "career", "")
	if code != 200 || stepIDs(out) != "pdf_tools:update@0.0.124 career:install@0.0.806" {
		t.Fatalf("%d %v", code, out)
	}
	pdf := stepsOf(out)[0]
	if pdf["installedVersion"] != "0.0.0" || pdf["dependents"].([]any)[0] != "notes" || !marked(pdf, "restarts") {
		t.Fatalf("%v", pdf)
	}
}

func TestADisabledDependencyIsOnlyEnabled(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	e.installRow(t, "browser", "0.0.245", false, "installed", "")
	e.installRow(t, "pdf_tools", "0.0.124", true, "running", "")
	code, out := e.plan(id, "career", "")
	if code != 200 || stepIDs(out) != "browser:enable@0.0.245 career:install@0.0.806" {
		t.Fatalf("%d %v", code, out)
	}
	if s := stepsOf(out)[0]; !marked(s, "was_disabled") || s["repositoryId"] != "" || s["sizeBytes"].(float64) != 0 {
		t.Fatalf("an enable step downloads nothing: %v", s)
	}
}

// What the installed tools declared (their stored requirements, with the upper bound) protects them.
func TestAnUpdateThatWouldBreakAnInstalledToolIsBlockedWithTheReason(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = planFile("Main",
		planTool("career", "1.0.0", `[{"id":"pdf_tools","minVersion":"0.0.100"}]`, 10),
		planTool("pdf_tools", "0.0.124", "", 10))
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	e.installRow(t, "pdf_tools", "0.0.50", true, "running", id)
	e.installRow(t, "notes", "1.0.0", true, "running", id, tool_entity.ToolRequiredTool{RequiredSlug: "pdf_tools", MinVersion: "0.0.1", MaxVersion: "0.0.60"})
	code, out := e.plan(id, "career", "")
	if code != http.StatusOK || planOf(out)["ok"] != false || len(stepsOf(out)) != 0 {
		t.Fatalf("%d %v", code, out)
	}
	pr := planOf(out)["problems"].([]any)[0].(map[string]any)
	if pr["kind"] != "would_break_installed" || pr["toolId"] != "pdf_tools" || !strings.Contains(pr["message"].(string), "notes") ||
		!strings.Contains(pr["message"].(string), "0.0.1 to 0.0.60") || pr["requiredBy"].([]any)[0] != "career" {
		t.Fatalf("%v", pr)
	}
	if _, isList := planOf(out)["steps"].([]any); !isList {
		t.Fatal("a plan with problems has an empty list of steps, not null")
	}
}

func TestADependencyNoRepositoryOffersIsAProblemNotAnError(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = planFile("Main", planTool("career", "1.0.0", `[{"id":"ghost"}]`, 10))
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	code, out := e.plan(id, "career", "")
	pr := planOf(out)["problems"].([]any)[0].(map[string]any)
	if code != http.StatusOK || planOf(out)["ok"] != false || pr["kind"] != "not_in_any_repository" || pr["toolId"] != "ghost" || !strings.Contains(pr["message"].(string), "Main") {
		t.Fatalf("%d %v", code, out)
	}
}

// Other registered repositories are searched, and what comes from them is marked.
func TestADependencyFromAnotherRegisteredRepositoryIsMarked(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = planFile("Main", planTool("career", "1.0.0", `[{"id":"browser"}]`, 10))
	srv.files["/other.json"] = planFile("Other", planTool("browser", "2.0.0", "", 20))
	main := addRepo(t, e, srv.srv.URL+"/main.json")
	other := addRepo(t, e, srv.srv.URL+"/other.json")
	code, out := e.plan(main, "career", "")
	if code != 200 || stepIDs(out) != "browser:install@2.0.0 career:install@1.0.0" {
		t.Fatalf("%d %v", code, out)
	}
	b := stepsOf(out)[0]
	if b["repositoryId"] != other || b["repositoryName"] != "Other" || !marked(b, "other_repository") {
		t.Fatalf("%v", b)
	}
	if marked(stepsOf(out)[1], "other_repository") {
		t.Fatalf("the chosen repository's own tool is not marked: %v", stepsOf(out)[1])
	}
	if s := planOf(out)["searched"].([]any); len(s) != 2 || s[0].(map[string]any)["id"] != main {
		t.Fatalf("%v", s)
	}
}

func TestARepositoryThatCouldNotBeRefreshedIsMarkedStale(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = planFile("Main", planTool("career", "1.0.0", `[{"id":"browser"}]`, 10))
	srv.files["/old.json"] = planFile("Old", planTool("browser", "2.0.0", "", 20))
	main := addRepo(t, e, srv.srv.URL+"/main.json")
	old := addRepo(t, e, srv.srv.URL+"/old.json")
	// the only source of the dependency is a repository whose last refresh failed
	if err := repo_persistent.NewRepositoryPersistentRepo(e.di.db).RecordError(old, "boom", "2026-11-01T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	code, out := e.plan(main, "career", "")
	pr := planOf(out)["problems"].([]any)[0].(map[string]any)
	if code != 200 || pr["kind"] != "stale_only_source" || !strings.Contains(pr["message"].(string), "2026-11-01") {
		t.Fatalf("%d %v", code, out)
	}
	if s := planOf(out)["searched"].([]any)[1].(map[string]any); s["stale"] != true || s["refreshedAt"] == "" {
		t.Fatalf("%v", s)
	}
	// a fresh repository that offers it too makes it a plan, and the stale one is not what is chosen first only by chance of its name
	srv.files["/zfresh.json"] = planFile("Zfresh", planTool("browser", "2.0.0", "", 20))
	addRepo(t, e, srv.srv.URL+"/zfresh.json")
	code, out = e.plan(main, "career", "")
	if code != 200 || stepIDs(out) != "browser:install@2.0.0 career:install@1.0.0" {
		t.Fatalf("%d %v", code, out)
	}
	if b := stepsOf(out)[0]; !marked(b, "other_repository") || !marked(b, "stale_repository") {
		t.Fatalf("%v", b["marks"])
	}
}

func TestTheRequestedToolAndVersionAreChecked(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	if code, _ := e.plan("nope", "career", ""); code != http.StatusNotFound {
		t.Fatalf("an unknown repository: %d", code)
	}
	if code, _ := e.plan(id, "nope", ""); code != http.StatusNotFound {
		t.Fatalf("an unknown tool: %d", code)
	}
	if code, _ := e.plan(id, "career", `{"versionId":"v9"}`); code != http.StatusNotFound {
		t.Fatalf("an unknown version: %d", code)
	}
	if code, _ := e.plan(id, "career", `{not json`); code != http.StatusBadRequest {
		t.Fatalf("a broken body: %d", code)
	}
	if code, out := e.plan(id, "career", `{"versionId":"v1"}`); code != 200 || planOf(out)["ok"] != true {
		t.Fatalf("a version asked for by id: %d %v", code, out)
	}
	// nothing to do is an answer, not an error
	e.installRow(t, "career", "0.0.806", true, "running", id)
	code, out := e.plan(id, "career", "")
	if code != 200 || planOf(out)["ok"] != false || planOf(out)["problems"].([]any)[0].(map[string]any)["kind"] != "already_installed" {
		t.Fatalf("%d %v", code, out)
	}
}

func newContext(e *env, rec *httptest.ResponseRecorder, method, body string, vars map[string]string) request.RequestContext {
	ctx := request.NewContext(rec, httptest.NewRequest(method, "/x", strings.NewReader(body)), e.di)
	for k, v := range vars {
		ctx.GetURI().SetPathVariable(k, v)
	}
	return ctx
}

// marked says whether a step carries a mark. (The test repositories are on 127.0.0.1, so their steps also carry local_network.)
func marked(step map[string]any, mark string) bool {
	for _, m := range step["marks"].([]any) {
		if m == mark {
			return true
		}
	}
	return false
}

func TestRepositoriesOnALocalAddressAreMarkedAsSuch(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	_, out := e.plan(id, "career", "")
	for _, s := range stepsOf(out) {
		if !marked(s, "local_network") {
			t.Errorf("%v comes from a repository on a local address", s["toolId"])
		}
	}
}

// A tool that is not running does not restart when it is updated.
func TestAnUpdatedToolThatIsNotRunningDoesNotRestart(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	e.installRow(t, "browser", "0.0.245", true, "stopped", id)
	e.installRow(t, "pdf_tools", "0.0.0", true, "stopped", id)
	e.installRow(t, "notes", "1.0.0", true, "running", id, tool_entity.ToolRequiredTool{RequiredSlug: "pdf_tools"})
	_, out := e.plan(id, "career", "")
	pdf := stepsOf(out)[0]
	if pdf["toolId"] != "pdf_tools" || marked(pdf, "restarts") || pdf["dependents"].([]any)[0] != "notes" {
		t.Fatalf("%v", pdf)
	}
}

// An update keeps a disabled tool disabled, so the plan says to enable it afterwards.
func TestAnUpdatedDisabledToolIsEnabledAfterwards(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	e.installRow(t, "browser", "0.0.245", true, "running", id)
	e.installRow(t, "pdf_tools", "0.0.0", false, "installed", id)
	_, out := e.plan(id, "career", "")
	pdf := stepsOf(out)[0]
	if pdf["action"] != "update" || pdf["enableAfter"] != true || !marked(pdf, "was_disabled") || stepsOf(out)[1]["enableAfter"] != false {
		t.Fatalf("%v", out)
	}
}

// The repository a tool was installed from is searched before the others (alphabetically first and newest do not win over where it came from).
func TestAToolIsUpdatedFromTheRepositoryItWasInstalledFrom(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = planFile("Main", planTool("career", "1.0.0", `[{"id":"browser","minVersion":"2.0.0"}]`, 10))
	srv.files["/alpha.json"] = planFile("Alpha", planTool("browser", "2.5.0", "", 10))
	srv.files["/other.json"] = planFile("Other", planTool("browser", "2.2.0", "", 10))
	main := addRepo(t, e, srv.srv.URL+"/main.json")
	addRepo(t, e, srv.srv.URL+"/alpha.json")
	other := addRepo(t, e, srv.srv.URL+"/other.json")
	e.installRow(t, "browser", "1.0.0", true, "running", other)
	code, out := e.plan(main, "career", "")
	if code != 200 || stepIDs(out) != "browser:update@2.2.0 career:install@1.0.0" || stepsOf(out)[0]["repositoryId"] != other {
		t.Fatalf("%d %v", code, out)
	}
}

// Who made the installed tool (kept with it at install) is compared with who made the update.
func TestAnUpdateByAnotherPublisherIsNotAppliedAutomatically(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = planFile("Main", planTool("career", "1.0.0", `[{"id":"browser","minVersion":"2.0.0"}]`, 10))
	srv.files["/eve.json"] = []byte(strings.Replace(string(planFile("Eve", planTool("browser", "3.0.0", "", 10))), "Test Author", "Eve", 1))
	main := addRepo(t, e, srv.srv.URL+"/main.json")
	addRepo(t, e, srv.srv.URL+"/eve.json")
	e.installRow(t, "browser", "1.0.0", true, "running", "")
	code, out := e.plan(main, "career", "")
	pr := planOf(out)["problems"].([]any)[0].(map[string]any)
	if code != 200 || pr["kind"] != "publisher_changed" || !strings.Contains(pr["message"].(string), "Eve") || !strings.Contains(pr["message"].(string), "Test Author") {
		t.Fatalf("%d %v", code, out)
	}
}

// An enable step of a tool that stated no authors still has a list of authors, empty.
func TestAnEnableStepOfAToolWithoutAuthorsHasAnEmptyList(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	e.installRow(t, "browser", "0.0.245", false, "installed", "")
	e.installRow(t, "pdf_tools", "0.0.124", true, "running", "")
	if _, err := e.di.db.Exec(`UPDATE tools SET metadata = NULL WHERE slug = 'browser'`); err != nil {
		t.Fatal(err)
	}
	_, out := e.plan(id, "career", "")
	b := stepsOf(out)[0]
	if b["action"] != "enable" {
		t.Fatalf("%v", out)
	}
	if list, ok := b["authors"].([]any); !ok || len(list) != 0 {
		t.Fatalf("authors must be an empty list, never null: %v", b["authors"])
	}
}

// The version the user picked is the one planned, not the current one.
func TestTheVersionTheUserPickedIsThePlannedOne(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	two := func(newReq, oldReq string) string {
		return fmt.Sprintf(`{"id":"career","currentVersionId":"v2","versions":[
		  {"versionId":"v1","name":"Career","version":"1.0.0","authors":[{"name":"A"}],"license":"MIT","requires":%s,"packages":[{"os":"any","arch":"any","url":"files/c1.zip","sha256":%q,"sizeBytes":10}]},
		  {"versionId":"v2","name":"Career","version":"2.0.0","authors":[{"name":"A"}],"license":"MIT","requires":%s,"packages":[{"os":"any","arch":"any","url":"files/c2.zip","sha256":%q,"sizeBytes":10}]}]}`,
			oldReq, strings.Repeat("b", 64), newReq, strings.Repeat("c", 64))
	}
	srv.files["/main.json"] = planFile("Main", two(`[{"id":"browser"}]`, `[]`), planTool("browser", "1.0.0", "", 5))
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	_, out := e.plan(id, "career", "")
	if stepIDs(out) != "browser:install@1.0.0 career:install@2.0.0" {
		t.Fatalf("the current version is the default: %s", stepIDs(out))
	}
	_, out = e.plan(id, "career", `{"versionId":"v1"}`)
	if stepIDs(out) != "career:install@1.0.0" {
		t.Fatalf("the picked version, with its own requirements: %s", stepIDs(out))
	}
}

// The repository detail (what the repository page draws) lists what each tool needs.
func TestTheRepositoryDetailListsWhatEachToolNeeds(t *testing.T) {
	e, srv := newEnv(t), newCountingServer(t)
	srv.files["/main.json"] = careerFile()
	id := addRepo(t, e, srv.srv.URL+"/main.json")
	code, out := e.call(GetHandler, "GET", "/x", "", map[string]string{"id": id})
	if code != 200 {
		t.Fatalf("%d %v", code, out)
	}
	byID := map[string]map[string]any{}
	for _, tl := range out["message"].(map[string]any)["tools"].([]any) {
		m := tl.(map[string]any)
		byID[m["id"].(string)] = m
	}
	reqs := byID["career"]["requires"].([]any)
	if len(reqs) != 2 || reqs[0].(map[string]any)["id"] != "browser" || reqs[1].(map[string]any)["id"] != "pdf_tools" || reqs[1].(map[string]any)["minVersion"] != "0.0.1" || reqs[0].(map[string]any)["maxVersion"] != "" {
		t.Fatalf("%v", reqs)
	}
	if list, ok := byID["browser"]["requires"].([]any); !ok || len(list) != 0 {
		t.Fatalf("a tool that needs nothing has an empty list: %v", byID["browser"]["requires"])
	}
}

// Plan dependency-install, step 10: when an installed tool pins the one to update, the preview offers the tool that makes room: the dependent's newer
// version, whose own plan brings the blocked tool along. Offered only when that really is the case.
func TestABlockedUpdateOffersTheNewerDependentThatMakesRoom(t *testing.T) {
	pinned := tool_entity.ToolRequiredTool{ToolID: "id-career", RequiredSlug: "browser", MinVersion: "1.0.0", MaxVersion: "1.5.0"}
	setup := func(careerRequires string) (*env, string) {
		e, srv := newEnv(t), newCountingServer(t)
		e.installRow(t, "browser", "1.0.0", true, "running", "")
		e.installRow(t, "career", "1.0.0", true, "running", "", pinned)
		srv.files["/main.json"] = planFile("Main", planTool("browser", "2.0.0", "", 100), planTool("career", "2.0.0", careerRequires, 100))
		return e, addRepo(t, e, srv.srv.URL+"/main.json")
	}

	e, id := setup(`[{"id":"browser","minVersion":"2.0.0"}]`)
	code, out := e.plan(id, "browser", "")
	p := planOf(out)
	alts, _ := p["alternatives"].([]any)
	if code != http.StatusOK || p["ok"] != false || len(alts) != 1 {
		t.Fatalf("%d %v", code, p)
	}
	a := alts[0].(map[string]any)
	if a["toolId"] != "career" || a["forToolId"] != "browser" || a["version"] != "2.0.0" || a["repositoryId"] != id || a["versionId"] == "" {
		t.Fatalf("%v", a)
	}
	// the alternative is a real plan: updating Career brings Browser along
	_, out = e.plan(a["repositoryId"].(string), "career", "")
	if stepIDs(out) != "browser:update@2.0.0 career:update@2.0.0" {
		t.Fatalf("%s", stepIDs(out))
	}

	// a newer Career that still pins Browser makes no room: nothing is offered
	e, id = setup(`[{"id":"browser","maxVersion":"1.5.0"}]`)
	_, out = e.plan(id, "browser", "")
	if alts, _ = planOf(out)["alternatives"].([]any); len(alts) != 0 || planOf(out)["ok"] != false {
		t.Fatalf("%v", planOf(out))
	}
	// a plan that works has none
	_, out = e.plan(id, "career", "")
	if alts, _ = planOf(out)["alternatives"].([]any); alts == nil || len(alts) != 0 {
		t.Fatalf("a list, never null: %v", planOf(out)["alternatives"])
	}
}
