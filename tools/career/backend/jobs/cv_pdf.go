// cv_pdf.go — "Generate CV PDF": lets an AI conversation build a CV
// tailored to one specific job posting (reading that job's own
// requirements plus a chosen profile's personas/skills/experience),
// render it as a PDF via pdf_tools' own generate_pdf MCP tool, and
// record the result. Mirrors job_match.go's own split into two
// persistence paths, deliberately never overlapping:
//   - StartCvGeneration/UpdateCvGenerationStatus (below) back the
//     human-facing PUT /jobs/cv endpoint (routeHandler.go) —
//     starts/clears the tracking row, records a client-observed
//     failure.
//   - SaveGeneratedCvPdf (below) backs the AI-facing save_cv_pdf MCP
//     tool — the ONLY way a completed PDF is ever recorded.
//
// pdf_tools' own generate_pdf does not upload to the platform's Media
// store itself — it only writes to its own local uploadsDir and hands
// back a resource link (a URI containing an "id" query parameter,
// e.g. "/api/v1/tools/pdf_tools/proxy/files?id=<uuid>"). Rather than
// have save_cv_pdf try to move those bytes into Media itself — it runs
// as a stdio MCP subprocess with no caller HTTP session to
// authenticate a Media upload with, and storing a live user token for
// later reuse would repeat exactly the capability-token anti-pattern
// cv_import.go's own history already moved away from — the CORE app's
// own conversation orchestrator (api/src/conversation/chat.go)
// promotes the pdfResource argument into a real, permanent Media file
// id in-process (its own trusted, same-process disk read + Media
// write, never an HTTP round trip or a token) BEFORE this tool call
// ever reaches this backend at all, via the manifest's own declarative
// promote_media_param mechanism (tools/career/manifest.json's
// save_cv_pdf entry). So by the time SaveGeneratedCvPdf runs, its own
// pdfResource argument already IS a permanent Media file id — this
// package never CREATES a Media row, over HTTP or otherwise, and the
// whole flow runs fully detached from any frontend tab.
//
// It does retitle that row, right before recording it
// (prepareCvPdfMediaTitle, below) — the one thing the CORE app's own
// promotion step can't do itself, since it has no access to this job's
// own title/portal data. That's a small, service-token-authenticated
// HTTP callback (career-tool-backend/media.SetTitle), not a departure
// from the reasoning above: it still never authenticates as any end
// user. Calling it BEFORE SaveGeneratedCvPdf, not after, also makes it
// the one reliable way this package can confirm pdfResource is a real,
// already-promoted row it owns — a bare UUID the AI echoed back
// without ever going through promotion looks identical, by shape
// alone, to a real one, and this check catches exactly that (a
// live-observed bug, see prepareCvPdfMediaTitle's own doc comment). See
// plan/ai/tools/career/step-XX-cv-pdf.md and
// plan/ai/media/step-10-obligatory-title.md.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"career-tool-backend/db"
	"career-tool-backend/domainevent"
	"career-tool-backend/media"
)

// StartCvGeneration begins tracking a new AI-driven CV generation
// attempt for jobId — upserts a fresh 'generating' row, clearing any
// previous conversation/media reference/error from an earlier attempt
// (re-generating always starts clean, no history kept, same
// convention job_matches itself already uses). Called only from the
// human-facing PUT /jobs/cv handler — never by the AI itself.
func StartCvGeneration(jobId, profileId, conversationId string) error {
	if err := RequireJobExists(jobId); err != nil {
		return err
	}
	_, err := db.JobsDB.Exec(
		`INSERT INTO job_cv_pdfs (job_id, profile_id, status, conversation_id, media_file_id, error, updated_at)
		 VALUES (?, ?, 'generating', ?, NULL, NULL, datetime('now'))
		 ON CONFLICT(job_id) DO UPDATE SET
			profile_id = excluded.profile_id,
			status = 'generating',
			conversation_id = excluded.conversation_id,
			media_file_id = NULL,
			error = NULL,
			updated_at = datetime('now')`,
		jobId, profileId, conversationId,
	)
	return err
}

// UpdateCvGenerationStatus records a client-observed failure — status
// is always 'failed' in practice (the human-facing endpoint has no
// other reason to call this; a successful render is only ever
// recorded by SaveGeneratedCvPdf below). Mirrors updateJobMatchStatus's
// own reasoning exactly: a turn that ends without the AI ever calling
// save_cv_pdf is a real failure, not a silently-stuck "generating"
// state.
func UpdateCvGenerationStatus(jobId, status string, errText *string) error {
	if err := RequireJobExists(jobId); err != nil {
		return err
	}
	_, err := db.JobsDB.Exec(
		`UPDATE job_cv_pdfs SET status = ?, error = ?, conversation_id = NULL, updated_at = datetime('now') WHERE job_id = ?`,
		status, orNull(errText), jobId,
	)
	return err
}

