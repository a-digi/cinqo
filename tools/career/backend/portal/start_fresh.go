package portal

import (
	"errors"

	"career-tool-backend/db"
)

// start_fresh.go (step 161) lets the user wipe a link's earlier "Generate with AI" history and begin again:
// the attempt rows (which drive the "Job list: no instructions after N attempts" and "Probably can't be
// crawled" pills), the leftover AI error and conversation ids, the pending "does this page list jobs?"
// question and, only when asked, the stored crawl instructions. The conversations those attempts ran in are
// not deleted here (a tool does not delete core data): their ids are returned so the frontend can delete them
// through the core, with the user's own permission. See plan/ai/tools/career/step-161-start-fresh-for-a-link.md.

// ErrGenerationRunning is returned while an AI generation is still running for the link: wiping its rows
// from under a live turn would leave the run unaccounted for.
var ErrGenerationRunning = errors.New("a generation is still running for this link; stop it first")

type StartFreshResult struct {
	// ConversationIDs are the conversations of this link's earlier attempts (both kinds, plus the link's
	// current AI conversation ids), without repeats, oldest first.
	ConversationIDs []string `json:"conversationIds"`
	AttemptsRemoved int      `json:"attemptsRemoved"`
}

// StartFresh clears one link's AI history in a single transaction. clearInstructions also removes the stored
// listing and job-detail crawl instructions; without it they are kept. Only this link's rows are touched.
func StartFresh(linkID string, clearInstructions bool) (StartFreshResult, error) {
	result := StartFreshResult{ConversationIDs: []string{}}
	if err := RequirePortalLinkExists(linkID); err != nil {
		return result, err
	}
	// A stale 'running' row (a crashed run) must not block the reset.
	if _, err := ReconcileStaleInstructionAttempts(); err != nil {
		return result, err
	}

	tx, err := db.JobsDB.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	var running int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM instruction_attempts WHERE portal_link_id = ? AND status = 'running'`, linkID).Scan(&running); err != nil {
		return result, err
	}
	if running > 0 {
		return result, ErrGenerationRunning
	}

	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			result.ConversationIDs = append(result.ConversationIDs, id)
		}
	}
	rows, err := tx.Query(`SELECT conversation_id FROM instruction_attempts WHERE portal_link_id = ? AND conversation_id IS NOT NULL ORDER BY started_at, rowid`, linkID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		add(id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	var listingConv, detailConv *string
	if err := tx.QueryRow(`SELECT instructions_ai_conversation_id, job_detail_instructions_ai_conversation_id FROM portal_links WHERE id = ?`, linkID).Scan(&listingConv, &detailConv); err != nil {
		return result, err
	}
	for _, c := range []*string{listingConv, detailConv} {
		if c != nil {
			add(*c)
		}
	}

	res, err := tx.Exec(`DELETE FROM instruction_attempts WHERE portal_link_id = ?`, linkID)
	if err != nil {
		return result, err
	}
	n, _ := res.RowsAffected()
	result.AttemptsRemoved = int(n)

	query := `UPDATE portal_links SET
		instructions_ai_error = NULL, instructions_ai_error_at = NULL, instructions_ai_conversation_id = NULL, instructions_ai_error_dismissed_at = NULL,
		job_detail_instructions_ai_error = NULL, job_detail_instructions_ai_error_at = NULL, job_detail_instructions_ai_conversation_id = NULL, job_detail_instructions_ai_error_dismissed_at = NULL,
		jobs_question_asked_at = NULL, jobs_question_note = NULL, `
	if clearInstructions {
		query += `crawl_instructions = NULL, job_detail_crawl_instructions = NULL, job_detail_instructions_origin = '', `
	}
	query += `updated_at = datetime('now') WHERE id = ?`
	if _, err := tx.Exec(query, linkID); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}
