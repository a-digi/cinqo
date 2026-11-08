package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
)

// noInstalls says nothing is downloaded and refuses to start anything.
type noInstalls struct{}

func (noInstalls) ModelState(catalog.Model) entity.FileInstall {
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}
func (noInstalls) RuntimeState(catalog.Runtime) entity.FileInstall {
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}
func (noInstalls) Start(string, hardware.Info) error              { return nil }
func (noInstalls) Remove() error                                  { return nil }
func (noInstalls) CancelDownloads()                               {}
func (noInstalls) ModelFailure(catalog.Model) *entity.Failure     { return nil }
func (noInstalls) RuntimeFailure(catalog.Runtime) *entity.Failure { return nil }

// fakeRT is a runtime whose answers the tests choose.
type fakeRT struct {
	err      error            // why nothing can be switched on (nil: everything can)
	perModel map[string]error // the answer for particular models
	status   entity.EngineStatus
	result   entity.TestResult
	testErr  error
	asked    *[]string       // the model ids CanEnable was asked about
	tools    map[string]bool // the models the runtime says can use tools
}

func (f fakeRT) CanUseTools(modelID string) bool { return f.tools[modelID] }

func (f fakeRT) CanEnable(modelID string) error {
	if f.asked != nil {
		*f.asked = append(*f.asked, modelID)
	}
	if e, ok := f.perModel[modelID]; ok {
		return e
	}
	return f.err
}

func (f fakeRT) Status() entity.EngineStatus {
	if f.status.State == "" {
		return entity.EngineStatus{State: entity.EngineStopped}
	}
	return f.status
}

func (f fakeRT) Test(context.Context, string) (entity.TestResult, error) { return f.result, f.testErr }

func withMachine(t *testing.T, info hardware.Info) {
	t.Helper()
	old := detect
	detect = func() hardware.Info { return info }
	t.Cleanup(func() { detect = old })
}

func shipped(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func find(r localAiResponse, id string) (catalogModelResponse, bool) {
	for _, m := range r.Catalog {
		if m.ID == id {
			return m, true
		}
	}
	return catalogModelResponse{}, false
}

func TestTheResponseTellsWhichModelsFitThisMachine(t *testing.T) {
	// 5 GiB (5.37 GB) of memory: the lighter model needs about 4.6 GB by the rule and fits, the better one needs about 6.3 GB and does not.
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 5 << 30, CPUs: 8})
	r := respond(entity.Settings{}, shipped(t), fakeRT{}, noInstalls{})

	if !r.Hardware.Supported || r.Hardware.OS != "darwin" || r.Hardware.RAMBytes != 5<<30 || r.Hardware.CPUs != 8 {
		t.Errorf("hardware = %+v", r.Hardware)
	}
	phi, ok := find(r, "phi-4-mini-instruct-q4_k_m")
	if !ok {
		t.Fatal("phi missing")
	}
	qwen, _ := find(r, "qwen2.5-1.5b-instruct-q4_k_m")
	if phi.Fits == nil || *phi.Fits {
		t.Errorf("phi needs %d bytes, the machine has %d: must not fit", phi.MinRAMBytes, uint64(5<<30))
	}
	if qwen.Fits == nil || !*qwen.Fits {
		t.Errorf("qwen needs %d bytes, the machine has %d: must fit", qwen.MinRAMBytes, uint64(5<<30))
	}
	if phi.MinRAMBytes != hardware.MinRAMBytes(phi.SizeBytes) || phi.License != "MIT" || phi.ContextTokens != 131072 || phi.CheckedOn == "" || phi.Source == "" {
		t.Errorf("phi = %+v", phi)
	}
	if raw, _ := json.Marshal(qwen); !strings.Contains(string(raw), `"tools":`) {
		t.Errorf("the field is named tools in the JSON: %s", raw)
	}
	if r.CatalogCheckedOn != shipped(t).CheckedOn {
		t.Errorf("catalogCheckedOn = %q", r.CatalogCheckedOn)
	}
}

