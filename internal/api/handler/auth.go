package handler

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/auth"
	"github.com/whg517/fleet/internal/infra/config"
)

// AuthHandler handles authentication HTTP endpoints.
// It is a thin layer: parse request → call service → return JSON/redirect.
type AuthHandler struct {
	svc      auth.Service
	jwtCfg   config.JWTConfig
	logger   *zap.Logger
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(svc auth.Service, jwtCfg config.JWTConfig, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		svc:    svc,
		jwtCfg: jwtCfg,
		logger: logger,
	}
}

// Login initiates the OIDC flow by redirecting to the IdP.
// GET /api/v1/auth/login
func (h *AuthHandler) Login(c echo.Context) error {
	ctx := c.Request().Context()

	url, _, err := h.svc.LoginURL(ctx)
	if err != nil {
		h.logger.Error("failed to generate login URL", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to initiate login",
		})
	}

	return c.Redirect(http.StatusFound, url)
}

// Callback handles the OIDC provider redirect, exchanges code for tokens.
// GET /api/v1/auth/callback?code=...&state=...
func (h *AuthHandler) Callback(c echo.Context) error {
	ctx := c.Request().Context()

	code := c.QueryParam("code")
	state := c.QueryParam("state")

	if code == "" || state == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "missing code or state parameter",
		})
	}

	pair, err := h.svc.HandleCallback(ctx, code, state)
	if err != nil {
		h.logger.Error("callback failed", zap.Error(err))
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication failed",
		})
	}

	// Redirect to frontend with tokens in fragment (OAuth2 implicit-like pattern)
	// Frontend extracts tokens from URL fragment.
	frontendURL := h.jwtCfg.FrontendURL
	redirectURL := frontendURL + "/auth/callback#" +
		"access_token=" + pair.AccessToken +
		"&refresh_token=" + pair.RefreshToken +
		"&expires_in=" + fmt.Sprintf("%d", pair.ExpiresIn)

	return c.Redirect(http.StatusFound, redirectURL)
}

// Me returns the current authenticated user's info.
// GET /api/v1/auth/me
func (h *AuthHandler) Me(c echo.Context) error {
	ctx := c.Request().Context()

	token := extractBearerToken(c)
	if token == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "missing or invalid Authorization header",
		})
	}

	info, err := h.svc.GetMe(ctx, token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "invalid or expired token",
			})
		}
		h.logger.Error("get me failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch user info",
		})
	}

	return c.JSON(http.StatusOK, info)
}

// Refresh rotates the refresh token and returns a new token pair.
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c echo.Context) error {
	ctx := c.Request().Context()

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}
	if req.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "refresh_token is required",
		})
	}

	pair, err := h.svc.Refresh(ctx, req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "invalid or expired refresh token",
			})
		}
		h.logger.Error("refresh failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to refresh token",
		})
	}

	return c.JSON(http.StatusOK, pair)
}

// Logout revokes the user session.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c echo.Context) error {
	ctx := c.Request().Context()

	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}
	if req.RefreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "refresh_token is required",
		})
	}

	if err := h.svc.Logout(ctx, req.RefreshToken); err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			return c.JSON(http.StatusOK, map[string]string{
				"status": "already logged out",
			})
		}
		h.logger.Error("logout failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to logout",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "logged out",
	})
}

// extractBearerToken extracts the token from the Authorization: Bearer <token> header.
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
