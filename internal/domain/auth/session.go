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

// SessionManager manages access/refresh tokens using JWT + Redis.
type SessionManager struct {
	jwtSecret   []byte
	accessTTL   time.Duration
	refreshTTL  time.Duration
	redisClient *redis.Client
}

// NewSessionManager creates a SessionManager from config.
func NewSessionManager(cfg config.JWTConfig, rdb *redis.Client) *SessionManager {
	return &SessionManager{
		jwtSecret:   []byte(cfg.Secret),
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
			ExpiresAt: jwt.NewNumericDate(now.Add(sm.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			Subject:   userID,
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
	val, err := json.Marshal(map[string]string{
		"user_id": userID,
		"email":   email,
		"name":    name,
	})
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
	})
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

	var session struct {
		UserID string   `json:"user_id"`
		Email  string   `json:"email"`
		Name   string   `json:"name"`
		Roles  []string `json:"roles"`
	}
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
