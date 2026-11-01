package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"career-tool-backend/db"
	"career-tool-backend/personamatch"
	"career-tool-backend/personamatchconsumer"
)

// The persona scores of one job (step 141), for the job's page: every persona of every profile with how well it fits the
// job, best first, and a way to score the job again now. The scores are the rows the PersonaJobMatchConsumer wrote; the
// names are read as they are now. See plan/ai/tools/career/step-141-persona-job-match.md.

const maxPersonaMatchBodyBytes = 4 << 10

// personaMatchView is one persona's score for the job with who it is.
type personaMatchView struct {
	PersonaID   string `json:"personaId"`
	PersonaName string `json:"personaName"`
	ProfileID   string `json:"profileId"`
	ProfileName string `json:"profileName"`
	Score       int    `json:"score"`
}

// personaMatchesAnswer is what both routes answer: the matches, best first, and the id of the best persona ("" when
// there is none).
type personaMatchesAnswer struct {
	Matches []personaMatchView `json:"matches"`
	Best    string             `json:"best"`
}

// matchPersona is a persona with its profile's name, in the order they were created (profiles, then personas), which
// breaks ties between equal scores.
type matchPersona struct {
	ID, Name, ProfileID, ProfileName string
}

// listMatchPersonas reads every persona with its profile's name; a var so a test can make it fail.
var listMatchPersonas = func() ([]matchPersona, error) {
	rows, err := db.CareerDB.Query(
		`SELECT p.id, p.name, p.profile_id, pr.first_name, pr.last_name
		 FROM personas p JOIN profiles pr ON pr.id = p.profile_id
		 ORDER BY pr.created_at, pr.id, p.created_at, p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []matchPersona
	for rows.Next() {
		var p matchPersona
		var first, last string
		if err := rows.Scan(&p.ID, &p.Name, &p.ProfileID, &first, &last); err != nil {
			return nil, err
		}
		p.ProfileName = strings.TrimSpace(first + " " + last)
		if p.ProfileName == "" { // a profile without a name, as the pages label it
			p.ProfileName = "Profile " + p.ProfileID[:min(8, len(p.ProfileID))]
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// buildPersonaMatches turns a job's rows into the answer: the personas that still exist, highest score first, equal
// scores in the order the personas were created; Best is the first one. The list is never nil.
func buildPersonaMatches(rows []personamatch.Row, personas []matchPersona) personaMatchesAnswer {
	rank := make(map[string]int, len(personas))
	byID := make(map[string]matchPersona, len(personas))
	for i, p := range personas {
		rank[p.ID] = i
		byID[p.ID] = p
	}
	out := personaMatchesAnswer{Matches: []personaMatchView{}}
	for _, r := range rows {
		p, ok := byID[r.PersonaID]
		if !ok {
			continue
		}
		out.Matches = append(out.Matches, personaMatchView{PersonaID: p.ID, PersonaName: p.Name, ProfileID: p.ProfileID, ProfileName: p.ProfileName, Score: r.Score})
	}
	sort.SliceStable(out.Matches, func(i, j int) bool {
		if out.Matches[i].Score != out.Matches[j].Score {
			return out.Matches[i].Score > out.Matches[j].Score
		}
		return rank[out.Matches[i].PersonaID] < rank[out.Matches[j].PersonaID]
	})
	if len(out.Matches) > 0 {
		out.Best = out.Matches[0].PersonaID
	}
	return out
}

func jobExists(jobID string) (bool, error) {
	var one int
	err := db.JobsDB.QueryRow(`SELECT 1 FROM jobs WHERE id = ?`, jobID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// answerPersonaMatches reads a job's rows and writes the answer.
func answerPersonaMatches(w http.ResponseWriter, jobID string) {
	rows, err := personamatch.ForJob(jobID)
	if err != nil {
		http.Error(w, "failed to read the persona matches: "+err.Error(), http.StatusInternalServerError)
		return
	}
	personas, err := listMatchPersonas()
	if err != nil {
		http.Error(w, "failed to read the personas: "+err.Error(), http.StatusInternalServerError)
		return
	}
	db.WriteJSON(w, buildPersonaMatches(rows, personas))
}

// jobPersonaMatchesHandler handles GET /job-persona-matches?jobId=: every persona's score for the job, best first.
// 400 without a jobId, 404 for a job that does not exist; a job nobody could judge answers an empty list.
func jobPersonaMatchesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId query parameter is required", http.StatusBadRequest)
		return
	}
	ok, err := jobExists(jobID)
	if err != nil {
		http.Error(w, "failed to read the job: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "unknown job", http.StatusNotFound)
		return
	}
	answerPersonaMatches(w, jobID)
}

// jobPersonaMatchesRecalculateHandler handles POST /job-persona-matches/recalculate {jobId}: it scores the job for every
// persona now, with the same code the consumer runs, replaces the job's rows and answers like the GET. 400 for a bad body
// or no jobId, 404 for an unknown job, 500 when the scoring or the store fails (the earlier rows are then kept).
func jobPersonaMatchesRecalculateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPersonaMatchBodyBytes)).Decode(&body); err != nil || body.JobID == "" {
		http.Error(w, "jobId is required", http.StatusBadRequest)
		return
	}
	ok, err := jobExists(body.JobID)
	if err != nil {
		http.Error(w, "failed to read the job: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "unknown job", http.StatusNotFound)
		return
	}
	outcomes, err := recalculatePersonaMatches([]string{body.JobID})
	if err != nil {
		http.Error(w, "failed to score the job: "+err.Error(), http.StatusInternalServerError)
		return
	}
	for _, o := range outcomes {
		if o.Status == personamatchconsumer.Failed {
			http.Error(w, "failed to score the job: "+o.Err.Error(), http.StatusInternalServerError)
			return
		}
	}
	answerPersonaMatches(w, body.JobID)
}

// recalculatePersonaMatches is the consumer's own batch; a var so a test can make it fail.
var recalculatePersonaMatches = personamatchconsumer.ProcessBatch
