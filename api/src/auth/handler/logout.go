package auth_handler

import (
	"net/http"

	"github.com/a-digi/coco-server/server/request"
)

type LogoutHandler struct{}

func (h *LogoutHandler) ServeHTTP(reqCtx request.RequestContext) {
	w := reqCtx.GetWriter()
	r := reqCtx.GetRequest()
	secure := r.TLS != nil

	// Clears all three cookies callback.go can set.
	http.SetCookie(w, &http.Cookie{
		Name:     "access_token",
		Value:    "",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/api/v1/auth/renew",
		MaxAge:   -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "user_info",
		Value:    "",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
		MaxAge:   -1,
	})

	reqCtx.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
