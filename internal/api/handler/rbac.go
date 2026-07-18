package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/rbac"
)

// RBACHandler handles role and permission management HTTP endpoints.
type RBACHandler struct {
	svc    rbac.Service
	logger *zap.Logger
}

// NewRBACHandler creates an RBACHandler.
func NewRBACHandler(svc rbac.Service, logger *zap.Logger) *RBACHandler {
	return &RBACHandler{svc: svc, logger: logger}
}

// RoleDefinition represents a role and its description.
type RoleDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// AssignRolesRequest is the body for PUT /api/v1/rbac/users/:id/roles.
type AssignRolesRequest struct {
	Roles []string `json:"roles"`
}

// UserRolesResponse is returned by GET /api/v1/rbac/users/:id/roles.
type UserRolesResponse struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
}

// ListRoles returns all defined roles in the system.
// GET /api/v1/rbac/roles
func (h *RBACHandler) ListRoles(c echo.Context) error {
	roles := make([]RoleDefinition, 0, len(rbac.RoleNames))
	for _, name := range rbac.RoleNames {
		desc := rbac.RoleDescriptions[name]
		roles = append(roles, RoleDefinition{Name: name, Description: desc})
	}
	return successResponse(c, roles)
}

// GetUserRoles returns the roles assigned to a user.
// GET /api/v1/rbac/users/:id/roles
func (h *RBACHandler) GetUserRoles(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user id is required"})
	}

	roles, err := h.svc.GetRolesForUser(userID)
	if err != nil {
		h.logger.Error("failed to get user roles", zap.String("user_id", userID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch roles"})
	}

	return c.JSON(http.StatusOK, UserRolesResponse{
		UserID: userID,
		Roles:  roles,
	})
}

// AssignUserRoles assigns roles to a user (admin only).
// PUT /api/v1/rbac/users/:id/roles
func (h *RBACHandler) AssignUserRoles(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user id is required"})
	}

	var req AssignRolesRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	// Limit number of roles to prevent abuse
	if len(req.Roles) > 10 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "too many roles (max 10)"})
	}

	// Validate roles
	validRoles := make(map[string]bool, len(rbac.RoleNames))
	for _, r := range rbac.RoleNames {
		validRoles[r] = true
	}
	for _, r := range req.Roles {
		if !validRoles[r] {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid role: " + r,
			})
		}
	}

	// Get current roles
	currentRoles, err := h.svc.GetRolesForUser(userID)
	if err != nil {
		h.logger.Error("failed to get current roles", zap.String("user_id", userID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch current roles"})
	}

	// Build sets for diffing
	currentSet := make(map[string]bool, len(currentRoles))
	for _, r := range currentRoles {
		currentSet[r] = true
	}
	newSet := make(map[string]bool, len(req.Roles))
	for _, r := range req.Roles {
		newSet[r] = true
	}

	// Remove roles that are no longer assigned
	for _, role := range currentRoles {
		if !newSet[role] {
			if _, err := h.svc.DeleteRoleForUser(userID, role, "*"); err != nil {
				h.logger.Error("failed to remove role",
					zap.String("user_id", userID),
					zap.String("role", role),
					zap.Error(err))
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update roles"})
			}
		}
	}

	// Add new roles
	for _, role := range req.Roles {
		if !currentSet[role] {
			if _, err := h.svc.AddRoleForUser(userID, role, "*"); err != nil {
				h.logger.Error("failed to add role",
					zap.String("user_id", userID),
					zap.String("role", role),
					zap.Error(err))
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to update roles"})
			}
		}
	}

	// Return updated roles
	updatedRoles, err := h.svc.GetRolesForUser(userID)
	if err != nil {
		return c.JSON(http.StatusOK, UserRolesResponse{UserID: userID, Roles: req.Roles})
	}

	return c.JSON(http.StatusOK, UserRolesResponse{
		UserID: userID,
		Roles:  updatedRoles,
	})
}

// GetPermissions returns the permission matrix for the authenticated user.
// GET /api/v1/rbac/permissions
func (h *RBACHandler) GetPermissions(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	if userID == "" {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}

	perms, err := h.svc.GetUserPermissions(userID)
	if err != nil {
		h.logger.Error("failed to get permissions", zap.String("user_id", userID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch permissions"})
	}

	return successResponse(c, perms)
}

// DisableUser adds a user to the blacklist (instant revocation).
// POST /api/v1/rbac/users/:id/disable
func (h *RBACHandler) DisableUser(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user id is required"})
	}

	ctx := c.Request().Context()
	if err := h.svc.AddToBlacklist(ctx, userID); err != nil {
		h.logger.Error("failed to disable user", zap.String("user_id", userID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to disable user"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "disabled"})
}

// EnableUser removes a user from the blacklist.
// POST /api/v1/rbac/users/:id/enable
func (h *RBACHandler) EnableUser(c echo.Context) error {
	userID := c.Param("id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user id is required"})
	}

	ctx := c.Request().Context()
	if err := h.svc.RemoveFromBlacklist(ctx, userID); err != nil {
		if errors.Is(err, rbac.ErrUserNotBlacklisted) {
			return c.JSON(http.StatusOK, map[string]string{"status": "already enabled"})
		}
		h.logger.Error("failed to enable user", zap.String("user_id", userID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enable user"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "enabled"})
}
