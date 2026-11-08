package localai

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/dialect/phijson"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
)

// The tests in this file need the Phi dialect; they are deleted together with it.

func TestEveryDialectTheCatalogNamesIsRegisteredByDefault(t *testing.T) {
	cat, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	reg := defaultDialects()
	for _, m := range cat.Models {
		if m.ToolDialect == "" {
			continue
		}
		if _, ok := reg.Lookup(m.ToolDialect); !ok {
			t.Errorf("%s names the dialect %q, which defaultDialects does not register", m.ID, m.ToolDialect)
		}
	}
	if got := strings.Join(reg.Names(), ","); got != phijson.Name+",qwen-json" {
		t.Errorf("registered = %q", got)
	}
}

func TestTheCompleterTeachesPhiItsDialectAndReadsTheAnswerBack(t *testing.T) {
	db := openMainDB(t)
	saveSettings(t, db, true, phiID)
	c := completer{settings: query.NewSettingsQueryRepo(db)}
	e := &recordingEngine{reply: `[{"name": "list_profiles", "arguments": {}}]`}
	withEngine(t, e)
	withDialects(t, defaultDialects())
	res, err := c.ChatCompletion(context.Background(), http.DefaultClient, "", "", phiID, []chatcompleter.Message{{Role: "user", Content: "list"}}, toolDefs())
	if err != nil {
		t.Fatal(err)
	}
	if len(e.gotTools) != 0 || len(e.gotMessages) != 2 || !strings.Contains(e.gotMessages[0].Content, "<|tool|>") {
		t.Errorf("phi request: tools %d messages %+v", len(e.gotTools), e.gotMessages)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].Name != "list_profiles" || res.Usage.Cost == nil || *res.Usage.Cost != 0 {
		t.Errorf("phi result: %+v", res)
	}
}

func TestTheDefaultRegistryGivesPhiAndQwenTools(t *testing.T) {
	withDialects(t, defaultDialects())
	for _, id := range []string{phiID, qwenID} {
		if !toolsUsable(id) {
			t.Errorf("%s must have tools with the default dialects", id)
		}
	}
}
