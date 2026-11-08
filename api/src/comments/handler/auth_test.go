package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func cookieRequest(t *testing.T, payload any) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	if payload != nil {
		raw, _ := json.Marshal(payload)
		r.AddCookie(&http.Cookie{Name: "user_info", Value: base64.RawURLEncoding.EncodeToString(raw)})
	}
	return r
}

func TestNameFromUserInfoCookie(t *testing.T) {
	type info = map[string]string
	for name, tc := range map[string]struct {
		cookie any
		want   string
	}{
		"name wins":               {info{"user_id": "u1", "name": "Ada Lovelace", "preferred_username": "ada", "email": "a@x"}, "Ada Lovelace"},
		"then preferred username": {info{"user_id": "u1", "preferred_username": "ada", "email": "a@x"}, "ada"},
		"then email":              {info{"user_id": "u1", "email": "a@x"}, "a@x"},
		"blank name is skipped":   {info{"user_id": "u1", "name": "  ", "email": "a@x"}, "a@x"},
		"no user id is accepted":  {info{"name": "Ada"}, "Ada"},
		"another user's cookie":   {info{"user_id": "someone-else", "name": "Mallory"}, ""},
		"nothing readable":        {info{"user_id": "u1"}, ""},
		"no cookie":               {nil, ""},
	} {
		if got := nameFromUserInfoCookie(cookieRequest(t, tc.cookie), "u1"); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}

	bad := httptest.NewRequest(http.MethodGet, "/x", nil)
	bad.AddCookie(&http.Cookie{Name: "user_info", Value: "!!not base64!!"})
	if got := nameFromUserInfoCookie(bad, "u1"); got != "" {
		t.Errorf("a malformed cookie gave %q", got)
	}
	notJSON := httptest.NewRequest(http.MethodGet, "/x", nil)
	notJSON.AddCookie(&http.Cookie{Name: "user_info", Value: base64.RawURLEncoding.EncodeToString([]byte("nope"))})
	if got := nameFromUserInfoCookie(notJSON, "u1"); got != "" {
		t.Errorf("a non-JSON cookie gave %q", got)
	}
}
