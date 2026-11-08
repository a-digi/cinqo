package localai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	req  *http.Request // the request the handler sees (a plain one when nil)
}

func (f fakeRequest) GetWriter() http.ResponseWriter { return f.w }
func (f fakeRequest) GetDI() serverdi.Context        { return f.di }
func (f fakeRequest) GetRequest() *http.Request {
	if f.req != nil {
		return f.req
	}
	return httptest.NewRequest("POST", "/", nil)
}
func (f fakeRequest) BindJSON(dest interface{}) error {
	return json.Unmarshal([]byte(f.body), dest)
}

// fakeRuntime is a runtime whose answers the tests choose.
type fakeRuntime struct {
	err     error
	status  entity.EngineStatus
	result  entity.TestResult
	testErr error
	asked   *[]string // the model ids CanEnable was asked about
	tested  *[]string // the model ids Test was asked about
	ctxs    *[]context.Context
	during  func(context.Context) // runs inside Test, with the context the handler gave it
}

func (f fakeRuntime) CanUseTools(string) bool { return false }

func (f fakeRuntime) CanEnable(modelID string) error {
	if f.asked != nil {
		*f.asked = append(*f.asked, modelID)
	}
	return f.err
}

func (f fakeRuntime) Status() entity.EngineStatus {
	if f.status.State == "" {
		return entity.EngineStatus{State: entity.EngineStopped}
	}
	return f.status
}

func (f fakeRuntime) Test(ctx context.Context, modelID string) (entity.TestResult, error) {
	if f.ctxs != nil {
		*f.ctxs = append(*f.ctxs, ctx)
	}
	if f.during != nil {
		f.during(ctx)
	}
	if f.tested != nil {
		*f.tested = append(*f.tested, modelID)
	}
	return f.result, f.testErr
}

// fakeInstalls records what the handlers ask of it.
type fakeInstalls struct {
	startModel  string
	startSystem hardware.Info
	startErr    error
	removeErr   error
	removed     bool
	starts      int
	cancels     int
	modelFail   *entity.Failure
	runtimeFail *entity.Failure
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
	f.starts++
	return f.startErr
}

func (f *fakeInstalls) CancelDownloads() { f.cancels++ }

