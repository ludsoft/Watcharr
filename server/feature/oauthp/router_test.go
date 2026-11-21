package oauthp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sbondCo/Watcharr/config"
	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/internal/testutil"
	"github.com/sbondCo/Watcharr/router"
)

const testSecret = "testsecret0123456789abcdef0123456789"

func setup(t *testing.T) (*gin.Engine, *Service) {
	t.Helper()
	testutil.SetupLogging()
	gin.SetMode(gin.TestMode)
	db := testutil.SetupDB(t)
	// a user to authorize as
	if res := db.Create(&entity.User{Username: "alice", Password: "x"}); res.Error != nil {
		t.Fatalf("create user: %v", res.Error)
	}
	os.Setenv("OAUTH_CLIENTS", `[{"client_id":"postarr","client_secret":"postarr-secret","redirect_uris":["http://localhost:5150/connect/watcharr/callback"],"name":"Postarr","auto_approve":true}]`)
	t.Cleanup(func() { os.Unsetenv("OAUTH_CLIENTS") })
	cfg := &config.ServerConfig{JWT_SECRET: testSecret}
	svc, err := NewService(db, cfg, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	e := gin.New()
	br := router.NewBaseRouter(db, e.Group("/api"), cfg)
	NewRouter(br, svc).AddRoutes()
	e.GET("/.well-known/openid-configuration", svcDiscovery(svc))
	return e, svc
}

func svcDiscovery(s *Service) gin.HandlerFunc {
	r := &Router{s: s}
	return func(c *gin.Context) { r.Discovery(c) }
}

func bearer(userID uint, username string) string {
	j := jwt.NewWithClaims(jwt.SigningMethodHS256, entity.TokenClaims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Issuer:   "watcharr",
		},
	})
	s, _ := j.SignedString([]byte(testSecret))
	return s
}

func do(e *gin.Engine, method, path, auth string, form url.Values) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestDiscovery(t *testing.T) {
	e, _ := setup(t)
	w := do(e, "GET", "/.well-known/openid-configuration", "", nil)
	if w.Code != 200 {
		t.Fatalf("discovery status = %d", w.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["token_endpoint"] == nil || doc["authorization_endpoint"] == nil {
		t.Fatalf("discovery missing endpoints: %v", doc)
	}
}

func TestAuthorizeRedirectsToLoginWhenUnauthenticated(t *testing.T) {
	e, _ := setup(t)
	w := do(e, "GET", "/api/oauth/authorize?response_type=code&client_id=postarr&redirect_uri="+url.QueryEscape("http://localhost:5150/connect/watcharr/callback")+"&state=xyz", "", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/api/oauth/login?return_to=") {
		t.Fatalf("location = %q", loc)
	}
}

func TestAuthorizeAutoApproveIssuesCode(t *testing.T) {
	e, _ := setup(t)
	auth := bearer(1, "alice")
	w := do(e, "GET", "/api/oauth/authorize?response_type=code&client_id=postarr&redirect_uri="+url.QueryEscape("http://localhost:5150/connect/watcharr/callback")+"&state=xyz", auth, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "http://localhost:5150/connect/watcharr/callback?code=") {
		t.Fatalf("location = %q", loc)
	}
	if !strings.Contains(loc, "state=xyz") {
		t.Fatalf("state missing in %q", loc)
	}
}

func TestAuthorizeBadClient(t *testing.T) {
	e, _ := setup(t)
	w := do(e, "GET", "/api/oauth/authorize?response_type=code&client_id=nope&redirect_uri=http://x", bearer(1, "alice"), nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestTokenExchangeHappyPathAndReuse(t *testing.T) {
	e, svc := setup(t)
	code, err := svc.IssueCode("postarr", "http://localhost:5150/connect/watcharr/callback", "openid read", 1)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:5150/connect/watcharr/callback"},
		"client_id":     {"postarr"},
		"client_secret": {"postarr-secret"},
	}
	w := do(e, "POST", "/api/oauth/token", "", form)
	if w.Code != 200 {
		t.Fatalf("token status = %d body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	tok, ok := resp["access_token"].(string)
	if !ok || tok == "" {
		t.Fatalf("no access_token in %v", resp)
	}
	// the token must be a valid watcharr JWT for user 1
	claims := &entity.TokenClaims{}
	_, err = jwt.ParseWithClaims(tok, claims, func(t *jwt.Token) (interface{}, error) { return []byte(testSecret), nil })
	if err != nil || claims.UserID != 1 || claims.Username != "alice" {
		t.Fatalf("bad access token claims: %v %v", claims, err)
	}
	// code is one-time: reuse must fail
	w2 := do(e, "POST", "/api/oauth/token", "", form)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("reuse status = %d, want 400", w2.Code)
	}
}

func TestTokenExchangeWrongSecret(t *testing.T) {
	e, svc := setup(t)
	code, _ := svc.IssueCode("postarr", "http://localhost:5150/connect/watcharr/callback", "read", 1)
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"http://localhost:5150/connect/watcharr/callback"},
		"client_id":     {"postarr"},
		"client_secret": {"wrong"},
	}
	w := do(e, "POST", "/api/oauth/token", "", form)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUserinfo(t *testing.T) {
	e, _ := setup(t)
	w := do(e, "GET", "/api/oauth/userinfo", bearer(1, "alice"), nil)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["sub"] != "1" || body["preferred_username"] != "alice" {
		t.Fatalf("userinfo = %v", body)
	}
}
