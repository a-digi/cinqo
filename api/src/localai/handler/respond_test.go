package handler

import (
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
func (noInstalls) Start(string, hardware.Info) error { return nil }
func (noInstalls) Remove() error                     { return nil }

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
	r := respond(entity.Settings{}, shipped(t), nil, noInstalls{})

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
	if r.CatalogCheckedOn != shipped(t).CheckedOn {
		t.Errorf("catalogCheckedOn = %q", r.CatalogCheckedOn)
	}
}

func TestUnknownMemoryIsNullNotAGuess(t *testing.T) {
	withMachine(t, hardware.Info{OS: "linux", Arch: "amd64", RAMBytes: 0, CPUs: 2})
	raw, err := json.Marshal(respond(entity.Settings{}, shipped(t), nil, noInstalls{}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"fits":null`) || strings.Contains(string(raw), `"fits":true`) || strings.Contains(string(raw), `"fits":false`) {
		t.Errorf("fits must be null for every model when the memory is unknown: %s", raw)
	}
}

func TestAnUnsupportedSystemIsSaidSo(t *testing.T) {
	withMachine(t, hardware.Info{OS: "windows", Arch: "amd64", RAMBytes: 64 << 30, CPUs: 16})
	if respond(entity.Settings{}, shipped(t), nil, noInstalls{}).Hardware.Supported {
		t.Error("windows is not supported yet")
	}
}

func TestTheResponseNeverCarriesADownloadAddressOrAHash(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 15 << 30, CPUs: 8})
	raw, _ := json.Marshal(respond(entity.Settings{}, shipped(t), nil, noInstalls{}))
	for _, secret := range []string{"huggingface.co", "sha256", "88c00229914083cd", ".gguf"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("the response must not contain %q: %s", secret, raw)
		}
	}
}

func TestAnEmptyCatalogIsAnEmptyListNotNull(t *testing.T) {
	withMachine(t, hardware.Info{})
	raw, _ := json.Marshal(respond(entity.Settings{}, catalog.Catalog{}, nil, noInstalls{}))
	if !strings.Contains(string(raw), `"catalog":[]`) {
		t.Errorf("want an empty list, got %s", raw)
	}
}

func TestTheStatusSaysWhetherItCanBeSwitchedOnAndWhyNot(t *testing.T) {
	withMachine(t, hardware.Info{OS: "darwin", Arch: "arm64", RAMBytes: 16 << 30, CPUs: 8})
	c := shipped(t)

	ready := respond(entity.Settings{}, c, nil, noInstalls{}).Status
	if !ready.CanEnable || ready.Reason != "" || ready.State != "off" {
		t.Errorf("status = %+v, want can enable, no reason, off", ready)
	}
	notYet := respond(entity.Settings{}, c, errors.New("the managed runtime is not available yet"), noInstalls{}).Status
	if notYet.CanEnable || notYet.Reason != "the managed runtime is not available yet" {
		t.Errorf("status = %+v, want cannot enable with the reason", notYet)
	}
	on := respond(entity.Settings{Enabled: true, ModelID: "phi-4-mini-instruct-q4_k_m"}, c, nil, noInstalls{}).Status
	if on.State != "configured" {
		t.Errorf("state = %q, want configured", on.State)
	}
	if got := respond(entity.Settings{Enabled: true}, c, nil, noInstalls{}).Status.State; got != "off" {
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
	r := respond(entity.Settings{}, shipped(t), nil, states{
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
	r := respond(entity.Settings{}, shipped(t), nil, states{"llama-cpp-b12064-macos-arm64": {State: entity.InstallReady}})
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