// errNoCvGenerationInProgress is returned by SaveGeneratedCvPdf when no
// job_cv_pdfs row exists yet for jobId — StartCvGeneration must always
// run first (from the human-facing PUT /jobs/cv handler) so profile_id
// is already recorded before the AI's own tool call arrives.
var errNoCvGenerationInProgress = errors.New("no CV generation in progress for this job")

// SaveGeneratedCvPdf is the ONLY function that ever records a
// completed CV — called exclusively by the save_cv_pdf MCP tool
// (below), never by any human-facing HTTP path. mediaFileId is already
// a real, permanent Media file id by the time this runs — see this
// file's own top doc comment for where that promotion happens.
func SaveGeneratedCvPdf(jobId, mediaFileId string) error {
	if err := RequireJobExists(jobId); err != nil {
		return err
	}
	res, err := db.JobsDB.Exec(
		`UPDATE job_cv_pdfs SET status = 'completed', media_file_id = ?, conversation_id = NULL, error = NULL, updated_at = datetime('now') WHERE job_id = ?`,
		mediaFileId, jobId,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errNoCvGenerationInProgress
	}
	return nil
}

// ReconcileOrphanedCvGenerations mirrors ReconcileOrphanedJobMatches
// exactly, for the same reason: a hidden conversation's own turn is
// not a subprocess this Go process could ever reattach to after a
// restart, so any job_cv_pdfs row still 'generating' from before this
// process started is definitely orphaned. Called once at boot,
// alongside ReconcileOrphanedJobMatches. See
// plan/ai/tools/career/step-XX-cv-pdf.md.
func ReconcileOrphanedCvGenerations() error {
	_, err := db.JobsDB.Exec(
		`UPDATE job_cv_pdfs SET status = 'failed', error = 'Interrupted by a server restart', conversation_id = NULL, updated_at = datetime('now')
		 WHERE status = 'generating'`,
	)
	return err
}

// publishCvGeneratedEvent is the Domain Events plan's own step 6
// dogfood integration
// (plan/ai/domain-events/step-06-end-to-end-dogfood.md) — the smallest
// real proof that a TOOL-originated event reaches a core-native
// listener, complementing step 1's own core-originated dogfood
// (cinqo.tool.installed). Publishing this event is not part of
// save_cv_pdf's own contract — the CV is already fully recorded by
// SaveGeneratedCvPdf by the time this runs — so any failure here is
// logged and otherwise ignored, never turning an already-successful
// save_cv_pdf call into a reported failure.
//
// Runs synchronously, not as a fire-and-forget goroutine: this MCP
// tool call executes inside a one-shot "--mcp" subprocess
// (tool_mcp.Invoke's own "one spawn per call" model) that exits
// shortly after this handler returns, so a detached goroutine here
// could easily never get to finish its own HTTP call. domainevent.Publish's
// own short internal timeout is what keeps an unreachable core from
// hanging save_cv_pdf's own response for long. See
// plan/ai/tools/career/step-66-career-event-publishers.md for the
// shared domainevent.Publish helper this now delegates to (originally
// a standalone implementation here, factored out once step 66 needed
// the exact same mechanism from two more call sites).
func publishCvGeneratedEvent(jobId, mediaFileId string) {
	domainevent.Publish("career.cv.generated", map[string]string{"job_id": jobId, "media_file_id": mediaFileId})
}

// prepareCvPdfMediaTitle computes "{job_title}_{portal_name}.pdf"
// (just "{job_title}.pdf" when the job has no portal — GetJobByID's
// own LEFT JOIN leaves PortalName empty in that case) and sets it on
// mediaFileId via media.SetTitle — called BEFORE SaveGeneratedCvPdf,
// not after: this doubles as the only way this package can verify
// mediaFileId is a real, already-promoted Media row it owns, since it
// has no direct DB access to check that itself (see this file's own
// top doc comment). A definitive media.ErrMediaNotOwned is passed
// straight back to the caller, which must reject the whole save_cv_pdf
// call rather than ever recording a dead reference on the job — this
// is precisely the live-observed bug this function was added to catch
// (the AI passing generate_pdf's own bare resource id instead of its
// full 'uri' field, which never gets promoted by
// api/src/conversation/chat.go's own resolvePromoteMediaArgument, so
// it reaches this backend looking like a syntactically valid but
// entirely unpromoted UUID — indistinguishable from a real one by
// shape alone). GetJobByID failing here (jobId itself unknown) is NOT
// surfaced — RegisterSaveCvPdf's own subsequent SaveGeneratedCvPdf call
// already produces the correct "unknown job" error for that case, so
// it isn't duplicated; there is also no title to compute without a
// real job. See plan/ai/media/step-10-obligatory-title.md.
func prepareCvPdfMediaTitle(jobId, mediaFileId string) error {
	j, err := GetJobByID(jobId)
	if err != nil {
		return nil
	}
	title := j.Title + ".pdf"
	if j.PortalName != "" {
		title = fmt.Sprintf("%s_%s.pdf", j.Title, j.PortalName)
	}
	return media.SetTitle(mediaFileId, title)
}

