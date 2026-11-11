// Package access verifies the signed JWT that Cloudflare Access adds to every
// request in the header Cf-Access-Jwt-Assertion.
package access

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Header is the request header that carries the Access JWT.
const Header = "Cf-Access-Jwt-Assertion"

// Claims are the verified claims the application uses.
type Claims struct {
	Email string
}

// Verifier checks Access JWTs for one team domain and one application audience.
type Verifier struct {
	issuer   string
	audience string
	certsURL string
	client   *http.Client
	now      func() time.Time

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

// NewVerifier returns a verifier for the team domain (for example
// "team.cloudflareaccess.com") and the audience tag of the application.
func NewVerifier(teamDomain, audience string) *Verifier {
	issuer := "https://" + teamDomain
	return &Verifier{
		issuer:   issuer,
		audience: audience,
		certsURL: issuer + "/cdn-cgi/access/certs",
		client:   &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
	}
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtClaims struct {
	Iss   string          `json:"iss"`
	Aud   json.RawMessage `json:"aud"`
	Exp   int64           `json:"exp"`
	Nbf   int64           `json:"nbf"`
	Email string          `json:"email"`
}

// Verify checks signature, issuer, audience and expiry of the token.
func (v *Verifier) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed token")
	}
	var h jwtHeader
	if err := decodeJSON(parts[0], &h); err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	if h.Alg != "RS256" {
		return nil, errors.New("unsupported algorithm")
	}
	key, err := v.key(h.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("bad signature encoding")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return nil, errors.New("signature mismatch")
	}
	var c jwtClaims
	if err := decodeJSON(parts[1], &c); err != nil {
		return nil, fmt.Errorf("claims: %w", err)
	}
	if c.Iss != v.issuer {
		return nil, errors.New("wrong issuer")
	}
	if !audienceMatches(c.Aud, v.audience) {
		return nil, errors.New("wrong audience")
	}
	now := v.now().Unix()
	if c.Exp == 0 || now >= c.Exp {
		return nil, errors.New("token expired")
	}
	if c.Nbf != 0 && now < c.Nbf {
		return nil, errors.New("token not yet valid")
	}
	if c.Email == "" {
		return nil, errors.New("no e-mail claim")
	}
	return &Claims{Email: c.Email}, nil
}

// Middleware rejects requests without a valid Access JWT with 401 and passes
// the verified claims to the handler.
func (v *Verifier) Middleware(next func(http.ResponseWriter, *http.Request, *Claims)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get(Header)
		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		c, err := v.Verify(tok)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r, c)
	})
}

func audienceMatches(raw json.RawMessage, want string) bool {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		for _, a := range many {
			if a == want {
				return true
			}
		}
	}
	return false
}

func decodeJSON(seg string, out any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

const keyCacheTTL = time.Hour

// key returns the public key for kid and refreshes the key set when the key is
// unknown or the cache is older than keyCacheTTL. Refreshes for unknown keys
// are limited to one per minute.
func (v *Verifier) key(kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	age := v.now().Sub(v.fetched)
	if k, ok := v.keys[kid]; ok && age < keyCacheTTL {
		return k, nil
	}
	if v.keys == nil || age >= keyCacheTTL || age >= time.Minute {
		if err := v.refreshLocked(); err != nil {
			if k, ok := v.keys[kid]; ok {
				return k, nil
			}
			return nil, err
		}
	}
	k, ok := v.keys[kid]
	if !ok {
		return nil, errors.New("unknown key id")
	}
	return k, nil
}

func (v *Verifier) refreshLocked() error {
	resp, err := v.client.Get(v.certsURL)
	if err != nil {
		return fmt.Errorf("fetch keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch keys: status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decode keys: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	v.keys = keys
	v.fetched = v.now()
	return nil
}
