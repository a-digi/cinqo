// crypto.go encrypts stored login credentials at rest —
// AES-256-GCM, stdlib only, mirroring api/src/platform/crypto's own
// already-approved scheme exactly (this tool is its own Go module and
// can't import that internal package, so this is a deliberate,
// structurally identical duplicate — same small-helper-duplication
// convention this codebase already uses elsewhere). See
// plan/ai/tools/browser/step-07-login-profiles-and-credential-isolation.md.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const cryptoKeySize = 32 // AES-256

// loadOrGenerateCryptoKey reads the 32-byte encryption key from path
// if present, otherwise generates a fresh one via crypto/rand and
// persists it (0600, parent directory 0700). Deliberately not derived
// from anything else stored alongside it — key material lives outside
// browser.db itself, so a leaked copy of just the database is never
// enough on its own to recover a plaintext password.
func loadOrGenerateCryptoKey(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if len(raw) != cryptoKeySize {
			return nil, fmt.Errorf("crypto: key file %q has %d bytes, want %d", path, len(raw), cryptoKeySize)
		}
		return raw, nil
	}

	key := make([]byte, cryptoKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("crypto: generate key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("crypto: create key directory: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("crypto: persist key: %w", err)
	}
	return key, nil
}

// encryptSecret returns base64(nonce || ciphertext) — a random nonce
// generated per call, prepended so decryptSecret can split it back
// off.
func encryptSecret(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptSecret reverses encryptSecret.
func decryptSecret(ciphertext string, key []byte) (string, error) {
	sealed, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(sealed) < nonceSize {
		return "", fmt.Errorf("crypto: ciphertext too short")
	}
	nonce, sealedRest := sealed[:nonceSize], sealed[nonceSize:]

	plain, err := gcm.Open(nil, nonce, sealedRest, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// maskUsername returns a display-safe form — the real value is never
// what a human meant to hide, but masking it anyway keeps
// GET /login-credentials from being a plain, complete membership list
// of every configured login at a glance. Matches
// platform_crypto.Mask's own convention: first 6 and last 4
// characters, or all dots if too short for that to make sense.
func maskUsername(plaintext string) string {
	if len(plaintext) <= 8 {
		return "••••••••"
	}
	return plaintext[:6] + "…" + plaintext[len(plaintext)-4:]
}
