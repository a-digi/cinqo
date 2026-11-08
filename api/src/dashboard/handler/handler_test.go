package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/dashboard"
)

type stubProvider struct {
	def  dashboard.Definition
	data []byte
	err  error
}

func (s stubProvider) Definition() dashboard.Definition { return s.def }
func (s stubProvider) Data(context.Context, dashboard.Caller) ([]byte, error) {
	return s.data, s.err
}

func def(key, scope string) dashboard.Definition {
	return dashboard.Definition{
		Provider: "t", Key: key, Title: key, Description: "d", Kind: dashboard.KindStat,
		Size: dashboard.Size{W: 2, H: 1}, RefreshSeconds: 30, RequiredScope: scope,
	}
}

func engine(t *testing.T, ps ...dashboard.Provider) *dashboard.Engine {
	r := dashboard.NewRegistry()
	for _, p := range ps {
		if err := r.Register(p); err != nil {
			t.Fatal(err)
		}
	}
	return dashboard.NewEngine(r)
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestServeWidgetsListsOnlyWhatTheCallerMaySee(t *testing.T) {
	e := engine(t,
		stubProvider{def: def("open", "")},
		stubProvider{def: def("admin", "tool:t:admin")},
	)
	rec := httptest.NewRecorder()
	serveWidgets(rec, e, dashboard.Caller{UserID: "u"}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	list, _ := decode(t, rec)["message"].([]any)
	if len(list) != 1 {
		t.Fatalf("widgets = %v", list)
	}
	w := list[0].(map[string]any)
	if w["id"] != "t.open" || w["provider"] != "t" || w["kind"] != "stat" || w["refreshSeconds"] != float64(30) {
		t.Errorf("widget = %v", w)
	}
	if size := w["size"].(map[string]any); size["w"] != float64(2) || size["h"] != float64(1) {
		t.Errorf("size = %v", size)
	}
}

func TestServeWidgetsEmptyIsAnArrayNotNull(t *testing.T) {
	rec := httptest.NewRecorder()
	serveWidgets(rec, engine(t), dashboard.Caller{}, nil)
	if list, ok := decode(t, rec)["message"].([]any); !ok || len(list) != 0 {
		t.Errorf("message = %v", decode(t, rec)["message"])
	}
}

func TestServeDataStates(t *testing.T) {
	e := engine(t,
		stubProvider{def: def("ok", ""), data: []byte(`{"label":"Jobs","value":7}`)},
		stubProvider{def: def("down", ""), err: dashboard.ErrUnavailable},
		stubProvider{def: def("bad", ""), data: []byte(`{"label":"x"}`)},
		stubProvider{def: def("secret", "tool:t:admin"), data: []byte(`{"label":"S","value":1}`)},
	)
	caller := dashboard.Caller{UserID: "u"}
	for id, want := range map[string]string{"t.ok": "ok", "t.down": "unavailable", "t.bad": "error"} {
		rec := httptest.NewRecorder()
		serveData(context.Background(), rec, e, caller, id)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: a widget failure must still be a 200, got %d", id, rec.Code)
			continue
		}
		msg := decode(t, rec)["message"].(map[string]any)
		if msg["state"] != want || msg["kind"] != "stat" || msg["schemaVersion"] != float64(1) {
			t.Errorf("%s: %v", id, msg)
		}
		if want == "ok" && msg["data"].(map[string]any)["value"] != float64(7) {
			t.Errorf("%s: data %v", id, msg["data"])
		}
		if want != "ok" && msg["data"] != nil {
			t.Errorf("%s: a failed widget must carry no data: %v", id, msg["data"])
		}
	}
	for _, id := range []string{"t.secret", "t.missing"} {
		rec := httptest.NewRecorder()
		serveData(context.Background(), rec, e, caller, id)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", id, rec.Code)
		}
	}
}

type memStore struct {
	saved map[string][]dashboard.LayoutEntry
	err   error
}

func (m *memStore) ReplaceAll(userID string, entries []dashboard.LayoutEntry) error {
	if m.err != nil {
		return m.err
	}
	if m.saved == nil {
		m.saved = map[string][]dashboard.LayoutEntry{}
	}
	m.saved[userID] = entries
	return nil
}

func put(t *testing.T, e *dashboard.Engine, caller dashboard.Caller, store layoutWriter, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/v1/dashboard/layout", strings.NewReader(body))
	_ = saveLayout(rec, r, e, caller, store)
	return rec
}

func TestServeWidgetsAppliesTheSavedLayout(t *testing.T) {
	e := engine(t,
		stubProvider{def: def("a", "")}, stubProvider{def: def("b", "")}, stubProvider{def: def("c", "")},
	)
	saved := []dashboard.LayoutEntry{{WidgetID: "t.c"}, {WidgetID: "t.a", Hidden: true}, {WidgetID: "t.gone"}}
	rec := httptest.NewRecorder()
	serveWidgets(rec, e, dashboard.Caller{UserID: "u"}, saved)
	list := decode(t, rec)["message"].([]any)
	var got []string
	hidden := map[string]bool{}
	for _, x := range list {
		w := x.(map[string]any)
		got = append(got, w["id"].(string))
		hidden[w["id"].(string)] = w["hidden"].(bool)
	}
	if want := []string{"t.c", "t.a", "t.b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if hidden["t.c"] || !hidden["t.a"] || hidden["t.b"] {
		t.Errorf("hidden = %v", hidden)
	}
}

func TestSaveLayout(t *testing.T) {
	e := engine(t,
		stubProvider{def: def("a", "")},
		stubProvider{def: def("b", "")},
		stubProvider{def: def("secret", "tool:t:admin")},
	)
	caller := dashboard.Caller{UserID: "alice"}
	store := &memStore{}

	rec := put(t, e, caller, store, `{"widgets":[{"id":"t.b","hidden":true},{"id":"t.secret"},{"id":"t.a"},{"id":"t.b"},{"id":"t.nope"}]}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	want := []dashboard.LayoutEntry{{WidgetID: "t.b", Hidden: true}, {WidgetID: "t.a"}}
	if !reflect.DeepEqual(store.saved["alice"], want) {
		t.Errorf("saved %v, want %v (forbidden, unknown and duplicate ids dropped)", store.saved["alice"], want)
	}

	if rec := put(t, e, caller, store, `{"widgets":[]}`); rec.Code != http.StatusNoContent || len(store.saved["alice"]) != 0 {
		t.Errorf("an empty list must clear: %d %v", rec.Code, store.saved["alice"])
	}

	for name, body := range map[string]string{
		"not json":       `nope`,
		"no widgets key": `{}`,
		"null widgets":   `{"widgets":null}`,
		"wrong type":     `{"widgets":"a"}`,
	} {
		if rec := put(t, e, caller, store, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, rec.Code)
		}
	}

	many := make([]string, dashboard.MaxLayoutEntries+1)
	for i := range many {
		many[i] = `{"id":"t.a"}`
	}
	if rec := put(t, e, caller, store, `{"widgets":[`+strings.Join(many, ",")+`]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("too many entries: status %d, want 400", rec.Code)
	}

	failing := &memStore{err: errors.New("disk full")}
	if rec := put(t, e, caller, failing, `{"widgets":[]}`); rec.Code != http.StatusInternalServerError {
		t.Errorf("storage failure: status %d, want 500", rec.Code)
	}
}
