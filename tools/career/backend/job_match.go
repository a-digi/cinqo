// job_match.go — "Job Match": lets an AI conversation assess how well
// one job posting fits a specific career Profile (by inspecting every
// Persona under it, via career.db's own get_persona_details) and
// record a 0-100 score. Mirrors the "Generate with AI" mechanic
// already established for crawl instructions (generateInstructions.ts,
// CrawlPanel.tsx) one level up: a hidden conversation, tracked via a
// conversation id field the frontend can resume watching across a
// reload, finishing by either a score (success) or a recorded error
// (failure).
//
// Split into two persistence paths, deliberately never overlapping:
//   - startJobMatch/updateJobMatchStatus (below) back the human-facing
//     PUT /jobs/match endpoint (http.go) — starts/clears the tracking
//     row, records a client-observed failure. Can NEVER set score or
//     persona_id.
//   - saveJobMatchResult (below) backs the AI-facing save_job_match
//     MCP tool — the ONLY place a score is ever written.
//
// See plan/ai/tools/career/step-XX-job-match.md.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startJobMatch begins tracking a new match attempt for jobId —
// upserts a fresh 'matching' row, clearing any previous score/persona/
// error from an earlier attempt (re-matching always starts clean, no
// history kept). Called only from the human-facing PUT /jobs/match
// handler (http.go), never by the AI.
func startJobMatch(jobId, profileId, conversationId string) error {
	if err := requireJobExists(jobId); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, conversation_id, error, updated_at)
		 VALUES (?, ?, NULL, NULL, 'matching', ?, NULL, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET
			profile_id = excluded.profile_id,
			persona_id = NULL,
			score = NULL,
			status = 'matching',
			conversation_id = excluded.conversation_id,
			error = NULL,
			updated_at = datetime('now')`,
		jobId, profileId, conversationId,
	)
	return err
}

// updateJobMatchStatus records the outcome of a match attempt observed
// client-side — status is always required (the two real callers are
// "the turn finished, clear the conversation id" [status: completed,
// but see below] and "the turn failed" [status: failed, errText set]).
// Deliberately never touches score/persona_id — those only ever come
// from saveJobMatchResult, below.
//
// A SUCCESSFUL turn does not call this with status='completed' at
// all: save_job_match (the AI's own tool call) already wrote
// status='completed' with the real score before the turn's own reply
// even finishes, so the frontend's own "turn completed" handler has
// nothing left to record — it only ever needs this function for the
// FAILURE path (a turn that errors, or reaches its own conversation-
// level failure, without the AI ever having called save_job_match at
// all).
func updateJobMatchStatus(jobId, status string, errText *string) error {
	if err := requireJobExists(jobId); err != nil {
		return err
	}
	_, err := jobsDB.Exec(
		`UPDATE job_matches SET status = ?, error = ?, conversation_id = NULL, updated_at = datetime('now') WHERE job_id = ?`,
		status, orNull(errText), jobId,
	)
	return err
}

// saveJobMatchResult is the ONLY function that ever writes a score —
// called exclusively by the save_job_match MCP tool (below), never by
// any human-facing HTTP path. A plain upsert (not requiring a prior
// startJobMatch row to already exist) so the AI's own call is
// self-sufficient even if, for whatever reason, no tracking row was
// created ahead of it.
var errInvalidMatchScore = errors.New("score must be an integer between 0 and 100")

func saveJobMatchResult(jobId, profileId, personaId string, score int) error {
	if err := requireJobExists(jobId); err != nil {
		return err
	}
	if score < 0 || score > 100 {
		return errInvalidMatchScore
	}
	_, err := jobsDB.Exec(
		`INSERT INTO job_matches (job_id, profile_id, persona_id, score, status, conversation_id, error, updated_at)
		 VALUES (?, ?, ?, ?, 'completed', NULL, NULL, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET
			profile_id = excluded.profile_id,
			persona_id = excluded.persona_id,
			score = excluded.score,
			status = 'completed',
			conversation_id = NULL,
			error = NULL,
			updated_at = datetime('now')`,
		jobId, profileId, personaId, score,
	)
	return err
}

// reconcileOrphanedJobMatches mirrors reconcileOrphanedCrawlRuns
// exactly, for the same reason: a hidden conversation's own turn is
// not a subprocess this Go process could ever reattach to after a
// restart, so any job_matches row still 'matching' from before this
// process started is definitely orphaned. Called once at boot,
// alongside reconcileOrphanedCrawlRuns. See
// plan/ai/tools/career/step-XX-job-match.md.
func reconcileOrphanedJobMatches() error {
	_, err := jobsDB.Exec(
		`UPDATE job_matches SET status = 'failed', error = 'Interrupted by a server restart', conversation_id = NULL, updated_at = datetime('now')
		 WHERE status = 'matching'`,
	)
	return err
}

// --- MCP registration ---

type saveJobMatchArgs struct {
	JobID     string `json:"jobId" jsonschema:"the job's own id, from get_job/list_jobs/search_jobs"`
	ProfileID string `json:"profileId" jsonschema:"the profile this match was requested against"`
	PersonaID string `json:"personaId" jsonschema:"the single persona under this profile you judged the best fit for this job, from list_personas"`
	Score     int    `json:"score" jsonschema:"how well this job matches that persona, as an integer 0-100 (100 = perfect match)"`
}

func registerSaveJobMatch(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_job_match",
		Description: "Record the result of assessing how well a job matches a career profile — call this ONCE, " +
			"after you've read the job's own description (get_job), inspected every persona under the given " +
			"profile (list_personas, then get_persona_details for each to see its own skills/experience/personal " +
			"details), and decided which single persona fits best and how well. Fails if jobId is unknown or " +
			"score isn't an integer 0-100.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveJobMatchArgs) (*mcp.CallToolResult, any, error) {
		if args.JobID == "" {
			return errResult("jobId is required"), nil, nil
		}
		if args.ProfileID == "" {
			return errResult("profileId is required"), nil, nil
		}
		if args.PersonaID == "" {
			return errResult("personaId is required"), nil, nil
		}
		if err := saveJobMatchResult(args.JobID, args.ProfileID, args.PersonaID, args.Score); err != nil {
			if errors.Is(err, errUnknownJob) {
				return errResult(fmt.Sprintf("unknown job id %q", args.JobID)), nil, nil
			}
			if errors.Is(err, errInvalidMatchScore) {
				return errResult(err.Error()), nil, nil
			}
			return errResult(fmt.Sprintf("failed to save job match: %v", err)), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "saved"}}}, nil, nil
	})
}

type getJobArgs struct {
	JobID string `json:"jobId" jsonschema:"the job's own id, from list_jobs/search_jobs"`
}

// registerGetJob adds get_job — a precise, single-job counterpart to
// list_jobs/search_jobs, needed by the job-matching workflow to
// re-check a job's own description after triggering a detail crawl
// mid-conversation (crawl_urls_with_subagents runs in the background;
// this is how the AI later confirms whether it actually produced a
// description). Reuses getJobByID (jobs.go) — the exact function the
// Jobs page's own Eye-icon details view already calls over HTTP.
func registerGetJob(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_job",
		Description: "Read one job posting by id — every field save_job/save_portal_job/list_jobs/search_jobs expose, in one precise call. Fails if jobId is unknown.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getJobArgs) (*mcp.CallToolResult, any, error) {
		if args.JobID == "" {
			return errResult("jobId is required"), nil, nil
		}
		j, err := getJobByID(args.JobID)
		if err != nil {
			if errors.Is(err, errUnknownJob) {
				return errResult(fmt.Sprintf("unknown job id %q", args.JobID)), nil, nil
			}
			return errResult(fmt.Sprintf("failed to load job: %v", err)), nil, nil
		}
		return jsonResult(map[string]any{"job": j})
	})
}
