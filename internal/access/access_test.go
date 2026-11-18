package access

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testIssuer = "https://team.cloudflareaccess.com"
	testAUD    = "aud-tag"
)

type fixture struct {
	key     *rsa.PrivateKey
	fetches atomic.Int32
	v       *Verifier
	now     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{key: key, now: time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.fetches.Add(1)
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	t.Cleanup(srv.Close)
	f.v = newVerifier(testIssuer, srv.URL, testAUD, srv.Client())
	f.v.now = func() time.Time { return f.now }
	return f
}

func (f *fixture) token(t *testing.T, header, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signed := enc(header) + "." + enc(claims)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signed + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (f *fixture) claims() map[string]any {
	return map[string]any{
		"iss": testIssuer, "aud": []string{testAUD}, "email": "owner@example.com",
		"exp": f.now.Add(time.Hour).Unix(), "nbf": f.now.Add(-time.Minute).Unix(),
	}
}

var goodHeader = map[string]any{"alg": "RS256", "kid": "k1"}

func TestVerifyAcceptsAValidToken(t *testing.T) {
	f := newFixture(t)
	id, err := f.v.Verify(context.Background(), f.token(t, goodHeader, f.claims()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Email != "owner@example.com" {
		t.Errorf("email = %q", id.Email)
	}
	// A second token uses the cached keys.
	if _, err := f.v.Verify(context.Background(), f.token(t, goodHeader, f.claims())); err != nil {
		t.Fatal(err)
	}
	if n := f.fetches.Load(); n != 1 {
		t.Errorf("keys fetched %d times, want 1", n)
	}
}

func TestVerifyAcceptsAStringAudience(t *testing.T) {
	f := newFixture(t)
	c := f.claims()
	c["aud"] = testAUD
	if _, err := f.v.Verify(context.Background(), f.token(t, goodHeader, c)); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyRefuses(t *testing.T) {
	f := newFixture(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	cases := map[string]func() string{
		"wrong audience": func() string {
			c := f.claims()
			c["aud"] = []string{"another-app"}
			return f.token(t, goodHeader, c)
		},
		"wrong issuer": func() string {
			c := f.claims()
			c["iss"] = "https://evil.cloudflareaccess.com"
			return f.token(t, goodHeader, c)
		},
		"expired": func() string {
			c := f.claims()
			c["exp"] = f.now.Add(-2 * time.Minute).Unix()
			return f.token(t, goodHeader, c)
		},
		"no expiry": func() string {
			c := f.claims()
			delete(c, "exp")
			return f.token(t, goodHeader, c)
		},
		"not yet valid": func() string {
			c := f.claims()
			c["nbf"] = f.now.Add(10 * time.Minute).Unix()
			return f.token(t, goodHeader, c)
		},
		"alg none": func() string {
			tok := f.token(t, map[string]any{"alg": "none", "kid": "k1"}, f.claims())
			return tok[:strings.LastIndex(tok, ".")+1]
		},
		"HS256": func() string { return f.token(t, map[string]any{"alg": "HS256", "kid": "k1"}, f.claims()) },
		"unknown key": func() string {
			return f.token(t, map[string]any{"alg": "RS256", "kid": "k9"}, f.claims())
		},
		"signed by another key": func() string {
			saved := f.key
			f.key = other
			defer func() { f.key = saved }()
			return f.token(t, goodHeader, f.claims())
		},
		"tampered claims": func() string {
			tok := f.token(t, goodHeader, f.claims())
			parts := strings.Split(tok, ".")
			c := f.claims()
			c["email"] = "someone@else.com"
			b, _ := json.Marshal(c)
			parts[1] = base64.RawURLEncoding.EncodeToString(b)
			return strings.Join(parts, ".")
		},
		"garbage": func() string { return "not-a-token" },
	}
	for name, makeToken := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := f.v.Verify(context.Background(), makeToken()); err == nil {
				t.Error("Verify accepted it")
			}
		})
	}
}

func TestUnknownKeyRefetchIsRateLimited(t *testing.T) {
	f := newFixture(t)
	tok := f.token(t, map[string]any{"alg": "RS256", "kid": "k9"}, f.claims())
	for i := 0; i < 5; i++ {
		f.v.Verify(context.Background(), tok)
	}
	if n := f.fetches.Load(); n != 1 {
		t.Errorf("keys fetched %d times within the wait, want 1", n)
	}
	f.now = f.now.Add(refetchMinWait)
	f.v.Verify(context.Background(), tok)
	if n := f.fetches.Load(); n != 2 {
		t.Errorf("keys fetched %d times after the wait, want 2", n)
	}
}

func TestMiddleware(t *testing.T) {
	f := newFixture(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := f.v.Middleware(log, map[string]bool{"/healthz": true}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "inside")
	}))
	do := func(path string, set func(*http.Request)) int {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if set != nil {
			set(req)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	good := f.token(t, goodHeader, f.claims())
	if code := do("/", nil); code != http.StatusForbidden {
		t.Errorf("no token: %d, want 403", code)
	}
	if code := do("/healthz", nil); code != http.StatusOK {
		t.Errorf("health check: %d, want 200", code)
	}
	if code := do("/", func(r *http.Request) { r.Header.Set(HeaderName, good) }); code != http.StatusOK {
		t.Errorf("header token: %d, want 200", code)
	}
	if code := do("/api/v1/overview", func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieName, Value: good}) }); code != http.StatusOK {
		t.Errorf("cookie token: %d, want 200", code)
	}
	if code := do("/", func(r *http.Request) { r.Header.Set(HeaderName, "x.y.z") }); code != http.StatusForbidden {
		t.Errorf("bad token: %d, want 403", code)
	}
}
