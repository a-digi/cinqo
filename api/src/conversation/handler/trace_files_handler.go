// trace_files_handler.go lists one conversation's AI trace files for an administrator: which turns have a
// file, how big, how recent. The per-conversation GetLogsHandler does the same for the conversation's
// owner only; this one is what lets the admin AI Logs page open a file in the file reader
// (plan/ai/file-reader/file-reader-engine.md, step 2). It lists names and sizes; it never opens a file.
package handler

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
)

// idShape is the exact shape of a conversation id and a turn run id: a lowercase UUID.
var idShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// listTraceFiles returns the trace files of one conversation, newest first. A conversation with no folder
// (nothing was ever logged for it) has an empty list.
func listTraceFiles(conversationsDir, conversationID string) ([]traceLogSummaryResponse, error) {
	entries, err := os.ReadDir(conversation.TraceLogDir(conversationsDir, conversationID))
	if err != nil {
		if os.IsNotExist(err) {
			return []traceLogSummaryResponse{}, nil
		}
		return nil, err
	}
	logs := make([]traceLogSummaryResponse, 0, len(entries))
	for _, e := range entries {
		name, isTxt := strings.CutSuffix(e.Name(), ".txt")
		if !isTxt || !e.Type().IsRegular() || !idShape.MatchString(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		logs = append(logs, traceLogSummaryResponse{
			TurnRunID:  name,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ModifiedAt > logs[j].ModifiedAt })
	return logs, nil
}

// GetTraceFilesHandler handles GET /api/v1/conversations/logs/{conversationId} — admin-only
// (cinqo:super:admin, declared in the route), like the overview it follows from.
func GetTraceFilesHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("conversationId")
	if !idShape.MatchString(id) {
		response.ErrorResponse(w, http.StatusBadRequest, "conversationId is not a valid id")
		return
	}
	logs, err := listTraceFiles(logsRoot(reqCtx), id)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list trace logs")
		return
	}
	response.SuccessResponse(w, http.StatusOK, map[string]any{"conversationId": id, "logs": logs})
}
