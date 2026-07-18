package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/rbac"
)

// RBACMiddleware checks user permissions via Casbin before allowing the request.
// It must be placed AFTER AuthMiddleware so that user context is populated.
// The middleware:
//  1. Checks if the user is blacklisted (instant revocation).
//  2. Iterates over the user's roles and allows if any role passes Enforce.
//  3. Users with no roles are implicitly denied (except on public paths).
func RBACMiddleware(rbacSvc rbac.Service, logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract userID from auth context
			userID, _ := c.Get(ContextKeyUserID).(string)
			if userID == "" {
				// No authenticated user — let it through (public path or auth not required)
				return next(c)
			}

			ctx := c.Request().Context()

			// Check blacklist (instant revocation)
			blacklisted, err := rbacSvc.IsBlacklisted(ctx, userID)
			if err != nil {
				logger.Error("rbac: blacklist check failed", zap.Error(err))
				// Fail-open on infra error for availability; log the issue
			}
			if blacklisted {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "user account has been disabled",
				})
			}

			// Extract roles from JWT claims (set by AuthMiddleware)
			roles := extractRoles(c)
			if len(roles) == 0 {
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "no roles assigned",
				})
			}

			path := c.Request().URL.Path
			method := c.Request().Method

			// Check each role — allow if any role grants permission
			allowed := false
			for _, role := range roles {
				ok, err := rbacSvc.Enforce(role, "*", path, method)
				if err != nil {
					logger.Error("rbac: enforce failed",
						zap.String("role", role),
						zap.String("path", path),
						zap.String("method", method),
						zap.Error(err),
					)
					continue
				}
				if ok {
					allowed = true
					break
				}
			}

			if !allowed {
				logger.Info("rbac: access denied",
					zap.String("user_id", userID),
					zap.Strings("roles", roles),
					zap.String("path", path),
					zap.String("method", method),
				)
				return c.JSON(http.StatusForbidden, map[string]string{
					"error": "forbidden",
				})
			}

			return next(c)
		}
	}
}

// extractRoles safely extracts roles from the Echo context.
func extractRoles(c echo.Context) []string {
	raw := c.Get(ContextKeyRoles)
	if raw == nil {
		return nil
	}
	roles, ok := raw.([]string)
	if !ok {
		return nil
	}
	return roles
}
