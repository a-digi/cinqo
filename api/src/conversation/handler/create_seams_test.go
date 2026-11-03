package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/chatcompleter"
	"github.com/a-digi/cinqo/src/platform/entity"
)

// fakeCreateRequest is just enough of a request for CreateHandler's checks that run before any database is touched.
type fakeCreateRequest struct {
	request.RequestContext
	w    *httptest.ResponseRecorder
	body string
}

func (f fakeCreateRequest) GetWriter() http.ResponseWriter { return f.w }
func (f fakeCreateRequest) BindJSON(dest interface{}) error {
	return json.Unmarshal([]byte(f.body), dest)
}

type refusingCompleter struct{}

func (refusingCompleter) ChatCompletion(context.Context, *http.Client, string, string, string, []chatcompleter.Message, []chatcompleter.ToolDef) (*chatcompleter.ChatCompletionResult, error) {
	return nil, errors.New("not called")
}

func createWith(body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	CreateHandler(fakeCreateRequest{w: rec, body: body})
	return rec
}

func TestCreateRefusesWhatAPlatformsSeamsForbid(t *testing.T) {
	available := false
	models := []string{"only-model"}
	if err := platform.Register(platform.Entry{
		Info:        entity.Platform{ID: "create-seams", Name: "Seams"},
		Completer:   refusingCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
		Models:      func() []string { return models },
		Available:   func() bool { return available },
		NoTools:     true,
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name      string
		available bool
		body      string
		wantText  string
	}{
		{"unavailable platform", false, `{"platformId":"create-seams","model":"only-model"}`, "not available"},
		{"model outside the dynamic list", true, `{"platformId":"create-seams","model":"other"}`, "selectable models"},
		{"no model although the list has one", true, `{"platformId":"create-seams"}`, "selectable models"},
		{"composer mode on a no-tools platform", true, `{"platformId":"create-seams","model":"only-model","orchestrationMode":"composer"}`, "offers no tools"},
		{"an allowlist on a no-tools platform", true, `{"platformId":"create-seams","model":"only-model","allowedTools":["list_jobs"]}`, "offers no tools"},
	}
	for _, c := range cases {
		available = c.available
		rec := createWith(c.body)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.wantText) {
			t.Errorf("%s: status %d body %s, want 400 containing %q", c.name, rec.Code, rec.Body.String(), c.wantText)
		}
	}
}
