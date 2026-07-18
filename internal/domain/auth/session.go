package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"

	"github.com/whg517/fleet/internal/infra/config"
)

// SessionErrors grouped for callers.
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrInvalidToken    = errors.New("invalid token")
)

// sessionData is the canonical struct stored in Redis for refresh tokens.
// Using a struct (not map[string]string) ensures roles ([]string) survive serialization.
type sessionData struct {
	UserID string   `json:"user_id"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Roles  []string `json:"roles"`
}

// SessionManager manages access/refresh tokens using JWT + Redis.
type SessionManager struct {
	jwtSecret   []byte
	issuer      string
	audience    string
	accessTTL   time.Duration
	refreshTTL  time.Duration
	redisClient *redis.Client
}

// NewSessionManager creates a SessionManager from config.
func NewSessionManager(cfg config.JWTConfig, rdb *redis.Client) *SessionManager {
	issuer := cfg.Issuer
	if issuer == "" {
		issuer = "fleet"
	}
	audience := cfg.Audience
	if audience == "" {
		audience = "fleet-api"
	}
	return &SessionManager{
		jwtSecret:   []byte(cfg.Secret),
		issuer:      issuer,
		audience:    audience,
		accessTTL:   cfg.AccessTTL,
		refreshTTL:  cfg.RefreshTTL,
		redisClient: rdb,
	}
}

// Claims defines the JWT claims embedded in the access token.
type Claims struct {
	UserID string   `json:"uid"`
	Email  string   `json:"email"`
	Name   string   `json:"name"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// TokenPair holds the access and refresh tokens returned to the client.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"` // seconds
}

// GenerateTokens creates a new access/refresh token pair for a user.
func (sm *SessionManager) GenerateTokens(ctx context.Context, userID, email, name string, roles []string) (*TokenPair, error) {
	// Access token (JWT, short-lived)
	now := time.Now()
	claims := &Claims{
		UserID: userID,
		Email:  email,
		Name:   name,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:     sm.issuer,
			Audience:   []string{sm.audience},
			ExpiresAt:  jwt.NewNumericDate(now.Add(sm.accessTTL)),
			IssuedAt:   jwt.NewNumericDate(now),
			Subject:    userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString(sm.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// Refresh token (opaque random, stored in Redis)
	refreshToken, err := generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	key := refreshKey(refreshToken)
	sd := sessionData{
		UserID: userID,
		Email:  email,
		Name:   name,
		Roles:  roles,
	}
	val, err := json.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session data: %w", err)
	}

	if err := sm.redisClient.Set(ctx, key, val, sm.refreshTTL).Err(); err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(sm.accessTTL.Seconds()),
	}, nil
}

// ValidateAccessToken parses and validates a JWT access token.
func (sm *SessionManager) ValidateAccessToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return sm.jwtSecret, nil
	}, jwt.WithIssuer(sm.issuer), jwt.WithAudience(sm.audience))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshTokens validates a refresh token and issues a new pair (rotation).
func (sm *SessionManager) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	key := refreshKey(refreshToken)

	data, err := sm.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to lookup refresh token: %w", err)
	}

	// Delete old refresh token (rotation)
	if err := sm.redisClient.Del(ctx, key).Err(); err != nil {
		return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
	}

	var session sessionData
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return sm.GenerateTokens(ctx, session.UserID, session.Email, session.Name, session.Roles)
}

// RevokeSession deletes a refresh token from Redis.
func (sm *SessionManager) RevokeSession(ctx context.Context, refreshToken string) error {
	key := refreshKey(refreshToken)
	deleted, err := sm.redisClient.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to delete refresh token: %w", err)
	}
	if deleted == 0 {
		return ErrSessionNotFound
	}
	return nil
}

// CreateExchangeCode stores a one-time exchange code in Redis that can be
// redeemed for the token pair via ExchangeToken. This avoids leaking tokens
// through URL fragments.
func (sm *SessionManager) CreateExchangeCode(ctx context.Context, pair *TokenPair) (string, error) {
	code, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate exchange code: %w", err)
	}

	data, err := json.Marshal(pair)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token pair: %w", err)
	}

	key := exchangeKey(code)
	if err := sm.redisClient.Set(ctx, key, data, 10*time.Second).Err(); err != nil {
		return "", fmt.Errorf("failed to store exchange code: %w", err)
	}

	return code, nil
}

// ConsumeExchangeCode redeems a one-time exchange code and returns the token pair.
// The code is deleted immediately after retrieval (single-use).
func (sm *SessionManager) ConsumeExchangeCode(ctx context.Context, code string) (*TokenPair, error) {
	key := exchangeKey(code)
	data, err := sm.redisClient.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("failed to lookup exchange code: %w", err)
	}

	// Delete immediately (single-use)
	sm.redisClient.Del(ctx, key)

	var pair TokenPair
	if err := json.Unmarshal(data, &pair); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token pair: %w", err)
	}

	return &pair, nil
}

func refreshKey(token string) string {
	return "session:refresh:" + token
}

func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func exchangeKey(code string) string {
	return "auth:exchange:" + code
}
