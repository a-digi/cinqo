package shared

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// loginsession.go (plan 195, step 1) is the store for a login SESSION a person chose to keep: the cookies of ONE site, captured from the
// login window after the person logged in there, sealed with AES-GCM and kept in this tool's own database, so Katana can read the pages of
// that site's logged-in area. It is NOT a stored password: this tool still keeps no password and no login form (plan 182,
// no_stored_credentials_test.go). Rules that hold for every function here:
//
//   - The cookies never leave this package except through OpenLoginSession, which only the crawler's fetch calls, and only for the host the
//     session was made for. No route, log line, error text or AI tool carries a cookie name, value or the sealed bytes.
//   - A session belongs to one host (the link's own host). Another host gets nothing: not the cookies, and the sealed row cannot be
//     unsealed as another host's (the host is part of the authenticated data of the seal).
//   - A session lives until the earlier of its longest-living cookie and LoginSessionMaxAge; expired rows are deleted, never replayed.
//   - The key is a random 256-bit key in a 0600 file next to the database, created the first time a session is saved (a fresh install has
//     none). Honest limit: it sits next to the data on the same machine, so the seal protects a copied database file (a backup, a shared
//     folder), not someone who owns this user account. See plan/ai/tools/browser/step-195-katana-with-login-session.md, decision D3.

const (
	// LoginSessionMaxAge is the ceiling for how long a saved session lives, whatever its cookies say (plan 195, D6).
	LoginSessionMaxAge = 30 * 24 * time.Hour

	// LoginSessionValid and LoginSessionExpired are the two states a caller sees.
	LoginSessionValid   = "valid"
	LoginSessionExpired = "expired"

	loginSessionKeyFile = "login_sessions.key"
)

var (
	// ErrLoginSessionNotFound: no such session (never saved, forgotten, or deleted with its expiry).
	ErrLoginSessionNotFound = errors.New("login session not found")
	// ErrLoginSessionWrongHost: the session was made for another site.
	ErrLoginSessionWrongHost = errors.New("login session belongs to another site")
	// ErrLoginSessionExpired: the session is past its lifetime.
	ErrLoginSessionExpired = errors.New("login session expired")
	// ErrLoginSessionNoCookies: nothing to keep for this host.
	ErrLoginSessionNoCookies = errors.New("no cookies of this site to keep")
	// ErrLoginSessionUnreadable: the sealed row does not open (wrong or lost key, damaged row).
	ErrLoginSessionUnreadable = errors.New("login session cannot be opened")
)

// loginSessionKeyDir is the folder of the key file: the database folder, set by InitDB.
var (
	loginSessionKeyDir string
	loginSessionKeyMu  sync.Mutex
	loginSessionNow    = time.Now // a test replaces it
)

// SessionCookie is one cookie of a saved session. Expires is a Unix time in seconds; 0 or less means a session cookie (no expiry).
type SessionCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires"`
}

// LoginSessionInfo is everything a caller may learn about a saved session: never a cookie.
type LoginSessionInfo struct {
	ID         string `json:"id"`
	Host       string `json:"host"`
	State      string `json:"state"`
	CapturedAt string `json:"capturedAt"`
	ExpiresAt  string `json:"expiresAt"`
	LastOKAt   string `json:"lastOkAt,omitempty"`
}

