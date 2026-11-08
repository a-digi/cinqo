// jobs_question.go (step 91) is the human-in-the-loop half of "the crawler
// found 0 items": the generation agent asks, through
// ask_user_if_page_has_jobs, whether the page has jobs at all, and the
// user answers yes or no from the link itself. The agent can only ask —
// the answer arrives through PUT /portal-links (AnswerJobsQuestion) — so
// it can never answer for the user. See
// plan/ai/tools/career/step-91-ask-user-when-listing-is-empty.md.
package portal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
)

// maxJobsQuestionNoteLen caps the agent's one-line observation. The note
// can echo page content, so it is bounded and only ever shown as text.
const maxJobsQuestionNoteLen = 300

var (
	ErrNoPendingJobsQuestion     = errors.New("this link has no pending question")
	ErrJobsQuestionAlreadyAsked  = errors.New("a question is already pending for this link")
	ErrJobsQuestionNoteRequired  = errors.New("note is required")
	ErrInvalidJobsQuestionAnswer = errors.New(`jobsQuestionAnswer must be "yes" or "no"`)
)

// JobsQuestion is a link's pending "does this page list jobs?" question.
type JobsQuestion struct {
	AskedAt string `json:"askedAt"`
	Note    string `json:"note"`
}

// AskJobsQuestion records the pending question. One per link: a second ask
// while one is pending is rejected, so a looping agent cannot re-prompt.
func AskJobsQuestion(linkID, note string) error {
	if err := RequirePortalLinkExists(linkID); err != nil {
		return err
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return ErrJobsQuestionNoteRequired
	}
	if utf8.RuneCountInString(note) > maxJobsQuestionNoteLen {
		note = string([]rune(note)[:maxJobsQuestionNoteLen])
	}
	res, err := db.JobsDB.Exec(
		`UPDATE portal_links SET jobs_question_asked_at = ?, jobs_question_note = ? WHERE id = ? AND jobs_question_asked_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), note, linkID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrJobsQuestionAlreadyAsked
	}
	return nil
}

// AnswerJobsQuestion settles the pending question. Both answers clear it;
// "no" (the page has no jobs) also marks the zero-job streak as expected.
// "yes" stores nothing further and leaves the running attempt open: the
// caller tells the agent in the conversation that its approach was wrong,
// and the same attempt ends when that turn does.
func AnswerJobsQuestion(linkID, answer string) error {
	if answer != "yes" && answer != "no" {
		return ErrInvalidJobsQuestionAnswer
	}
	if err := RequirePortalLinkExists(linkID); err != nil {
		return err
	}
	res, err := db.JobsDB.Exec(
		`UPDATE portal_links SET jobs_question_asked_at = NULL, jobs_question_note = NULL WHERE id = ? AND jobs_question_asked_at IS NOT NULL`,
		linkID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoPendingJobsQuestion
	}
	if answer == "no" {
		// The agent stopped to ask, so its listing attempt is still
		// 'running'. The page simply has no jobs — that is neither a
		// success nor a failed attempt, so it is dropped instead of
		// counting toward step 87's "probably can't be crawled".
		if _, err := db.JobsDB.Exec(
			`DELETE FROM instruction_attempts WHERE portal_link_id = ? AND kind = ? AND status = 'running'`,
			linkID, InstructionKindListing,
		); err != nil {
			return err
		}
		return SetEmptyCrawlAcknowledged(linkID, true)
	}
	return nil
}

type askUserIfPageHasJobsArgs struct {
	PortalLinkID string `json:"portalLinkId" jsonschema:"the portal link's own id"`
	Note         string `json:"note" jsonschema:"one short line saying what you tried and found, e.g. '2 attempts, container .job-result matched nothing' (max 300 characters)"`
}

func RegisterAskUserIfPageHasJobs(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "ask_user_if_page_has_jobs",
		Description: "Ask the user, with a yes/no question, whether this portal link's page " +
			"actually lists any jobs. Call this ONLY when your test crawl of the draft " +
			"listing instructions still extracted 0 items after you already made one fix " +
			"attempt (check_crawl_extraction_result itemsFound == 0 twice) — then you " +
			"cannot tell a page that really has no jobs from selectors that are wrong. " +
			"Ask at most once per task. The question is shown to the user on the link; " +
			"you cannot see or give the answer. After a successful call, do NOT save any " +
			"instructions and make no more tool calls: end your turn immediately with one " +
			"short line. The user's answer arrives as a new message in this conversation.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args askUserIfPageHasJobsArgs) (*mcp.CallToolResult, any, error) {
		if err := AskJobsQuestion(args.PortalLinkID, args.Note); err != nil {
			if errors.Is(err, ErrUnknownPortalLink) {
				return db.ErrResult(fmt.Sprintf("unknown portal link id %q", args.PortalLinkID)), nil, nil
			}
			return db.ErrResult(err.Error()), nil, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Question recorded. End your turn now with one short line; the user's answer will arrive as a new message."}}}, nil, nil
	})
}
