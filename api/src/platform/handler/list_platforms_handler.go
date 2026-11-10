package handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/platform"
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"
)

// platformResponse is {id, name} plus the platform's own selectable
// model list — empty (never null) when no model choice applies, so the
// frontend's create-conversation flow knows whether to show a model
// picker at all. See
// plan/ai/conversation/step-07-fixed-platform-and-model-per-conversation.md.
type platformResponse struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Models []string `json:"models"`
	// RequiresKey is true for a platform reached with a stored API key
	// (the cloud platforms): the frontend shows "add a key" only then.
	RequiresKey bool `json:"requiresKey"`
	// HasKey is true when the platform is ready as far as keys go: a key is stored for it, or it needs none (the local AI). A screen
	// that starts a conversation offers only the platforms with HasKey (plan/ai/conversation/step-229). It says nothing about the key.
	HasKey bool `json:"hasKey"`
	// OffersTools is false for a platform whose conversations are offered no tools and run in direct mode only: the
	// frontend then does not offer composer mode for it.
	OffersTools bool `json:"offersTools"`
	// NeedsShortSteps says the platform's models do better with work split into short, one-action messages than with one long
	// prompt (a caller that can split its work should).
	NeedsShortSteps bool `json:"needsShortSteps"`
	// ContextTokens is how much a conversation on the platform can hold at once; absent when the platform states none (the cloud
	// platforms), which no caller treats as a limit.
	ContextTokens *int `json:"contextTokens,omitempty"`
	// ModelNames maps a model id to a readable name, for the models the platform offers right now; absent when the platform gives none
	// (the cloud platforms), in which case the id is the name.
	ModelNames map[string]string `json:"modelNames,omitempty"`
	// UnsuitableFor maps a model id to the tasks it was measured not to manage (a tool can warn before it starts), for the models offered
	// right now; absent when none is flagged (the cloud platforms).
	UnsuitableFor map[string][]string `json:"unsuitableFor,omitempty"`
	// Drivers maps a model id to how a caller should run a long task for it ("code": in code, with small questions), for the offered models that
	// have one; absent when none has (the cloud platforms).
	Drivers map[string]string `json:"drivers,omitempty"`
}

// ListPlatformsHandler handles GET /api/v1/platforms. See
// plan/ai/platform/step-05-platform-and-key-api.md.
func ListPlatformsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	db := reqCtx.GetDI().GetDatabaseManager().Connector.DB
	withKey, err := platform_query.NewKeyQueryRepo(db).PlatformsWithKey()
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to read the platform keys")
		return
	}
	response.SuccessResponse(w, http.StatusOK, platformResponses(withKey))
}

// platformResponses is the list the endpoint returns: every registered platform that is available right now. withKey is the set of platform
// ids that have a stored key.
func platformResponses(withKey map[string]bool) []platformResponse {
	entries := platform.List()
	out := make([]platformResponse, 0, len(entries))
	for _, info := range entries {
		entry, _ := platform.Lookup(info.ID)
		if !entry.IsAvailable() {
			continue
		}
		models := entry.SelectableModelList()
		out = append(out, platformResponse{ID: info.ID, Name: info.Name, Models: models, RequiresKey: entry.RequiresKey(), HasKey: !entry.RequiresKey() || withKey[info.ID], OffersTools: entry.OffersAllTools(), NeedsShortSteps: entry.ShortSteps, ContextTokens: contextOf(entry), ModelNames: namesOf(entry, models), UnsuitableFor: unsuitableOf(entry, models), Drivers: driversOf(entry, models)})
	}
	return out
}

// namesOf is the readable names of the models offered now, nil when the platform gives none or none of its offered models has one.
func namesOf(entry platform.Entry, offered []string) map[string]string {
	if entry.ModelNames == nil {
		return nil
	}
	all := entry.ModelNames()
	var out map[string]string
	for _, id := range offered {
		if name := all[id]; name != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[id] = name
		}
	}
	return out
}

// driversOf is the drivers of the offered models, nil when none has one.
func driversOf(entry platform.Entry, offered []string) map[string]string {
	if entry.ModelDriver == nil {
		return nil
	}
	all := entry.ModelDriver()
	var out map[string]string
	for _, id := range offered {
		if d := all[id]; d != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[id] = d
		}
	}
	return out
}

// unsuitableOf is the tasks the offered models were measured not to manage, nil when the platform flags none of them.
func unsuitableOf(entry platform.Entry, offered []string) map[string][]string {
	if entry.ModelUnsuitable == nil {
		return nil
	}
	all := entry.ModelUnsuitable()
	var out map[string][]string
	for _, id := range offered {
		if tasks := all[id]; len(tasks) > 0 {
			if out == nil {
				out = map[string][]string{}
			}
			out[id] = append([]string(nil), tasks...)
		}
	}
	return out
}

// contextOf is the context the platform states, nil when it states none (or a value that is no limit).
func contextOf(entry platform.Entry) *int {
	if entry.ContextTokens == nil {
		return nil
	}
	if n := entry.ContextTokens(); n > 0 {
		return &n
	}
	return nil
}
