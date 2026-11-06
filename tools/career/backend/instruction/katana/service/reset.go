package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// "Start over" (plan 192, D14): the person's way to let the AI begin a link's Katana instructions from the very beginning. It clears what the
// Katana engine made (the job list rule, the picked job-detail data), the request flags and the AI's attempt history, and hands back the
// ids of the conversations of those attempts so the screen can delete them through the core (a tool does not delete core data). It never
// deletes a crawled job, and it never touches crawl_instructions: those are Rod's, and switching back to Rod must still find them.

// The two scopes.
const (
	ScopeAll        = "all"         // the job list rule and the job details
	ScopeJobDetails = "job_details" // only the job details; the job list rule stays
)

// ErrCrawlRunning is returned while a crawl is running for the link.
var ErrCrawlRunning = errors.New("a crawl is running for this link; wait until it has finished")

// ErrBadScope is returned for a scope that is neither of the two.
var ErrBadScope = errors.New(`scope must be "all" or "job_details"`)

// StartOverResult is what a reset did.
type StartOverResult struct {
	ConversationIDs []string `json:"conversationIds"`
	AttemptsRemoved int      `json:"attemptsRemoved"`
	RuleCleared     bool     `json:"ruleCleared"`
}

// StartOver clears one link's Katana instructions in a single transaction. Only this link's rows are touched.
func StartOver(linkID, scope string) (StartOverResult, error) {
	result := StartOverResult{ConversationIDs: []string{}}
	if scope != ScopeAll && scope != ScopeJobDetails {
		return result, ErrBadScope
	}
	if err := portal.RequirePortalLinkExists(linkID); err != nil {
		return result, err
	}
	if _, err := portal.ReconcileStaleInstructionAttempts(); err != nil {
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
		return result, portal.ErrGenerationRunning
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM crawl_runs WHERE portal_link_id = ? AND status = 'running'`, linkID).Scan(&running); err != nil {
		return result, err
	}
	if running > 0 {
		return result, ErrCrawlRunning
	}

	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			result.ConversationIDs = append(result.ConversationIDs, id)
		}
	}
	kindFilter, convColumns := ``, []string{"instructions_ai_conversation_id", "job_detail_instructions_ai_conversation_id"}
	if scope == ScopeJobDetails {
		kindFilter, convColumns = ` AND kind = 'job_detail'`, []string{"job_detail_instructions_ai_conversation_id"}
	}
	rows, err := tx.Query(`SELECT conversation_id FROM instruction_attempts WHERE portal_link_id = ? AND conversation_id IS NOT NULL`+kindFilter+` ORDER BY started_at, rowid`, linkID)
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
	for _, col := range convColumns {
		var id *string
		if err := tx.QueryRow(`SELECT `+col+` FROM portal_links WHERE id = ?`, linkID).Scan(&id); err != nil {
			return result, err
		}
		if id != nil {
			add(*id)
		}
	}

	res, err := tx.Exec(`DELETE FROM instruction_attempts WHERE portal_link_id = ?`+kindFilter, linkID)
	if err != nil {
		return result, err
	}
	n, _ := res.RowsAffected()
	result.AttemptsRemoved = int(n)

	set := []string{
		`job_detail_crawl_instructions = NULL`, `job_detail_instructions_origin = ''`,
		`job_detail_instructions_ai_requested = 0`, `job_detail_crawl_requested = 0`,
		`job_detail_instructions_ai_error = NULL`, `job_detail_instructions_ai_error_at = NULL`,
		`job_detail_instructions_ai_conversation_id = NULL`, `job_detail_instructions_ai_error_dismissed_at = NULL`,
	}
	if scope == ScopeAll {
		set = append(set,
			`link_rules = NULL`, `crawl_method = 'instructions'`, `listing_crawl_requested = 0`,
			`instructions_ai_error = NULL`, `instructions_ai_error_at = NULL`,
			`instructions_ai_conversation_id = NULL`, `instructions_ai_error_dismissed_at = NULL`,
			`jobs_question_asked_at = NULL`, `jobs_question_note = NULL`)
		result.RuleCleared = true
	}
	if _, err := tx.Exec(`UPDATE portal_links SET `+strings.Join(set, ", ")+`, updated_at = datetime('now') WHERE id = ?`, linkID); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	Forget(linkID)
	return result, nil
}

// Forget drops the AI session's scratch of a link (a reset starts from nothing).
func Forget(linkID string) {
	scratchMu.Lock()
	defer scratchMu.Unlock()
	delete(scratches, linkID)
}

type startOverBody struct {
	PortalLinkID string `json:"portalLinkId"`
	Scope        string `json:"scope"`
}

// StartOverHandler handles POST /portal-links/katana/start-over (the screen, behind core's proxy and the person's session).
func StartOverHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body startOverBody
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil || strings.TrimSpace(body.PortalLinkID) == "" {
		http.Error(w, "portalLinkId and scope are required", http.StatusBadRequest)
		return
	}
	res, err := StartOver(strings.TrimSpace(body.PortalLinkID), body.Scope)
	switch {
	case errors.Is(err, ErrBadScope):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, portal.ErrGenerationRunning), errors.Is(err, ErrCrawlRunning):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		portal.WritePortalLinkAwareError(w, "start over", err)
	default:
		db.WriteJSON(w, res)
	}
}
