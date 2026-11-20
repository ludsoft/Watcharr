// auth_oidc contains the logic for OpenID Connect authentication.
//
// Flow (authorization code + PKCE):
//  1. Frontend calls `StartOIDCLogin` with the redirect uri (our
//     frontend `/oidc_callback` page). We generate state, nonce and
//     a PKCE verifier, remember them in memory and return the
//     provider's authorization url.
//  2. Provider redirects the browser back to our frontend, which posts
//     the `code` and `state` to `FinishOIDCLogin`.
//  3. We exchange the code, verify the id token (signature, issuer,
//     audience, expiry and nonce), find/create the user and return a
//     Watcharr JWT, just like every other login method.

package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/sbondCo/Watcharr/config"
	"github.com/sbondCo/Watcharr/database/entity"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// Path of the frontend page the provider must redirect back to.
const OIDCCallbackPath = "/oidc_callback"

// How long a user has to complete the login at their provider.
const oidcPendingLoginTTL = 10 * time.Minute

type OIDCStartRequest struct {
	RedirectURI string `json:"redirectUri" binding:"required"`
}

type OIDCStartResponse struct {
	URL string `json:"url"`
}

type OIDCCallbackRequest struct {
	Code  string `json:"code" binding:"required"`
	State string `json:"state" binding:"required"`
}

// A login that has been started, but not finished yet.
type oidcPendingLogin struct {
	nonce       string
	verifier    string
	redirectURI string
	expiresAt   time.Time
}

// Discovered provider, cached so we don't hit the discovery
// endpoint on every login.
type oidcProvider struct {
	issuer   string
	clientID string
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
}

type OIDCService struct {
	db          *gorm.DB
	cfg         *config.ServerConfig
	authService *Service

	mu       sync.Mutex
	pending  map[string]oidcPendingLogin
	provider *oidcProvider
}

func NewOIDCService(db *gorm.DB, cfg *config.ServerConfig, authService *Service) *OIDCService {
	return &OIDCService{
		db:          db,
		cfg:         cfg,
		authService: authService,
		pending:     map[string]oidcPendingLogin{},
	}
}

// Is OIDC auth configured on this server?
func (s *OIDCService) OIDCIsEnabled() bool {
	return s.cfg.OIDC.Enabled &&
		s.cfg.OIDC.Issuer != "" &&
		s.cfg.OIDC.ClientID != ""
}

// Name to show on the login button.
func (s *OIDCService) ButtonName() string {
	if s.cfg.OIDC.ButtonName != "" {
		return s.cfg.OIDC.ButtonName
	}
	return "Single Sign-On"
}

// Get (and lazily discover) our provider.
// Discovery is retried on the next login if it fails, so an
// unavailable provider never stops Watcharr from starting.
func (s *OIDCService) getProvider(ctx context.Context) (*oidcProvider, error) {
	s.mu.Lock()
	p := s.provider
	s.mu.Unlock()
	if p != nil && p.issuer == s.cfg.OIDC.Issuer && p.clientID == s.cfg.OIDC.ClientID {
		return p, nil
	}
	provider, err := oidc.NewProvider(ctx, s.cfg.OIDC.Issuer)
	if err != nil {
		return nil, err
	}
	p = &oidcProvider{
		issuer:   s.cfg.OIDC.Issuer,
		clientID: s.cfg.OIDC.ClientID,
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: s.cfg.OIDC.ClientID}),
	}
	s.mu.Lock()
	s.provider = p
	s.mu.Unlock()
	return p, nil
}

func (s *OIDCService) oauth2Config(p *oidcProvider, redirectURI string) *oauth2.Config {
	scopes := s.cfg.OIDC.Scopes
	if len(scopes) == 0 {
		scopes = []string{"profile", "email"}
	}
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	return &oauth2.Config{
		ClientID:     s.cfg.OIDC.ClientID,
		ClientSecret: s.cfg.OIDC.ClientSecret,
		Endpoint:     p.provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}
}

