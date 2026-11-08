package shared

import (
	"os"
	"path/filepath"
	"testing"
)

func tableExists(t *testing.T, name string) bool {
	t.Helper()
	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

// This tool stores no login: a fresh database has no credentials table and no key file is made.
func TestAFreshInstallHasNoCredentialStore(t *testing.T) {
	dbPath := initSettingsDB(t)
	if tableExists(t, "login_credentials") {
		t.Error("a fresh install must not create login_credentials")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dbPath), credentialsKeyFile)); !os.IsNotExist(err) {
		t.Errorf("no key file expected, stat error = %v", err)
	}
}

// An install from an older version has the table with rows and the key file: both are deleted at start, the rest of the database stays.
func TestAnOlderInstallsStoredCredentialsAndKeyAreRemovedAtStart(t *testing.T) {
	dbPath := initSettingsDB(t)
	dbDir := filepath.Dir(dbPath)
	if _, err := DB.Exec(`CREATE TABLE login_credentials (domain TEXT PRIMARY KEY, username TEXT NOT NULL, encrypted_password TEXT NOT NULL, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO login_credentials (domain, username, encrypted_password) VALUES ('a.example', 'u', 'x'), ('b.example', 'v', 'y')`); err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dbDir, credentialsKeyFile)
	if err := os.WriteFile(keyPath, make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO cloudflare_domains (domain, reason) VALUES ('keep.example', 'r')`); err != nil {
		t.Fatal(err)
	}

	removeStoredCredentials(DB, dbDir)

	if tableExists(t, "login_credentials") {
		t.Error("login_credentials must be dropped")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Errorf("the key file must be deleted, stat error = %v", err)
	}
	var kept int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM cloudflare_domains`).Scan(&kept); err != nil || kept != 1 {
		t.Errorf("an unrelated table lost its data: %d, %v", kept, err)
	}

	removeStoredCredentials(DB, dbDir) // idempotent: nothing left, nothing fails
}

// The real start (InitDB) does it too, not only the helper.
func TestInitDBRemovesAnOlderInstallsCredentials(t *testing.T) {
	root := t.TempDir()
	dbDir := filepath.Join(root, "db")
	t.Setenv("TOOL_DB_DIR", dbDir)
	t.Setenv("TOOL_LOGS_DIR", filepath.Join(root, "logs"))
	t.Setenv("TOOL_CACHE_DIR", filepath.Join(root, "cache"))
	if err := InitDB(); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`CREATE TABLE login_credentials (domain TEXT PRIMARY KEY, username TEXT, encrypted_password TEXT)`); err != nil {
		t.Fatal(err)
	}
	DB.Close()
	if err := os.WriteFile(filepath.Join(dbDir, credentialsKeyFile), make([]byte, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DB.Close() })
	if tableExists(t, "login_credentials") {
		t.Error("InitDB must drop login_credentials")
	}
	if _, err := os.Stat(filepath.Join(dbDir, credentialsKeyFile)); !os.IsNotExist(err) {
		t.Errorf("InitDB must delete the key file, stat error = %v", err)
	}
}