func (f *fakeInstalls) ModelFailure(catalog.Model) *entity.Failure     { return f.modelFail }
func (f *fakeInstalls) RuntimeFailure(catalog.Runtime) *entity.Failure { return f.runtimeFail }

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
	withMac(t)
	body := `{"enabled":true,"modelId":" qwen2.5-1.5b-instruct-q4_k_m ","contextTokens":8192,"idleMinutes":5}`
	ready := &fakeInstalls{states: map[string]entity.FileInstall{"qwen2.5-1.5b-instruct-q4_k_m": {State: entity.InstallReady}, macRuntime: {State: entity.InstallReady}}}
	rec := callWith("LocalAiUpdateSettings", canRun, ready, db, body)
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
	for _, name := range []string{"LocalAiGet", "LocalAiUpdateSettings", "LocalAiInstall", "LocalAiRemoveDownloads", "LocalAiTest"} {
		if h[name] == nil {
			t.Errorf("no handler named %s", name)
		}
	}
	if len(h) != 5 {
		t.Errorf("%d handlers, want 5: a handler without a route is dead code, a route without a handler fails at start", len(h))
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

func TestTheTestEndpointRunsTheChosenModelAndReturnsTheTimedResult(t *testing.T) {
	db := openMainDB(t)
	var tested []string
	rt := fakeRuntime{tested: &tested, result: entity.TestResult{OK: true, StartSeconds: 2.5, Seconds: 0.4, PromptTokens: 12, CompletionTokens: 4, TokensPerSecond: 10, Reply: "OK"}}
	rec := callWith("LocalAiTest", rt, &fakeInstalls{}, db, `{"modelId":" phi-4-mini-instruct-q4_k_m "}`)
	if rec.Code != http.StatusOK || len(tested) != 1 || tested[0] != "phi-4-mini-instruct-q4_k_m" {
		t.Fatalf("status %d tested %v body %s", rec.Code, tested, rec.Body.String())
	}
	var out struct {
		Message entity.TestResult `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Message != rt.result {
		t.Errorf("body = %s (%v)", rec.Body.String(), err)
	}
	for _, key := range []string{`"ok":true`, `"startSeconds":2.5`, `"tokensPerSecond":10`, `"reply":"OK"`} {
		if !strings.Contains(rec.Body.String(), key) {
			t.Errorf("the JSON must contain %s: %s", key, rec.Body.String())
		}
	}

	// Without a modelId the model of the settings is tested.
	saveSettings(t, db, false, "qwen2.5-1.5b-instruct-q4_k_m")
	tested = nil
	if rec := callWith("LocalAiTest", rt, &fakeInstalls{}, db, `{}`); rec.Code != http.StatusOK || len(tested) != 1 || tested[0] != "qwen2.5-1.5b-instruct-q4_k_m" {
		t.Errorf("status %d tested %v", rec.Code, tested)
	}
}

func TestTheTestEndpointAnswersAFailureOfTheModelAsAResultNotAnError(t *testing.T) {
	db := openMainDB(t)
	rt := fakeRuntime{result: entity.TestResult{OK: false, Error: "the model is not downloaded yet"}}
	rec := callWith("LocalAiTest", rt, &fakeInstalls{}, db, `{"modelId":"x"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":false`) || !strings.Contains(rec.Body.String(), "not downloaded yet") {
		t.Errorf("status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestTheTestEndpointRefusesWhatCannotBeTested(t *testing.T) {
	cases := map[string]struct {
		body    string
		rt      fakeRuntime
		want    int
		message string
	}{
		"not json":          {`{"modelId":`, fakeRuntime{}, 400, "invalid request body"},
		"no model anywhere": {`{}`, fakeRuntime{}, 400, "choose a model first"},
		"an unknown model":  {`{"modelId":"gpt-4"}`, fakeRuntime{testErr: entity.ErrUnknownModel}, 400, "not one of the models"},
		"an internal error": {`{"modelId":"x"}`, fakeRuntime{testErr: errors.New("open /secret/path: denied")}, 500, "could not be run"},
	}
	for name, c := range cases {
		db := openMainDB(t)
		rec := callWith("LocalAiTest", c.rt, &fakeInstalls{}, db, c.body)
		if rec.Code != c.want || !strings.Contains(rec.Body.String(), c.message) || strings.Contains(rec.Body.String(), "/secret/path") {
			t.Errorf("%s: status %d body %s, want %d containing %q", name, rec.Code, rec.Body.String(), c.want, c.message)
		}
	}
}

func TestTheModelOfTheRequestIsWhatTheRuntimeIsAskedAbout(t *testing.T) {
	db := openMainDB(t)
	var asked []string
	rt := fakeRuntime{asked: &asked}
	call("LocalAiUpdateSettings", rt, db, `{"enabled":true,"modelId":" qwen2.5-1.5b-instruct-q4_k_m ","contextTokens":4096,"idleMinutes":10}`)
	found := false
	for _, id := range asked {
		if id == "qwen2.5-1.5b-instruct-q4_k_m" {
			found = true
		}
		if id == " qwen2.5-1.5b-instruct-q4_k_m " {
			t.Error("the id must be trimmed before the runtime is asked")
		}
	}
	if !found {
		t.Errorf("the runtime must be asked about the model of the request, was asked about %v", asked)
	}
}

func TestSwitchingOnIsRefusedWithTheReasonOfThatModel(t *testing.T) {
	db := openMainDB(t)
	rt := fakeRuntime{err: errors.New("the model is not downloaded yet")}
	rec := call("LocalAiUpdateSettings", rt, db, `{"enabled":true,"modelId":"phi-4-mini-instruct-q4_k_m","contextTokens":4096,"idleMinutes":10}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "the model is not downloaded yet") {
		t.Errorf("status %d body %s", rec.Code, rec.Body.String())
	}
	var enabled bool
	if err := db.QueryRow(`SELECT enabled FROM local_ai_settings WHERE id = 1`).Scan(&enabled); err != nil || enabled {
		t.Error("nothing may be stored")
	}
}

func TestGetReportsTheStateOfTheModelProgramAndTheReasonPerModel(t *testing.T) {
	db := openMainDB(t)
	rt := fakeRuntime{status: entity.EngineStatus{State: entity.EngineFailed, ModelID: "m", Error: "the model program stopped unexpectedly"}, err: errors.New("choose a model first")}
	rec := call("LocalAiGet", rt, db, "")
	var out struct {
		Message struct {
			Engine  entity.EngineStatus `json:"engine"`
			Catalog []struct {
				Enable struct {
					OK     bool   `json:"ok"`
					Reason string `json:"reason"`
				} `json:"enable"`
			} `json:"catalog"`
		} `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Message.Engine.State != entity.EngineFailed || out.Message.Engine.Error != "the model program stopped unexpectedly" {
		t.Errorf("engine = %+v", out.Message.Engine)
	}
	for _, m := range out.Message.Catalog {
		if m.Enable.OK || m.Enable.Reason != "choose a model first" {
			t.Errorf("enable = %+v", m.Enable)
		}
	}
}

func TestTheTestRunsUnderTheRequestAndAFourMinuteLimit(t *testing.T) {
	db := openMainDB(t)
	var ctxs []context.Context
	reqCtx, cancel := context.WithCancel(context.Background())
	stoppedWithTheRequest := false
	rt := fakeRuntime{ctxs: &ctxs, result: entity.TestResult{OK: true}, during: func(ctx context.Context) {
		cancel() // the page is left while the model is working
		stoppedWithTheRequest = ctx.Err() != nil
	}}
	req := httptest.NewRequest("POST", "/", nil).WithContext(reqCtx)
	rec := httptest.NewRecorder()
	handler.Handlers(rt, &fakeInstalls{})["LocalAiTest"].ServeHTTP(fakeRequest{w: rec, di: fakeDI{db: db}, body: `{"modelId":"phi-4-mini-instruct-q4_k_m"}`, req: req})
	if len(ctxs) != 1 {
		t.Fatalf("the test was not run: %d %s", len(ctxs), rec.Body.String())
	}
	deadline, ok := ctxs[0].Deadline()
	if left := time.Until(deadline); !ok || left < 3*time.Minute+30*time.Second || left > 4*time.Minute {
		t.Errorf("the test must be limited to 4 minutes: %v (%v left)", ok, left)
	}
	if !stoppedWithTheRequest {
		t.Errorf("leaving the page (the request ends) must stop the test")
	}
}

func TestASizeTheComputerHasNoMemoryForIsRefusedAndNothingIsStored(t *testing.T) {
	restore := handler.SetMachine(func() hardware.Info { return hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8} })
	defer restore()
	db := openMainDB(t)
	put := func(tokens int) *httptest.ResponseRecorder {
		return call("LocalAiUpdateSettings", canRun, db, fmt.Sprintf(`{"enabled":false,"modelId":"phi-4-mini-instruct-q4_k_m","contextTokens":%d,"idleMinutes":10}`, tokens))
	}
	for _, tokens := range []int{4096, 16384, 65536} {
		if rec := put(tokens); rec.Code != http.StatusOK {
			t.Errorf("%d tokens fit 16 GiB: status %d %s", tokens, rec.Code, rec.Body.String())
		}
	}
	rec := put(131072)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "a context of 131,072 tokens needs about 21.4 GiB") || !strings.Contains(rec.Body.String(), "this computer has 16.0 GiB") || !strings.Contains(rec.Body.String(), "Choose at most 65,536 tokens") {
		t.Fatalf("status %d body %s", rec.Code, rec.Body.String())
	}
	stored := decode(t, call("LocalAiGet", canRun, db, ""))
	if stored.Data.Settings["contextTokens"] != float64(65536) {
		t.Errorf("a refused change must store nothing: %v", stored.Data.Settings)
	}
}

func TestAnUnknownAmountOfMemoryNeverRefusesASize(t *testing.T) {
	restore := handler.SetMachine(func() hardware.Info { return hardware.Info{OS: "linux", Arch: "amd64", RAMBytes: 0, CPUs: 4} })
	defer restore()
	db := openMainDB(t)
	rec := call("LocalAiUpdateSettings", canRun, db, `{"enabled":false,"modelId":"phi-4-mini-instruct-q4_k_m","contextTokens":131072,"idleMinutes":10}`)
	if rec.Code != http.StatusOK {
		t.Errorf("status %d %s", rec.Code, rec.Body.String())
	}
}

func TestAMachineThatCannotHoldTheModelStillLetsItBeChosenWhileOff(t *testing.T) {
	restore := handler.SetMachine(func() hardware.Info { return hardware.Info{OS: "linux", Arch: "amd64", RAMBytes: 4 << 30, CPUs: 4} })
	defer restore()
	db := openMainDB(t)
	// Phi needs 5.9 GiB; this computer has 4. The "needs more memory" rule speaks about that when switching on; choosing it stays possible.
	rec := call("LocalAiUpdateSettings", canRun, db, `{"enabled":false,"modelId":"phi-4-mini-instruct-q4_k_m","contextTokens":4096,"idleMinutes":10}`)
	if rec.Code != http.StatusOK {
		t.Errorf("status %d %s", rec.Code, rec.Body.String())
	}
}

// --- plan 205, step 1: switching on starts what is missing ---

const (
	modelOn    = `{"enabled":true,"modelId":"qwen2.5-1.5b-instruct-q4_k_m","contextTokens":4096,"idleMinutes":10}`
	modelOff   = `{"enabled":false,"modelId":"qwen2.5-1.5b-instruct-q4_k_m","contextTokens":4096,"idleMinutes":10}`
	macRuntime = "llama-cpp-b12064-macos-arm64"
)

func withMac(t *testing.T) {
	t.Helper()
	restore := handler.SetMachine(func() hardware.Info { return hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8} })
	t.Cleanup(restore)
}

func storedEnabled(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var enabled bool
	if err := db.QueryRow(`SELECT enabled FROM local_ai_settings WHERE id = 1`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	return enabled
}

func TestSwitchingOnWithNothingDownloadedSavesTheWishAndStartsTheDownloads(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{}
	rec := callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn)
	if rec.Code != http.StatusOK || !storedEnabled(t, db) {
		t.Fatalf("status %d enabled %v body %s", rec.Code, storedEnabled(t, db), rec.Body.String())
	}
	if inst.starts != 1 || inst.startModel != "qwen2.5-1.5b-instruct-q4_k_m" || inst.startSystem.OS != "darwin" {
		t.Errorf("the missing downloads must be started once for the chosen model: %+v", inst)
	}
}

func TestSwitchingOnWithOnlyTheProgramMissingStartsTheDownloadsToo(t *testing.T) {
	withMac(t) // the user's case: the models are on disk, the program is not
	db := openMainDB(t)
	inst := &fakeInstalls{states: map[string]entity.FileInstall{"qwen2.5-1.5b-instruct-q4_k_m": {State: entity.InstallReady}}}
	if rec := callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn); rec.Code != http.StatusOK || inst.starts != 1 {
		t.Fatalf("status %d starts %d body %s", rec.Code, inst.starts, rec.Body.String())
	}
}

func TestSwitchingOnWithEverythingThereStartsNothing(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{states: map[string]entity.FileInstall{
		"qwen2.5-1.5b-instruct-q4_k_m": {State: entity.InstallReady}, macRuntime: {State: entity.InstallReady},
	}}
	if rec := callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn); rec.Code != http.StatusOK || inst.starts != 0 {
		t.Fatalf("status %d starts %d", rec.Code, inst.starts)
	}
}

