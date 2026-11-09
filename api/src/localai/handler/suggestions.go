package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/localai/catalog"
	"github.com/a-digi/cinqo/src/localai/entity"
	"github.com/a-digi/cinqo/src/localai/hardware"
	"github.com/a-digi/cinqo/src/localai/huggingface"
	"github.com/a-digi/cinqo/src/localai/models"
	"github.com/a-digi/cinqo/src/localai/repository/persistent"
	"github.com/a-digi/cinqo/src/localai/repository/query"
	"github.com/a-digi/cinqo/src/toolrepository/fetch"
)

// Plan 220, step 3: a tool suggests a model, Cinqo looks it up and stores the suggestion. Nothing is downloaded and nothing is added to the list of
// models until a person confirms it (a later step).

// MaxPendingSuggestions is how many suggestions may wait at once: a tool cannot flood the page.
const MaxPendingSuggestions = 20

// Limits of the words a tool writes.
const (
	maxNameRunes        = 100
	maxWhyRunes         = 300
	maxSuggestedByRunes = 60
)

// Resolver looks a model up (huggingface.Resolve with the real fetcher in the program).
type Resolver func(ctx context.Context, req huggingface.Request) (huggingface.Resolved, error)

var resolve Resolver = func(ctx context.Context, req huggingface.Request) (huggingface.Resolved, error) {
	return huggingface.Resolve(ctx, fetch.New(), req)
}

// SetResolver replaces how a model is looked up and returns the way back. The program never calls it: it exists so a test can answer without the
// network.
func SetResolver(f Resolver) (restore func()) {
	old := resolve
	resolve = f
	return func() { resolve = old }
}

var now = func() time.Time { return time.Now().UTC() }

