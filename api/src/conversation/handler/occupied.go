package handler

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/platform"
)

// The scopes that may unload a model another conversation left loaded: the same rule as the unload route of the local AI (plan 225).
const (
	manageScope     = "cinqo:platform:manage"
	superAdminScope = "cinqo:super:admin"
)

// occupiedData is the data of the 409 answer a start gets when it would displace another model.
type occupiedData struct {
	LoadedModel     string                     `json:"loadedModel"`
	LoadedModelName string                     `json:"loadedModelName"`
	State           string                     `json:"state"`
	Conversations   []platform.ConversationRef `json:"conversations"`
	OthersUsing     int                        `json:"othersUsing"`
	CanRelease      bool                       `json:"canRelease"`
}

// occupiedBody is the whole 409 answer: the usual error envelope, a code a caller can act on and the data.
type occupiedBody struct {
	Error   bool         `json:"error"`
	Message string       `json:"message"`
	Code    string       `json:"code"`
	Data    occupiedData `json:"data"`
}

// occupiedAnswer decides what a start on model of this platform gets back (plan/ai/conversation/step-241): 0 and no body when the start may go on,
// otherwise the status and the body to answer with. Another model of the platform that is loaded or loading is a conflict (409). With release set
// the other model is unloaded first, never forced and only for a caller who may unload: not allowed is a 403, in use is the same 409.
func occupiedAnswer(entry platform.Entry, model, userID string, scopes []string, release bool) (int, *occupiedBody) {
	mayRelease := slices.Contains(scopes, manageScope) || slices.Contains(scopes, superAdminScope)
	c := entry.CheckOccupied(model, userID, mayRelease)
	if c == nil {
		return 0, nil
	}
	if release {
		if !mayRelease {
			return http.StatusForbidden, &occupiedBody{Error: true, Message: "unloading a model needs the permission to manage the AI platforms", Code: "forbidden", Data: dataOf(c)}
		}
		if c.State != platform.OccupantInUse && entry.ReleaseOccupant(model) {
			return 0, nil
		}
		// it became busy at that moment, or could not be unloaded: say what is there now
		if c = entry.CheckOccupied(model, userID, mayRelease); c == nil {
			return 0, nil
		}
	}
	return http.StatusConflict, &occupiedBody{Error: true, Message: c.Message, Code: c.Code, Data: dataOf(c)}
}

func dataOf(c *platform.Conflict) occupiedData {
	convs := c.Conversations
	if convs == nil {
		convs = []platform.ConversationRef{}
	}
	return occupiedData{LoadedModel: c.LoadedModel, LoadedModelName: c.LoadedModelName, State: c.State, Conversations: convs, OthersUsing: c.OthersUsing, CanRelease: c.CanRelease}
}

// writeOccupied answers a start that must not go on.
func writeOccupied(w http.ResponseWriter, status int, body *occupiedBody) {
	response.BuildHeaders(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
