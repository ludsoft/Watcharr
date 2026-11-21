package oauthp

import (
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/router"
)

type Router struct {
	br *router.BaseRouter
	s  *Service
}

func NewRouter(br *router.BaseRouter, s *Service) *Router {
	return &Router{br: br, s: s}
}

func (r *Router) AddRoutes() {
	g := r.br.Router.Group("/oauth")

	// Discovery (public).
	g.GET("/.well-known/openid-configuration", r.Discovery)
	// Start of the flow + consent (browser, cookie or bearer auth).
	g.GET("/authorize", r.Authorize)
	g.POST("/authorize", r.AuthorizeApprove)
	// Built-in login page used when the browser has no session yet.
	g.GET("/login", r.LoginPage)
	g.POST("/login", r.Login)
	// Server-to-server code exchange (public, client authenticates by secret).
	g.POST("/token", r.Token)
	// Resource-server userinfo (bearer).
	g.GET("/userinfo", r.Userinfo)
	g.GET("/jwks", r.Jwks)
}

// ---- helpers ----

type authorizeParams struct {
	ResponseType string `form:"response_type" binding:"required"`
	ClientID     string `form:"client_id" binding:"required"`
	RedirectURI  string `form:"redirect_uri" binding:"required"`
	Scope        string `form:"scope"`
	State        string `form:"state"`
}

func (p authorizeParams) validate(s *Service) (*OauthClient, error) {
	if p.ResponseType != "code" {
		return nil, errors.New("unsupported_response_type")
	}
	client, err := s.FindClient(p.ClientID)
	if err != nil {
		return nil, err
	}
	if !client.ValidateRedirectURI(p.RedirectURI) {
		return nil, ErrRedirectMismatch
	}
	return client, nil
}

// currentUser resolves the browser user from the Authorization header or the
// session cookie set by the built-in login page.
func (r *Router) currentUser(c *gin.Context) (*entity.TokenClaims, bool) {
	raw := c.GetHeader("Authorization")
	if raw == "" {
		if ck, err := c.Cookie(SessionCookie); err == nil {
			raw = ck
		}
	}
	if raw == "" {
		return nil, false
	}
	claims := &entity.TokenClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(r.br.Cfg.JWT_SECRET), nil
	})
	if err != nil || claims.UserID == 0 {
		return nil, false
	}
	return claims, true
}

func oauthError(c *gin.Context, redirectURI, state, errCode, desc string) {
	// If we have a validated redirect target, send the error there (OAuth2 spec);
	// otherwise render a plain 400.
	if redirectURI != "" {
		q := url.Values{}
		q.Set("error", errCode)
		q.Set("error_description", desc)
		if state != "" {
			q.Set("state", state)
		}
		c.Redirect(http.StatusFound, redirectURI+"?"+q.Encode())
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": errCode, "error_description": desc})
}

// ---- discovery ----

func (r *Router) Discovery(c *gin.Context) {
	base := "http://" + c.Request.Host
	if c.Request.TLS != nil {
		base = "https://" + c.Request.Host
	}
	c.JSON(http.StatusOK, gin.H{
		"issuer":                                base,
		"authorization_endpoint":                base + "/api/oauth/authorize",
		"token_endpoint":                        base + "/api/oauth/token",
		"userinfo_endpoint":                     base + "/api/oauth/userinfo",
		"jwks_uri":                              base + "/api/oauth/jwks",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"HS256"},
		"scopes_supported":                      []string{"openid", "read"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic", "none"},
		"claims_supported":                      []string{"sub", "preferred_username"},
	})
}

// ---- authorize ----

func (r *Router) Authorize(c *gin.Context) {
	var p authorizeParams
	if err := c.ShouldBindQuery(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	client, err := p.validate(r.s)
	if err != nil {
		oauthError(c, "", p.State, "invalid_client", err.Error())
		return
	}
	claims, authed := r.currentUser(c)
	if !authed {
		// Send the browser to our built-in login page, then back here.
		returnTo := c.Request.URL.RequestURI()
		c.Redirect(http.StatusFound, "/api/oauth/login?return_to="+url.QueryEscape(returnTo))
		return
	}
	if client.AutoApprove {
		r.issueAndRedirect(c, client, p, claims.UserID)
		return
	}
	// Consent page.
	renderPage(c, consentTemplate, gin.H{
		"ClientName": client.Name,
		"Scope":      p.Scope,
		"Params":     p,
	})
}

func (r *Router) AuthorizeApprove(c *gin.Context) {
	var p authorizeParams
	if err := c.ShouldBind(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	client, err := p.validate(r.s)
	if err != nil {
		oauthError(c, "", p.State, "invalid_client", err.Error())
		return
	}
	claims, authed := r.currentUser(c)
	if !authed {
		c.Redirect(http.StatusFound, "/api/oauth/login?return_to="+url.QueryEscape(c.Request.URL.RequestURI()))
		return
	}
	r.issueAndRedirect(c, client, p, claims.UserID)
}

func (r *Router) issueAndRedirect(c *gin.Context, client *OauthClient, p authorizeParams, userID uint) {
	code, err := r.s.IssueCode(client.ClientID, p.RedirectURI, p.Scope, userID)
	if err != nil {
		oauthError(c, p.RedirectURI, p.State, "server_error", err.Error())
		return
	}
	q := url.Values{}
	q.Set("code", code)
	if p.State != "" {
		q.Set("state", p.State)
	}
	c.Redirect(http.StatusFound, p.RedirectURI+"?"+q.Encode())
}

// ---- built-in login ----

func (r *Router) LoginPage(c *gin.Context) {
	returnTo := c.Query("return_to")
	renderPage(c, loginTemplate, gin.H{"ReturnTo": returnTo})
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	ReturnTo string `json:"return_to"`
}

func (r *Router) Login(c *gin.Context) {
	var lr loginRequest
	if err := c.ShouldBindJSON(&lr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	resp, err := r.s.auth.Login(&entity.User{Username: lr.Username, Password: lr.Password})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_grant", "error_description": "bad credentials"})
		return
	}
	c.SetCookie(SessionCookie, resp.Token, 3600, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"redirect": lr.ReturnTo})
}

// ---- token ----

type tokenRequest struct {
	GrantType    string `form:"grant_type" binding:"required"`
	Code         string `form:"code" binding:"required"`
	RedirectURI  string `form:"redirect_uri" binding:"required"`
	ClientID     string `form:"client_id" binding:"required"`
	ClientSecret string `form:"client_secret"`
}

func (r *Router) Token(c *gin.Context) {
	var tr tokenRequest
	if err := c.ShouldBind(&tr); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "error_description": err.Error()})
		return
	}
	if tr.GrantType != "authorization_code" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported_grant_type"})
		return
	}
	client, err := r.s.FindClient(tr.ClientID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_client"})
		return
	}
	token, scope, err := r.s.ExchangeCode(client, tr.ClientSecret, tr.Code, tr.RedirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_grant", "error_description": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   604800,
		"scope":        scope,
		// HS256: the id_token is the same signed JWT (aud = client).
		"id_token": token,
	})
}

