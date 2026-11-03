package localai

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a-digi/coco-logger/logger"
	"github.com/a-digi/coco-orm/orm"
	serverdi "github.com/a-digi/coco-server/server/di"
	"github.com/a-digi/coco-server/server/request"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/handler"
	"github.com/a-digi/cinqo/src/localai/hardware"
)

type fakeDI struct{ db *sql.DB }

func (f fakeDI) GetDatabaseManager() *orm.DatabaseManager {
	return &orm.DatabaseManager{Connector: &orm.Connector{DB: f.db}}
}
func (f fakeDI) GetLogger() logger.Logger { return nil }

var _ serverdi.Context = fakeDI{}

type fakeRequest struct {
	request.RequestContext
	w    *httptest.ResponseRecorder
	di   serverdi.Context
	body string
}

func (f fakeRequest) GetWriter() http.ResponseWriter { return f.w }
func (f fakeRequest) GetDI() serverdi.Context        { return f.di }
func (f fakeRequest) BindJSON(dest interface{}) error {
	return json.Unmarshal([]byte(f.body), dest)
}

// fakeRuntime says whether the local AI can be switched on.
type fakeRuntime struct{ err error }

func (f fakeRuntime) CanEnable() error { return f.err }

// fakeInstalls records what the handlers ask of it.
type fakeInstalls struct {
	startModel  string
	startSystem hardware.Info
	startErr    error
	removeErr   error
	removed     bool
	states      map[string]entity.FileInstall
}

