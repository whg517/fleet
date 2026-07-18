package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/infra/config"
	"github.com/whg517/fleet/internal/store/ent"
	"github.com/whg517/fleet/internal/store/ent/user"
)

// Service defines the authentication business operations.
type Service interface {
	LoginURL(ctx context.Context) (authURL, state string, err error)
	HandleCallback(ctx context.Context, code, state string) (*TokenPair, error)
	GetMe(ctx context.Context, accessToken string) (*UserInfo, error)
	Refresh(ctx context.Context, refreshToken string) (*TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	CreateExchangeCode(ctx context.Context, pair *TokenPair) (string, error)
	ConsumeExchangeCode(ctx context.Context, code string) (*TokenPair, error)
}

// UserInfo is the current-user response model.
type UserInfo struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Status string   `json:"status"`
	Roles  []string `json:"roles"`
}

// tokenResponse represents the IdP token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// oidcUserInfo represents claims returned by the IdP userinfo endpoint.
type oidcUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// authServiceImpl is the concrete Service implementation.
type authServiceImpl struct {
	cfg         config.OIDCConfig
	jwtCfg      config.JWTConfig
	entClient   *ent.Client
	redisClient *redis.Client
	sessionMgr  *SessionManager
	httpClient  *http.Client
	logger      *zap.Logger
}

// NewService creates a new auth Service.
func NewService(oidcCfg config.OIDCConfig, jwtCfg config.JWTConfig, entClient *ent.Client, rdb *redis.Client, logger *zap.Logger) Service {
	return &authServiceImpl{
		cfg:        oidcCfg,
		jwtCfg:     jwtCfg,
		entClient:  entClient,
		redisClient: rdb,
		sessionMgr: NewSessionManager(jwtCfg, rdb),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		logger:     logger,
	}
}

// LoginURL generates an authorization URL with PKCE challenge.
// The state + verifier are stored in Redis for later validation.
func (s *authServiceImpl) LoginURL(ctx context.Context) (string, string, error) {
	state, err := generateRandomString(16)
	if err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}

	verifier, err := generateCodeVerifier()
	if err != nil {
		return "", "", fmt.Errorf("generate code verifier: %w", err)
	}

	nonce, err := generateRandomString(16)
	if err != nil {
		return "", "", fmt.Errorf("generate nonce: %w", err)
	}

	challenge := computeCodeChallenge(verifier)

	// Persist state → {verifier, nonce} in Redis (10 min TTL)
	authSt := AuthState{CodeVerifier: verifier, Nonce: nonce}
	data, err := json.Marshal(authSt)
	if err != nil {
		return "", "", fmt.Errorf("marshal auth state: %w", err)
	}

	if err := s.redisClient.Set(ctx, stateKey(state), data, 10*time.Minute).Err(); err != nil {
		return "", "", fmt.Errorf("store auth state: %w", err)
	}

	url := authorizationEndpoint(s.cfg, state, challenge)
	return url, state, nil
}

// HandleCallback processes the OIDC authorization code callback.
func (s *authServiceImpl) HandleCallback(ctx context.Context, code, state string) (*TokenPair, error) {
	// 1. Validate state from Redis
	stateData, err := s.redisClient.Get(ctx, stateKey(state)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("invalid or expired state")
		}
		return nil, fmt.Errorf("lookup auth state: %w", err)
	}
	// Delete state immediately (single-use)
	s.redisClient.Del(ctx, stateKey(state))

	var authSt AuthState
	if err := json.Unmarshal(stateData, &authSt); err != nil {
		return nil, fmt.Errorf("unmarshal auth state: %w", err)
	}

	// 2. Exchange code for tokens
	tokens, err := s.exchangeCodeForTokens(ctx, code, authSt.CodeVerifier)
	if err != nil {
		return nil, fmt.Errorf("exchange code: %w", err)
	}

	// 3. Fetch userinfo
	oidcUser, err := s.fetchUserInfo(ctx, tokens.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("fetch userinfo: %w", err)
	}

	// 4. Find or create user
	user, err := s.findOrCreateUser(ctx, oidcUser)
	if err != nil {
		return nil, fmt.Errorf("find or create user: %w", err)
	}

	// 5. Determine roles (placeholder: empty for now, will integrate with Role entity later)
	roles := []string{}

	// 6. Generate session tokens
	pair, err := s.sessionMgr.GenerateTokens(ctx, user.ID, user.Email, user.Name, roles)
	if err != nil {
		return nil, fmt.Errorf("generate tokens: %w", err)
	}

	s.logger.Info("user authenticated",
		zap.String("user_id", user.ID),
		zap.String("email", user.Email),
	)

	return pair, nil
}

