package profilehandlers

import (
	"career-tool-backend/handlerutil"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"

	"career-tool-backend/db"
	"career-tool-backend/jobs/jobboard"
	"career-tool-backend/profile"
	"career-tool-backend/profile/profilejobmatchconsumer"
	"career-tool-backend/profile/profilerules"
)

// The auto-add rules routes (step 140), behind tool:career:jobs like the board routes the settings page also uses. See
// plan/ai/tools/career/step-140-profile-job-match-consumer.md. They live here, not in the profilerules package, for the
// reason the board routes do: they answer for profiles.

// writeProfileRulesError maps a profilerules, jobboard or profile error to its HTTP answer: what the caller got wrong
// is a 400 (the percentage, an unknown or foreign status, an unknown profile on a write, too many rules, an order that
// is not exactly the profile's rules), a second rule for
// one percentage is a 409, a rule or a profile that is not there on a read or a change of a rule is a 404.
func writeProfileRulesError(w http.ResponseWriter, action string, err error, unknownProfileIs404 bool) {
	switch {
	case errors.Is(err, profile.ErrUnknownProfile):
		code := http.StatusBadRequest
		if unknownProfileIs404 {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
	case errors.Is(err, profilerules.ErrInvalidPercent), errors.Is(err, profilerules.ErrTooManyRules), errors.Is(err, profilerules.ErrBadOrder),
		errors.Is(err, jobboard.ErrUnknownStatus), errors.Is(err, jobboard.ErrStatusNotOnBoard):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, profilerules.ErrDuplicatePercent):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, profilerules.ErrUnknownRule):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "failed to "+action+": "+err.Error(), http.StatusInternalServerError)
	}
}

// ProfileMatchRulesHandler handles GET (?profileId=, one profile's rules, highest percentage first), POST (add a rule),
// PUT (change a rule's percentage and status) and DELETE (?id=) on /profile-match-rules.
func ProfileMatchRulesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		profileID := r.URL.Query().Get("profileId")
		if profileID == "" {
			http.Error(w, "profileId query parameter is required", http.StatusBadRequest)
			return
		}
		rules, err := profilerules.List(profileID)
		if err != nil {
			writeProfileRulesError(w, "list the rules", err, true)
			return
		}
		db.WriteJSON(w, map[string]any{"rules": rules})

	case http.MethodPost:
		var body struct {
			ProfileID  string `json:"profileId"`
			MinPercent int    `json:"minPercent"`
			StatusID   string `json:"statusId"`
		}
		if !handlerutil.DecodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ProfileID == "" {
			http.Error(w, "profileId is required", http.StatusBadRequest)
			return
		}
		rule, err := profilerules.Create(body.ProfileID, body.MinPercent, body.StatusID)
		if err != nil {
			writeProfileRulesError(w, "add the rule", err, false)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(rule)

	case http.MethodPut:
		var body struct {
			ID         string `json:"id"`
			MinPercent int    `json:"minPercent"`
			StatusID   string `json:"statusId"`
		}
		if !handlerutil.DecodeJobBoardBody(w, r, &body) {
			return
		}
		if body.ID == "" {
			http.Error(w, "id is required", http.StatusBadRequest)
			return
		}
		rule, err := profilerules.Update(body.ID, body.MinPercent, body.StatusID)
		if err != nil {
			writeProfileRulesError(w, "change the rule", err, false)
			return
		}
		db.WriteJSON(w, rule)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}
		if err := profilerules.Delete(id); err != nil {
			writeProfileRulesError(w, "delete the rule", err, false)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ProfileMatchRulesOrderHandler handles PUT /profile-match-rules/order { profileId, ruleIds } (step 143): it sets the order
// of a profile's rules, the first id being tried first, and answers { rules } in that order. 400 for a bad body, no
// profileId, or ids that are not exactly the profile's rules (each once; nothing then changes), 404 for an unknown
// profile, 405 for another method.
func ProfileMatchRulesOrderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProfileID string   `json:"profileId"`
		RuleIDs   []string `json:"ruleIds"`
	}
	if !handlerutil.DecodeJobBoardBody(w, r, &body) {
		return
	}
	if body.ProfileID == "" {
		http.Error(w, "profileId is required", http.StatusBadRequest)
		return
	}
	rules, err := profilerules.Reorder(body.ProfileID, body.RuleIDs)
	if err != nil {
		writeProfileRulesError(w, "reorder the rules", err, true)
		return
	}
	db.WriteJSON(w, map[string]any{"rules": rules})
}

// applyRules is what the apply route runs; a var so a test can watch it, hold it and make it fail.
var applyRules = profilejobmatchconsumer.ApplyToExisting

// applying is the set of profiles whose rules are being applied right now: one pass per profile at a time, so a double
// click (or two tabs) cannot run the same pass twice at once.
var applying = struct {
	sync.Mutex
	profiles map[string]bool
}{profiles: map[string]bool{}}

// startApplying marks a profile as being applied and reports whether it was free; done frees it again.
func startApplying(profileID string) (done func(), ok bool) {
	applying.Lock()
	defer applying.Unlock()
	if applying.profiles[profileID] {
		return nil, false
	}
	applying.profiles[profileID] = true
	return func() {
		applying.Lock()
		delete(applying.profiles, profileID)
		applying.Unlock()
	}, true
}

// ProfileMatchRulesApplyHandler handles POST /profile-match-rules/apply { profileId, dryRun, includeRemoved } (steps 143, 144): it applies the
// profile's rules to the jobs that already exist, or with dryRun only counts what it would do, and answers the summary
// ({ jobs, notJudged, below, onBoard, removed, total, statuses }). A card already on the board is never moved, and a job the
// person took off the board is left out (removed) unless includeRemoved is set (default false); the first rule of the
// profile's list that fits a job decides. 400 for a bad body, no profileId, or a profile without rules; 404 for an unknown
// profile; 405 for another method; 409 while another pass for that profile is running; 500 when the pass fails, with
// { error, summary } where summary is what was done before the failure (what was added stays). The request's context
// stops the pass between chunks if the caller goes away.
func ProfileMatchRulesApplyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		ProfileID      string `json:"profileId"`
		DryRun         bool   `json:"dryRun"`
		IncludeRemoved bool   `json:"includeRemoved"`
	}
	if !handlerutil.DecodeJobBoardBody(w, r, &body) {
		return
	}
	if body.ProfileID == "" {
		http.Error(w, "profileId is required", http.StatusBadRequest)
		return
	}
	if err := profile.RequireProfileExists(body.ProfileID); err != nil {
		writeProfileRulesError(w, "apply the rules", err, true)
		return
	}
	done, free := startApplying(body.ProfileID)
	if !free {
		http.Error(w, "the rules of this profile are being applied already", http.StatusConflict)
		return
	}
	defer done()
	summary, err := applyRules(r.Context(), body.ProfileID, body.DryRun, body.IncludeRemoved)
	switch {
	case errors.Is(err, profilejobmatchconsumer.ErrNoRules):
		http.Error(w, "this profile has no rules: add a rule first", http.StatusBadRequest)
	case errors.Is(err, profile.ErrUnknownProfile):
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		log.Printf("profile rules: applying the rules of profile %s failed: %v", body.ProfileID, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "failed to apply the rules: " + err.Error(), "summary": summary})
	default:
		db.WriteJSON(w, summary)
	}
}
