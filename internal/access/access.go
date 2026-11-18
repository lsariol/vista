// Package access lets through only requests signed in by Cloudflare Access.
//
// Cloudflare Access sits in front of Vista's hostname: it signs the visitor in,
// then forwards the request with a JWT in the Cf-Access-Jwt-Assertion header.
// Vista checks that token itself as well, so a request that reaches it some
// other way (another container on the network, a misrouted hostname) gets nothing.
package access

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HeaderName is where Cloudflare Access puts the signed token.
const HeaderName = "Cf-Access-Jwt-Assertion"

// cookieName holds the same token when a browser makes the request directly.
const cookieName = "CF_Authorization"

const (
	keysMaxAge     = time.Hour        // refetch the signing keys at least this often
	refetchMinWait = 30 * time.Second // and no more often than this for an unknown key
	clockSkew      = time.Minute
)

// Verifier checks Cloudflare Access tokens for one application.
type Verifier struct {
	issuer   string
	certsURL string
	audience string
	client   *http.Client
	now      func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
	triedAt   time.Time
}

// NewVerifier returns a Verifier for the team <team>.cloudflareaccess.com and
// the application whose audience tag is aud.
func NewVerifier(team, aud string) *Verifier {
	issuer := "https://" + team + ".cloudflareaccess.com"
	return newVerifier(issuer, issuer+"/cdn-cgi/access/certs", aud, &http.Client{Timeout: 5 * time.Second})
}

func newVerifier(issuer, certsURL, aud string, client *http.Client) *Verifier {
	return &Verifier{issuer: issuer, certsURL: certsURL, audience: aud, client: client, now: time.Now}
}

// Identity is who Cloudflare Access signed in.
type Identity struct {
	Email string
}

// errNoToken means the request carried no token at all.
var errNoToken = errors.New("no Cloudflare Access token on the request")

// Verify checks a token and returns who it belongs to.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("the token isn't a JWT")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := decodeSegment(parts[0], &header); err != nil {
		return Identity{}, fmt.Errorf("the token's header: %w", err)
	}
	if header.Alg != "RS256" {
		return Identity{}, fmt.Errorf("the token is signed with %q; only RS256 is accepted", header.Alg)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, errors.New("the token's signature isn't base64url")
	}
	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return Identity{}, err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return Identity{}, errors.New("the token's signature doesn't match Cloudflare's key")
	}

	var claims struct {
		Iss   string   `json:"iss"`
		Aud   audience `json:"aud"`
		Exp   *int64   `json:"exp"`
		Nbf   *int64   `json:"nbf"`
		Email string   `json:"email"`
	}
	if err := decodeSegment(parts[1], &claims); err != nil {
		return Identity{}, fmt.Errorf("the token's claims: %w", err)
	}
	now := v.now()
	switch {
	case claims.Iss != v.issuer:
		return Identity{}, fmt.Errorf("the token was issued by %q, not %q", claims.Iss, v.issuer)
	case !claims.Aud.has(v.audience):
		return Identity{}, errors.New("the token is for another Access application (audience tag differs)")
	case claims.Exp == nil:
		return Identity{}, errors.New("the token has no expiry")
	case now.After(time.Unix(*claims.Exp, 0).Add(clockSkew)):
		return Identity{}, errors.New("the token has expired")
	case claims.Nbf != nil && now.Add(clockSkew).Before(time.Unix(*claims.Nbf, 0)):
		return Identity{}, errors.New("the token isn't valid yet")
	}
	return Identity{Email: claims.Email}, nil
}

// key returns the signing key kid, fetching Cloudflare's keys when they're old
// or don't include it.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	now := v.now()
	if k, ok := v.keys[kid]; ok && now.Sub(v.fetchedAt) < keysMaxAge {
		return k, nil
	}
	if now.Sub(v.triedAt) >= refetchMinWait || v.keys == nil {
		v.triedAt = now
		keys, err := v.fetchKeys(ctx)
		if err != nil {
			if k, ok := v.keys[kid]; ok {
				return k, nil // keep using a known key while Cloudflare is unreachable
			}
			return nil, err
		}
		v.keys, v.fetchedAt = keys, now
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("the token's key %q isn't one of Cloudflare's current keys", kid)
}

func (v *Verifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare Access: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Cloudflare Access: fetching the signing keys: %w; check that Vista can reach %s", err, v.certsURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cloudflare Access: %s answered %s; check VISTA_ACCESS_TEAM", v.certsURL, resp.Status)
	}
	var body struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("Cloudflare Access: reading the signing keys: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(body.Keys))
	for _, k := range body.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, errN := base64.RawURLEncoding.DecodeString(k.N)
		e, errE := base64.RawURLEncoding.DecodeString(k.E)
		if errN != nil || errE != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("Cloudflare Access: %s listed no RSA keys", v.certsURL)
	}
	return keys, nil
}

// Middleware lets a request through only with a valid token. Paths in open
// (the health check) need none.
func (v *Verifier) Middleware(log *slog.Logger, open map[string]bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if open[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}
		token := r.Header.Get(HeaderName)
		if token == "" {
			if c, err := r.Cookie(cookieName); err == nil {
				token = c.Value
			}
		}
		err := errNoToken
		if token != "" {
			_, err = v.Verify(r.Context(), token)
		}
		if err != nil {
			log.Warn("request refused", "component", "access", "path", r.URL.Path,
				"client_ip", r.Header.Get("CF-Connecting-IP"), "err", err)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, "Vista is private. Sign in through Cloudflare Access at its address.\n")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// audience is the aud claim, which is a string or a list of strings.
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return errors.New("aud is neither a string nor a list")
	}
	*a = many
	return nil
}

func (a audience) has(want string) bool {
	for _, s := range a {
		if s == want {
			return true
		}
	}
	return false
}

func decodeSegment(seg string, into any) error {
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return errors.New("not base64url")
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return errors.New("not JSON")
	}
	return nil
}