// --- MCP registration ---

type saveCvPdfArgs struct {
	JobID string `json:"jobId" jsonschema:"the job's own id, from get_job/list_jobs/search_jobs"`
	// PdfResource's own JSON key (pdfResource) is what the manifest's
	// own promote_media_param points at — the AI still passes
	// generate_pdf's own returned resource URI here, unmodified and
	// unaware of anything else; the platform's own conversation
	// orchestrator has already replaced it with a real Media file id
	// by the time this Go code ever sees it. See this file's own top
	// doc comment.
	PdfResource string `json:"pdfResource" jsonschema:"the exact 'uri' field from generate_pdf's own returned resource link, unmodified"`
}

// RegisterSaveCvPdf adds save_cv_pdf — call this exactly once, right
// after generate_pdf, to record the CV you just rendered for this job.
func RegisterSaveCvPdf(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "save_cv_pdf",
		Description: "Record a CV PDF you just rendered for a job via generate_pdf — call this ONCE, immediately " +
			"after generate_pdf returns its resource link, passing that link's own 'uri' field unmodified. Fails if " +
			"jobId is unknown or no CV generation is currently in progress for it (the human-facing UI always starts " +
			"one before sending you this request).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args saveCvPdfArgs) (*mcp.CallToolResult, any, error) {
		if args.JobID == "" {
			return db.ErrResult("jobId is required"), nil, nil
		}
		if args.PdfResource == "" {
			return db.ErrResult("pdfResource is required"), nil, nil
		}
		// Safety net, not the primary fix (that lives in the platform's
		// own resolvePromoteMediaArgument, api/src/conversation/chat.go —
		// see its own doc comment): by the time this handler runs,
		// pdfResource should already have been promoted from a
		// generate_pdf resource link into a real, permanent Media file id
		// (a plain UUID, no "/" or "://" anywhere in it — see
		// media.PersistLocalFile). A value that still looks like a URL or
		// path means promotion did not happen, for whatever reason — this
		// package has no way to fix that itself (it doesn't talk to
		// Media, see this file's own top doc comment), but persisting
		// that raw value anyway would silently record a job's own CV as
		// "completed" while its own download link 404s forever, exactly
		// what was live-observed before this check existed. Rejecting
		// here instead gives the AI an immediate, actionable error it can
		// often just retry from (generate_pdf again, then pass that
		// fresh, unmodified result straight into this call) within the
		// SAME turn, rather than a human discovering a dead download link
		// days later. See
		// plan/ai/tools/career/step-XX-cv-pdf-download-404.md.
		if strings.Contains(args.PdfResource, "://") || strings.HasPrefix(args.PdfResource, "/") {
			return db.ErrResult(
				"pdfResource still looks like a raw resource link (" + args.PdfResource + "), not a promoted Media file id — " +
					"this usually means it was not passed through unmodified. Call generate_pdf again and pass its own " +
					"returned 'uri' field straight into this call's pdfResource argument, exactly as returned — do not " +
					"add a domain/host in front of it or otherwise retype it.",
			), nil, nil
		}
		// Verifies pdfResource is a real, already-promoted Media row this
		// tool owns BEFORE ever recording it on the job — see
		// prepareCvPdfMediaTitle's own doc comment for the exact bug this
		// catches (a syntactically-valid-looking but never-promoted UUID,
		// which the check above can't distinguish from a real one).
		if err := prepareCvPdfMediaTitle(args.JobID, args.PdfResource); err != nil {
			if errors.Is(err, media.ErrMediaNotOwned) {
				return db.ErrResult(
					"pdfResource (" + args.PdfResource + ") does not correspond to a real, already-promoted Media file " +
						"owned by this tool — this usually means only generate_pdf's own resource id was extracted and " +
						"passed here, instead of its FULL 'uri' field. Call generate_pdf again and pass its own returned " +
						"'uri' field straight into this call's pdfResource argument, exactly as returned, with nothing " +
						"added, removed, or retyped.",
				), nil, nil
			}
		}
		if err := SaveGeneratedCvPdf(args.JobID, args.PdfResource); err != nil {
			if errors.Is(err, ErrUnknownJob) {
				return db.ErrResult(fmt.Sprintf("unknown job id %q", args.JobID)), nil, nil
			}
			if errors.Is(err, errNoCvGenerationInProgress) {
				return db.ErrResult("no CV generation is currently in progress for this job"), nil, nil
			}
			return db.ErrResult(fmt.Sprintf("failed to save CV pdf: %v", err)), nil, nil
		}
		publishCvGeneratedEvent(args.JobID, args.PdfResource)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "saved"}}}, nil, nil
	})
}