func prepareLoginSessions(db *sql.DB, dbDir string) error {
	loginSessionKeyDir = dbDir
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS login_sessions (
		id             TEXT PRIMARY KEY,
		host           TEXT NOT NULL,
		sealed_cookies BLOB NOT NULL,
		captured_at    TEXT NOT NULL,
		expires_at     TEXT NOT NULL,
		last_ok_at     TEXT,
		state          TEXT NOT NULL DEFAULT 'valid' CHECK (state IN ('valid','expired'))
	)`); err != nil {
		return err
	}
	_, err := db.Exec(`DELETE FROM login_sessions WHERE expires_at <= ?`, loginSessionNow().UTC().Format(time.RFC3339))
	return err
}

// loginSessionKey returns the key, creating the file (mode 0600) when it does not exist yet and create is true.
func loginSessionKey(create bool) ([]byte, error) {
	loginSessionKeyMu.Lock()
	defer loginSessionKeyMu.Unlock()
	if loginSessionKeyDir == "" {
		return nil, errors.New("login session store is not initialised")
	}
	path := filepath.Join(loginSessionKeyDir, loginSessionKeyFile)
	if b, err := os.ReadFile(path); err == nil {
		if len(b) != 32 {
			return nil, ErrLoginSessionUnreadable
		}
		return b, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if !create {
		return nil, ErrLoginSessionUnreadable
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := f.Write(key); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	return key, nil
}

func sealSession(key []byte, id, host string, cookies []SessionCookie) ([]byte, error) {
	plain, err := json.Marshal(cookies)
	if err != nil {
		return nil, err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, []byte(id+"\x00"+host)), nil
}

func openSession(key []byte, id, host string, sealed []byte) ([]SessionCookie, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, ErrLoginSessionUnreadable
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(id+"\x00"+host))
	if err != nil {
		return nil, ErrLoginSessionUnreadable
	}
	var cookies []SessionCookie
	if err := json.Unmarshal(plain, &cookies); err != nil {
		return nil, ErrLoginSessionUnreadable
	}
	return cookies, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// cookieMatchesHost is the browser's domain match: a cookie for example.com (or .example.com) goes to example.com and every subdomain, a
// host-only cookie for www.example.com only to that host.
func cookieMatchesHost(cookieDomain, host string) bool {
	d := strings.ToLower(strings.TrimPrefix(cookieDomain, "."))
	h := strings.ToLower(host)
	return d != "" && (h == d || strings.HasSuffix(h, "."+d))
}

// hostAllowed says whether a fetch for host may use a session made for sessionHost: the same host or a subdomain of it.
func hostAllowed(sessionHost, host string) bool {
	s, h := strings.ToLower(sessionHost), strings.ToLower(host)
	return s != "" && (h == s || strings.HasSuffix(h, "."+s))
}

// SaveLoginSession seals the cookies of host (cookies of other sites in the list are dropped, already expired ones too) and stores them. It
// returns the session's info only. A new session for the same host does not replace an older one: the caller decides (Forget / replace).
func SaveLoginSession(host string, cookies []SessionCookie) (LoginSessionInfo, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return LoginSessionInfo{}, ErrLoginSessionNoCookies
	}
	now := loginSessionNow()
	var keep []SessionCookie
	var latest float64
	for _, c := range cookies {
		if c.Name == "" || !cookieMatchesHost(c.Domain, host) {
			continue
		}
		if c.Expires > 0 && time.Unix(int64(c.Expires), 0).Before(now) {
			continue
		}
		keep = append(keep, c)
		if c.Expires > latest {
			latest = c.Expires
		}
	}
	if len(keep) == 0 {
		return LoginSessionInfo{}, ErrLoginSessionNoCookies
	}
	expires := now.Add(LoginSessionMaxAge)
	if latest > 0 {
		if byCookies := time.Unix(int64(latest), 0); byCookies.Before(expires) {
			expires = byCookies
		}
	}

	key, err := loginSessionKey(true)
	if err != nil {
		return LoginSessionInfo{}, fmt.Errorf("could not prepare the login session key: %w", err)
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return LoginSessionInfo{}, err
	}
	id := hex.EncodeToString(idBytes)
	sealed, err := sealSession(key, id, host, keep)
	if err != nil {
		return LoginSessionInfo{}, errors.New("could not seal the login session")
	}
	captured := now.UTC().Format(time.RFC3339)
	exp := expires.UTC().Format(time.RFC3339)
	if _, err := DB.Exec(`INSERT INTO login_sessions (id, host, sealed_cookies, captured_at, expires_at, state) VALUES (?, ?, ?, ?, ?, 'valid')`,
		id, host, sealed, captured, exp); err != nil {
		return LoginSessionInfo{}, errors.New("could not store the login session")
	}
	return LoginSessionInfo{ID: id, Host: host, State: LoginSessionValid, CapturedAt: captured, ExpiresAt: exp}, nil
}

func scanLoginSessionInfo(row interface{ Scan(...any) error }) (LoginSessionInfo, error) {
	var info LoginSessionInfo
	var lastOK sql.NullString
	var state string
	if err := row.Scan(&info.ID, &info.Host, &info.CapturedAt, &info.ExpiresAt, &lastOK, &state); err != nil {
		return LoginSessionInfo{}, err
	}
	info.State = state
	info.LastOKAt = lastOK.String
	if exp, err := time.Parse(time.RFC3339, info.ExpiresAt); err == nil && !exp.After(loginSessionNow()) {
		info.State = LoginSessionExpired
	}
	return info, nil
}

// GetLoginSession returns the info of one session (never cookies). A session past its lifetime is reported with state "expired" until the
// next start of the tool or an explicit delete, so a caller can tell the person to log in again.
func GetLoginSession(id string) (LoginSessionInfo, error) {
	info, err := scanLoginSessionInfo(DB.QueryRow(`SELECT id, host, captured_at, expires_at, last_ok_at, state FROM login_sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return LoginSessionInfo{}, ErrLoginSessionNotFound
	}
	return info, err
}

// ListLoginSessions returns every saved session's info, newest first (never cookies).
func ListLoginSessions() ([]LoginSessionInfo, error) {
	rows, err := DB.Query(`SELECT id, host, captured_at, expires_at, last_ok_at, state FROM login_sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LoginSessionInfo{}
	for rows.Next() {
		info, err := scanLoginSessionInfo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CapturedAt > out[j].CapturedAt })
	return out, rows.Err()
}

// DeleteLoginSession forgets a session. Deleting one that does not exist is not an error (idempotent).
func DeleteLoginSession(id string) error {
	_, err := DB.Exec(`DELETE FROM login_sessions WHERE id = ?`, id)
	return err
}

// OpenLoginSession returns the cookies a fetch of host may send: only for a session made for that host (or a parent of it), only while it
// is valid, and only the cookies whose own domain matches host. The caller sets them in the fetch's own browser context; they must never be
// logged or returned by a route.
func OpenLoginSession(id, host string) ([]SessionCookie, error) {
	var sessionHost, expiresAt, state string
	var sealed []byte
	err := DB.QueryRow(`SELECT host, sealed_cookies, expires_at, state FROM login_sessions WHERE id = ?`, id).Scan(&sessionHost, &sealed, &expiresAt, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrLoginSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	if !hostAllowed(sessionHost, host) {
		return nil, ErrLoginSessionWrongHost
	}
	if exp, perr := time.Parse(time.RFC3339, expiresAt); perr != nil || !exp.After(loginSessionNow()) || state != LoginSessionValid {
		return nil, ErrLoginSessionExpired
	}
	key, err := loginSessionKey(false)
	if err != nil {
		return nil, ErrLoginSessionUnreadable
	}
	all, err := openSession(key, id, sessionHost, sealed)
	if err != nil {
		return nil, err
	}
	now := loginSessionNow()
	var out []SessionCookie
	for _, c := range all {
		if !cookieMatchesHost(c.Domain, host) {
			continue
		}
		if c.Expires > 0 && time.Unix(int64(c.Expires), 0).Before(now) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
