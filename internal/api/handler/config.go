package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/config"
)

// ConfigHandler handles configuration management API endpoints.
type ConfigHandler struct {
	svc config.Service
}

// NewConfigHandler creates a ConfigHandler.
func NewConfigHandler(svc config.Service) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

// UpdateValues updates Helm values for a service+environment.
// PUT /api/v1/services/:sid/environments/:eid/values
func (h *ConfigHandler) UpdateValues(c echo.Context) error {
	sid := c.Param("sid")
	eid := c.Param("eid")

	var req config.UpdateValuesReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	// Set path params and user context
	req.ServiceID = sid
	req.EnvironmentID = eid
	if uid, ok := c.Get("user_id").(string); ok {
		req.ChangedBy = uid
	}
	if oid, ok := c.Get("org_id").(string); ok {
		req.OrgID = oid
	}

	snap, err := h.svc.UpdateValues(c.Request().Context(), sid, eid, req)
	if err != nil {
		return configErrorResponse(c, err)
	}

	c.Logger().Infof("audit: config values updated service=%s env=%s snapshot=%s", sid, eid, snap.ID)

	return successResponse(c, snap)
}

// GetValues returns the current Helm values for a service+environment.
// GET /api/v1/services/:sid/environments/:eid/values
func (h *ConfigHandler) GetValues(c echo.Context) error {
	sid := c.Param("sid")
	eid := c.Param("eid")

	values, err := h.svc.GetValues(c.Request().Context(), sid, eid)
	if err != nil {
		return configErrorResponse(c, err)
	}

	return successResponse(c, map[string]any{
		"service_id":     sid,
		"environment_id": eid,
		"values":         values,
	})
}

// ListHistory returns a paginated list of config changes.
// GET /api/v1/services/:sid/environments/:eid/config-history
func (h *ConfigHandler) ListHistory(c echo.Context) error {
	sid := c.Param("sid")
	eid := c.Param("eid")

	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	result, err := h.svc.ListHistory(c.Request().Context(), sid, eid, page, pageSize)
	if err != nil {
		return configErrorResponse(c, err)
	}

	return paginatedResponse(c, result.Snapshots, result.Page, result.PageSize, result.Total)
}

// Diff compares two config versions.
// GET /api/v1/services/:sid/environments/:eid/config-history/:snapshotId/diff?to=<snapshotId>
func (h *ConfigHandler) Diff(c echo.Context) error {
	sid := c.Param("sid")
	eid := c.Param("eid")
	fromVer := c.Param("snapshotId")
	toVer := c.QueryParam("to")

	diff, err := h.svc.Diff(c.Request().Context(), sid, eid, fromVer, toVer)
	if err != nil {
		return configErrorResponse(c, err)
	}

	return successResponse(c, diff)
}

// configErrorResponse maps config domain errors to HTTP responses.
func configErrorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, config.ErrConfigNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "config snapshot not found"},
		})
	case errors.Is(err, config.ErrNoActiveDeployment):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NO_ACTIVE_DEPLOYMENT", Message: "no active deployment for this service and environment"},
		})
	case errors.Is(err, config.ErrArgoCDUnavailable):
		return c.JSON(http.StatusServiceUnavailable, map[string]APIError{
			"error": {Code: "ARGOCD_UNAVAILABLE", Message: "argo cd is temporarily unavailable"},
		})
	case errors.Is(err, config.ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request parameters"},
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]APIError{
			"error": {Code: "INTERNAL", Message: "internal server error"},
		})
	}
}
