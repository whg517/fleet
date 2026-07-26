package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/deployment"
)

// DeploymentHandler handles deployment management API endpoints.
type DeploymentHandler struct {
	svc deployment.Service
}

// NewDeploymentHandler creates a DeploymentHandler.
func NewDeploymentHandler(svc deployment.Service) *DeploymentHandler {
	return &DeploymentHandler{svc: svc}
}

// Create creates a new deployment.
// POST /api/v1/deployments
func (h *DeploymentHandler) Create(c echo.Context) error {
	var req deployment.CreateDeploymentReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	// Set CreatedBy from authenticated user context (not from request body)
	if uid, ok := c.Get("user_id").(string); ok {
		req.CreatedBy = uid
	}

	d, err := h.svc.Create(c.Request().Context(), req)
	if err != nil {
		return deploymentErrorResponse(c, err)
	}

	c.Logger().Infof("audit: deployment created id=%s service_id=%s version=%s", d.ID, d.ServiceID, d.Version)

	return createdResponse(c, d)
}

// List returns a paginated list of deployments.
// GET /api/v1/deployments
func (h *DeploymentHandler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	filter := deployment.DeploymentFilter{
		OrgID:         c.QueryParam("org_id"),
		ServiceID:     c.QueryParam("service_id"),
		EnvironmentID: c.QueryParam("environment_id"),
		Status:        c.QueryParam("status"),
		Page:          page,
		PageSize:      pageSize,
	}

	result, err := h.svc.List(c.Request().Context(), filter)
	if err != nil {
		return deploymentErrorResponse(c, err)
	}

	return paginatedResponse(c, result.Deployments, result.Page, result.PageSize, result.Total)
}

// Get returns a single deployment by ID.
// GET /api/v1/deployments/:id
func (h *DeploymentHandler) Get(c echo.Context) error {
	d, err := h.svc.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return deploymentErrorResponse(c, err)
	}
	return successResponse(c, d)
}

// GetStatus fetches the latest sync/health status from Argo CD.
// GET /api/v1/deployments/:id/status
func (h *DeploymentHandler) GetStatus(c echo.Context) error {
	d, err := h.svc.GetStatus(c.Request().Context(), c.Param("id"))
	if err != nil {
		return deploymentErrorResponse(c, err)
	}
	return successResponse(c, d)
}

// Rollback rolls back to the previous healthy version.
// POST /api/v1/deployments/:id/rollback
func (h *DeploymentHandler) Rollback(c echo.Context) error {
	d, err := h.svc.Rollback(c.Request().Context(), c.Param("id"))
	if err != nil {
		return deploymentErrorResponse(c, err)
	}

	c.Logger().Infof("audit: deployment rollback initiated id=%s", c.Param("id"))

	return successResponse(c, d)
}

// deploymentErrorResponse maps deployment domain errors to HTTP responses.
func deploymentErrorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, deployment.ErrDeploymentNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "deployment not found"},
		})
	case errors.Is(err, deployment.ErrDeploymentAlreadyExists):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "CONFLICT", Message: "deployment already exists for this service and environment"},
		})
	case errors.Is(err, deployment.ErrArgoCDUnavailable):
		return c.JSON(http.StatusServiceUnavailable, map[string]APIError{
			"error": {Code: "ARGOCD_UNAVAILABLE", Message: "argo cd is temporarily unavailable"},
		})
	case errors.Is(err, deployment.ErrNoHealthyVersion):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "NO_HEALTHY_VERSION", Message: "no previous healthy version to rollback to"},
		})
	case errors.Is(err, deployment.ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid input"},
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]APIError{
			"error": {Code: "INTERNAL", Message: "internal server error"},
		})
	}
}
