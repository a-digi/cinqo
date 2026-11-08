// Package platform_crypto encrypts platform API keys at rest —
// AES-256-GCM, stdlib only, matching the real reference implementation
// verified before designing this. See
// plan/ai/platform/step-02-ai-key-data-model-and-encryption.md.
package platform_crypto

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

const keySize = 32 // AES-256

// LoadOrGenerateKey reads the 32-byte encryption key from path if
// present, otherwise generates a fresh one via crypto/rand and persists
// it (0600, parent directory 0700). Deliberately not sourced from
// config.json or any other config-supplied secret — key material must
// live outside the SQLite file itself, so a leaked/backed-up copy of
// just the database is never enough to recover a plaintext key.
func LoadOrGenerateKey(path string) ([]byte, error) {
	if raw, err := os.ReadFile(path); err == nil {
		if len(raw) != keySize {
			return nil, fmt.Errorf("platform_crypto: key file %q has %d bytes, want %d", path, len(raw), keySize)
		}
		return raw, nil
	}

	key := make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("platform_crypto: generate key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("platform_crypto: create key directory: %w", err)
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, fmt.Errorf("platform_crypto: persist key: %w", err)
	}
	return key, nil
}

// Encrypt returns base64(nonce || ciphertext) — a random nonce
// generated per call, prepended so Decrypt can split it back off.
func Encrypt(plaintext string, key []byte) (string, error) {
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

// Decrypt reverses Encrypt.
func Decrypt(ciphertext string, key []byte) (string, error) {
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
		return "", fmt.Errorf("platform_crypto: ciphertext too short")
	}
	nonce, sealedRest := sealed[:nonceSize], sealed[nonceSize:]

	plain, err := gcm.Open(nil, nonce, sealedRest, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// Mask returns a display-safe form of plaintext — the real value never
// appears in any API response, only this. Matches the reference
// exactly: first 6 and last 4 characters, or all dots if too short for
// that to make sense.
func Mask(plaintext string) string {
	if len(plaintext) <= 8 {
		return "••••••••"
	}
	return plaintext[:6] + "…" + plaintext[len(plaintext)-4:]
}
