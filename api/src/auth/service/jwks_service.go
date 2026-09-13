package auth_service

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// jwksResponse handles both a standard bare JWKS document {"keys":[...]}
// and coco-iam's own response envelope {"success":true,"message":{"keys":[...]}}.
type jwksResponse struct {
	Keys    []jwk        `json:"keys"`
	Message *jwksPayload `json:"message"`
}

type jwksPayload struct {
	Keys []jwk `json:"keys"`
}

// JwksService validates coco-iam-issued JWTs. It supports RS256 via a
// JWKS endpoint fetched over HTTP (refreshed hourly, with an on-demand
// retry when an unknown kid is seen), and falls back to a static HS256
// shared secret for local development against no running coco-iam
// instance. It satisfies coco-oauth's TokenValidator interface
// structurally (Validate(token string) (string, []string, time.Time, error)).
type JwksService struct {
	jwksURL     string
	hs256Secret []byte
	issuer      string
	audience    string
	keys        map[string]*rsa.PublicKey
	mu          sync.RWMutex
}

func NewJwksService(jwksURL, hs256Secret, issuer, audience string) *JwksService {
	return &JwksService{
		jwksURL:     jwksURL,
		hs256Secret: []byte(hs256Secret),
		issuer:      issuer,
		audience:    audience,
		keys:        make(map[string]*rsa.PublicKey),
	}
}

// FetchKeys pulls the current JWKS document and replaces the key cache.
// A no-op when jwksURL is empty (dev/HS256-only mode).
func (s *JwksService) FetchKeys() error {
	if s.jwksURL == "" {
		return nil
	}
	resp, err := http.Get(s.jwksURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	jwkList := result.Keys
	if len(jwkList) == 0 && result.Message != nil {
		jwkList = result.Message.Keys
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, k := range jwkList {
		if k.Kty != "RSA" || k.N == "" || k.E == "" {
			continue
		}
		pub, err := parseJWK(k.N, k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = pub
	}

	s.mu.Lock()
	s.keys = keys
	s.mu.Unlock()
	return nil
}

// Start performs an initial fetch (if jwksURL is set) and then refreshes
// hourly until ctx is cancelled.
func (s *JwksService) Start(ctx context.Context, log func(string, ...any)) {
	if s.jwksURL != "" {
		if err := s.FetchKeys(); err != nil {
			log("JWKS: initial fetch failed: %v", err)
		}
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if s.jwksURL != "" {
					if err := s.FetchKeys(); err != nil {
						log("JWKS: refresh failed: %v", err)
					}
				}
			}
		}
	}()
}

// Validate parses and verifies token, returning its subject, scopes, and
// expiry. RS256 tokens are verified against the cached JWKS (one on-demand
// refetch if the kid isn't cached yet); HS256 tokens are verified against
// the configured shared secret.
func (s *JwksService) Validate(token string) (string, []string, time.Time, error) {
	if token == "" {
		return "", nil, time.Time{}, fmt.Errorf("token empty")
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256", "HS256"}))
	tok, err := parser.Parse(token, func(t *jwt.Token) (any, error) {
		alg, _ := t.Header["alg"].(string)
		switch alg {
		case "RS256":
			kid, _ := t.Header["kid"].(string)
			s.mu.RLock()
			key, ok := s.keys[kid]
			s.mu.RUnlock()
			if !ok {
				if ferr := s.FetchKeys(); ferr == nil {
					s.mu.RLock()
					key, ok = s.keys[kid]
					s.mu.RUnlock()
				}
				if !ok {
					return nil, fmt.Errorf("unknown kid: %s", kid)
				}
			}
			return key, nil
		case "HS256":
			if len(s.hs256Secret) == 0 {
				return nil, fmt.Errorf("HS256 not configured")
			}
			return s.hs256Secret, nil
		default:
			return nil, fmt.Errorf("unsupported alg: %s", alg)
		}
	})
	if err != nil || !tok.Valid {
		return "", nil, time.Time{}, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return "", nil, time.Time{}, fmt.Errorf("invalid claims")
	}

	if s.issuer != "" {
		iss, _ := claims["iss"].(string)
		if strings.TrimRight(iss, "/") != strings.TrimRight(s.issuer, "/") {
			return "", nil, time.Time{}, fmt.Errorf("issuer mismatch: token=%q config=%q", iss, s.issuer)
		}
	}
	if s.audience != "" {
		aud := claimAud(claims)
		if aud != s.audience {
			return "", nil, time.Time{}, fmt.Errorf("audience mismatch: token=%q config=%q", aud, s.audience)
		}
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", nil, time.Time{}, fmt.Errorf("subject missing")
	}

	// make(..., 0), not var (nil slice) — a nil slice marshals to JSON
	// `null` for a scopeless token, not `[]`. MeHandler passes this
	// straight through as the auth/me response's "scopes" field, and the
	// frontend's AuthContext stores it as-is; a `null` there crashes
	// AuthGuard's `userScopes.includes(...)` at runtime despite the
	// TypeScript type claiming `string[]` (never validated against the
	// real response). Same convention every repository in this codebase
	// already follows for list endpoints.
	scopes := make([]string, 0)
	if sc, ok := claims["scope"].(string); ok && sc != "" {
		scopes = strings.Fields(sc)
	}

	var exp time.Time
	if e, ok := claims["exp"].(float64); ok {
		exp = time.Unix(int64(e), 0)
	}

	return sub, scopes, exp, nil
}

func parseJWK(n, e string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(n)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(e)
	if err != nil {
		return nil, err
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: int(new(big.Int).SetBytes(eb).Int64()),
	}, nil
}

func claimAud(claims jwt.MapClaims) string {
	switch a := claims["aud"].(type) {
	case string:
		return a
	case []any:
		if len(a) > 0 {
			if s, ok := a[0].(string); ok {
				return s
			}
		}
	}
	return ""
}