// ---- userinfo / jwks ----

func (r *Router) Userinfo(c *gin.Context) {
	raw := c.GetHeader("Authorization")
	claims := &entity.TokenClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(r.br.Cfg.JWT_SECRET), nil
	})
	if err != nil || claims.UserID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sub":                strconv.FormatUint(uint64(claims.UserID), 10),
		"preferred_username": claims.Username,
	})
}

func (r *Router) Jwks(c *gin.Context) {
	// HS256 is symmetric; no public keys to publish.
	c.JSON(http.StatusOK, gin.H{"keys": []any{}})
}

// ---- pages ----

func renderPage(c *gin.Context, tpl string, data any) {
	t, err := template.New("page").Parse(tpl)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	if err := t.Execute(c.Writer, data); err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
	}
}

const loginTemplate = `<!doctype html>
<html><head><meta charset="utf-8"><title>Watcharr sign in</title>
<style>body{font-family:system-ui,sans-serif;background:#121417;color:#e8eaf0;display:grid;place-items:center;min-height:100vh;margin:0}form{background:#1b1f27;padding:2rem;border-radius:10px;display:grid;gap:.75rem;width:280px}input{padding:.6rem;border-radius:6px;border:1px solid #333a47;background:#121417;color:inherit}button{padding:.6rem;border:0;border-radius:6px;background:#4f8cff;color:#fff;cursor:pointer}h1{font-size:1.2rem;margin:0}</style>
</head><body>
<form id="f"><h1>Watcharr sign in</h1>
<input name="username" placeholder="Username" required autofocus>
<input name="password" type="password" placeholder="Password" required>
<button>Sign in</button><p id="err" style="color:#ff7a7a"></p></form>
<script>
document.getElementById('f').onsubmit = async (e) => {
  e.preventDefault();
  const fd = new FormData(e.target);
  const r = await fetch('/api/oauth/login', {method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({username: fd.get('username'), password: fd.get('password'), return_to: {{printf "%q" .ReturnTo}} })});
  if (r.ok) { window.location = (await r.json()).redirect || '/'; }
  else { document.getElementById('err').textContent = 'Invalid credentials'; }
};
</script>
</body></html>`

const consentTemplate = `<!doctype html>
<html><head><meta charset="utf-8"><title>Authorize {{.ClientName}}</title>
<style>body{font-family:system-ui,sans-serif;background:#121417;color:#e8eaf0;display:grid;place-items:center;min-height:100vh;margin:0}.card{background:#1b1f27;padding:2rem;border-radius:10px;width:320px}button{padding:.6rem;border:0;border-radius:6px;background:#4f8cff;color:#fff;cursor:pointer;width:100%}a{color:#9aa3b2}</style>
</head><body><div class="card">
<h1 style="font-size:1.2rem">{{.ClientName}} wants access</h1>
<p>Scopes: {{if .Scope}}{{.Scope}}{{else}}read{{end}}</p>
<form method="POST" action="/api/oauth/authorize">
<input type="hidden" name="response_type" value="{{.Params.ResponseType}}">
<input type="hidden" name="client_id" value="{{.Params.ClientID}}">
<input type="hidden" name="redirect_uri" value="{{.Params.RedirectURI}}">
<input type="hidden" name="scope" value="{{.Params.Scope}}">
<input type="hidden" name="state" value="{{.Params.State}}">
<button>Allow</button>
</form></div></body></html>`
