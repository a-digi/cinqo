// instruction_failures.go (plan/ai/tools/career/step-215-clear-failures-when-fully-instructed.md) clears what a link remembers of the times the AI
// failed to write its instructions once the link is fully instructed: the failed attempts (the counters behind "no instructions after N attempts"
// and "Probably can't be crawled") and the error texts of the two kinds. A failure says nothing about a link whose instructions exist, and it may
// even arrive after they were saved (a run that ran out of rounds two minutes after the link got its instructions), so the clearing is done
// wherever the truth can change: when instructions are stored, when a failure is recorded, when an error text is about to be stored, and once at
// start-up for what is already there.
package portal

import (
	"log"

	"career-tool-backend/db"
)

// instructedSnapshot is what "fully instructed" is judged from, for one link: the same facts the pipeline reads (loadPipelineRows).
type instructedSnapshot struct {
	// Engine is the link's own engine ("" follows the Browser's, "rod" reads it the Rod way).
	Engine        string
	RequiresLogin bool
	// HasAI / HasRule: the link has AI-written job-list instructions / a link-crawler rule.
	HasAI, HasRule bool
	// HasDetail: the link has job-detail instructions.
	HasDetail bool
}

// fullyInstructed: the job list and the job details both have instructions, judged as the pipeline judges them with no Browser reading ("" counts
// either a rule or AI instructions), except that a link set to Rod, or one that needs a login, counts its AI instructions only (plans 190, 197).
func fullyInstructed(s instructedSnapshot) bool {
	return s.HasDetail && listingPresent(rowMethod("", s.Engine), s.RequiresLogin, s.HasAI, s.HasRule)
}

const instructedColumns = `COALESCE(TRIM(crawl_instructions), '') <> '',
	        COALESCE(TRIM(link_rules), '') <> '',
	        COALESCE(requires_login, 0) <> 0,
	        COALESCE(crawl_engine, ''),
	        COALESCE(TRIM(job_detail_crawl_instructions), '') <> ''`

// IsFullyInstructed reads the link's stored instructions and judges them; false for a link that does not exist.
func IsFullyInstructed(linkID string) (bool, error) {
	var s instructedSnapshot
	rows, err := db.JobsDB.Query(`SELECT `+instructedColumns+` FROM portal_links WHERE id = ?`, linkID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	if err := rows.Scan(&s.HasAI, &s.HasRule, &s.RequiresLogin, &s.Engine, &s.HasDetail); err != nil {
		return false, err
	}
	return fullyInstructed(s), nil
}

// ClearInstructionFailures removes, for one link, its failed instruction attempts (both kinds) and the error texts of both kinds. Succeeded
// attempts, a running attempt and the conversation ids stay (the chat of the last run still opens). Returns how many failed attempts went.
func ClearInstructionFailures(linkID string) (int, error) {
	tx, err := db.JobsDB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`DELETE FROM instruction_attempts WHERE portal_link_id = ? AND status = 'failed'`, linkID)
	if err != nil {
		return 0, err
	}
	removed, _ := res.RowsAffected()
	if _, err := tx.Exec(`UPDATE portal_links SET
		instructions_ai_error = NULL, instructions_ai_error_at = NULL, instructions_ai_error_dismissed_at = NULL,
		job_detail_instructions_ai_error = NULL, job_detail_instructions_ai_error_at = NULL, job_detail_instructions_ai_error_dismissed_at = NULL
		WHERE id = ?`, linkID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(removed), nil
}

// ClearFailuresIfInstructed clears the link's failures when it is fully instructed; a no-op otherwise.
func ClearFailuresIfInstructed(linkID string) error {
	ok, err := IsFullyInstructed(linkID)
	if err != nil || !ok {
		return err
	}
	_, err = ClearInstructionFailures(linkID)
	return err
}

// clearAfterChange is what the save and failure paths call: best-effort, because what triggered it (the instructions are saved, the failure is
// recorded) already happened and must not turn into an error of its own.
func clearAfterChange(linkID string) {
	if err := ClearFailuresIfInstructed(linkID); err != nil {
		log.Printf("instruction failures: could not clear them for link %s: %v", linkID, err)
	}
}

// ClearFailuresForInstructedLinks walks every link once and clears the instructed ones; for start-up, so what is already stored is cleaned
// without a click. Returns how many links were cleaned: the instructed ones that had a failed attempt or an error to clear.
func ClearFailuresForInstructedLinks() (int, error) {
	rows, err := db.JobsDB.Query(`SELECT id, ` + instructedColumns + ` FROM portal_links`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		var s instructedSnapshot
		if err := rows.Scan(&id, &s.HasAI, &s.HasRule, &s.RequiresLogin, &s.Engine, &s.HasDetail); err != nil {
			rows.Close()
			return 0, err
		}
		if fullyInstructed(s) {
			ids = append(ids, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close() // one connection only: the writes start after this result set is closed
	cleaned := 0
	for _, id := range ids {
		var failed, errors int
		if err := db.JobsDB.QueryRow(`SELECT
			(SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = ?1 AND status = 'failed'),
			(SELECT COUNT(*) FROM portal_links WHERE id = ?1 AND (instructions_ai_error IS NOT NULL OR job_detail_instructions_ai_error IS NOT NULL
			   OR instructions_ai_error_at IS NOT NULL OR job_detail_instructions_ai_error_at IS NOT NULL
			   OR instructions_ai_error_dismissed_at IS NOT NULL OR job_detail_instructions_ai_error_dismissed_at IS NOT NULL))`, id).Scan(&failed, &errors); err != nil {
			return cleaned, err
		}
		if failed == 0 && errors == 0 {
			continue
		}
		if _, err := ClearInstructionFailures(id); err != nil {
			return cleaned, err
		}
		cleaned++
	}
	return cleaned, nil
}