func TestUnknownMemoryIsNullNotAGuess(t *testing.T) {
	withMachine(t, hardware.Info{OS: "linux", Arch: "amd64", RAMBytes: 0, CPUs: 2})
	raw, err := json.Marshal(respond(entity.Settings{}, shipped(t), fakeRT{}, noInstalls{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"fits":null`) || strings.Contains(string(raw), `"fits":true`) || strings.Contains(string(raw), `"fits":false`) {
		t.Errorf("fits must be null for every model when the memory is unknown: %s", raw)
	}
}

func TestAnUnsupportedSystemIsSaidSo(t *testing.T) {
	withMachine(t, hardware.Info{OS: "windows", Arch: "amd64", RAMBytes: 64 << 30, CPUs: 16})
	if respond(entity.Settings{}, shipped(t), fakeRT{}, noInstalls{}).Hardware.Supported {
		t.Error("windows is not supported yet")
	}
}

func TestTheResponseNeverCarriesADownloadAddressOrAHash(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 15 << 30, CPUs: 8})
	raw, _ := json.Marshal(respond(entity.Settings{}, shipped(t), fakeRT{}, noInstalls{}))
	for _, secret := range []string{"huggingface.co", "sha256", "88c00229914083cd", ".gguf"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the response must not contain %q: %s", secret, raw)
		}
	}
}

func TestAnEmptyCatalogIsAnEmptyListNotNull(t *testing.T) {
	withMachine(t, hardware.Info{})
	raw, _ := json.Marshal(respond(entity.Settings{}, catalog.Catalog{}, fakeRT{}, noInstalls{}))
	if !strings.Contains(string(raw), `"catalog":[]`) {
		t.Errorf("want an empty list, got %s", raw)
	}
}

func TestTheStatusSaysWhetherItCanBeSwitchedOnAndWhyNot(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	c := shipped(t)

	ready := respond(entity.Settings{}, c, fakeRT{}, noInstalls{}).Status
	if !ready.CanEnable || ready.Reason != "" || ready.State != "off" {
		t.Errorf("status = %+v, want can enable, no reason, off", ready)
	}
	notYet := respond(entity.Settings{}, c, fakeRT{err: errors.New("the managed runtime is not available yet")}, noInstalls{}).Status
	if notYet.CanEnable || notYet.Reason != "the managed runtime is not available yet" {
		t.Errorf("status = %+v, want cannot enable with the reason", notYet)
	}
	on := respond(entity.Settings{Enabled: true, ModelID: "phi-4-mini-instruct-q4_k_m"}, c, fakeRT{}, states{
		"phi-4-mini-instruct-q4_k_m": {State: entity.InstallReady}, "llama-cpp-b12064-macos-arm64": {State: entity.InstallReady},
	}).Status
	if on.State != "configured" {
		t.Errorf("state = %q, want configured (switched on and everything is there)", on.State)
	}
	if got := respond(entity.Settings{Enabled: true}, c, fakeRT{}, noInstalls{}).Status.State; got != "off" {
		t.Errorf("on without a model: state = %q, want off", got)
	}
}

// states answers from a table, by model id and runtime id.
type states map[string]entity.FileInstall

func (st states) ModelState(m catalog.Model) entity.FileInstall {
	if v, ok := st[m.ID]; ok {
		return v
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}
func (st states) CancelDownloads()                               {}
func (st states) ModelFailure(catalog.Model) *entity.Failure     { return nil }
func (st states) RuntimeFailure(catalog.Runtime) *entity.Failure { return nil }
func (st states) RuntimeState(r catalog.Runtime) entity.FileInstall {
	if v, ok := st[r.ID]; ok {
		return v
	}
	return entity.FileInstall{State: entity.InstallNotDownloaded}
}
func (st states) Start(string, hardware.Info) error { return nil }
func (st states) Remove() error                     { return nil }

func TestTheResponseCarriesTheInstallStateOfEveryModelAndTheRuntime(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	r := respond(entity.Settings{}, shipped(t), fakeRT{}, states{
		"phi-4-mini-instruct-q4_k_m":   {State: entity.InstallDownloading, DownloadID: "job-1", PartialBytes: 1234},
		"qwen2.5-1.5b-instruct-q4_k_m": {State: entity.InstallReady},
		"llama-cpp-b12064-macos-arm64": {State: entity.InstallPartial, PartialBytes: 99},
		"llama-cpp-b12064-linux-amd64": {State: entity.InstallReady}, // another system's runtime must not be reported
	})
	phi, _ := find(r, "phi-4-mini-instruct-q4_k_m")
	qwen, _ := find(r, "qwen2.5-1.5b-instruct-q4_k_m")
	if phi.Install.State != entity.InstallDownloading || phi.Install.DownloadID != "job-1" || phi.Install.PartialBytes != 1234 {
		t.Errorf("phi install = %+v", phi.Install)
	}
	if qwen.Install.State != entity.InstallReady {
		t.Errorf("qwen install = %+v", qwen.Install)
	}
	if !r.Runtime.Supported || r.Runtime.SizeBytes != 16548551 || r.Runtime.Install.State != entity.InstallPartial || r.Runtime.Install.PartialBytes != 99 || r.Runtime.Name == "" {
		t.Errorf("runtime = %+v", r.Runtime)
	}
}

func TestASystemWithoutARuntimeSaysSoAndReportsNothingDownloaded(t *testing.T) {
	withMachine(t, hardware.Info{OS: "windows", Arch: "amd64", RAMBytes: 16 << 30, CPUs: 8})
	r := respond(entity.Settings{}, shipped(t), fakeRT{}, states{"llama-cpp-b12064-macos-arm64": {State: entity.InstallReady}})
	if r.Runtime.Supported || r.Runtime.Install.State != entity.InstallNotDownloaded || r.Runtime.Name != "" || r.Runtime.SizeBytes != 0 {
		t.Errorf("runtime = %+v, want unsupported and empty", r.Runtime)
	}
	raw, _ := json.Marshal(r)
	for _, key := range []string{`"runtime":{"supported":false`, `"install":{"state":"not_downloaded","partialBytes":0,"downloadId":""}`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the JSON must contain %s: %s", key, raw)
		}
	}
}

func TestEveryModelSaysWhetherItCanBeSwitchedOnAndTheStatusIsAboutTheSavedModel(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	var asked []string
	rt := fakeRT{
		perModel: map[string]error{"phi-4-mini-instruct-q4_k_m": nil, "qwen2.5-1.5b-instruct-q4_k_m": errors.New("the model is not downloaded yet")},
		asked:    &asked,
	}
	r := respond(entity.Settings{ModelID: "qwen2.5-1.5b-instruct-q4_k_m"}, shipped(t), rt, noInstalls{})
	phi, _ := find(r, "phi-4-mini-instruct-q4_k_m")
	qwen, _ := find(r, "qwen2.5-1.5b-instruct-q4_k_m")
	if !phi.Enable.OK || phi.Enable.Reason != "" {
		t.Errorf("phi enable = %+v, want ok", phi.Enable)
	}
	if qwen.Enable.OK || qwen.Enable.Reason != "the model is not downloaded yet" {
		t.Errorf("qwen enable = %+v, want the reason", qwen.Enable)
	}
	// The top-level status is about the model that is saved in the settings (qwen), not about the others.
	if r.Status.CanEnable || r.Status.Reason != "the model is not downloaded yet" {
		t.Errorf("status = %+v, want qwen's reason", r.Status)
	}
	want := map[string]bool{"qwen2.5-1.5b-instruct-q4_k_m": true, "phi-4-mini-instruct-q4_k_m": true}
	for _, id := range asked {
		delete(want, id)
	}
	if len(want) != 0 {
		t.Errorf("CanEnable was asked about %v, want every model and the saved one", asked)
	}
}

func TestTheResponseCarriesTheStateOfTheModelProgram(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	for _, st := range []entity.EngineStatus{
		{State: entity.EngineStopped},
		{State: entity.EngineStarting, ModelID: "phi-4-mini-instruct-q4_k_m"},
		{State: entity.EngineReady, ModelID: "phi-4-mini-instruct-q4_k_m"},
		{State: entity.EngineFailed, ModelID: "phi-4-mini-instruct-q4_k_m", Error: "the model program stopped unexpectedly"},
	} {
		r := respond(entity.Settings{}, shipped(t), fakeRT{status: st}, noInstalls{})
		if r.Engine != st {
			t.Errorf("engine = %+v, want %+v", r.Engine, st)
		}
	}
	raw, _ := json.Marshal(respond(entity.Settings{}, shipped(t), fakeRT{}, noInstalls{}))
	if !strings.Contains(string(raw), `"engine":{"state":"stopped","error":"","modelId":""}`) {
		t.Errorf("the JSON must carry the engine: %s", raw)
	}
}

func TestTheToolsFieldFollowsTheRuntimeNotTheCatalogFlag(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	// The catalog marks both Phi and Qwen as able to use tools; the runtime says only Qwen can right now (the dialect Phi
	// needs is not registered). The page must follow the runtime.
	r := respond(entity.Settings{}, shipped(t), fakeRT{tools: map[string]bool{"qwen2.5-1.5b-instruct-q4_k_m": true}}, noInstalls{})
	for id, want := range map[string]bool{"qwen2.5-1.5b-instruct-q4_k_m": true, "phi-4-mini-instruct-q4_k_m": false, "qwen2.5-7b-instruct-q4_k_m": false} {
		m, ok := find(r, id)
		if !ok || m.Tools != want {
			t.Errorf("%s: tools %v (found %v), want %v", id, m.Tools, ok, want)
		}
	}
	raw, _ := json.Marshal(r)
	if !strings.Contains(string(raw), `"tools":true`) || !strings.Contains(string(raw), `"tools":false`) {
		t.Errorf("both values must reach the JSON: %s", raw)
	}
}

func TestTheResponseSaysWhatTheCacheCostsAndTheLargestSizeThatFits(t *testing.T) {
	big := hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8}
	small := hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 8 << 30, CPUs: 8}
	unknown := hardware.Info{OS: "linux", Arch: "amd64", RAMBytes: 0, CPUs: 2}
	type want struct {
		kv      int64
		largest any // an int, or nil for "not known"
	}
	for _, c := range []struct {
		name string
		info hardware.Info
		want map[string]want
	}{
		{"16 GiB", big, map[string]want{
			"phi-4-mini-instruct-q4_k_m":       {131072, 65536},
			"qwen2.5-1.5b-instruct-q4_k_m":     {28672, 32768}, // its own maximum, not a larger offered size
			"qwen2.5-7b-instruct-q4_k_m":       {57344, 32768},
			"qwen2.5-coder-3b-instruct-q4_k_m": {36864, 32768}, // 2.1 GB file: its own maximum fits (plan 196)
		}},
		{"8 GiB", small, map[string]want{
			"phi-4-mini-instruct-q4_k_m":       {131072, 16384},
			"qwen2.5-1.5b-instruct-q4_k_m":     {28672, 32768},
			"qwen2.5-7b-instruct-q4_k_m":       {57344, 0}, // 8.45 GiB at the baseline: nothing fits, and that is known (not null)
			"qwen2.5-coder-3b-instruct-q4_k_m": {36864, 32768},
		}},
		{"unknown memory", unknown, map[string]want{
			"phi-4-mini-instruct-q4_k_m":       {131072, nil},
			"qwen2.5-1.5b-instruct-q4_k_m":     {28672, nil},
			"qwen2.5-7b-instruct-q4_k_m":       {57344, nil},
			"qwen2.5-coder-3b-instruct-q4_k_m": {36864, nil},
		}},
	} {
		withMachine(t, c.info)
		r := respond(entity.Settings{}, shipped(t), fakeRT{}, noInstalls{})
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Catalog []struct {
				ID              string `json:"id"`
				KVBytesPerToken int64  `json:"kvBytesPerToken"`
				LargestContext  *int   `json:"largestContext"`
			} `json:"catalog"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		for _, m := range decoded.Catalog {
			w := c.want[m.ID]
			if m.KVBytesPerToken != w.kv {
				t.Errorf("%s %s: kvBytesPerToken %d, want %d", c.name, m.ID, m.KVBytesPerToken, w.kv)
			}
			switch exp := w.largest.(type) {
			case nil:
				if m.LargestContext != nil {
					t.Errorf("%s %s: largestContext %d, want null", c.name, m.ID, *m.LargestContext)
				}
			case int:
				if m.LargestContext == nil {
					t.Errorf("%s %s: largestContext null, want %d", c.name, m.ID, exp)
				} else if *m.LargestContext != exp {
					t.Errorf("%s %s: largestContext %d, want %d", c.name, m.ID, *m.LargestContext, exp)
				}
			}
		}
		if !strings.Contains(string(raw), `"largestContext":null`) && c.name == "unknown memory" {
			t.Errorf("an unknown amount of memory must be null in the JSON: %s", raw)
		}
	}
}

func TestTheResponseRecommendsAContextSizeOnlyForModelsThatCanUseTools(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	cat := shipped(t)
	// A model that reads little: the recommendation never exceeds what it can read.
	cat.Models = append(cat.Models, catalog.Model{ID: "small", Name: "Small", SizeBytes: 1 << 30, ContextTokens: 8192, KVBytesPerToken: 1000, Tools: true})
	r := respond(entity.Settings{}, cat, fakeRT{tools: map[string]bool{"phi-4-mini-instruct-q4_k_m": true, "qwen2.5-1.5b-instruct-q4_k_m": true, "small": true}}, noInstalls{})
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Catalog []struct {
			ID                 string `json:"id"`
			RecommendedContext *int   `json:"recommendedContext"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"phi-4-mini-instruct-q4_k_m": 16384, "qwen2.5-1.5b-instruct-q4_k_m": 16384, "qwen2.5-7b-instruct-q4_k_m": nil, "small": 8192}
	for _, m := range decoded.Catalog {
		switch exp := want[m.ID].(type) {
		case nil:
			if m.RecommendedContext != nil {
				t.Errorf("%s: recommended %d, want none (no tools)", m.ID, *m.RecommendedContext)
			}
		case int:
			if m.RecommendedContext == nil || *m.RecommendedContext != exp {
				t.Errorf("%s: recommended %v, want %d", m.ID, m.RecommendedContext, exp)
			}
		}
	}
	if entity.RecommendedToolContext != 16384 {
		t.Errorf("the recommendation is the measured minimum of a CV import: %d", entity.RecommendedToolContext)
	}
}

func TestAModelWithAnUnknownMaximumStillGetsTheUsualRecommendation(t *testing.T) {
	if got := recommendedContext(catalog.Model{ID: "x", ContextTokens: 0}, true); got == nil || *got != 16384 {
		t.Errorf("recommended %v, want 16384 (never 0)", got)
	}
	if got := recommendedContext(catalog.Model{ID: "x", ContextTokens: 16384}, true); got == nil || *got != 16384 {
		t.Errorf("exactly the usual size: %v", got)
	}
	if got := recommendedContext(catalog.Model{ID: "x", ContextTokens: 32768}, false); got != nil {
		t.Errorf("no tools, no recommendation: %d", *got)
	}
}

// --- plan 205, step 2: the derived state ---

// failing is a table of states plus the failures the installer remembers.
type failing struct {
	states
	model, program *entity.Failure
}

func (f failing) ModelFailure(catalog.Model) *entity.Failure     { return f.model }
func (f failing) RuntimeFailure(catalog.Runtime) *entity.Failure { return f.program }

const (
	tPhi = "phi-4-mini-instruct-q4_k_m"
	tMac = "llama-cpp-b12064-macos-arm64"
)

func stateOf(t *testing.T, s entity.Settings, rt fakeRT, in Installs) statusResponse {
	t.Helper()
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	return respond(s, shipped(t), rt, in).Status
}

var phiOn = entity.Settings{Enabled: true, ModelID: tPhi}

func TestOnlyEverythingThereMeansFullyOn(t *testing.T) {
	ready := states{tPhi: {State: entity.InstallReady}, tMac: {State: entity.InstallReady}}
	if got := stateOf(t, phiOn, fakeRT{}, ready); got.State != StateConfigured || got.Preparing != nil || got.LastFailure != nil {
		t.Errorf("%+v", got)
	}
	// a file deleted by hand: the next read says so, nothing is stored
	gone := states{tPhi: {State: entity.InstallReady}}
	if got := stateOf(t, phiOn, fakeRT{}, gone); got.State != StateInterrupted {
		t.Errorf("the program deleted: %+v", got)
	}
	gone = states{tMac: {State: entity.InstallReady}}
	if got := stateOf(t, phiOn, fakeRT{}, gone); got.State != StateInterrupted {
		t.Errorf("the model deleted: %+v", got)
	}
	// switched off, or on without a model, is off whatever is on disk
	if got := stateOf(t, entity.Settings{ModelID: tPhi}, fakeRT{}, ready); got.State != StateOff {
		t.Errorf("off: %+v", got)
	}
	if got := stateOf(t, entity.Settings{Enabled: true}, fakeRT{}, ready); got.State != StateOff {
		t.Errorf("on without a model: %+v", got)
	}
}

func TestWhileSomethingDownloadsItIsPreparingAndSaysWhat(t *testing.T) {
	program := stateOf(t, phiOn, fakeRT{}, states{tPhi: {State: entity.InstallReady}, tMac: {State: entity.InstallDownloading, DownloadID: "d-program"}})
	if program.State != StatePreparing || program.Preparing == nil || program.Preparing.Step != "program" || program.Preparing.DownloadID != "d-program" ||
		!strings.Contains(program.Preparing.Name, "llama.cpp") || program.Preparing.SizeBytes <= 0 {
		t.Errorf("program: %+v %+v", program, program.Preparing)
	}
	model := stateOf(t, phiOn, fakeRT{}, states{tMac: {State: entity.InstallReady}, tPhi: {State: entity.InstallDownloading, DownloadID: "d-model"}})
	if model.State != StatePreparing || model.Preparing == nil || model.Preparing.Step != "model" || model.Preparing.DownloadID != "d-model" || model.Preparing.SizeBytes <= 0 {
		t.Errorf("model: %+v %+v", model, model.Preparing)
	}
	both := stateOf(t, phiOn, fakeRT{}, states{tMac: {State: entity.InstallDownloading, DownloadID: "p"}, tPhi: {State: entity.InstallDownloading, DownloadID: "m"}})
	if both.Preparing == nil || both.Preparing.Step != "program" || both.Preparing.DownloadID != "p" {
		t.Errorf("the program comes first: %+v", both.Preparing)
	}
	// a running download wins over an old failure
	running := failing{states: states{tMac: {State: entity.InstallDownloading, DownloadID: "p"}}, program: &entity.Failure{What: "program", Message: "old"}}
	if got := stateOf(t, phiOn, fakeRT{}, running); got.State != StatePreparing || got.LastFailure != nil {
		t.Errorf("%+v", got)
	}
}

func TestAFailedDownloadWithNothingRunningIsFailedWithItsReason(t *testing.T) {
	pf := &entity.Failure{What: "program", Message: "the server answered with status 503"}
	mf := &entity.Failure{What: "model", Message: "the connection kept failing"}
	got := stateOf(t, phiOn, fakeRT{}, failing{states: states{tPhi: {State: entity.InstallReady}}, program: pf})
	if got.State != StateFailed || got.LastFailure == nil || got.LastFailure.What != "program" || got.LastFailure.Message != pf.Message {
		t.Errorf("program failure: %+v", got)
	}
	got = stateOf(t, phiOn, fakeRT{}, failing{states: states{tMac: {State: entity.InstallReady}}, model: mf})
	if got.State != StateFailed || got.LastFailure == nil || got.LastFailure.What != "model" {
		t.Errorf("model failure: %+v", got)
	}
	// a failure of something that is on disk now is history
	got = stateOf(t, phiOn, fakeRT{}, failing{states: states{tPhi: {State: entity.InstallReady}, tMac: {State: entity.InstallReady}}, program: pf})
	if got.State != StateConfigured || got.LastFailure != nil {
		t.Errorf("a failure of something that is ready must not show: %+v", got)
	}
	// an old failure of a model that is on disk now must not make a missing program look failed: it is only interrupted
	got = stateOf(t, phiOn, fakeRT{}, failing{states: states{tPhi: {State: entity.InstallReady}}, model: mf})
	if got.State != StateInterrupted || got.LastFailure != nil {
		t.Errorf("a failure of a model that is ready must not show: %+v", got)
	}
	// the program's failure is shown first when both failed
	got = stateOf(t, phiOn, fakeRT{}, failing{states: states{}, program: pf, model: mf})
	if got.LastFailure == nil || got.LastFailure.What != "program" {
		t.Errorf("both failed: %+v", got.LastFailure)
	}
}

func TestNothingRunningAndNoFailureKnownIsInterrupted(t *testing.T) {
	// after a restart: a partial file is on disk, no download is running, no failure is known
	got := stateOf(t, phiOn, fakeRT{}, states{tMac: {State: entity.InstallPartial, PartialBytes: 99}})
	if got.State != StateInterrupted || got.Preparing != nil || got.LastFailure != nil {
		t.Errorf("%+v", got)
	}
}

func TestWhatNoDownloadFixesIsFailedNotInterruptedWhileSwitchedOn(t *testing.T) {
	// the memory changed (or the settings were edited by hand) after it was switched on
	got := stateOf(t, phiOn, fakeRT{err: errors.New("this computer has 4.0 GiB of memory and the model needs about 6.0 GiB")}, states{tPhi: {State: entity.InstallReady}, tMac: {State: entity.InstallReady}})
	if got.State != StateFailed || got.LastFailure == nil || got.LastFailure.What != "system" || !strings.Contains(got.LastFailure.Message, "4.0 GiB") {
		t.Errorf("%+v", got)
	}
	if got.CanEnable {
		t.Error("canEnable must stay false")
	}
}