func TestADownloadThatIsRunningIsJoinedNotSkipped(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{states: map[string]entity.FileInstall{
		"qwen2.5-1.5b-instruct-q4_k_m": {State: entity.InstallReady}, macRuntime: {State: entity.InstallDownloading, DownloadID: "d1"},
	}}
	if rec := callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn); rec.Code != http.StatusOK || inst.starts != 1 {
		t.Fatalf("a running program download is still not ready, so the start (which joins it) is asked: status %d starts %d", rec.Code, inst.starts)
	}
}

func TestWhatNoDownloadFixesStillRefusesSwitchingOnAndStartsNothing(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{}
	rec := callWith("LocalAiUpdateSettings", fakeRuntime{err: errors.New("this computer has 4.0 GiB of memory and the model needs about 6.0 GiB")}, inst, db, modelOn)
	if rec.Code != http.StatusBadRequest || inst.starts != 0 || storedEnabled(t, db) {
		t.Fatalf("status %d starts %d enabled %v", rec.Code, inst.starts, storedEnabled(t, db))
	}
}

func TestADownloadThatCannotStartKeepsTheSavedWishAndSaysSo(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{startErr: errors.New("boom")}
	rec := callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "download could not be started") || !storedEnabled(t, db) {
		t.Fatalf("status %d enabled %v body %s", rec.Code, storedEnabled(t, db), rec.Body.String())
	}
}

