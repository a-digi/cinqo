// Package handler serves the local AI module's HTTP API. See plan/ai/small-llm-local/step-181-local-ai-small-models.md.
package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"
	"github.com/a-digi/coco-server/server/routing"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
)

// handlerFunc adapts a plain function to coco-server's handler interface (the same tiny adapter config/routing has,
// kept here so the module needs nothing from the core's config package).
type handlerFunc func(request.RequestContext)

func (f handlerFunc) ServeHTTP(reqCtx request.RequestContext) { f(reqCtx) }

// Runtime is what the API needs from the engine: whether the local AI can be switched on with a model (nil when it can,
// otherwise the reason), the state of the model program, and a timed test request.
type Runtime interface {
	CanEnable(modelID string) error
	// CanUseTools says whether conversations on the model may be given tools (measured, and any dialect it needs is there).
	CanUseTools(modelID string) bool
	Status() entity.EngineStatus
	Test(ctx context.Context, modelID string) (entity.TestResult, error)
}

// Installs knows what is downloaded and starts and removes downloads.
type Installs interface {
	ModelState(m catalog.Model) entity.FileInstall
	RuntimeState(r catalog.Runtime) entity.FileInstall
	// ModelFailure and RuntimeFailure say why the last download of the model or of the program failed, nil when none did.
	ModelFailure(m catalog.Model) *entity.Failure
	RuntimeFailure(r catalog.Runtime) *entity.Failure
	// Start begins the downloads a model needs on this system; errors are entity.ErrUnknownModel, entity.ErrUnsupportedSystem
	// or an internal one.
	Start(modelID string, system hardware.Info) error
	// CancelDownloads stops the running downloads of the module (switching off while it is being set up).
	CancelDownloads()
	// Remove deletes everything downloaded; entity.ErrDownloadRunning while a download is running.
	Remove() error
	// RemoveModel deletes one model's file; entity.ErrDownloadRunning while that model is being downloaded.
	RemoveModel(m catalog.Model) error
}

type api struct {
	runtime  Runtime
	installs Installs
}

// Handlers maps the executor names of config/routes/route-localai.yaml to their handlers.
func Handlers(runtime Runtime, installs Installs) map[string]routing.HandlerInterface {
	a := api{runtime: runtime, installs: installs}
	return map[string]routing.HandlerInterface{
		"LocalAiGet":             handlerFunc(a.get),
		"LocalAiUpdateSettings":  handlerFunc(a.updateSettings),
		"LocalAiInstall":         handlerFunc(a.install),
		"LocalAiRemoveDownloads": handlerFunc(a.removeDownloads),
		"LocalAiTest":            handlerFunc(a.test),
		// a tool suggests a model, a person dismisses a suggestion (plan 220)
		"LocalAiSuggest":           handlerFunc(a.suggest),
		"LocalAiDismissSuggestion": handlerFunc(a.dismissSuggestion),
		// a person confirms a suggestion (it is added and downloaded) or removes an added model (plan 220, step 4)
		"LocalAiInstallModel": handlerFunc(a.installModel),
		"LocalAiRemoveModel":  handlerFunc(a.removeModel),
	}
}

// The states of the local AI (plan 205). Only "configured" means fully ON: switched on AND everything it needs is on disk and fits.
const (
	StateOff         = "off"         // switched off, or no model chosen
	StatePreparing   = "preparing"   // switched on; the program and/or the model are being downloaded now
	StateConfigured  = "configured"  // switched on and ready: conversations can use it
	StateFailed      = "failed"      // switched on, not ready, nothing is downloading, and the last attempt failed (or the computer cannot run it)
	StateInterrupted = "interrupted" // switched on, not ready, nothing is downloading, no failure known (after a restart, or files removed)
)

// statusResponse is what the settings page shows about the current state. State is one of the State constants. CanEnable says
// whether the local AI can be switched on at all right now (nothing a download fixes is missing), and Reason says why not
// when it cannot. Preparing says what is being downloaded while State is "preparing"; LastFailure says why it stopped while State
// is "failed".
type statusResponse struct {
	State       string          `json:"state"`
	CanEnable   bool            `json:"canEnable"`
	Reason      string          `json:"reason"`
	Preparing   *preparingStep  `json:"preparing"`
	LastFailure *entity.Failure `json:"lastFailure"`
}

