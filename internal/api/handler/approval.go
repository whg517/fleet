package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/approval"
)

// ApprovalHandler handles approval management API endpoints.
type ApprovalHandler struct {
	svc approval.Service
}

// NewApprovalHandler creates an ApprovalHandler.
func NewApprovalHandler(svc approval.Service) *ApprovalHandler {
	return &ApprovalHandler{svc: svc}
}

// Approve approves a pending deployment approval.
// POST /api/v1/deployments/:id/approve
func (h *ApprovalHandler) Approve(c echo.Context) error {
	deploymentID := c.Param("id")

	var req approval.ApproveReq
	if err := c.Bind(&req); err != nil {
		req.Comment = ""
	}

	if uid, ok := c.Get("user_id").(string); ok {
		req.ApproverID = uid
	}

	a, err := h.svc.Approve(c.Request().Context(), deploymentID, req)
	if err != nil {
		return approvalErrorResponse(c, err)
	}

	c.Logger().Infof("audit: approval approved deployment=%s approver=%s", deploymentID, req.ApproverID)

	return successResponse(c, a)
}

// Reject rejects a pending deployment approval.
// POST /api/v1/deployments/:id/reject
func (h *ApprovalHandler) Reject(c echo.Context) error {
	deploymentID := c.Param("id")

	var req approval.RejectReq
	if err := c.Bind(&req); err != nil {
		req.Comment = ""
	}

	if uid, ok := c.Get("user_id").(string); ok {
		req.ApproverID = uid
	}

	a, err := h.svc.Reject(c.Request().Context(), deploymentID, req)
	if err != nil {
		return approvalErrorResponse(c, err)
	}

	c.Logger().Infof("audit: approval rejected deployment=%s approver=%s", deploymentID, req.ApproverID)

	return successResponse(c, a)
}

// Get returns the approval status for a deployment.
// GET /api/v1/deployments/:id/approval
func (h *ApprovalHandler) Get(c echo.Context) error {
	a, err := h.svc.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return approvalErrorResponse(c, err)
	}
	return successResponse(c, a)
}

// List returns a paginated list of approvals.
// GET /api/v1/approvals
func (h *ApprovalHandler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	filter := approval.ApprovalFilter{
		OrgID:         c.QueryParam("org_id"),
		ServiceID:     c.QueryParam("service_id"),
		EnvironmentID: c.QueryParam("environment_id"),
		Status:        c.QueryParam("status"),
		Page:          page,
		PageSize:      pageSize,
	}

	result, err := h.svc.List(c.Request().Context(), filter)
	if err != nil {
		return approvalErrorResponse(c, err)
	}

	return paginatedResponse(c, result.Approvals, result.Page, result.PageSize, result.Total)
}

// approvalErrorResponse maps approval domain errors to HTTP responses.
func approvalErrorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, approval.ErrApprovalNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "approval not found"},
		})
	case errors.Is(err, approval.ErrApprovalNotPending):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "CONFLICT", Message: "approval is not pending"},
		})
	case errors.Is(err, approval.ErrDeploymentNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "deployment not found"},
		})
	case errors.Is(err, approval.ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid input"},
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]APIError{
			"error": {Code: "INTERNAL", Message: "internal server error"},
		})
	}
}
