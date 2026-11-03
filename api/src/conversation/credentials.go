package conversation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/a-digi/cinqo/src/platform"
	platform_crypto "github.com/a-digi/cinqo/src/platform/crypto"
	platform_query "github.com/a-digi/cinqo/src/platform/repository/query"
)

// ErrPlatformNotReady means a platform that supplies its own credentials (platform.Entry.Credentials) cannot
// answer right now; the wrapped error says why.
var ErrPlatformNotReady = errors.New("conversation: platform is not ready")

// platformFailureMessage is what a user is told when resolveEntry fails: plain words, never the key or any internals.
func platformFailureMessage(err error) string {
	switch {
	case errors.Is(err, ErrNoKeyForPlatform):
		return "No API key is registered for this conversation's platform."
	case errors.Is(err, ErrPlatformNotReady):
		reason := strings.TrimPrefix(err.Error(), ErrPlatformNotReady.Error()+": ")
		return "This conversation's platform cannot answer right now: " + reason + "."
	default:
		return "The platform could not be reached."
	}
}

// resolveEntry returns the platform entry to call and the token to send. For a platform with its own
// Credentials the entry's base URL is replaced by the one it reports (the entry is a copy, the registry is
// untouched) and the token is whatever it reports. Every other platform is reached with DefaultBaseURL and its
// most recent stored API key, decrypted: ErrNoKeyForPlatform when none exists. The plain key is only ever
// returned, never logged or put into an error.
func resolveEntry(ctx context.Context, mainDB *sql.DB, encryptionKey []byte, entry platform.Entry, platformID string) (platform.Entry, string, error) {
	if entry.Credentials != nil {
		baseURL, token, err := entry.Credentials(ctx)
		if err != nil {
			return entry, "", fmt.Errorf("%w: %v", ErrPlatformNotReady, err)
		}
		entry.DefaultBaseURL = baseURL
		return entry, token, nil
	}

	key, err := platform_query.NewKeyQueryRepo(mainDB).FindMostRecentForPlatform(platformID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return entry, "", ErrNoKeyForPlatform
		}
		return entry, "", fmt.Errorf("conversation: look up platform key: %w", err)
	}
	plainKey, err := platform_crypto.Decrypt(key.EncryptedKey, encryptionKey)
	if err != nil {
		return entry, "", fmt.Errorf("conversation: decrypt platform key: %w", err)
	}
	return entry, plainKey, nil
}
