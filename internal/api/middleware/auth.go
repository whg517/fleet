package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/auth"
)

// Context keys for storing authenticated user data.
const (
	ContextKeyUserID = "user_id"
	ContextKeyEmail  = "email"
	ContextKeyRoles  = "roles"
)

// publicPaths are routes that do not require authentication.
var publicPaths = []string{
	"/api/v1/auth/login",
	"/api/v1/auth/callback",
	"/api/v1/auth/token",
	"/api/v1/health",
}

// isPublicPath checks if a path is in the public whitelist.
func isPublicPath(path string) bool {
	for _, p := range publicPaths {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// AuthMiddleware validates JWT access tokens and injects user context.
// Requests to publicPaths are allowed through without token.
func AuthMiddleware(sessionMgr *auth.SessionManager, logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Path()

			// Skip auth for public endpoints
			if isPublicPath(path) {
				return next(c)
			}

			// Extract Bearer token
			token := extractBearerToken(c)
			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "missing or invalid Authorization header",
				})
			}

			// Validate token
			claims, err := sessionMgr.ValidateAccessToken(token)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidToken) {
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error": "invalid or expired token",
					})
				}
				logger.Error("token validation error", zap.Error(err))
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"error": "authentication error",
				})
			}

			// Inject user info into context
			c.Set(ContextKeyUserID, claims.UserID)
			c.Set(ContextKeyEmail, claims.Email)
			c.Set(ContextKeyRoles, claims.Roles)

			return next(c)
		}
	}
}

// extractBearerToken extracts the token from "Authorization: Bearer <token>".
func extractBearerToken(c echo.Context) string {
	authHeader := c.Request().Header.Get(echo.HeaderAuthorization)
	if authHeader == "" {
		return ""
	}
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