// Ensure the redirect uri points to our frontend callback page.
// The provider is what really enforces allowed redirect uris, this is
// just a sanity check so we don't build auth urls to random pages.
func validateOIDCRedirectURI(ru string) error {
	u, err := url.Parse(ru)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("redirect uri must be http(s)")
	}
	if u.Host == "" {
		return errors.New("redirect uri must be absolute")
	}
	if !strings.HasSuffix(u.Path, OIDCCallbackPath) || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("redirect uri must point to %s", OIDCCallbackPath)
	}
	return nil
}

// Start an OIDC login, returning the url to send the user to.
func (s *OIDCService) StartOIDCLogin(ctx context.Context, req OIDCStartRequest) (OIDCStartResponse, error) {
	if !s.OIDCIsEnabled() {
		return OIDCStartResponse{}, errors.New("oidc login not enabled")
	}
	if err := validateOIDCRedirectURI(req.RedirectURI); err != nil {
		slog.Error("StartOIDCLogin: Invalid redirect uri", "redirect_uri", req.RedirectURI, "error", err)
		return OIDCStartResponse{}, errors.New("invalid redirect uri")
	}
	p, err := s.getProvider(ctx)
	if err != nil {
		slog.Error("StartOIDCLogin: Failed to discover OIDC provider", "issuer", s.cfg.OIDC.Issuer, "error", err)
		return OIDCStartResponse{}, errors.New("failed to reach oidc provider")
	}
	state := oauth2.GenerateVerifier()
	nonce := oauth2.GenerateVerifier()
	verifier := oauth2.GenerateVerifier()

	s.mu.Lock()
	now := time.Now()
	for k, v := range s.pending {
		if now.After(v.expiresAt) {
			delete(s.pending, k)
		}
	}
	s.pending[state] = oidcPendingLogin{
		nonce:       nonce,
		verifier:    verifier,
		redirectURI: req.RedirectURI,
		expiresAt:   now.Add(oidcPendingLoginTTL),
	}
	s.mu.Unlock()

	authURL := s.oauth2Config(p, req.RedirectURI).AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
	return OIDCStartResponse{URL: authURL}, nil
}

// Finish an OIDC login, returning a Watcharr token.
func (s *OIDCService) FinishOIDCLogin(ctx context.Context, req OIDCCallbackRequest) (AuthResponse, error) {
	if !s.OIDCIsEnabled() {
		return AuthResponse{}, errors.New("oidc login not enabled")
	}
	// Each state can only be used once.
	s.mu.Lock()
	pl, ok := s.pending[req.State]
	delete(s.pending, req.State)
	s.mu.Unlock()
	if !ok || time.Now().After(pl.expiresAt) {
		slog.Error("FinishOIDCLogin: Unknown or expired state")
		return AuthResponse{}, errors.New("login expired, please try again")
	}
	p, err := s.getProvider(ctx)
	if err != nil {
		slog.Error("FinishOIDCLogin: Failed to discover OIDC provider", "issuer", s.cfg.OIDC.Issuer, "error", err)
		return AuthResponse{}, errors.New("failed to reach oidc provider")
	}
	oauthToken, err := s.oauth2Config(p, pl.redirectURI).Exchange(
		ctx,
		req.Code,
		oauth2.VerifierOption(pl.verifier),
	)
	if err != nil {
		slog.Error("FinishOIDCLogin: Code exchange failed", "error", err)
		return AuthResponse{}, errors.New("failed to exchange code")
	}
	rawIDToken, ok := oauthToken.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		slog.Error("FinishOIDCLogin: No id_token in token response")
		return AuthResponse{}, errors.New("no id token received")
	}
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		slog.Error("FinishOIDCLogin: Failed to verify id token", "error", err)
		return AuthResponse{}, errors.New("invalid id token")
	}
	if idToken.Nonce != pl.nonce {
		slog.Error("FinishOIDCLogin: Nonce mismatch")
		return AuthResponse{}, errors.New("invalid id token")
	}
	claims := map[string]any{}
	if err := idToken.Claims(&claims); err != nil {
		slog.Error("FinishOIDCLogin: Failed to parse id token claims", "error", err)
		return AuthResponse{}, errors.New("invalid id token")
	}
	// Many providers only put profile claims in the userinfo response.
	if ui, err := p.provider.UserInfo(ctx, oauth2.StaticTokenSource(oauthToken)); err == nil && ui.Subject == idToken.Subject {
		uiClaims := map[string]any{}
		if err := ui.Claims(&uiClaims); err == nil {
			for k, v := range uiClaims {
				if _, exists := claims[k]; !exists {
					claims[k] = v
				}
			}
		}
	} else if err != nil {
		slog.Debug("FinishOIDCLogin: Userinfo request failed, continuing with id token claims only", "error", err)
	}
	return s.loginOIDCUser(idToken.Issuer, idToken.Subject, s.usernameFromClaims(claims, idToken.Subject))
}

