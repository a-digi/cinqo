package applicationloghandlers

import (
	"encoding/json"
	"net/http"

	"career-tool-backend/applicationlog"
	"career-tool-backend/db"
)

// The application protocol (plan 176, step 2): the dated log of the communication of a job application, for one (profile, job) pair.
// Route application-messages, scope tool:career:jobs:
//
//	GET    ?profileId=&jobId=                       { messages: [...] }  oldest first
//	POST   {profileId, jobId, direction, channel, occurredAt, hasTime?, channelLabel?, subject?, body?, recruiterId?, counterpartName?,
//	        counterpartContact?, cvDocumentId?, externalUrl?}                the pair's list
//	PUT    {id, ...the same fields, each optional}                          the pair's list
//	DELETE ?id=                                     204
//
// Every unknown id and every refusal of a caller (a direction or channel outside the lists, a date that cannot be read, a text over its
// limit, a link that is not http(s)) is a 400 with the reason, and nothing is written; anything else is a 500.
func writeApplicationLogError(w http.ResponseWriter, action string, err error) {
	if applicationlog.IsRefusal(err) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
}

func ApplicationLogHandler(w http.ResponseWriter, r *http.Request) {
	reload := func(profileID, jobID string) {
		list, err := applicationlog.List(profileID, jobID)
		if err != nil {
			writeApplicationLogError(w, "reload the protocol", err)
			return
		}
		db.WriteJSON(w, map[string]any{"messages": list})
	}
	switch r.Method {
	case http.MethodGet:
		profileID, jobID := r.URL.Query().Get("profileId"), r.URL.Query().Get("jobId")
		if profileID == "" || jobID == "" {
			http.Error(w, "profileId and jobId query parameters are required", http.StatusBadRequest)
			return
		}
		reload(profileID, jobID)

	case http.MethodPost:
		var body struct {
			ProfileID string `json:"profileId"`
			JobID     string `json:"jobId"`
			applicationlog.Input
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProfileID == "" || body.JobID == "" {
			http.Error(w, "profileId, jobId, direction, channel and occurredAt are required", http.StatusBadRequest)
			return
		}
		if _, err := applicationlog.Add(body.ProfileID, body.JobID, body.Input); err != nil {
			writeApplicationLogError(w, "add the message", err)
			return
		}
		reload(body.ProfileID, body.JobID)

	case http.MethodPut:
		var body struct {
			ID string `json:"id"`
			applicationlog.Patch
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		updated, err := applicationlog.Update(body.ID, body.Patch)
		if err != nil {
			writeApplicationLogError(w, "update the message", err)
			return
		}
		reload(updated.ProfileID, updated.JobID)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := applicationlog.Delete(id); err != nil {
			writeApplicationLogError(w, "delete the message", err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
