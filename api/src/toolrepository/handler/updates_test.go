package handler

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"gopkg.in/yaml.v3"

	repo_entity "github.com/a-digi/cinqo/src/toolrepository/entity"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
	"github.com/a-digi/cinqo/src/toolrepository/service"
	"github.com/a-digi/cinqo/src/toolrepository/updatecheck"
)

// updatesEnv registers a checker, as cinqo.Start does, over the test database.
func updatesEnv(t *testing.T) *env {
	e := newEnv(t)
	s := service.New(e.di.db, fetch.New())
	c := updatecheck.New(context.Background(), s).WithPause(0)
	t.Cleanup(c.Stop)
	e.di.values[DIKeyUpdateChecker] = c
	return e
}

func payload(out map[string]any) map[string]any {
	m, _ := out["message"].(map[string]any)
	return m
}

func checkOf(out map[string]any) map[string]any {
	c, _ := payload(out)["check"].(map[string]any)
	return c
}

func updatesOf(out map[string]any) []any { u, _ := payload(out)["updates"].([]any); return u }

// waitChecked polls GET /tool-updates until the check is no longer running, the
// way the Tools page does.
func waitChecked(t *testing.T, e *env) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		code, out := e.call(UpdatesHandler, "GET", "/api/v1/tool-updates", "", nil)
		if code != http.StatusOK {
			t.Fatalf("GET: %d %v", code, out)
		}
		if checkOf(out)["state"] != "running" {
			return out
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the check never finished")
	return nil
}

func TestUpdatesBeforeAnyRunIsIdleAndEmpty(t *testing.T) {
	e := updatesEnv(t)
	code, out := e.call(UpdatesHandler, "GET", "/api/v1/tool-updates", "", nil)
	if code != http.StatusOK || checkOf(out)["state"] != "idle" || len(updatesOf(out)) != 0 || payload(out)["lastCheckedAt"] != "" {
		t.Fatalf("%d %v", code, out)
	}
	if updatesOf(out) == nil {
		t.Error("updates must be an empty array, not null")
	}
}

func TestWithoutACheckerTheRoutesAnswer503(t *testing.T) {
	e := newEnv(t)
	for name, h := range map[string]func(request.RequestContext){"get": UpdatesHandler, "post": CheckUpdatesHandler} {
		if code, _ := e.call(h, "GET", "/x", "", nil); code != http.StatusServiceUnavailable {
			t.Errorf("%s: %d", name, code)
		}
	}
}

func TestTheCheckFindsAnUpdateAndKeepsReportingItWhenTheRepositoryGoesAway(t *testing.T) {
	e := updatesEnv(t)
	rs := newRepoServer(t)
	v100, v110 := toolZip(t, "demo", "1.0.0"), toolZip(t, "demo", "1.1.0")
	rs.put("/repo/files/demo-1.0.0.zip", v100)
	rs.put("/repo/files/demo-1.1.0.zip", v110)
	file := func(current string) []byte {
		return []byte(fmt.Sprintf(`{"formatVersion":1,"name":"Test","tools":[{"id":"demo","currentVersionId":%q,"versions":[
		 {"versionId":"a","name":"Demo","version":"1.0.0",%s},{"versionId":"b","name":"Demo","version":"1.1.0",%s}]}]}`,
			current, pkgEntry("files/demo-1.0.0.zip", v100, sha(v100)), pkgEntry("files/demo-1.1.0.zip", v110, sha(v110))))
	}
	rs.put("/repo/tools.json", file("a"))

	code, out := e.call(AddHandler, "POST", "/api/v1/tool-repositories", fmt.Sprintf(`{"url":%q}`, rs.srv.URL+"/repo/tools.json"), nil)
	if code != http.StatusCreated {
		t.Fatalf("add: %d %v", code, out)
	}
	repoID := payload(out)["id"].(string)
	if code, out = e.call(InstallHandler, "POST", "/x", "", map[string]string{"id": repoID, "toolId": "demo"}); code != http.StatusCreated {
		t.Fatalf("install: %d %v", code, out)
	}

	// up to date: a check runs, finds nothing
	code, out = e.call(CheckUpdatesHandler, "POST", "/api/v1/tool-updates/check", "", nil)
	if code != http.StatusAccepted || checkOf(out)["state"] != "running" || checkOf(out)["trigger"] != "manual" {
		t.Fatalf("start: %d %v", code, out)
	}
	out = waitChecked(t, e)
	if c := checkOf(out); c["state"] != "done" || c["checkedRepositories"] != float64(1) || len(c["failed"].([]any)) != 0 {
		t.Fatalf("%v", c)
	}
	if len(updatesOf(out)) != 0 || payload(out)["lastCheckedAt"] == "" {
		t.Fatalf("no update yet: %v", out)
	}

	// the repository publishes 1.1.0; the next check picks it up
	rs.put("/repo/tools.json", file("b"))
	code, out = e.call(CheckUpdatesHandler, "POST", "/x", "", nil)
	if code != http.StatusAccepted {
		t.Fatalf("second start: %d", code)
	}
	runID := checkOf(out)["runId"]
	out = waitChecked(t, e)
	ups := updatesOf(out)
	if checkOf(out)["runId"] != runID || len(ups) != 1 {
		t.Fatalf("%v", out)
	}
	u := ups[0].(map[string]any)
	if u["slug"] != "demo" || u["installedVersion"] != "1.0.0" || u["latestVersion"] != "1.1.0" || u["versionId"] != "b" ||
		u["repositoryId"] != repoID || u["repositoryName"] != "Test" {
		t.Errorf("%v", u)
	}

	// the repository server goes away: the check reports it, and the update found
	// earlier is still listed (it comes from the stored catalog)
	rs.srv.Close()
	e.call(CheckUpdatesHandler, "POST", "/x", "", nil)
	out = waitChecked(t, e)
	failed := checkOf(out)["failed"].([]any)
	if len(failed) != 1 || failed[0].(map[string]any)["repositoryId"] != repoID || failed[0].(map[string]any)["error"] == "" {
		t.Fatalf("failure not reported: %v", checkOf(out))
	}
	if len(updatesOf(out)) != 1 {
		t.Errorf("the update must remain: %v", out)
	}
}

