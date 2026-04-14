// Package auth provides OAuth 2.1 Bearer token validation for the make-mcp
// HTTP server. It validates JWTs against a JWKS endpoint, extracts group
// claims, and provides middleware helpers for per-request access control.
package auth

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/golang-jwt/jwt/v5"
)

const jwksCacheTTL = 5 * time.Minute

// contextKey is the unexported type used for context values set by this package.
type contextKey struct{}

// Claims is the set of JWT claims extracted from a validated Bearer token.
type Claims struct {
	jwt.RegisteredClaims
	// Groups holds the group/role claim values. Keycloak populates "groups".
	Groups []string `json:"groups"`
}

// Validator validates Bearer tokens against a JWKS endpoint and caches the
// key set with a configurable TTL.
type Validator struct {
	issuer     string
	audience   string
	jwksURI    string
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	lastFetch time.Time
}

// New returns a Validator. httpClient must not be nil; use NewHTTPClient to
// build one that trusts a custom CA.
func New(issuer, audience, jwksURI string, httpClient *http.Client) *Validator {
	return &Validator{
		issuer:     issuer,
		audience:   audience,
		jwksURI:    jwksURI,
		httpClient: httpClient,
		keys:       make(map[string]*rsa.PublicKey),
	}
}

// NewHTTPClient builds an *http.Client that trusts a custom CA when cfg.CA is
// set (e.g. for a local Keycloak with a self-signed certificate). When cfg.CA
// is empty, the returned client uses the system trust store.
func NewHTTPClient(cfg config.TLSConfig) (*http.Client, error) {
	if cfg.CA == "" {
		return &http.Client{Timeout: 10 * time.Second}, nil
	}
	caPEM, err := os.ReadFile(cfg.CA)
	if err != nil {
		return nil, fmt.Errorf("auth: read CA file %q: %w", cfg.CA, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("auth: no valid PEM certificates found in %q", cfg.CA)
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool},
		},
		Timeout: 10 * time.Second,
	}, nil
}

// Validate parses and validates a raw JWT string. It returns the extracted
// Claims on success or an error describing the validation failure.
func (v *Validator) Validate(ctx context.Context, rawToken string) (*Claims, error) {
	// Parse without verification first to extract the key ID from the header.
	unverified, _, err := jwt.NewParser().ParseUnverified(rawToken, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("auth: parse token header: %w", err)
	}
	kid, _ := unverified.Header["kid"].(string)

	key, err := v.resolveKey(ctx, kid)
	if err != nil {
		return nil, err
	}

	claims := &Claims{}
	_, err = jwt.NewParser(
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
	).ParseWithClaims(rawToken, claims, func(_ *jwt.Token) (any, error) {
		return key, nil
	})
	if err != nil {
		return nil, fmt.Errorf("auth: invalid token: %w", err)
	}
	return claims, nil
}

// resolveKey looks up the RSA public key for kid. When the key is not in the
// cache or the cache has expired, the JWKS is refreshed once.
func (v *Validator) resolveKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// Fast path: key already cached and cache is fresh.
	if key, ok := v.cachedKey(kid); ok {
		return key, nil
	}
	// Cache miss or expired: refresh unconditionally.
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}
	if key, ok := v.cachedKey(kid); ok {
		return key, nil
	}
	return nil, fmt.Errorf("auth: key %q not found in JWKS", kid)
}

func (v *Validator) cachedKey(kid string) (*rsa.PublicKey, bool) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if time.Since(v.lastFetch) > jwksCacheTTL {
		return nil, false
	}
	key, ok := v.keys[kid]
	return key, ok
}

// refresh fetches the JWKS and repopulates the key cache.
func (v *Validator) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURI, nil)
	if err != nil {
		return fmt.Errorf("auth: build JWKS request: %w", err)
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth: fetch JWKS from %s: %w", v.jwksURI, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth: JWKS endpoint returned %d", resp.StatusCode)
	}

	var jwks struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return fmt.Errorf("auth: decode JWKS: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		pub, err := k.publicKey()
		if err != nil {
			// Skip unsupported key types; don't fail the whole refresh.
			continue
		}
		id := k.Kid
		if id == "" {
			id = k.Use
		}
		keys[id] = pub
	}

	v.mu.Lock()
	v.keys = keys
	v.lastFetch = time.Now()
	v.mu.Unlock()
	return nil
}

// jwk is the minimal JWK structure needed to reconstruct an RSA public key.
type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Kid string `json:"kid"`
	N   string `json:"n"` // base64url-encoded modulus
	E   string `json:"e"` // base64url-encoded exponent
}

func (k jwk) publicKey() (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, errors.New("unsupported key type")
	}
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode E: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// ContextWithClaims returns a derived context carrying the validated claims.
// Called by the HTTP transport after successful validation.
func ContextWithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, c)
}

// GroupsFromContext extracts the Groups claim from the context populated by
// the Bearer middleware. Returns nil when no claims are present.
func GroupsFromContext(ctx context.Context) []string {
	c, ok := ctx.Value(contextKey{}).(*Claims)
	if !ok || c == nil {
		return nil
	}
	return c.Groups
}

// BearerToken extracts the raw token string from the Authorization header.
// Returns "" when the header is absent or not a Bearer scheme.
func BearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		return ""
	}
	return h[len(prefix):]
}
