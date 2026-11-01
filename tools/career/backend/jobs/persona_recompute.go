package jobs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/jobmatcher"
	"career-tool-backend/persona"
	"career-tool-backend/personamatch"
)

// The recompute of one persona's scores over all jobs (step 142), for when the persona changed. See
// plan/ai/tools/career/step-142-persona-changed-rescore.md.

// PersonaFingerprint is a hash of everything the job matcher reads of a persona, plus the engine version: two
// personas with the same fingerprint score every job the same. It does not depend on the persona's id, name or
// description, on the company of an experience entry, or on the order of the skills and the entries (the matcher reads
// none of these as such), so a change that cannot move a score leaves it as it was.
func PersonaFingerprint(p jobmatcher.Persona) string {
	return fingerprintWith(jobmatcher.EngineVersion, p)
}

func fingerprintWith(engineVersion string, p jobmatcher.Persona) string {
	type entry struct {
		Title, Description, StartDate, EndDate string
	}
	skills := append([]string(nil), p.Skills...)
	sort.Strings(skills)
	entries := make([]entry, 0, len(p.Experience))
	for _, e := range p.Experience {
		entries = append(entries, entry{e.Title, e.Description, e.StartDate, e.EndDate})
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.StartDate != b.StartDate {
			return a.StartDate < b.StartDate
		}
		if a.EndDate != b.EndDate {
			return a.EndDate < b.EndDate
		}
		if a.Title != b.Title {
			return a.Title < b.Title
		}
		return a.Description < b.Description
	})
	raw, _ := json.Marshal(struct {
		Engine                                            string
		Headline, Summary, Location                       string
		DesiredTitles, DesiredLocations, RemotePreference string
		Skills                                            []string
		Experience                                        []entry
	}{engineVersion, p.Headline, p.Summary, p.Location, p.DesiredTitles, p.DesiredLocations, p.RemotePreference, skills, entries})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// LoadPersonaForMatch reads one persona as the matcher sees it, with its profile's id. persona.ErrUnknownPersona for a
// persona that does not exist.
func LoadPersonaForMatch(personaID string) (jobmatcher.Persona, string, error) {
	var profileID string
	err := db.CareerDB.QueryRow(`SELECT profile_id FROM personas WHERE id = ?`, personaID).Scan(&profileID)
	if errors.Is(err, sql.ErrNoRows) {
		return jobmatcher.Persona{}, "", persona.ErrUnknownPersona
	}
	if err != nil {
		return jobmatcher.Persona{}, "", err
	}
	personas, err := loadMatchPersonas(profileID)
	if err != nil {
		return jobmatcher.Persona{}, "", err
	}
	for _, p := range personas {
		if p.ID == personaID {
			return p, profileID, nil
		}
	}
	return jobmatcher.Persona{}, "", persona.ErrUnknownPersona // deleted between the two reads
}

// recomputeChunk is how many jobs one transaction covers; a var so a test can make it small.
var recomputeChunk = 200

// afterRecomputeChunk runs after each chunk is stored, and afterJobIDsListed after a chunk's job ids were read and before
// the jobs are loaded: hooks for tests (cancel the run, delete a job, break the store).
var (
	afterRecomputeChunk = func() {}
	afterJobIDsListed   = func() {}
)

// RecomputeOutcome says what RecomputePersona did.
type RecomputeOutcome struct {
	Skipped bool // the stored fingerprint already equals the persona's: nothing was read or written
	Jobs    int  // jobs scored
	Scored  int  // of those, the jobs the persona could judge (it has a row for each)
}

// RecomputePersona scores every job with the persona alone and brings that persona's rows up to date, unless its
// fingerprint says the stored scores were made from the same data (then it does nothing, Skipped). Jobs are read in
// chunks of recomputeChunk, each chunk stored in its own transaction: the row of a job the persona can judge is written
// or updated, the row of a job it cannot judge any more is removed, a job deleted meanwhile is skipped, and no other
// persona's row is touched. The state (the fingerprint) is stored only after the whole run succeeded, so a failed or
// cancelled run is simply run again. persona.ErrUnknownPersona for a persona that does not exist; ctx is checked between
// chunks and its error returned.
func RecomputePersona(ctx context.Context, personaID string) (RecomputeOutcome, error) {
	p, profileID, err := LoadPersonaForMatch(personaID)
	if err != nil {
		return RecomputeOutcome{}, err
	}
	fingerprint := PersonaFingerprint(p)
	if state, ok, err := personamatch.GetState(personaID); err != nil {
		return RecomputeOutcome{}, err
	} else if ok && state.Fingerprint == fingerprint {
		return RecomputeOutcome{Skipped: true}, nil
	}
	corpus, err := loadCorpus()
	if err != nil {
		return RecomputeOutcome{}, err
	}
	now := time.Now()
	var out RecomputeOutcome
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		ids, err := jobIDsAfter(after, recomputeChunk)
		if err != nil {
			return out, err
		}
		if len(ids) == 0 {
			break
		}
		afterJobIDsListed()
		scores := make([]personamatch.PersonaJobScore, 0, len(ids))
		for _, id := range ids {
			job, err := loadMatchJob(id)
			if errors.Is(err, ErrUnknownJob) { // deleted since the list was read
				continue
			}
			if err != nil {
				return out, err
			}
			out.Jobs++
			results := jobmatcher.ScoreEach(job, []jobmatcher.Persona{p}, corpus, now)
			if len(results) == 0 {
				scores = append(scores, personamatch.PersonaJobScore{JobID: id})
				continue
			}
			out.Scored++
			scores = append(scores, personamatch.PersonaJobScore{JobID: id, Judged: true, Score: results[0].Score, EngineVersion: results[0].EngineVersion})
		}
		if err := personamatch.ReplaceForPersona(personaID, profileID, scores); err != nil {
			return out, err
		}
		after = ids[len(ids)-1]
		afterRecomputeChunk()
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err := personamatch.SetState(personaID, profileID, fingerprint); err != nil {
		return out, err
	}
	return out, nil
}

// jobIDsAfter lists up to limit job ids greater than after, in id order (so every job is visited once, whatever is
// added or deleted meanwhile).
func jobIDsAfter(after string, limit int) ([]string, error) {
	rows, err := db.JobsDB.Query(`SELECT id FROM jobs WHERE id > ? ORDER BY id LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// StalePersonas lists the personas whose scores are not known to be made from their current data (step 142): the ones
// with no score state yet and the ones whose stored fingerprint differs from the fingerprint of the persona as it is now
// (it changed, or the engine version did). Profiles oldest first, each profile's personas oldest first, so the order
// never depends on chance. This is what the catch-up sweeper hands to the recompute consumer; a persona that cannot
// judge any job is not stale once its run has stored the state.
func StalePersonas() ([]string, error) {
	states, err := personamatch.AllStates()
	if err != nil {
		return nil, err
	}
	rows, err := db.CareerDB.Query(`SELECT id FROM profiles ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	var profileIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		profileIDs = append(profileIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	stale := []string{}
	for _, profileID := range profileIDs {
		personas, err := loadMatchPersonas(profileID)
		if err != nil {
			return nil, err
		}
		for _, p := range personas {
			if st, ok := states[p.ID]; !ok || st.Fingerprint != PersonaFingerprint(p) {
				stale = append(stale, p.ID)
			}
		}
	}
	return stale, nil
}
