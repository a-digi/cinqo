package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/toolrepository/service"
)

func TestNativeRepositoryOverHTTP(t *testing.T) {
	e := newEnv(t)
	link := "https://raw.githubusercontent.com/a-digi/cinqo/0.0.1/tools/repository.json"
	if err := service.New(e.di.db, nil).SyncNative(link); err != nil {
		t.Fatal(err)
	}

	code, out := e.call(ListHandler, "GET", "/api/v1/tool-repositories", "", nil)
	list, _ := out["message"].([]any)
	if code != http.StatusOK || len(list) != 1 {
		t.Fatalf("list: %d %v", code, out)
	}
	repo := list[0].(map[string]any)
	if repo["isNative"] != true || repo["url"] != link || repo["lastStatus"] != "never" {
		t.Errorf("%v", repo)
	}
	id := repo["id"].(string)

	code, out = e.call(DeleteHandler, "DELETE", "/x", "", map[string]string{"id": id})
	if code != http.StatusForbidden || !strings.Contains(message(out), "cannot be removed") {
		t.Errorf("delete native: %d %v", code, out)
	}
	if code, out = e.call(GetHandler, "GET", "/x", "", map[string]string{"id": id}); code != http.StatusOK || out["message"].(map[string]any)["isNative"] != true {
		t.Errorf("detail: %d %v", code, out)
	}
}