// GetMe validates the access token and returns current user info.
func (s *authServiceImpl) GetMe(ctx context.Context, accessToken string) (*UserInfo, error) {
	claims, err := s.sessionMgr.ValidateAccessToken(accessToken)
	if err != nil {
		return nil, err
	}

	// Fetch fresh user data from DB
	user, err := s.entClient.User.Get(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("fetch user: %w", err)
	}

	return &UserInfo{
		ID:     user.ID,
		Name:   user.Name,
		Email:  user.Email,
		Status: string(user.Status),
		Roles:  claims.Roles,
	}, nil
}

// Refresh rotates the refresh token and issues a new pair.
func (s *authServiceImpl) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	pair, err := s.sessionMgr.RefreshTokens(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	return pair, nil
}

// Logout revokes the user session.
func (s *authServiceImpl) Logout(ctx context.Context, refreshToken string) error {
	return s.sessionMgr.RevokeSession(ctx, refreshToken)
}

// CreateExchangeCode stores a one-time code in Redis that can be redeemed
// for a token pair, avoiding token leakage through URL fragments.
func (s *authServiceImpl) CreateExchangeCode(ctx context.Context, pair *TokenPair) (string, error) {
	return s.sessionMgr.CreateExchangeCode(ctx, pair)
}

// ConsumeExchangeCode redeems a one-time exchange code and returns the token pair.
func (s *authServiceImpl) ConsumeExchangeCode(ctx context.Context, code string) (*TokenPair, error) {
	return s.sessionMgr.ConsumeExchangeCode(ctx, code)
}

// --- internal helpers ---

func (s *authServiceImpl) exchangeCodeForTokens(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.cfg.RedirectURL)
	form.Set("client_id", s.cfg.ClientID)
	form.Set("client_secret", s.cfg.ClientSecret)
	form.Set("code_verifier", verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint(s.cfg), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(raw))
	}

	var tokens tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}

	return &tokens, nil
}

func (s *authServiceImpl) fetchUserInfo(ctx context.Context, accessToken string) (*oidcUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userinfoEndpoint(s.cfg), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(raw))
	}

	var info oidcUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	if info.Sub == "" {
		return nil, fmt.Errorf("userinfo missing sub claim")
	}

	return &info, nil
}

func (s *authServiceImpl) findOrCreateUser(ctx context.Context, info *oidcUserInfo) (*ent.User, error) {
	// Try to find by oidc_subject
	existing, err := s.entClient.User.
		Query().
		Where(user.OidcSubject(info.Sub)).
		Only(ctx)

	if err == nil {
		return existing, nil
	}
	if !ent.IsNotFound(err) {
		return nil, fmt.Errorf("query user: %w", err)
	}

	// Create new user
	name := info.Name
	if name == "" {
		name = info.Email
	}

	created, err := s.entClient.User.
		Create().
		SetOidcSubject(info.Sub).
		SetEmail(info.Email).
		SetName(name).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	s.logger.Info("created new user from OIDC",
		zap.String("user_id", created.ID),
		zap.String("email", created.Email),
	)

	return created, nil
}

func stateKey(state string) string {
	return "oidc:state:" + state
}