// suggestionResponse is a suggested model as the page shows it before anyone confirms it: where it is from, how big, what the tool says about it, and
// whether this computer has the memory for it. The download address and the hash are not sent.
type suggestionResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Repo          string `json:"repo"`
	File          string `json:"file"`
	Commit        string `json:"commit"`
	Publisher     string `json:"publisher"`
	SizeBytes     int64  `json:"sizeBytes"`
	MinRAMBytes   uint64 `json:"minRamBytes"`
	Quantization  string `json:"quantization"`
	License       string `json:"license"`
	ContextTokens int    `json:"contextTokens"`
	Why           string `json:"why"`
	SuggestedBy   string `json:"suggestedBy"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	Fits          *bool  `json:"fits"`
	// Created is true on the answer that stored a new suggestion, false when the same file was already known (Status then says what became of it).
	Created bool `json:"created"`
}

func toSuggestion(m entity.AddedModel, info hardware.Info) suggestionResponse {
	return suggestionResponse{
		ID: m.ID, Name: m.Name, Repo: m.Repo, File: m.File, Commit: m.Revision, Publisher: m.Publisher, SizeBytes: m.SizeBytes,
		MinRAMBytes: hardware.MinRAMBytes(m.SizeBytes), Quantization: m.Quantization, License: m.License, ContextTokens: m.ContextTokens, Why: m.Why,
		SuggestedBy: m.SuggestedBy, Status: m.Status, CreatedAt: m.CreatedAt, Fits: info.Fits(m.SizeBytes),
	}
}

// respondWithAdded adds to the page's answer what the database knows about added models: who suggested the ones in the list, and the suggestions
// that wait. A table that cannot be read leaves the answer as it is.
func respondWithAdded(db *sql.DB, out localAiResponse, c catalog.Catalog) localAiResponse {
	repo := query.NewAddedModelsQueryRepo(db)
	added, err := repo.ListAdded()
	if err != nil {
		return out
	}
	byID := map[string]entity.AddedModel{}
	for _, a := range added {
		byID[a.ID] = a
	}
	for i := range out.Catalog {
		if a, ok := byID[out.Catalog[i].ID]; ok {
			out.Catalog[i].Origin = "added"
			out.Catalog[i].AddedBy, out.Catalog[i].Repo, out.Catalog[i].Publisher, out.Catalog[i].Why = a.SuggestedBy, a.Repo, a.Publisher, a.Why
		}
	}
	if waiting, err := repo.ListByStatus(entity.AddedStatusSuggested); err == nil {
		out.Suggestions = make([]suggestionResponse, 0, len(waiting))
		for _, m := range waiting {
			out.Suggestions = append(out.Suggestions, toSuggestion(m, out.Hardware.Info))
		}
	}
	return out
}

type suggestRequest struct {
	Repo          string `json:"repo"`
	File          string `json:"file"`
	Revision      string `json:"revision"`
	Name          string `json:"name"`
	Why           string `json:"why"`
	SuggestedBy   string `json:"suggestedBy"`
	ContextTokens int    `json:"contextTokens"`
	SHA256        string `json:"sha256"`
	SizeBytes     int64  `json:"sizeBytes"`
}

// cleanText is a text a tool wrote, made safe to store and show: control characters become spaces, the edges are trimmed and it is cut to max
// characters. It is only ever shown as text, never used to decide anything.
func cleanText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

// suggest handles POST /api/v1/local-ai/suggestions: a tool says where a model is, Cinqo looks it up (the commit, the size, the SHA-256, the licence)
// and stores it as a suggestion. The same file at the same commit is stored once: a second suggestion answers the first.
func (a api) suggest(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	var body suggestRequest
	if err := reqCtx.BindJSON(&body); err != nil {
		response.ErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.ContextTokens != 0 && (body.ContextTokens < entity.MinContextTokens || body.ContextTokens > entity.MaxContextTokens) {
		response.ErrorResponse(w, http.StatusBadRequest, "contextTokens must be between 512 and 131072")
		return
	}
	if body.SizeBytes < 0 {
		response.ErrorResponse(w, http.StatusBadRequest, "sizeBytes must not be negative")
		return
	}
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}

	found, err := resolve(reqCtx.GetRequest().Context(), huggingface.Request{
		Repo: strings.TrimSpace(body.Repo), File: strings.TrimSpace(body.File), Revision: strings.TrimSpace(body.Revision),
		ExpectSHA256: strings.TrimSpace(body.SHA256), ExpectSizeBytes: body.SizeBytes,
	})
	if err != nil {
		response.ErrorResponse(w, resolveStatus(err), err.Error())
		return
	}

	repo := query.NewAddedModelsQueryRepo(db)
	info := detect()
	// the file is already known: answer what became of it, store nothing (a dismissed suggestion stays dismissed)
	if existing, err := repo.Find(found.ID); err == nil {
		s := toSuggestion(existing, info)
		response.SuccessResponse(w, http.StatusOK, s)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the suggestions")
		return
	}
	// the same file is one of the models shipped with the program
	if cat, err := models.Current(); err == nil {
		for _, m := range cat.Models {
			if m.SHA256 == found.SHA256 {
				response.ErrorResponse(w, http.StatusConflict, "this model is already in the list of models")
				return
			}
		}
	}
	waiting, err := repo.CountByStatus(entity.AddedStatusSuggested)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the suggestions")
		return
	}
	if waiting >= MaxPendingSuggestions {
		response.ErrorResponse(w, http.StatusTooManyRequests, "too many suggestions are waiting: dismiss or confirm some first")
		return
	}

	row := found.AddedModel(cleanText(body.SuggestedBy, maxSuggestedByRunes), now())
	if name := cleanText(body.Name, maxNameRunes); name != "" {
		row.Name = name
	}
	row.Why = cleanText(body.Why, maxWhyRunes)
	if body.ContextTokens != 0 {
		row.ContextTokens = body.ContextTokens
	}
	if err := persistent.NewAddedModelsPersistentRepo(db).Insert(&row); err != nil {
		// two suggestions of the same file at once: the table keeps one, and the answer is that one
		if existing, findErr := repo.Find(found.ID); findErr == nil {
			response.SuccessResponse(w, http.StatusOK, toSuggestion(existing, info))
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to store the suggestion")
		return
	}
	s := toSuggestion(row, info)
	s.Created = true
	response.SuccessResponse(w, http.StatusCreated, s)
}

// resolveStatus maps why a lookup failed to the answer: a name that is not valid is the caller's mistake, a file that cannot be used is a request that
// cannot be processed, and Hugging Face being unreachable is a bad gateway.
func resolveStatus(err error) int {
	switch {
	case errors.Is(err, huggingface.ErrInvalidRequest):
		return http.StatusBadRequest
	case errors.Is(err, huggingface.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, huggingface.ErrUnreachable):
		return http.StatusBadGateway
	default:
		return http.StatusUnprocessableEntity
	}
}

// dismissSuggestion handles POST /api/v1/local-ai/suggestions/{id}/dismiss. A dismissed suggestion is kept, so a tool does not offer the same file
// again. Dismissing twice is fine; a model that was already added is not a suggestion any more.
func (a api) dismissSuggestion(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}
	repo := query.NewAddedModelsQueryRepo(db)
	m, err := repo.Find(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !strings.HasPrefix(m.ID, entity.AddedIDPrefix)) {
		response.ErrorResponse(w, http.StatusNotFound, "no such suggestion")
		return
	}
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the suggestion")
		return
	}
	switch m.Status {
	case entity.AddedStatusAdded:
		response.ErrorResponse(w, http.StatusConflict, "this model was already added: remove it instead")
		return
	case entity.AddedStatusSuggested:
		if err := persistent.NewAddedModelsPersistentRepo(db).SetStatus(id, entity.AddedStatusDismissed, now().Format(time.RFC3339)); err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to dismiss the suggestion")
			return
		}
		m.Status = entity.AddedStatusDismissed
	}
	response.SuccessResponse(w, http.StatusOK, toSuggestion(m, detect()))
}

// demoteAdded puts every added model back to a suggestion after all downloaded files were removed. If the model chosen on the page was one of them,
// no model is chosen any more (the local AI is off at this point: removing is refused while it is on). Best effort: the files are already gone.
func demoteAdded(db *sql.DB, s entity.Settings) {
	added, err := query.NewAddedModelsQueryRepo(db).ListAdded()
	if err != nil {
		return
	}
	stamp := now().Format(time.RFC3339)
	repo := persistent.NewAddedModelsPersistentRepo(db)
	for _, m := range added {
		if err := repo.SetStatus(m.ID, entity.AddedStatusSuggested, stamp); err != nil {
			continue
		}
		if s.ModelID == m.ID {
			s.ModelID = ""
			_ = persistent.NewSettingsPersistentRepo(db).Save(s)
		}
	}
}

// installModel handles POST /api/v1/local-ai/models/{id}/install: a person confirms a suggested model. It becomes part of the list of models (added)
// and its download starts, into the same folder as every model. A model that is already added is simply downloaded (again). If the download cannot
// be started the model goes back to what it was.
func (a api) installModel(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}
	repo := query.NewAddedModelsQueryRepo(db)
	m, err := repo.Find(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !strings.HasPrefix(m.ID, entity.AddedIDPrefix)) {
		response.ErrorResponse(w, http.StatusNotFound, "no such suggested model")
		return
	}
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the model")
		return
	}
	previous := m.Status
	if previous != entity.AddedStatusAdded {
		if err := persistent.NewAddedModelsPersistentRepo(db).SetStatus(id, entity.AddedStatusAdded, now().Format(time.RFC3339)); err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to add the model")
			return
		}
	}
	if err := a.installs.Start(id, detect()); err != nil {
		if previous != entity.AddedStatusAdded {
			_ = persistent.NewAddedModelsPersistentRepo(db).SetStatus(id, previous, now().Format(time.RFC3339))
		}
		if errors.Is(err, entity.ErrUnsupportedSystem) || errors.Is(err, entity.ErrUnknownModel) {
			response.ErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "the download could not be started")
		return
	}
	a.respondPage(w, db, http.StatusAccepted)
}

// removeModel handles DELETE /api/v1/local-ai/models/{id}: remove one added model, its file and its row. The model chosen on the page cannot be
// removed (choose another first), nor one that is being downloaded (cancel it first), and a model that is not added is not removed here.
func (a api) removeModel(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	db, err := mainDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "database not configured")
		return
	}
	m, err := query.NewAddedModelsQueryRepo(db).Find(id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !strings.HasPrefix(m.ID, entity.AddedIDPrefix)) {
		response.ErrorResponse(w, http.StatusNotFound, "no such added model")
		return
	}
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the model")
		return
	}
	if m.Status != entity.AddedStatusAdded {
		response.ErrorResponse(w, http.StatusConflict, "this model was not added: dismiss the suggestion instead")
		return
	}
	s, err := query.NewSettingsQueryRepo(db).Load()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to load the local AI settings")
		return
	}
	if s.ModelID == id {
		response.ErrorResponse(w, http.StatusConflict, "this is the model chosen on the Local Agents page: choose another model first")
		return
	}
	if err := a.installs.RemoveModel(m.CatalogModel()); err != nil {
		if errors.Is(err, entity.ErrDownloadRunning) {
			response.ErrorResponse(w, http.StatusConflict, "this model is being downloaded: cancel the download first")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "the model file could not be removed")
		return
	}
	if err := persistent.NewAddedModelsPersistentRepo(db).Delete(id); err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to remove the model")
		return
	}
	a.respondPage(w, db, http.StatusOK)
}

// respondPage answers with the page's own answer (settings, models, suggestions, downloads), the way the other changes do.
func (a api) respondPage(w http.ResponseWriter, db *sql.DB, status int) {
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
	response.SuccessResponse(w, status, respondWithAdded(db, respond(s, c, a.runtime, a.installs), c))
}