// preparingStep is the download in progress: the program first, then the model. The page follows DownloadID with the generic
// download API (it also tells downloading from installing).
type preparingStep struct {
	Step       string `json:"step"` // "program" or "model"
	DownloadID string `json:"downloadId"`
	Name       string `json:"name"`
	SizeBytes  int64  `json:"sizeBytes"`
}

// hardwareResponse is what this machine can run. Supported says whether the managed local AI is meant to run on it at
// all (a system Cinqo does not support yet gets no download button).
type hardwareResponse struct {
	hardware.Info
	Supported bool `json:"supported"`
}

// catalogModelResponse is one model Cinqo knows how to download, with what this machine says about it. The download
// address and the hash are not sent: the page has no use for them. Fits is null when the memory is unknown.
type catalogModelResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Quantization  string `json:"quantization"`
	SizeBytes     int64  `json:"sizeBytes"`
	MinRAMBytes   uint64 `json:"minRamBytes"`
	ContextTokens int    `json:"contextTokens"`
	// Tools says the model was measured to call tools reliably (so a conversation may be given a list of tools).
	Tools bool `json:"tools"`
	// KVBytesPerToken is what the model's cache holds per token of context (0: not stated), so the page can show what a
	// context size costs in memory.
	KVBytesPerToken int64 `json:"kvBytesPerToken"`
	// LargestContext is the largest context size the page offers that this computer has the memory for with this model; null
	// when the memory or the cache size is not known (nothing is then refused).
	LargestContext *int `json:"largestContext"`
	// RecommendedContext is the context size recommended for tool tasks (a CV import, for example) with this model: only for a
	// model that can use tools, never more than the model reads; null otherwise. The page recommends it next to the size list.
	RecommendedContext *int   `json:"recommendedContext"`
	License            string `json:"license"`
	Source             string `json:"source"`
	Note               string `json:"note"`
	CheckedOn          string `json:"checkedOn"`
	Fits               *bool  `json:"fits"`
	// Install says whether the model file is downloaded, being downloaded (with the id of the download) or interrupted.
	Install entity.FileInstall `json:"install"`
	// Enable says whether the local AI can be switched on with this model right now, and if not, why not.
	Enable enableResponse `json:"enable"`
	// Origin is "catalog" for a model shipped in the program and "added" for one a person added after a tool suggested it (plan 220). For an
	// added model, AddedBy is who suggested it (the tool's own word), Repo and Publisher say where it is from, and Why is the reason given.
	Origin  string `json:"origin"`
	AddedBy string `json:"addedBy,omitempty"`
	// SuggestedBy lists every tool that suggested an added model (AddedBy is the first of them).
	SuggestedBy []string `json:"suggestedBy,omitempty"`
	Repo        string   `json:"repo,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	Why         string   `json:"why,omitempty"`
}

type enableResponse struct {
	OK     bool   `json:"ok"`
	Reason string `json:"reason"`
}

func enableOf(err error) enableResponse {
	if err != nil {
		return enableResponse{Reason: err.Error()}
	}
	return enableResponse{OK: true}
}

// runtimeResponse is the program that runs the models, for this system: Supported is false on a system that has none yet.
type runtimeResponse struct {
	Supported bool               `json:"supported"`
	Name      string             `json:"name"`
	SizeBytes int64              `json:"sizeBytes"`
	Install   entity.FileInstall `json:"install"`
}

type localAiResponse struct {
	Settings         entity.Settings        `json:"settings"`
	Status           statusResponse         `json:"status"`
	Hardware         hardwareResponse       `json:"hardware"`
	Catalog          []catalogModelResponse `json:"catalog"`
	CatalogCheckedOn string                 `json:"catalogCheckedOn"`
	Runtime          runtimeResponse        `json:"runtime"`
	// Engine is the state of the model program: stopped (it starts when a conversation needs it), starting, ready or failed.
	Engine entity.EngineStatus `json:"engine"`
	// Suggestions are the models tools suggested that nobody confirmed yet (plan 220): nothing of them is downloaded.
	Suggestions []suggestionResponse `json:"suggestions"`
	// Dismissed are the ids of the suggested models the person dismissed: a tool that suggests a model can say so instead of asking again (plan 221).
	Dismissed []string `json:"dismissed"`
}

// recommendedContext is the size recommended for tool tasks with model m, nil for a model that cannot use tools.
func recommendedContext(m catalog.Model, canUseTools bool) *int {
	if !canUseTools {
		return nil
	}
	size := entity.RecommendedToolContext
	if m.ContextTokens > 0 && m.ContextTokens < size {
		size = m.ContextTokens
	}
	return &size
}

// largestContext is the largest offered context size the machine has the memory for with model m, nil when that is not known.
func largestContext(m catalog.Model, info hardware.Info) *int {
	largest, known := hardware.LargestContext(m.SizeBytes, m.KVBytesPerToken, info.RAMBytes, entity.SizesUpTo(m.ContextTokens))
	if !known {
		return nil
	}
	return &largest
}

// detect reads the machine; a variable so tests can give a fixed one.
var detect = hardware.Detect

// SetMachine replaces how the handlers read the computer and returns the way back. The program never calls it: it exists so a
// test outside this package can give the handlers a computer with a fixed amount of memory.
func SetMachine(read func() hardware.Info) (restore func()) {
	old := detect
	detect = read
	return func() { detect = old }
}

func respond(s entity.Settings, c catalog.Catalog, rt Runtime, installs Installs) localAiResponse {
	cannotEnable := rt.CanEnable(s.ModelID)
	status := statusResponse{State: StateOff, CanEnable: cannotEnable == nil}
	if cannotEnable != nil {
		status.Reason = cannotEnable.Error()
	}
	info := detect()
	if s.Configured() {
		deriveStatus(&status, s, c, cannotEnable, installs, info)
	}
	models := make([]catalogModelResponse, 0, len(c.Models))
	for _, m := range c.Models {
		models = append(models, catalogModelResponse{
			ID: m.ID, Name: m.Name, Quantization: m.Quantization, SizeBytes: m.SizeBytes,
			MinRAMBytes: hardware.MinRAMBytes(m.SizeBytes), ContextTokens: m.ContextTokens, Tools: rt.CanUseTools(m.ID),
			License: m.License, Source: m.Source, Note: m.Note, CheckedOn: m.CheckedOn,
			Fits:               info.Fits(m.SizeBytes),
			KVBytesPerToken:    m.KVBytesPerToken,
			LargestContext:     largestContext(m, info),
			RecommendedContext: recommendedContext(m, rt.CanUseTools(m.ID)),
			Install:            installs.ModelState(m),
			Enable:             enableOf(rt.CanEnable(m.ID)),
			Origin:             "catalog",
		})
	}
	runtime := runtimeResponse{Install: entity.FileInstall{State: entity.InstallNotDownloaded}}
	if prog, ok := c.RuntimeFor(info.OS, info.Arch); ok {
		runtime = runtimeResponse{Supported: true, Name: prog.Name, SizeBytes: prog.SizeBytes, Install: installs.RuntimeState(prog)}
	}
	return localAiResponse{
		Settings:         s,
		Status:           status,
		Hardware:         hardwareResponse{Info: info, Supported: info.Supported()},
		Catalog:          models,
		CatalogCheckedOn: c.CheckedOn,
		Runtime:          runtime,
		Engine:           rt.Status(),
		Suggestions:      []suggestionResponse{},
		Dismissed:        []string{},
	}
}

func mainDB(reqCtx request.RequestContext) (*sql.DB, error) {
	manager := reqCtx.GetDI().GetDatabaseManager()
	if manager == nil || manager.Connector == nil || manager.Connector.DB == nil {
		return nil, errors.New("database not configured")
	}
	return manager.Connector.DB, nil
}

// get handles GET /api/v1/local-ai.
func (a api) get(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}
	s, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to load the local AI settings")
		return
	}
	c, err := models.Current()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "the local AI model catalog is invalid")
		return
	}
	response.SuccessResponse(w, http.StatusOK, respondWithAdded(db, respond(s, c, a.runtime, a.installs), c))
}

type updateSettingsRequest struct {
	Enabled       bool   `json:"enabled"`
	ModelID       string `json:"modelId"`
	ContextTokens int    `json:"contextTokens"`
	IdleMinutes   int    `json:"idleMinutes"`
}

// updateSettings handles PUT /api/v1/local-ai/settings. Every field is validated (entity.Validate) against the catalog
// before anything is stored, and the stored settings are returned.
func (a api) updateSettings(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body updateSettingsRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}

	c, err := models.Current()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "the local AI model catalog is invalid")
		return
	}
	cannotEnable := a.runtime.CanEnable(strings.TrimSpace(body.ModelID))
	checked, err := entity.Validate(entity.Settings{
		Enabled:       body.Enabled,
		ModelID:       body.ModelID,
		ContextTokens: body.ContextTokens,
		IdleMinutes:   body.IdleMinutes,
	}, c, cannotEnable)
	if err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	// A context size the machine has no memory for is refused (a size would otherwise be accepted up to the model's maximum).
	if err := entity.CheckMemory(checked, c, detect().RAMBytes); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	before, _ := query.NewSettingsQueryRepo(db).Load() // a read error counts as "was off": nothing is cancelled below
	if err := persistent.NewSettingsPersistentRepo(db).Save(checked); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to save the local AI settings")
		return
	}
	saved, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to reload the local AI settings")
		return
	}
	// Plan 205: switching on while the program or the model is not downloaded starts the missing downloads now, in the background.
	// The person's wish (enabled) is saved first, so the choice is never lost; a download that is already running is joined.
	if saved.Enabled && missingDownloads(c, a.installs, saved.ModelID) {
		if err := a.installs.Start(saved.ModelID, detect()); err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "the local AI was switched on, but the download could not be started: "+err.Error())
			return
		}
	}
	// Switching off while it is still being set up stops those downloads. Only the change from on to off does: a download the person
	// started with a model's own button while the local AI was off is theirs and keeps running.
	if before.Enabled && !saved.Enabled {
		a.installs.CancelDownloads()
	}
	response.SuccessResponse(w, http.StatusOK, respondWithAdded(db, respond(saved, c, a.runtime, a.installs), c))
}

// deriveStatus turns a switched-on setting into its real state from what is on disk and what is downloading (plan 205, D5): the
// wish is stored, the readiness is never stored, so a crash or a file deleted by hand cannot leave the page saying "on".
func deriveStatus(status *statusResponse, s entity.Settings, c catalog.Catalog, cannotEnable error, installs Installs, info hardware.Info) {
	m, mok := c.Find(s.ModelID)
	rt, rok := c.RuntimeFor(info.OS, info.Arch)
	if cannotEnable != nil || !mok || !rok { // what no download fixes (memory, an unsupported system)
		status.State = StateFailed
		msg := "the local AI cannot run here"
		if cannotEnable != nil {
			msg = cannotEnable.Error()
		}
		status.LastFailure = &entity.Failure{What: "system", Message: msg}
		return
	}
	rtState, mState := installs.RuntimeState(rt), installs.ModelState(m)
	if rtState.State == entity.InstallReady && mState.State == entity.InstallReady {
		status.State = StateConfigured
		return
	}
	switch { // the program is downloaded first, then the model
	case rtState.State == entity.InstallDownloading:
		status.State = StatePreparing
		status.Preparing = &preparingStep{Step: "program", DownloadID: rtState.DownloadID, Name: rt.Name, SizeBytes: rt.SizeBytes}
	case mState.State == entity.InstallDownloading:
		status.State = StatePreparing
		status.Preparing = &preparingStep{Step: "model", DownloadID: mState.DownloadID, Name: m.Name, SizeBytes: m.SizeBytes}
	default:
		if f := installs.RuntimeFailure(rt); f != nil && rtState.State != entity.InstallReady {
			status.State, status.LastFailure = StateFailed, f
		} else if f := installs.ModelFailure(m); f != nil && mState.State != entity.InstallReady {
			status.State, status.LastFailure = StateFailed, f
		} else {
			status.State = StateInterrupted
		}
	}
}

// missingDownloads says whether the program for this system or the model is not on disk yet (a running download counts as not
// there: starting again joins it).
func missingDownloads(c catalog.Catalog, installs Installs, modelID string) bool {
	m, ok := c.Find(modelID)
	if !ok {
		return false
	}
	info := detect()
	rt, ok := c.RuntimeFor(info.OS, info.Arch)
	if !ok {
		return false
	}
	return installs.RuntimeState(rt).State != entity.InstallReady || installs.ModelState(m).State != entity.InstallReady
}

type installRequest struct {
	ModelID string `json:"modelId"`
}

// install handles POST /api/v1/local-ai/install: start downloading the runtime and the model. Without a modelId the model
// of the settings is used. It answers 202 with the same status as GET, which now carries the ids of the running downloads.
func (a api) install(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body installRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}
	s, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to load the local AI settings")
		return
	}
	c, err := models.Current()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "the local AI model catalog is invalid")
		return
	}
	modelID := strings.TrimSpace(body.ModelID)
	if modelID == "" {
		modelID = s.ModelID
	}
	if modelID == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "choose a model first")
		return
	}
	if err := a.installs.Start(modelID, detect()); err != nil {
		switch {
		case errors.Is(err, entity.ErrUnknownModel), errors.Is(err, entity.ErrUnsupportedSystem):
			response.ErrorResponse(w, http.StatusBadRequest, err.Error())
		default:
			response.ErrorResponse(w, http.StatusInternalServerError, "the download could not be started")
		}
		return
	}
	response.SuccessResponse(w, http.StatusAccepted, respondWithAdded(db, respond(s, c, a.runtime, a.installs), c))
}

// removeDownloads handles DELETE /api/v1/local-ai/install: delete everything that was downloaded. It is refused while the
// local AI is switched on and while a download is running.
func (a api) removeDownloads(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}
	s, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to load the local AI settings")
		return
	}
	if s.Enabled {
		response.ErrorResponse(w, http.StatusConflict, "switch the local AI off before removing its downloaded files")
		return
	}
	if err := a.installs.Remove(); err != nil {
		if errors.Is(err, entity.ErrDownloadRunning) {
			response.ErrorResponse(w, http.StatusConflict, "a download is running: cancel it first")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "the downloaded files could not be removed")
		return
	}
	// the models people added are only metadata now: back to suggestions (they can be downloaded again), and the one chosen on the page, if it was
	// one of them, is no longer chosen
	demoteAdded(db, s)
	w.WriteHeader(http.StatusNoContent)
}

// testTimeout is how long the test may take: starting the program and loading a model from a cold disk can take minutes.
const testTimeout = 4 * time.Minute

// test handles POST /api/v1/local-ai/test: one tiny real conversation with the model (the model of the settings unless a modelId
// is given), timed. A failure of the model is part of the answer (ok false with the reason), not an error status.
func (a api) test(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body installRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	modelID := strings.TrimSpace(body.ModelID)
	if modelID == "" {
		db, err := mainDB(reqCtx)
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
			return
		}
		s, err := query.NewSettingsQueryRepo(db).Load()
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to load the local AI settings")
			return
		}
		modelID = s.ModelID
	}
	if modelID == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "choose a model first")
		return
	}
	ctx, cancel := context.WithTimeout(reqCtx.GetRequest().Context(), testTimeout)
	defer cancel()
	result, err := a.runtime.Test(ctx, modelID)
	if err != nil {
		if errors.Is(err, entity.ErrUnknownModel) {
			response.ErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "the test could not be run")
		return
	}
	response.SuccessResponse(w, http.StatusOK, result)
}
