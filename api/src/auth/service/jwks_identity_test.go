package auth_service

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func hs256Token(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestValidateIdentityAndDisplayName(t *testing.T) {
	svc := NewJwksService("", "secret", "", "")
	exp := time.Now().Add(time.Hour).Unix()

	for name, tc := range map[string]struct {
		claims jwt.MapClaims
		want   string
	}{
		"name wins":               {jwt.MapClaims{"sub": "u1", "exp": exp, "name": "Ada Lovelace", "preferred_username": "ada", "email": "ada@example.com"}, "Ada Lovelace"},
		"then preferred_username": {jwt.MapClaims{"sub": "u1", "exp": exp, "preferred_username": "ada", "email": "ada@example.com"}, "ada"},
		"then email":              {jwt.MapClaims{"sub": "u1", "exp": exp, "email": "ada@example.com"}, "ada@example.com"},
		"a blank name is skipped": {jwt.MapClaims{"sub": "u1", "exp": exp, "name": "  ", "email": "ada@example.com"}, "ada@example.com"},
		"nothing readable":        {jwt.MapClaims{"sub": "u1", "exp": exp}, ""},
		"a non-string name":       {jwt.MapClaims{"sub": "u1", "exp": exp, "name": 42}, ""},
	} {
		tc.claims["scope"] = "a b"
		id, err := svc.ValidateIdentity(hs256Token(t, "secret", tc.claims))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if id.Name != tc.want || id.Subject != "u1" || len(id.Scopes) != 2 || id.Expires.Unix() != exp {
			t.Errorf("%s: identity = %+v, want name %q", name, id, tc.want)
		}
	}

	// Validate still returns the same first three values.
	sub, scopes, expires, err := svc.Validate(hs256Token(t, "secret", jwt.MapClaims{"sub": "u9", "exp": exp, "scope": "x"}))
	if err != nil || sub != "u9" || len(scopes) != 1 || expires.Unix() != exp {
		t.Errorf("Validate = %s %v %v %v", sub, scopes, expires, err)
	}

	// A bad signature and a missing subject are still refused.
	if _, err := svc.ValidateIdentity(hs256Token(t, "other-secret", jwt.MapClaims{"sub": "u1", "exp": exp})); err == nil {
		t.Error("a token signed with another secret was accepted")
	}
	if _, err := svc.ValidateIdentity(hs256Token(t, "secret", jwt.MapClaims{"exp": exp})); err == nil {
		t.Error("a token without a subject was accepted")
	}
	if _, err := svc.ValidateIdentity(""); err == nil {
		t.Error("an empty token was accepted")
	}
}
