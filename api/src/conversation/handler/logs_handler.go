// logs_handler.go serves the AI trace logs conversation.trace_log.go
// writes, one per turn — a diagnostic view for understanding excessive
// token usage. Read-only, and deliberately separate from
// GetActiveTurnHandler/GetHandler (turn_handler.go/get_handler.go):
// those serve conversation content/status, this serves raw
// request/response transcripts. See
// plan/ai/conversation/step-36-ai-trace-logs.md.
package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/a-digi/coco-server/server/request"
	"github.com/a-digi/coco-server/server/response"

	"github.com/a-digi/cinqo/src/conversation"
	conversation_query "github.com/a-digi/cinqo/src/conversation/repository/query"
)

// traceLogSummaryResponse is one turn's own trace file, listed without
// ever reading its content — a directory listing's own ModTime/Size
// are enough for a list view, mirroring
// tools/browser/backend/crawl_log.go's own "list is cheap, detail
// reads the file" split.
type traceLogSummaryResponse struct {
	TurnRunID  string `json:"turnRunId"`
	SizeBytes  int64  `json:"sizeBytes"`
	ModifiedAt string `json:"modifiedAt"`
}

// GetLogsHandler handles GET /api/v1/conversations/{id}/logs — the
// caller's own conversation only (wrong owner and nonexistent both
// read as the same 404, matching every other handler in this
// feature). With no ?turnId= query param, lists every turn that has a
// trace file, newest first, reading only directory metadata. With
// ?turnId=<id>, streams that one turn's own raw .txt file via
// http.ServeContent (chunked/Range-capable for free, no manual
// pagination) — 404 if that specific file doesn't exist, distinct from
// "conversation not found."
func GetLogsHandler(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	id := reqCtx.GetURI().GetPathVariable("id")
	if id == "" {
		response.ErrorResponse(w, http.StatusBadRequest, "id is required")
		return
	}

	userID, err := callerUserID(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	db, err := conversationDB(reqCtx)
	if err != nil {
		response.ErrorResponse(w, http.StatusInternalServerError, "conversation database not configured")
		return
	}

	if _, err := conversation_query.NewConversationQueryRepo(db).FindOwnedByID(id, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			response.ErrorResponse(w, http.StatusNotFound, "conversation not found")
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to look up conversation")
		return
	}

	traceDir := conversation.TraceLogDir(logsRoot(reqCtx), id)

	if turnID := reqCtx.GetRequest().URL.Query().Get("turnId"); turnID != "" {
		path := conversation.TraceLogPath(logsRoot(reqCtx), id, turnID)
		f, err := os.Open(path)
		if err != nil {
			response.ErrorResponse(w, http.StatusNotFound, "trace log not found")
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			response.ErrorResponse(w, http.StatusInternalServerError, "failed to read trace log")
			return
		}
		http.ServeContent(w, reqCtx.GetRequest(), info.Name(), info.ModTime(), f)
		return
	}

	entries, err := os.ReadDir(traceDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No turn for this conversation has ever produced a trace
			// file yet — an empty list, not an error.
			response.SuccessResponse(w, http.StatusOK, map[string]any{"logs": []traceLogSummaryResponse{}})
			return
		}
		response.ErrorResponse(w, http.StatusInternalServerError, "failed to list trace logs")
		return
	}

	logs := make([]traceLogSummaryResponse, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		turnRunID := e.Name()
		turnRunID = turnRunID[:len(turnRunID)-len(filepath.Ext(turnRunID))]
		logs = append(logs, traceLogSummaryResponse{
			TurnRunID:  turnRunID,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].ModifiedAt > logs[j].ModifiedAt })

	response.SuccessResponse(w, http.StatusOK, map[string]any{"logs": logs})
}
