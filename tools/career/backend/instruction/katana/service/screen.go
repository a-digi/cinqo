package service

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

// callScreen runs one of the screen's own routes in process, as the person: the request carries the person's cached session as the cookie the
// route reads, so every check the screen has (link exists, no login link, own-site page, the Browser calls, the structured-data support, the
// validation before a save) is the one the AI gets, not a copy. It returns the status and the body.
func callScreen(h http.HandlerFunc, token string, body any) (int, []byte) {
	raw, err := json.Marshal(body)
	if err != nil {
		return http.StatusInternalServerError, []byte(err.Error())
	}
	req := httptest.NewRequest(http.MethodPost, "/screen", bytes.NewReader(raw))
	req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// screenProblem turns a screen route's refusal into what the AI is told.
func screenProblem(status int, body []byte) *problem {
	var j struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Advice  string `json:"advice"`
	}
	msg, code := string(bytes.TrimSpace(body)), "screen_refused"
	if json.Unmarshal(body, &j) == nil && j.Message != "" {
		msg, code = j.Message, j.Code
		if j.Advice != "" {
			msg += " (" + j.Advice + ")"
		}
	}
	if status == http.StatusBadGateway {
		msg += ". STOP and tell the person; do not guess"
	}
	return refuse(status, code, "%s", clip(msg, 400))
}
