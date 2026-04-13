package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/auth"
	"github.com/aidanhall34/make-mcp/pkg/config"
	"github.com/golang-jwt/jwt/v5"
)

// generateKey generates a fresh RSA key for test use.
func generateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

// jwksServer starts an httptest.Server that serves a JWKS containing key.
// It returns the server URL.
func jwksServer(t *testing.T, kid string, key *rsa.PrivateKey) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nB64 := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
		eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
		resp := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": kid,
					"use": "sig",
					"n":   nB64,
					"e":   eB64,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// makeToken signs a token with the given key and claims.
func makeToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	raw, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func TestValidate_ValidToken(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	kid := "key-1"
	srv := jwksServer(t, kid, key)

	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	token := makeToken(t, key, kid, jwt.MapClaims{
		"iss":    "https://issuer.example.com",
		"aud":    jwt.ClaimStrings{"make-mcp"},
		"sub":    "user-1",
		"exp":    time.Now().Add(time.Hour).Unix(),
		"groups": []string{"/admins"},
	})

	claims, err := v.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
	if len(claims.Groups) != 1 || claims.Groups[0] != "/admins" {
		t.Errorf("Groups: got %v, want [/admins]", claims.Groups)
	}
}

func TestValidate_ExpiredToken(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	kid := "key-1"
	srv := jwksServer(t, kid, key)

	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	token := makeToken(t, key, kid, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": jwt.ClaimStrings{"make-mcp"},
		"sub": "user-1",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	_, err := v.Validate(ctx, token)
	if err == nil {
		t.Fatal("Validate: expected error for expired token, got nil")
	}
}

func TestValidate_WrongIssuer(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	kid := "key-1"
	srv := jwksServer(t, kid, key)

	v := auth.New("https://correct-issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	token := makeToken(t, key, kid, jwt.MapClaims{
		"iss": "https://wrong-issuer.example.com",
		"aud": jwt.ClaimStrings{"make-mcp"},
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, err := v.Validate(ctx, token)
	if err == nil {
		t.Fatal("Validate: expected error for wrong issuer, got nil")
	}
}

func TestValidate_WrongAudience(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	kid := "key-1"
	srv := jwksServer(t, kid, key)

	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	token := makeToken(t, key, kid, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": jwt.ClaimStrings{"wrong-audience"},
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, err := v.Validate(ctx, token)
	if err == nil {
		t.Fatal("Validate: expected error for wrong audience, got nil")
	}
}

func TestValidate_UnknownKID(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	kid := "key-1"
	srv := jwksServer(t, kid, key)

	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	// Token signed with kid that doesn't exist in JWKS.
	token := makeToken(t, key, "unknown-kid", jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": jwt.ClaimStrings{"make-mcp"},
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	_, err := v.Validate(ctx, token)
	if err == nil {
		t.Fatal("Validate: expected error for unknown kid, got nil")
	}
}

func TestValidate_JWKSKeyRotation(t *testing.T) {
	ctx := context.Background()

	// Start with key1, then rotate to key2.
	key1 := generateKey(t)
	key2 := generateKey(t)
	kid1 := "key-1"
	kid2 := "key-2"

	// Start with key1 only.
	var serveKey2 bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		keys := []map[string]any{
			{
				"kty": "RSA",
				"kid": kid1,
				"n":   base64.RawURLEncoding.EncodeToString(key1.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key1.PublicKey.E)).Bytes()),
			},
		}
		if serveKey2 {
			keys = append(keys, map[string]any{
				"kty": "RSA",
				"kid": kid2,
				"n":   base64.RawURLEncoding.EncodeToString(key2.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key2.PublicKey.E)).Bytes()),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	}))
	t.Cleanup(srv.Close)

	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	// First call with key1 works.
	token1 := makeToken(t, key1, kid1, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": jwt.ClaimStrings{"make-mcp"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Validate(ctx, token1); err != nil {
		t.Fatalf("Validate key1: %v", err)
	}

	// Rotate: add key2 to JWKS.
	serveKey2 = true

	// A token signed with key2 triggers re-fetch (kid not cached yet).
	token2 := makeToken(t, key2, kid2, jwt.MapClaims{
		"iss": "https://issuer.example.com",
		"aud": jwt.ClaimStrings{"make-mcp"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Validate(ctx, token2); err != nil {
		t.Fatalf("Validate key2 after rotation: %v", err)
	}
}

func TestGroupsFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	groups := auth.GroupsFromContext(ctx)
	if groups != nil {
		t.Errorf("GroupsFromContext on empty context: got %v, want nil", groups)
	}
}

func TestGroupsFromContext_WithGroups(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	kid := "key-1"
	srv := jwksServer(t, kid, key)

	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})

	token := makeToken(t, key, kid, jwt.MapClaims{
		"iss":    "https://issuer.example.com",
		"aud":    jwt.ClaimStrings{"make-mcp"},
		"exp":    time.Now().Add(time.Hour).Unix(),
		"groups": []string{"/admins", "/developers"},
	})

	claims, err := v.Validate(ctx, token)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	ctx = auth.ContextWithClaims(ctx, claims)
	got := auth.GroupsFromContext(ctx)
	if len(got) != 2 || got[0] != "/admins" || got[1] != "/developers" {
		t.Errorf("GroupsFromContext: got %v, want [/admins /developers]", got)
	}
}

func TestBearerToken_Present(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer mytoken123")
	got := auth.BearerToken(r)
	if got != "mytoken123" {
		t.Errorf("BearerToken: got %q, want %q", got, "mytoken123")
	}
}

func TestBearerToken_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	got := auth.BearerToken(r)
	if got != "" {
		t.Errorf("BearerToken: got %q, want empty", got)
	}
}

func TestBearerToken_WrongScheme(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	got := auth.BearerToken(r)
	if got != "" {
		t.Errorf("BearerToken: got %q, want empty", got)
	}
}

func TestNewHTTPClient_NoCA(t *testing.T) {
	client, err := auth.NewHTTPClient(config.TLSConfig{})
	if err != nil {
		t.Fatalf("NewHTTPClient (no CA): %v", err)
	}
	if client == nil {
		t.Fatal("NewHTTPClient returned nil")
	}
}

func TestNewHTTPClient_MissingCAFile(t *testing.T) {
	_, err := auth.NewHTTPClient(config.TLSConfig{CA: "/nonexistent/ca.crt"})
	if err == nil {
		t.Fatal("NewHTTPClient: expected error for missing CA file, got nil")
	}
}

func TestValidate_MalformedToken(t *testing.T) {
	ctx := context.Background()
	key := generateKey(t)
	srv := jwksServer(t, "key-1", key)
	v := auth.New("https://issuer.example.com", "make-mcp", srv.URL, &http.Client{})
	_, err := v.Validate(ctx, "not.a.token")
	if err == nil {
		t.Fatal("Validate: expected error for malformed token, got nil")
	}
}