func (f *fakeInstalls) ModelState(m catalog.Model) entity.FileInstall {
	if st, ok := f.states[m.ID]; ok {
		return st
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

func (f *fakeInstalls) RuntimeState(r catalog.Runtime) entity.FileInstall {
	if st, ok := f.states[r.ID]; ok {
		return st
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}

func (f *fakeInstalls) Start(modelID string, system hardware.Info) error {
	f.startModel, f.startSystem = modelID, system
	return f.startErr
}

func (f *fakeInstalls) Remove() error { f.removed = true; return f.removeErr }

func call(name string, runtime handler.Runtime, db *sql.DB, body string) *httptest.ResponseRecorder {
	return callWith(name, runtime, &fakeInstalls{}, db, body)
}

func callWith(name string, runtime handler.Runtime, installs handler.Installs, db *sql.DB, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.Handlers(runtime, installs)[name].ServeHTTP(fakeRequest{w: rec, di: fakeDI{db: db}, body: body})
	return rec
}

type apiResponse struct {
	Data struct {
		Settings map[string]any `json:"settings"`
		Status   struct {
			State     string `json:"state"`
			CanEnable bool   `json:"canEnable"`
			Reason    string `json:"reason"`
		} `json:"status"`
	} `json:"message"`
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) apiResponse {
	t.Helper()
	var out apiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return out
}

var (
	canRun    = fakeRuntime{}
	cannotRun = fakeRuntime{err: errManagedNotBuilt}
)

func TestGetReturnsTheOffByDefaultSettingsWithoutAnyOwnServerFields(t *testing.T) {
	db := openMainDB(t)
	rec := call("LocalAiGet", cannotRun, db, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	if got.Data.Status.State != "off" || got.Data.Settings["enabled"] != false || got.Data.Settings["modelId"] != "" {
		t.Errorf("got %s", rec.Body.String())
	}
	if got.Data.Status.CanEnable || got.Data.Status.Reason != "the managed runtime is not available yet" {
		t.Errorf("status = %+v, want it to say the managed runtime is not there yet", got.Data.Status)
	}
	for _, gone := range []string{"id", "mode", "externalBaseUrl"} {
		if _, there := got.Data.Settings[gone]; there {
			t.Errorf("settings must not contain %q: %v", gone, got.Data.Settings)
		}
	}
	if call("LocalAiGet", canRun, db, "").Body.Len() == 0 || !decode(t, call("LocalAiGet", canRun, db, "")).Data.Status.CanEnable {
		t.Error("with a runtime that can run, the status must say so")
	}
}

func TestUpdateStoresAValidChangeAndReportsConfigured(t *testing.T) {
	db := openMainDB(t)
	body := `{"enabled":true,"modelId":" qwen2.5-1.5b-instruct-q4_k_m ","contextTokens":8192,"idleMinutes":5}`
	rec := call("LocalAiUpdateSettings", canRun, db, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	got := decode(t, rec)
	if got.Data.Status.State != "configured" || got.Data.Settings["modelId"] != "qwen2.5-1.5b-instruct-q4_k_m" {
		t.Errorf("got %s", rec.Body.String())
	}

	withEngine(t, &fakeEngine{})
	if e := Entry(db); !e.IsAvailable() {
		t.Error("the saved settings must make the platform available")
	}
	again := decode(t, call("LocalAiGet", canRun, db, ""))
	if again.Data.Settings["contextTokens"] != float64(8192) || again.Data.Settings["idleMinutes"] != float64(5) {
		t.Errorf("a get must return what was saved: %v", again.Data.Settings)
	}
}

func TestAModelCanBeChosenBeforeTheRuntimeExistsButNotSwitchedOn(t *testing.T) {
	db := openMainDB(t)
	chosen := call("LocalAiUpdateSettings", cannotRun, db, `{"enabled":false,"modelId":"phi-4-mini-instruct-q4_k_m","contextTokens":4096,"idleMinutes":10}`)
	if chosen.Code != http.StatusOK {
		t.Fatalf("choosing a model while off: status %d body %s", chosen.Code, chosen.Body.String())
	}
	if decode(t, chosen).Data.Settings["modelId"] != "phi-4-mini-instruct-q4_k_m" {
		t.Errorf("the choice was not stored: %s", chosen.Body.String())
	}
	on := call("LocalAiUpdateSettings", cannotRun, db, `{"enabled":true,"modelId":"phi-4-mini-instruct-q4_k_m","contextTokens":4096,"idleMinutes":10}`)
	if on.Code != http.StatusBadRequest || !strings.Contains(on.Body.String(), "managed runtime is not available yet") {
		t.Errorf("switching on: status %d body %s, want 400 with the reason", on.Code, on.Body.String())
	}
	var enabled bool
	if err := db.QueryRow(`SELECT enabled FROM local_ai_settings WHERE id = 1`).Scan(&enabled); err != nil || enabled {
		t.Errorf("a refused switch-on must store nothing (enabled %v, err %v)", enabled, err)
	}
}

func TestUpdateRefusesBadInputAndStoresNothing(t *testing.T) {
	cases := map[string]string{
		"not json":                    `{"enabled":`,
		"an empty body":               `{}`,
		"a model that is not offered": `{"enabled":false,"modelId":"gpt-4","contextTokens":4096,"idleMinutes":10}`,
		"an address as the model":     `{"enabled":false,"modelId":"http://127.0.0.1:8080/v1","contextTokens":4096,"idleMinutes":10}`,
		"switched on with no model":   `{"enabled":true,"modelId":"","contextTokens":4096,"idleMinutes":10}`,
		"a context out of range":      `{"enabled":false,"modelId":"","contextTokens":1,"idleMinutes":10}`,
		"more context than the model": `{"enabled":false,"modelId":"qwen2.5-1.5b-instruct-q4_k_m","contextTokens":65536,"idleMinutes":10}`,
		"an idle time out of range":   `{"enabled":false,"modelId":"","contextTokens":4096,"idleMinutes":99999}`,
	}
	for name, body := range cases {
		db := openMainDB(t)
		rec := call("LocalAiUpdateSettings", canRun, db, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d body %s, want 400", name, rec.Code, rec.Body.String())
		}
		dec := json.NewDecoder(strings.NewReader(rec.Body.String()))
		var first map[string]any
		if err := dec.Decode(&first); err != nil || dec.More() {
			t.Errorf("%s: the answer must be exactly one JSON object, got %q", name, rec.Body.String())
		}
		var enabled bool
		var model string
		if err := db.QueryRow(`SELECT enabled, model_id FROM local_ai_settings WHERE id = 1`).Scan(&enabled, &model); err != nil || enabled || model != "" {
			t.Errorf("%s: a refused change stored something (enabled %v model %q err %v)", name, enabled, model, err)
		}
	}
}

func TestHandlersAreRegisteredUnderTheNamesOfTheRouteFile(t *testing.T) {
	h := Handlers()
	for _, name := range []string{"LocalAiGet", "LocalAiUpdateSettings", "LocalAiInstall", "LocalAiRemoveDownloads"} {
		if h[name] == nil {
			t.Errorf("no handler named %s", name)
		}
	}
	if len(h) != 4 {
		t.Errorf("%d handlers, want 4: a handler without a route is dead code, a route without a handler fails at start", len(h))
	}
}

func TestGetAlsoReturnsTheHardwareAndTheCatalog(t *testing.T) {
	db := openMainDB(t)
	rec := call("LocalAiGet", canRun, db, "")
	var out struct {
		Message struct {
			Hardware map[string]any   `json:"hardware"`
			Catalog  []map[string]any `json:"catalog"`
			Checked  string           `json:"catalogCheckedOn"`
		} `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"os", "arch", "ramBytes", "cpus", "supported"} {
		if _, ok := out.Message.Hardware[key]; !ok {
			t.Errorf("hardware has no %q: %v", key, out.Message.Hardware)
		}
	}
	if len(out.Message.Catalog) < 2 || out.Message.Checked == "" {
		t.Errorf("catalog %v checked %q", out.Message.Catalog, out.Message.Checked)
	}
	for _, m := range out.Message.Catalog {
		for _, key := range []string{"id", "name", "sizeBytes", "minRamBytes", "contextTokens", "license", "fits"} {
			if _, ok := m[key]; !ok {
				t.Errorf("model %v has no %q", m["id"], key)
			}
		}
	}
}

func TestInstallStartsTheDownloadsOfTheChosenModel(t *testing.T) {
	db := openMainDB(t)
	inst := &fakeInstalls{}
	rec := callWith("LocalAiInstall", cannotRun, inst, db, `{"modelId":"qwen2.5-1.5b-instruct-q4_k_m"}`)
	if rec.Code != http.StatusAccepted || inst.startModel != "qwen2.5-1.5b-instruct-q4_k_m" {
		t.Fatalf("status %d model %q body %s", rec.Code, inst.startModel, rec.Body.String())
	}
	if inst.startSystem.OS == "" || inst.startSystem.Arch == "" {
		t.Errorf("the system must be passed on: %+v", inst.startSystem)
	}
	if decode(t, rec).Data.Status.State != "off" {
		t.Errorf("the answer must be the current status: %s", rec.Body.String())
	}

	// Without a modelId the model of the settings is used.
	saveSettings(t, db, false, "phi-4-mini-instruct-q4_k_m")
	inst = &fakeInstalls{}
	if rec := callWith("LocalAiInstall", cannotRun, inst, db, `{}`); rec.Code != http.StatusAccepted || inst.startModel != "phi-4-mini-instruct-q4_k_m" {
		t.Errorf("status %d model %q", rec.Code, inst.startModel)
	}
	inst = &fakeInstalls{}
	if rec := callWith("LocalAiInstall", cannotRun, inst, db, `{"modelId":"  qwen2.5-1.5b-instruct-q4_k_m "}`); inst.startModel != "qwen2.5-1.5b-instruct-q4_k_m" || rec.Code != http.StatusAccepted {
		t.Errorf("a padded id must be trimmed: %q", inst.startModel)
	}
}

func TestInstallRefusesWhatCannotWorkWithPlainMessages(t *testing.T) {
	cases := map[string]struct {
		body    string
		err     error
		setup   func(*testing.T, *sql.DB)
		want    int
		message string
	}{
		"not json":           {body: `{"modelId":`, want: 400, message: "invalid request body"},
		"no model anywhere":  {body: `{}`, want: 400, message: "choose a model first"},
		"an unknown model":   {body: `{"modelId":"gpt-4"}`, err: entity.ErrUnknownModel, want: 400, message: "not one of the models"},
		"an unsupported box": {body: `{"modelId":"x"}`, err: entity.ErrUnsupportedSystem, want: 400, message: "not supported yet"},
		"an internal error":  {body: `{"modelId":"x"}`, err: errors.New("open /secret/path: permission denied"), want: 500, message: "could not be started"},
	}
	for name, c := range cases {
		db := openMainDB(t)
		inst := &fakeInstalls{startErr: c.err}
		rec := callWith("LocalAiInstall", cannotRun, inst, db, c.body)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.message) {
			t.Errorf("%s: status %d body %s, want %d containing %q", name, rec.Code, rec.Body.String(), c.want, c.message)
		}
		if strings.Contains(rec.Body.String(), "/secret/path") {
			t.Errorf("%s: an internal path leaked: %s", name, rec.Body.String())
		}
	}
}

func TestRemovingTheDownloadsIsRefusedWhileOnOrDownloading(t *testing.T) {
	db := openMainDB(t)
	inst := &fakeInstalls{}
	saveSettings(t, db, true, "phi-4-mini-instruct-q4_k_m")
	rec := callWith("LocalAiRemoveDownloads", canRun, inst, db, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "switch the local AI off") || inst.removed {
		t.Errorf("while on: status %d body %s removed %v", rec.Code, rec.Body.String(), inst.removed)
	}

	saveSettings(t, db, false, "phi-4-mini-instruct-q4_k_m")
	inst = &fakeInstalls{removeErr: entity.ErrDownloadRunning}
	rec = callWith("LocalAiRemoveDownloads", canRun, inst, db, "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "cancel it first") {
		t.Errorf("while downloading: status %d body %s", rec.Code, rec.Body.String())
	}

	inst = &fakeInstalls{removeErr: errors.New("unlink /secret/path: busy")}
	rec = callWith("LocalAiRemoveDownloads", canRun, inst, db, "")
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "/secret/path") {
		t.Errorf("internal error: status %d body %s", rec.Code, rec.Body.String())
	}

	inst = &fakeInstalls{}
	rec = callWith("LocalAiRemoveDownloads", canRun, inst, db, "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 || !inst.removed {
		t.Errorf("status %d body %q removed %v, want 204, nothing, removed", rec.Code, rec.Body.String(), inst.removed)
	}
}
