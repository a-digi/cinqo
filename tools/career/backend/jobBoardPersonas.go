package main

import (
	"log"
	"sort"

	"career-tool-backend/jobboard"
	"career-tool-backend/persona"
	"career-tool-backend/persona/personamatch"
)

// The persona scores on a profile's job board (step 141). Each card gets how well each of the profile's own personas fits
// the job, from the rows the PersonaJobMatchConsumer wrote. The names come from the personas as they are now (a rename
// shows at once), and a row of a persona that no longer exists is skipped.

// boardPersona is a persona of the board's profile: its id and name.
type boardPersona struct {
	ID, Name string
}

// boardScores reads the persona scores of a profile's personas for some jobs; a var so a test can make it fail.
var boardScores = personamatch.ForJobs

// boardPersonas lists a profile's personas oldest first (created time, then id); a var so a test can make it fail.
var boardPersonas = func(profileID string) ([]boardPersona, error) {
	list, err := persona.ListPersonas(profileID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].CreatedAt != list[j].CreatedAt {
			return list[i].CreatedAt < list[j].CreatedAt
		}
		return list[i].ID < list[j].ID
	})
	out := make([]boardPersona, 0, len(list))
	for _, p := range list {
		out = append(out, boardPersona{ID: p.ID, Name: p.Name})
	}
	return out, nil
}

// personaScoresOf turns a job's rows into its card's list: the personas that still exist (in the order they were
// created, which breaks ties), highest score first, the first one marked best. Nil when none is left.
func personaScoresOf(rows []personamatch.Row, personas []boardPersona) []jobboard.PersonaScore {
	rank := make(map[string]int, len(personas))
	name := make(map[string]string, len(personas))
	for i, p := range personas {
		rank[p.ID] = i
		name[p.ID] = p.Name
	}
	var out []jobboard.PersonaScore
	for _, r := range rows {
		if _, ok := rank[r.PersonaID]; !ok {
			continue
		}
		out = append(out, jobboard.PersonaScore{PersonaID: r.PersonaID, PersonaName: name[r.PersonaID], Score: r.Score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return rank[out[i].PersonaID] < rank[out[j].PersonaID]
	})
	if len(out) > 0 {
		out[0].Best = true
	}
	return out
}

// attachPersonaScores fills the Personas of every card of the board. It is a side view: when the personas or the scores
// cannot be read it logs that and leaves the cards as they are, so the board itself never fails because of it.
func attachPersonaScores(board *jobboard.Board, profileID string) {
	var ids []string
	for _, c := range board.Columns {
		for _, card := range c.Jobs {
			ids = append(ids, card.JobID)
		}
	}
	if len(ids) == 0 {
		return
	}
	personas, err := boardPersonas(profileID)
	if err != nil {
		log.Printf("job board: could not read the personas of profile %s: %v", profileID, err)
		return
	}
	byJob, err := boardScores(profileID, ids)
	if err != nil {
		log.Printf("job board: could not read the persona scores of profile %s: %v", profileID, err)
		return
	}
	for ci := range board.Columns {
		for ji := range board.Columns[ci].Jobs {
			card := &board.Columns[ci].Jobs[ji]
			card.Personas = personaScoresOf(byJob[card.JobID], personas)
		}
	}
}
