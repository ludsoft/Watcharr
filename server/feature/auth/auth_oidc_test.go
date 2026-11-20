package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sbondCo/Watcharr/config"
	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/internal/testutil"
)

const testRedirectURI = "http://watcharr.test/oidc_callback"

// Minimal fake OIDC provider: discovery, jwks, authorize (code issuing is
// simulated by the test) and token endpoint with PKCE verification.
type fakeOIDCProvider struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	sub    string
	claims map[string]any
	// Set by the test from the authorize url, checked at token endpoint.
	challenge string
	nonce     string
	// Override the nonce put into the id token (to test mismatches).
	badNonce bool
}

func newFakeOIDCProvider(t *testing.T) *fakeOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &fakeOIDCProvider{t: t, key: key, sub: "user-123", claims: map[string]any{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.srv.URL,
			"authorization_endpoint":                p.srv.URL + "/authorize",
			"token_endpoint":                        p.srv.URL + "/token",
			"jwks_uri":                              p.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &p.key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"},
		}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "good-code" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != p.challenge {
			http.Error(w, `{"error":"invalid_grant","error_description":"pkce"}`, http.StatusBadRequest)
			return
		}
		if r.Form.Get("redirect_uri") != testRedirectURI {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at",
			"token_type":   "Bearer",
			"id_token":     p.idToken(),
		})
	})
	p.srv = httptest.NewServer(mux)
	t.Cleanup(p.srv.Close)
	return p
}

func (p *fakeOIDCProvider) idToken() string {
	nonce := p.nonce
	if p.badNonce {
		nonce = "wrong"
	}
	c := jwt.MapClaims{
		"iss":   p.srv.URL,
		"sub":   p.sub,
		"aud":   "watcharr",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"nonce": nonce,
	}
	for k, v := range p.claims {
		c[k] = v
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = "k1"
	s, err := tok.SignedString(p.key)
	if err != nil {
		p.t.Fatal(err)
	}
	return s
}

func setupOIDCTest(t *testing.T) (*OIDCService, *fakeOIDCProvider, *config.ServerConfig) {
	t.Helper()
	p := newFakeOIDCProvider(t)
	cfg := &config.ServerConfig{
		JWT_SECRET:      "test-secret",
		DEFAULT_COUNTRY: "US",
		OIDC: config.OIDCSetting{
			Enabled:  true,
			Issuer:   p.srv.URL,
			ClientID: "watcharr",
		},
	}
	db := testutil.SetupDB(t)
	authSvc := NewService(db, cfg, nil)
	return NewOIDCService(db, cfg, authSvc), p, cfg
}

// Run start, simulate the provider redirecting back, then finish.
func doOIDCLogin(t *testing.T, s *OIDCService, p *fakeOIDCProvider) (AuthResponse, error) {
	t.Helper()
	ctx := context.Background()
	start, err := s.StartOIDCLogin(ctx, OIDCStartRequest{RedirectURI: testRedirectURI})
	if err != nil {
		t.Fatalf("StartOIDCLogin: %v", err)
	}
	u, err := url.Parse(start.URL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Fatalf("missing PKCE params in auth url: %s", start.URL)
	}
	if q.Get("redirect_uri") != testRedirectURI {
		t.Fatalf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	p.challenge = q.Get("code_challenge")
	p.nonce = q.Get("nonce")
	return s.FinishOIDCLogin(ctx, OIDCCallbackRequest{Code: "good-code", State: q.Get("state")})
}

func parseClaims(t *testing.T, cfg *config.ServerConfig, tok string) *entity.TokenClaims {
	t.Helper()
	claims := &entity.TokenClaims{}
	_, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) {
		return []byte(cfg.JWT_SECRET), nil
	})
	if err != nil {
		t.Fatalf("returned token invalid: %v", err)
	}
	return claims
}

func TestOIDCLogin_CreatesThenReusesUser(t *testing.T) {
	testutil.SetupLogging()
	s, p, cfg := setupOIDCTest(t)
	p.claims["preferred_username"] = "alice"

	resp, err := doOIDCLogin(t, s, p)
	if err != nil {
		t.Fatalf("first login: %v", err)
	}
	c1 := parseClaims(t, cfg, resp.Token)
	if c1.Username != "alice" || c1.Type != entity.OIDC_USER {
		t.Fatalf("unexpected claims: %+v", c1)
	}

	// Second login with a changed username claim maps to the same user.
	p.claims["preferred_username"] = "alice-renamed"
	resp, err = doOIDCLogin(t, s, p)
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	c2 := parseClaims(t, cfg, resp.Token)
	if c2.UserID != c1.UserID {
		t.Fatalf("second login got user %d, want %d", c2.UserID, c1.UserID)
	}
}

func TestOIDCLogin_UsernameCollisionGetsSuffix(t *testing.T) {
	testutil.SetupLogging()
	s, p, cfg := setupOIDCTest(t)
	p.claims["preferred_username"] = "bob"
	if _, err := doOIDCLogin(t, s, p); err != nil {
		t.Fatal(err)
	}
	p.sub = "another-user"
	resp, err := doOIDCLogin(t, s, p)
	if err != nil {
		t.Fatal(err)
	}
	if c := parseClaims(t, cfg, resp.Token); c.Username != "bob2" {
		t.Fatalf("username = %q, want bob2", c.Username)
	}
}

func TestOIDCLogin_Rejections(t *testing.T) {
	testutil.SetupLogging()

	t.Run("nonce mismatch", func(t *testing.T) {
		s, p, _ := setupOIDCTest(t)
		p.badNonce = true
		if _, err := doOIDCLogin(t, s, p); err == nil {
			t.Fatal("expected error for nonce mismatch")
		}
	})

	t.Run("state reuse", func(t *testing.T) {
		s, p, _ := setupOIDCTest(t)
		ctx := context.Background()
		start, err := s.StartOIDCLogin(ctx, OIDCStartRequest{RedirectURI: testRedirectURI})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(start.URL)
		p.challenge = u.Query().Get("code_challenge")
		p.nonce = u.Query().Get("nonce")
		req := OIDCCallbackRequest{Code: "good-code", State: u.Query().Get("state")}
		if _, err := s.FinishOIDCLogin(ctx, req); err != nil {
			t.Fatalf("first finish: %v", err)
		}
		if _, err := s.FinishOIDCLogin(ctx, req); err == nil {
			t.Fatal("expected error when reusing state")
		}
	})

	t.Run("unknown state", func(t *testing.T) {
		s, _, _ := setupOIDCTest(t)
		_, err := s.FinishOIDCLogin(context.Background(), OIDCCallbackRequest{Code: "good-code", State: "nope"})
		if err == nil {
			t.Fatal("expected error for unknown state")
		}
	})

	t.Run("bad redirect uri", func(t *testing.T) {
		s, _, _ := setupOIDCTest(t)
		for _, ru := range []string{
			"http://watcharr.test/somewhere-else",
			"/oidc_callback",
			"javascript:alert(1)//oidc_callback",
			"http://watcharr.test/oidc_callback?x=1",
		} {
			if _, err := s.StartOIDCLogin(context.Background(), OIDCStartRequest{RedirectURI: ru}); err == nil {
				t.Errorf("expected error for redirect uri %q", ru)
			}
		}
	})

	t.Run("disabled", func(t *testing.T) {
		s, _, cfg := setupOIDCTest(t)
		cfg.OIDC.Enabled = false
		if _, err := s.StartOIDCLogin(context.Background(), OIDCStartRequest{RedirectURI: testRedirectURI}); err == nil {
			t.Fatal("expected error when disabled")
		}
	})
}