func TestSwitchingOffCancelsTheDownloadsButOnlyTheChangeFromOnToOff(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{}
	callWith("LocalAiUpdateSettings", canRun, inst, db, modelOff) // off -> off (a download the person started by hand must keep running)
	if inst.cancels != 0 {
		t.Errorf("off -> off cancelled %d times", inst.cancels)
	}
	callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn)
	callWith("LocalAiUpdateSettings", canRun, inst, db, modelOff) // on -> off
	if inst.cancels != 1 || storedEnabled(t, db) {
		t.Errorf("on -> off must cancel once: cancels %d enabled %v", inst.cancels, storedEnabled(t, db))
	}
	callWith("LocalAiUpdateSettings", canRun, inst, db, modelOn)
	callWith("LocalAiUpdateSettings", canRun, inst, db, `{"enabled":true,"modelId":"qwen2.5-1.5b-instruct-q4_k_m","contextTokens":8192,"idleMinutes":10}`) // on -> on
	if inst.cancels != 1 {
		t.Errorf("on -> on must not cancel: %d", inst.cancels)
	}
}

func TestSavingWhileSwitchedOffStartsNoDownload(t *testing.T) {
	withMac(t)
	db := openMainDB(t)
	inst := &fakeInstalls{}
	if rec := callWith("LocalAiUpdateSettings", canRun, inst, db, modelOff); rec.Code != http.StatusOK || inst.starts != 0 {
		t.Fatalf("choosing a model while off must download nothing: status %d starts %d", rec.Code, inst.starts)
	}
}
