// Package oauthp turns Watcharr into a minimal OAuth2 / OpenID Connect
// **provider** (authorization server), so third-party apps (eg postarr) can
// access a user's Watcharr data *on the user's behalf*:
//
//	browser → GET  /api/oauth/authorize?client_id=...&redirect_uri=...   (login+consent)
//	        → 303 redirect_uri?code=...&state=...
//	server  → POST /api/oauth/token (grant_type=authorization_code, code, client_secret)
//	        → 200 { "access_token": "<watcharr JWT for that user>", ... }
//
// The access token is an ordinary Watcharr login JWT (HS256, issuer "watcharr",
// claims userId/username/type), so every existing AuthRequired route authorizes
// the app exactly as the user themselves — no separate permission model.
//
// Clients are registered statically via the OAUTH_CLIENTS env var (JSON):
//
//	OAUTH_CLIENTS='[{"client_id":"postarr","client_secret":"s3cret",
//	  "redirect_uris":["https://postarr.example.com/connect/watcharr/callback"],
//	  "name":"Postarr","auto_approve":true}]'
//
// Endpoints:
//
//	GET  /.well-known/openid-configuration   discovery document
//	GET  /api/oauth/authorize                start of the flow (browser)
//	POST /api/oauth/authorize                consent approval (form)
//	GET  /api/oauth/login                    built-in login page (browser)
//	POST /api/oauth/login                    credentials → session cookie
//	POST /api/oauth/token                    code exchange (server-to-server)
//	GET  /api/oauth/userinfo                 Bearer → {sub, preferred_username}
//	GET  /api/oauth/jwks                     empty (HS256 symmetric signing)
package oauthp

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sbondCo/Watcharr/config"
	"github.com/sbondCo/Watcharr/database/entity"
	"github.com/sbondCo/Watcharr/feature/auth"
	"github.com/sbondCo/Watcharr/util"
	"gorm.io/gorm"
)

// OauthClient is one statically registered OAuth2 client.
type OauthClient struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURIs []string `json:"redirect_uris"`
	Name         string   `json:"name"`
	// AutoApprove skips the consent page for this (trusted, first-party) client.
	AutoApprove bool `json:"auto_approve"`
}

// CodeTTL is how long an authorization code may be exchanged.
const CodeTTL = 10 * time.Minute

// SessionCookie is the browser cookie set by the built-in login page.
const SessionCookie = "watcharr_oauth_session"

var (
	ErrClientNotFound   = errors.New("unknown client_id")
	ErrRedirectMismatch = errors.New("redirect_uri is not registered for this client")
	ErrBadGrant         = errors.New("invalid grant")
)

type Service struct {
	db     *gorm.DB
	cfg    *config.ServerConfig
	auth   *auth.Service
	clients []OauthClient
}

func NewService(db *gorm.DB, cfg *config.ServerConfig, authService *auth.Service) (*Service, error) {
	s := &Service{db: db, cfg: cfg, auth: authService}
	raw := os.Getenv("OAUTH_CLIENTS")
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &s.clients); err != nil {
			return nil, errors.New("OAUTH_CLIENTS is not valid JSON: " + err.Error())
		}
	}
	return s, nil
}

func (s *Service) Clients() []OauthClient { return s.clients }

// FindClient returns the client if clientID is registered.
func (s *Service) FindClient(clientID string) (*OauthClient, error) {
	for i := range s.clients {
		if s.clients[i].ClientID == clientID {
			return &s.clients[i], nil
		}
	}
	return nil, ErrClientNotFound
}

// ValidateRedirectURI checks the redirect_uri against the client's list.
func (c *OauthClient) ValidateRedirectURI(redirectURI string) bool {
	for _, u := range c.RedirectURIs {
		if u == redirectURI {
			return true
		}
	}
	return false
}

// IssueCode creates a one-time authorization code for userID.
func (s *Service) IssueCode(clientID, redirectURI, scope string, userID uint) (string, error) {
	code, err := util.GenerateString(32)
	if err != nil {
		return "", err
	}
	rec := entity.OauthAuthCode{
		Code:        code,
		ClientID:    clientID,
		RedirectURI: redirectURI,
		Scope:       scope,
		UserID:      userID,
		ExpiresAt:   time.Now().Add(CodeTTL),
	}
	if res := s.db.Create(&rec); res.Error != nil {
		return "", res.Error
	}
	return code, nil
}

// ExchangeCode validates the code and returns a signed Watcharr JWT for its user.
func (s *Service) ExchangeCode(client *OauthClient, clientSecret, code, redirectURI string) (string, string, error) {
	if client.ClientSecret != "" && clientSecret != client.ClientSecret {
		return "", "", ErrBadGrant
	}
	var rec entity.OauthAuthCode
	res := s.db.Where("code = ?", code).First(&rec)
	if res.Error != nil {
		return "", "", ErrBadGrant
	}
	if rec.Used || time.Now().After(rec.ExpiresAt) || rec.ClientID != client.ClientID || rec.RedirectURI != redirectURI {
		return "", "", ErrBadGrant
	}
	// one-time use
	if res := s.db.Model(&rec).Update("used", true); res.Error != nil {
		return "", "", res.Error
	}
	var user entity.User
	if res := s.db.Select("id", "username", "type").First(&user, rec.UserID); res.Error != nil {
		return "", "", res.Error
	}
	token, err := s.signJWT(user.ID, user.Username, user.Type)
	if err != nil {
		return "", "", err
	}
	return token, rec.Scope, nil
}

// signJWT mirrors auth.Service.signJWT so OAuth access tokens are ordinary
// Watcharr login tokens (same secret, issuer, claims) — every AuthRequired
// route then authorizes the third-party app exactly as the user themselves.
func (s *Service) signJWT(userID uint, userName string, userType entity.UserType) (string, error) {
	j := jwt.NewWithClaims(jwt.SigningMethodHS256, entity.TokenClaims{
		UserID:   userID,
		Username: userName,
		Type:     userType,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(time.Now()),
			Issuer:   "watcharr",
		},
	})
	return j.SignedString([]byte(s.cfg.JWT_SECRET))
}
