package crawler

import (
	"net/http"
	"net/url"
	"strings"

	"browser-tool-backend/shared"
)

// ai_context.go (plan 183, step 1) lets an AI tool call work in the login window a PERSON opened for that AI conversation, instead of on the
// headless tab (where a page behind a login is the logged-out one). The id of the conversation is NOT something the model supplies: core puts it
// in the environment of the tool process (TOOL_CONVERSATION_ID) and shared.CallSibling copies it into the request as "aiContextId", always
// overwriting whatever the arguments held. A window is used only when a person opened it for that exact id with aiMayUse; the AI cannot open,
// confirm, move or end a window. No such window = nothing changes (the call runs on the headless tab as always).

// codeLoginWindowOtherSite is the answer when the conversation's window belongs to another site than the page the AI asked for.
const codeLoginWindowOtherSite = "login_window_other_site"

// applyAIContext decides where one AI call runs. When aiContextID names a window opened for the AI (aiMayUse), it sets requestID and
// useLoginWindow so the call takes the same path a crawl run's window call takes (confirmed window only, the login form seen again answers
// login_required) and reports viaAI true. rawURL is the page the call asks for ("" when it works on the page already loaded in the window):
// another host than the window's is refused (*crawlError, HTTP 409). A call that is not an AI call (no aiContextId), already names a window or a
// run (useLoginWindow, requestId: a crawl run's own call), or has no window for the id is returned unchanged with viaAI false.
func applyAIContext(aiContextID, rawURL string, requestID *string, useLoginWindow *bool) (viaAI bool, refusal *crawlError) {
	if aiContextID == "" || *useLoginWindow || *requestID != "" || shared.LoginWindows == nil {
		return false, nil
	}
	info := shared.LoginWindows.AIWindow(aiContextID)
	if !info.Exists || !info.AIMayUse {
		return false, nil
	}
	if strings.TrimSpace(rawURL) != "" {
		u, err := url.Parse(strings.TrimSpace(rawURL))
		if err != nil || !strings.EqualFold(u.Hostname(), info.Domain) {
			return false, &crawlError{
				Code:    codeLoginWindowOtherSite,
				Message: "The login window of this conversation is for " + info.Domain + " only: this page is on another site",
				Reason:  info.Domain,
			}
		}
	}
	*requestID = aiContextID
	*useLoginWindow = true
	return true, nil
}

// writeAIContextRefusal answers a refusal of applyAIContext: HTTP 409 with the error's JSON, like login_required.
func writeAIContextRefusal(w http.ResponseWriter, refusal *crawlError) {
	writeCrawlErrorJSON(w, http.StatusConflict, refusal)
}