// Pick a username from the claims.
func (s *OIDCService) usernameFromClaims(claims map[string]any, sub string) string {
	keys := []string{"preferred_username", "name", "email"}
	if s.cfg.OIDC.UsernameClaim != "" {
		keys = append([]string{s.cfg.OIDC.UsernameClaim}, keys...)
	}
	for _, k := range keys {
		if v, ok := claims[k].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return sub
}

// Find the user linked to this OIDC identity, or create one.
func (s *OIDCService) loginOIDCUser(issuer, sub, username string) (AuthResponse, error) {
	clientID := issuer + "|" + sub
	slog.Debug("loginOIDCUser: A user is logging in", "client_id", clientID, "username", username)
	dbUser := new(entity.User)
	userIdQ := s.db.
		Select("user_id").
		Where("name = ? AND client_id = ?", entity.UserServiceNameOIDC, clientID).
		Table("user_services")
	res := s.db.Where("type = ?", entity.OIDC_USER).Where("id = (?)", userIdQ).Take(&dbUser)
	if res.Error != nil {
		if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
			slog.Error("loginOIDCUser: Failed to lookup user in db", "error", res.Error)
			return AuthResponse{}, errors.New("error locating user in db")
		}
		slog.Info("loginOIDCUser: Creating new user from OIDC identity", "client_id", clientID, "username", username)
		uniqueName, err := s.uniqueUsername(username)
		if err != nil {
			return AuthResponse{}, err
		}
		dbUser.Username = uniqueName
		dbUser.Type = entity.OIDC_USER
		dbUser.Country = &s.cfg.DEFAULT_COUNTRY
		dbUser.UserServices = []entity.UserServices{{
			Name:     entity.UserServiceNameOIDC,
			ClientID: clientID,
			AutoSync: false,
		}}
		if err := s.db.Create(&dbUser).Error; err != nil {
			slog.Error("loginOIDCUser: Failed to create new user in db", "error", err)
			return AuthResponse{}, errors.New("failed to create new user")
		}
	}
	token, err := s.authService.signJWT(dbUser.ID, dbUser.Username, entity.OIDC_USER)
	if err != nil {
		slog.Error("loginOIDCUser: Failed to sign new jwt", "error", err)
		return AuthResponse{}, errors.New("failed to get auth token")
	}
	return AuthResponse{Token: token}, nil
}

// Usernames are unique per user type, so if two OIDC identities share a
// username, suffix the newer one with a number.
func (s *OIDCService) uniqueUsername(base string) (string, error) {
	name := base
	for i := 2; i < 1000; i++ {
		var count int64
		// Unscoped: soft deleted users still hold their username in the unique index.
		err := s.db.Unscoped().Model(&entity.User{}).
			Where("username = ? AND type = ?", name, entity.OIDC_USER).
			Count(&count).Error
		if err != nil {
			slog.Error("uniqueUsername: Failed to count users", "error", err)
			return "", errors.New("failed to create new user")
		}
		if count == 0 {
			return name, nil
		}
		name = fmt.Sprintf("%s%d", base, i)
	}
	return "", errors.New("failed to find a free username")
}
