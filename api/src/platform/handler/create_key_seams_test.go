package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/platform"
	"github.com/a-digi/cinqo/src/platform/entity"
)

type fakeKeyRequest struct {
	request.RequestContext
	w    *httptest.ResponseRecorder
	body string
}

func (f fakeKeyRequest) GetWriter() http.ResponseWriter { return f.w }
func (f fakeKeyRequest) BindJSON(dest interface{}) error {
	return json.Unmarshal([]byte(f.body), dest)
}

func TestNoKeyCanBeAddedForAPlatformThatNeedsNone(t *testing.T) {
	if err := platform.Register(platform.Entry{
		Info:        entity.Platform{ID: "keys-keyless", Name: "Keyless"},
		Completer:   noopCompleter{},
		Credentials: func(context.Context) (string, string, error) { return "http://127.0.0.1:1", "", nil },
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	CreateKeyHandler(fakeKeyRequest{w: rec, body: `{"platform":"keys-keyless","key":"k","label":"l"}`})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "needs no API key") {
		t.Errorf("status %d body %s, want 400 \"needs no API key\"", rec.Code, rec.Body.String())
	}
}
