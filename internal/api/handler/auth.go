package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/auth"
	"github.com/whg517/fleet/internal/infra/config"
)

// Cookie names for auth tokens.
const (
	cookieAccessToken  = "access_token"
	cookieRefreshToken = "refresh_token"
)

// Cookie path for auth tokens.
const cookiePath = "/api/v1"

// AuthHandler handles authentication HTTP endpoints.
// It is a thin layer: parse request → call service → return JSON/redirect.
type AuthHandler struct {
	svc    auth.Service
	jwtCfg config.JWTConfig
	envCfg config.ServerConfig
	logger *zap.Logger
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(svc auth.Service, jwtCfg config.JWTConfig, envCfg config.ServerConfig, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		svc:    svc,
		jwtCfg: jwtCfg,
		envCfg: envCfg,
		logger: logger,
	}
}

// isProduction returns true if running in production environment.
func (h *AuthHandler) isProduction() bool {
	return h.envCfg.Environment == "production"
}

// setAuthCookies sets HttpOnly cookies for access and refresh tokens.
func (h *AuthHandler) setAuthCookies(c echo.Context, pair *auth.TokenPair) {
	secure := h.isProduction()

	c.SetCookie(&http.Cookie{
		Name:     cookieAccessToken,
		Value:    pair.AccessToken,
		Path:     cookiePath,
		MaxAge:   int((30 * time.Minute).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	c.SetCookie(&http.Cookie{
		Name:     cookieRefreshToken,
		Value:    pair.RefreshToken,
		Path:     cookiePath,
		MaxAge:   int((8 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookies expires the auth cookies.
func (h *AuthHandler) clearAuthCookies(c echo.Context) {
	secure := h.isProduction()

	c.SetCookie(&http.Cookie{
		Name:     cookieAccessToken,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	c.SetCookie(&http.Cookie{
		Name:     cookieRefreshToken,
		Value:    "",
		Path:     cookiePath,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
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
// Instead of exposing tokens in the URL fragment, it stores the token pair
// behind a one-time exchange code and redirects to the frontend with only
// the exchange code as a query parameter.
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

	// Store tokens behind a one-time exchange code (10s TTL)
	exchangeCode, err := h.svc.CreateExchangeCode(ctx, pair)
	if err != nil {
		h.logger.Error("failed to create exchange code", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to initiate session",
		})
	}

	// Redirect to frontend with only the exchange code (no tokens in URL)
	frontendURL := h.jwtCfg.FrontendURL
	redirectURL := frontendURL + "/auth/callback?code=" + exchangeCode

	return c.Redirect(http.StatusFound, redirectURL)
}

// ExchangeToken redeems a one-time exchange code for a token pair.
// Sets HttpOnly cookies with the tokens and also returns them in JSON body
// for clients that cannot use cookies.
// POST /api/v1/auth/token
func (h *AuthHandler) ExchangeToken(c echo.Context) error {
	ctx := c.Request().Context()

	code := c.FormValue("code")
	if code == "" {
		// Also accept JSON body for flexibility
		var req struct {
			Code string `json:"code"`
		}
		if err := c.Bind(&req); err == nil && req.Code != "" {
			code = req.Code
		}
	}
	if code == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "missing code",
		})
	}

	pair, err := h.svc.ConsumeExchangeCode(ctx, code)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidToken) {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "invalid or expired code",
			})
		}
		h.logger.Error("exchange token failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to exchange token",
		})
	}

	// Set HttpOnly cookies
	h.setAuthCookies(c, pair)

	return c.JSON(http.StatusOK, pair)
}

// Me returns the current authenticated user's info.
// GET /api/v1/auth/me
func (h *AuthHandler) Me(c echo.Context) error {
	ctx := c.Request().Context()

	token := extractToken(c)
	if token == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "missing or invalid token",
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
// Reads refresh_token from Cookie first, falls back to JSON body.
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(c echo.Context) error {
	ctx := c.Request().Context()

	// Try cookie first
	refreshToken := readCookie(c, cookieRefreshToken)

	// Fall back to JSON body
	if refreshToken == "" {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.Bind(&req); err == nil {
			refreshToken = req.RefreshToken
		}
	}
	if refreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "refresh_token is required",
		})
	}

	pair, err := h.svc.Refresh(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrSessionNotFound) {
			// Clear stale cookies if refresh fails
			h.clearAuthCookies(c)
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "invalid or expired refresh token",
			})
		}
		h.logger.Error("refresh failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to refresh token",
		})
	}

	// Set fresh cookies with new tokens
	h.setAuthCookies(c, pair)

	return c.JSON(http.StatusOK, pair)
}

// Logout revokes the user session and clears auth cookies.
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(c echo.Context) error {
	ctx := c.Request().Context()

	// Try cookie first
	refreshToken := readCookie(c, cookieRefreshToken)

	// Fall back to JSON body
	if refreshToken == "" {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := c.Bind(&req); err == nil {
			refreshToken = req.RefreshToken
		}
	}

	// Always clear cookies regardless
	h.clearAuthCookies(c)

	if refreshToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "refresh_token is required",
		})
	}

	if err := h.svc.Logout(ctx, refreshToken); err != nil {
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

// extractToken extracts the access token from either the Authorization
// header (Bearer) or the access_token cookie.
func extractToken(c echo.Context) string {
	// Try Authorization header first
	authHeader := c.Request().Header.Get(echo.HeaderAuthorization)
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	// Fall back to cookie
	return readCookie(c, cookieAccessToken)
}

// readCookie safely reads a cookie value by name.
func readCookie(c echo.Context, name string) string {
	cookie, err := c.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
