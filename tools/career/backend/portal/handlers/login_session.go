package portalhandlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"time"

	"career-tool-backend/db"
	"career-tool-backend/portal"
)

// login_session.go (plan 195, step 2): which login session the Browser tool keeps for a link. This tool stores only the opaque id of that
// session on the link (portal_links.login_session_id); the cookies live sealed in the Browser tool and never pass through here. The page
// reads the session's state from the Browser tool itself.
//
//	GET    /portal-links/login-session?linkId=            -> {"sessionId": "..."}  ("" when none)
//	PUT    /portal-links/login-session {linkId, sessionId} -> attach (replaces, and the Browser forgets the replaced one)
//	DELETE /portal-links/login-session?linkId=            -> forget: clears the id and has the Browser forget the session
//
// Removing a link, or marking it as no longer needing a login, forgets its session the same way.

var forgetHTTPClient = &http.Client{Timeout: 15 * time.Second}

// forgetBrowserSession asks the Browser tool, with the person's own session, to forget a saved login session. Best effort: when the Browser
// is down or the call fails, the Browser deletes the row on its own at the end of the session's lifetime (30 days at most), so a failure is
// not an error for the caller.
func forgetBrowserSession(r *http.Request, sessionID string) {
	if sessionID == "" {
		return
	}
	coreURL := os.Getenv("CORE_API_URL")
	cookie, err := r.Cookie("access_token")
	if coreURL == "" || err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, coreURL+"/api/v1/tools/browser/proxy/login-session?id="+url.QueryEscape(sessionID), nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+cookie.Value)
	resp, err := forgetHTTPClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// forgetLinkLoginSession clears a link's session id and has the Browser forget the session. Used when the link is removed or stops
// needing a login. Errors are swallowed: the link's own change must not fail because a session could not be forgotten.
func forgetLinkLoginSession(r *http.Request, linkID string) {
	prev, err := portal.GetLinkLoginSession(linkID)
	if err != nil || prev == "" {
		return
	}
	if _, err := portal.SetLinkLoginSession(linkID, ""); err != nil {
		return
	}
	forgetBrowserSession(r, prev)
}

type loginSessionBody struct {
	LinkID    string `json:"linkId"`
	SessionID string `json:"sessionId"`
}

// PortalLinkLoginSessionHandler handles /portal-links/login-session.
func PortalLinkLoginSessionHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		linkID := r.URL.Query().Get("linkId")
		if linkID == "" {
			http.Error(w, "linkId query parameter is required", http.StatusBadRequest)
			return
		}
		id, err := portal.GetLinkLoginSession(linkID)
		if err != nil {
			writeLoginSessionError(w, err)
			return
		}
		db.WriteJSON(w, map[string]string{"sessionId": id})

	case http.MethodPut:
		var body loginSessionBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil || body.LinkID == "" || body.SessionID == "" {
			http.Error(w, "linkId and sessionId are required", http.StatusBadRequest)
			return
		}
		prev, err := portal.SetLinkLoginSession(body.LinkID, body.SessionID)
		if err != nil {
			writeLoginSessionError(w, err)
			return
		}
		if prev != "" && prev != body.SessionID {
			forgetBrowserSession(r, prev) // the older login of this link is replaced, not kept next to the new one
		}
		db.WriteJSON(w, map[string]string{"sessionId": body.SessionID})

	case http.MethodDelete:
		linkID := r.URL.Query().Get("linkId")
		if linkID == "" {
			http.Error(w, "linkId query parameter is required", http.StatusBadRequest)
			return
		}
		prev, err := portal.SetLinkLoginSession(linkID, "")
		if err != nil {
			writeLoginSessionError(w, err)
			return
		}
		forgetBrowserSession(r, prev)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeLoginSessionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, portal.ErrUnknownPortalLink):
		http.Error(w, "unknown portal link", http.StatusNotFound)
	case errors.Is(err, portal.ErrLoginSessionNeedsLoginLink):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}
