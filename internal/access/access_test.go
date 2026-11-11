package access

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const (
	testIssuer   = "https://team.example.com"
	testAudience = "aud-123"
)

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func sign(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	h, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": kid})
	c, _ := json.Marshal(claims)
	signing := b64(h) + "." + b64(c)
	d := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

func newTestVerifier(t *testing.T, key *rsa.PrivateKey) *Verifier {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA",
			"n": b64(key.N.Bytes()), "e": b64([]byte{1, 0, 1}),
		}}})
	}))
	t.Cleanup(srv.Close)
	v := NewVerifier("team.example.com", testAudience)
	v.certsURL = srv.URL
	return v
}

func validClaims() map[string]any {
	return map[string]any{
		"iss": testIssuer, "aud": []string{testAudience},
		"exp": time.Now().Add(time.Hour).Unix(), "email": "a@example.com",
	}
}

func TestVerify(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	v := newTestVerifier(t, key)

	mod := func(k string, val any) map[string]any {
		c := validClaims()
		c[k] = val
		return c
	}
	tests := []struct {
		name    string
		token   string
		wantErr bool
	}{
		{"valid", sign(t, key, "k1", validClaims()), false},
		{"valid string audience", sign(t, key, "k1", mod("aud", testAudience)), false},
		{"wrong key", sign(t, other, "k1", validClaims()), true},
		{"unknown kid", sign(t, key, "k2", validClaims()), true},
		{"wrong audience", sign(t, key, "k1", mod("aud", []string{"other"})), true},
		{"wrong issuer", sign(t, key, "k1", mod("iss", "https://evil.example.com")), true},
		{"expired", sign(t, key, "k1", mod("exp", time.Now().Add(-time.Minute).Unix())), true},
		{"no email", sign(t, key, "k1", mod("email", "")), true},
		{"garbage", "not.a.jwt", true},
		{"alg none", strings.Join([]string{b64([]byte(`{"alg":"none"}`)), b64([]byte(`{}`)), ""}, "."), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := v.Verify(tc.token)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && c.Email != "a@example.com" {
				t.Fatalf("email = %q", c.Email)
			}
		})
	}
}

func TestMiddlewareIgnoresForgedEmailHeader(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	v := newTestVerifier(t, key)
	h := v.Middleware(func(w http.ResponseWriter, r *http.Request, c *Claims) {})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Cf-Access-Authenticated-User-Email", "admin@example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rec.Code)
	}
}
