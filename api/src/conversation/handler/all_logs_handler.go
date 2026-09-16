// all_logs_handler.go serves an admin-wide overview of where every
// conversation's own AI trace logs live — one row per conversation
// that has ever produced at least one, showing its folder and how many
// files exist, without ever reading a file's own content. Distinct
// from logs_handler.go's GetLogsHandler, which is scoped to the
// caller's own single conversation. See
// plan/ai/conversation/step-38-ai-logs-overview-page.md.
package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

func unixToRFC3339(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).UTC().Format(time.RFC3339)
}

type allLogsEntryResponse struct {
	ConversationID string `json:"conversationId"`
	// Title is best-effort — "" when the conversation was since deleted
	// but its own trace folder wasn't (deleting a conversation never
	// touches its trace logs; a deliberate, separate concern).
	Title      string `json:"title,omitempty"`
	Folder     string `json:"folder"`
	LogCount   int    `json:"logCount"`
	ModifiedAt string `json:"modifiedAt"`
}

// GetAllLogsHandler handles GET /api/v1/conversations/logs —
// admin-only (cinqo:super:admin), unlike every per-conversation
// handler in this package: this lists every conversation's own trace
// folder, not just the caller's own, which is exactly the point (an
// admin diagnosing token usage needs to find which conversation's logs
// to look at without already knowing its ID). Reads only directory
// metadata — never a log file's own content.
func GetAllLogsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()

	db, err := conversationDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation database not configured")
		return
	}

	convQuery := conversation_query.NewConversationQueryRepo(db)
	out, err := listAllTraceLogs(logsRoot(reqCtx), func(conversationID string) string {
		conv, err := convQuery.FindByID(conversationID)
		if err != nil {
			return ""
		}
		return conv.Title
	})
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list conversation trace logs")
		return
	}

	response.SuccessResponse(w, http.StatusOK, map[string]any{"conversations": out})
}

// listAllTraceLogs walks conversationsDir for every subdirectory that
// has produced at least one trace file, returning one summary per
// conversation, newest first — pure directory-metadata I/O plus one
// title lookup per conversation, no HTTP/DI dependency, so it's
// directly testable against a real temp directory. lookupTitle is
// injected rather than a hardcoded query-repo call for exactly that
// reason.
func listAllTraceLogs(conversationsDir string, lookupTitle func(conversationID string) string) ([]allLogsEntryResponse, error) {
	entries, err := os.ReadDir(conversationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []allLogsEntryResponse{}, nil
		}
		return nil, err
	}

	out := make([]allLogsEntryResponse, 0, len(entries))
	for _, e := range entries {
		// Only real per-conversation trace folders — the existing flat
		// <id>_conversation.md files sit in this same directory and
		// are never directories, so IsDir() alone already excludes
		// them; no name-suffix check needed.
		if !e.IsDir() {
			continue
		}
		conversationID := e.Name()
		traceDir := conversation.TraceLogDir(conversationsDir, conversationID)

		logEntries, err := os.ReadDir(traceDir)
		if err != nil {
			continue
		}
		var latest int64
		count := 0
		for _, le := range logEntries {
			if le.IsDir() {
				continue
			}
			count++
			if info, err := le.Info(); err == nil {
				if mt := info.ModTime().Unix(); mt > latest {
					latest = mt
				}
			}
		}
		if count == 0 {
			continue
		}

		absFolder, err := filepath.Abs(traceDir)
		if err != nil {
			absFolder = traceDir
		}

		out = append(out, allLogsEntryResponse{
			ConversationID: conversationID,
			Title:          lookupTitle(conversationID),
			Folder:         absFolder,
			LogCount:       count,
			ModifiedAt:     unixToRFC3339(latest),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModifiedAt > out[j].ModifiedAt })
	return out, nil
}
