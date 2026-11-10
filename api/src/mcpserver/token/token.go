// Package token makes and recognises MCP access tokens: "cmcp_" plus 32 random bytes in URL-safe base64. The value is high-entropy, so a plain SHA-256
// is enough to store it (there is nothing to brute-force); the plain value exists only in the answer to the request that made it.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

const (
	// Prefix starts every token, so a secret scanner and a person can tell what it is.
	Prefix = "cmcp_"
	// randomBytes is the entropy of a token.
	randomBytes = 32
	// visibleChars is how much of a token the list shows (the Prefix and the first characters of the random part).
	visibleChars = len(Prefix) + 7
)

// New makes a token. plain is shown to the person once; hash and visible are what is stored.
func New() (plain, hash, visible string, err error) {
	b := make([]byte, randomBytes)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	plain = Prefix + base64.RawURLEncoding.EncodeToString(b)
	return plain, Hash(plain), Visible(plain), nil
}

// Hash is the stored form of a token: lower-case hex of its SHA-256.
func Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// Visible is the part of a token the list may show.
func Visible(plain string) string {
	if len(plain) <= visibleChars {
		return plain
	}
	return plain[:visibleChars] + "…"
}

// LooksLikeToken reports whether s has the shape of a token (the prefix and a full-length random part), so a request that cannot be one is refused before the
// database is asked.
func LooksLikeToken(s string) bool {
	if !strings.HasPrefix(s, Prefix) {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(s[len(Prefix):])
	return err == nil && len(raw) == randomBytes
}

// Equal compares two hashes in constant time.
func Equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