func TestStartingWhileRunningReturnsTheSameRun(t *testing.T) {
	e := newEnv(t)
	// a checker over a repository list that blocks, so the run stays running
	hold := make(chan struct{})
	c := updatecheck.New(context.Background(), blockingRepos{hold}).WithPause(0)
	t.Cleanup(c.Stop)
	e.di.values[DIKeyUpdateChecker] = c

	code, first := e.call(CheckUpdatesHandler, "POST", "/x", "", nil)
	code2, second := e.call(CheckUpdatesHandler, "POST", "/x", "", nil)
	if code != http.StatusAccepted || code2 != http.StatusOK || checkOf(first)["runId"] != checkOf(second)["runId"] || checkOf(second)["state"] != "running" {
		t.Fatalf("%d %d %v %v", code, code2, checkOf(first), checkOf(second))
	}
	if code, out := e.call(UpdatesHandler, "GET", "/x", "", nil); code != http.StatusOK || checkOf(out)["state"] != "running" {
		t.Errorf("GET while running: %d %v", code, out)
	}
	close(hold)
	waitChecked(t, e)
}

func TestTheUpdateRoutesRequireToolManage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "config", "routes", "route-toolupdates.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Routes []struct {
			Path, Method, Executor, Security string
			Scopes                           []string
		} `yaml:"routes"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	routesGo, _ := os.ReadFile(filepath.Join("..", "..", "..", "config", "routes", "routes.go"))
	if len(doc.Routes) != 2 {
		t.Fatalf("%d routes", len(doc.Routes))
	}
	for _, r := range doc.Routes {
		if !strings.HasPrefix(r.Path, "/api/v1/tool-updates") || r.Security != "authenticated" ||
			strings.Join(r.Scopes, ",") != "tool:manage,cinqo:super:admin" || !strings.Contains(string(routesGo), `"`+r.Executor+`"`) {
			t.Errorf("%+v", r)
		}
	}
}

// blockingRepos is one repository whose refresh waits until hold is closed.
type blockingRepos struct{ hold chan struct{} }

func (b blockingRepos) List() ([]*repo_entity.Repository, error) {
	return []*repo_entity.Repository{{ID: "x", Name: "x"}}, nil
}

func (b blockingRepos) Refresh(ctx context.Context, id string) (*repo_entity.Repository, error) {
	select {
	case <-b.hold:
	case <-ctx.Done():
	}
	return &repo_entity.Repository{ID: id, Name: "x", LastStatus: repo_entity.StatusOK}, nil
}
