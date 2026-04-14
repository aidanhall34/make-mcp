package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanhall34/make-mcp/pkg/config"
)

func TestNewHTTPClient_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.crt")
	os.WriteFile(caFile, []byte("not a certificate"), 0644)

	_, err := NewHTTPClient(config.TLSConfig{CA: caFile})
	if err == nil {
		t.Fatal("expected error for invalid PEM, got nil")
	}
}

func TestRefresh_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	v := New("iss", "aud", srv.URL, &http.Client{})
	err := v.refresh(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestRefresh_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	v := New("iss", "aud", srv.URL, &http.Client{})
	err := v.refresh(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestRefresh_UnsupportedKeyType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "EC",
					"kid": "key-1",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := New("iss", "aud", srv.URL, &http.Client{})
	err := v.refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(v.keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(v.keys))
	}
}

func TestJWK_PublicKey_Errors(t *testing.T) {
	// Unsupported kty
	k := jwk{Kty: "EC"}
	_, err := k.publicKey()
	if err == nil {
		t.Error("expected error for kty=EC, got nil")
	}

	// Invalid N
	k = jwk{Kty: "RSA", N: "!!!"}
	_, err = k.publicKey()
	if err == nil {
		t.Error("expected error for invalid N, got nil")
	}

	// Invalid E
	k = jwk{Kty: "RSA", N: "c29tZW5vbmNl", E: "!!!"}
	_, err = k.publicKey()
	if err == nil {
		t.Error("expected error for invalid E, got nil")
	}
}

func TestResolveKey_ExpiredCache(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	nB64 := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"kid": "key-1",
					"n":   nB64,
					"e":   eB64,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := New("iss", "aud", srv.URL, &http.Client{})

	// Pre-populate cache but set lastFetch to long ago.
	v.keys["key-1"] = &key.PublicKey
	v.lastFetch = time.Now().Add(-2 * jwksCacheTTL)

	// Should trigger refresh.
	got, err := v.resolveKey(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("resolveKey: %v", err)
	}
	if got == nil {
		t.Fatal("got nil key")
	}
}

func TestJWK_KidFallback(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	nB64 := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
	eB64 := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"keys": []map[string]any{
				{
					"kty": "RSA",
					"use": "sig", // no kid
					"n":   nB64,
					"e":   eB64,
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	v := New("iss", "aud", srv.URL, &http.Client{})
	err := v.refresh(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, ok := v.keys["sig"]; !ok {
		t.Error("expected key to be indexed by 'use' (sig) when kid is missing")
	}
}
