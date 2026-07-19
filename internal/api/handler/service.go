package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/service"
)

// ServiceHandler handles service catalog API endpoints.
type ServiceHandler struct {
	svc service.Service
}

// NewServiceHandler creates a ServiceHandler.
func NewServiceHandler(svc service.Service) *ServiceHandler {
	return &ServiceHandler{svc: svc}
}

// Create registers a new service.
// POST /api/v1/services
func (h *ServiceHandler) Create(c echo.Context) error {
	var req service.CreateServiceReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	s, err := h.svc.Create(c.Request().Context(), req)
	if err != nil {
		return serviceErrorResponse(c, err)
	}

	c.Logger().Infof("audit: service created id=%s name=%s", s.ID, s.Name)

	return createdResponse(c, s)
}

// List returns a paginated list of services.
// GET /api/v1/services
func (h *ServiceHandler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	filter := service.ServiceFilter{
		OrgID:    c.QueryParam("org_id"),
		Name:     c.QueryParam("name"),
		Team:     c.QueryParam("team"),
		Status:   c.QueryParam("status"),
		Page:     page,
		PageSize: pageSize,
		Labels:   parseLabelFilter(c.QueryParam("labels")),
	}

	result, err := h.svc.List(c.Request().Context(), filter)
	if err != nil {
		return serviceErrorResponse(c, err)
	}

	return paginatedResponse(c, result.Services, result.Page, result.PageSize, result.Total)
}

// Get returns a single service by ID.
// GET /api/v1/services/:id
func (h *ServiceHandler) Get(c echo.Context) error {
	s, err := h.svc.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return serviceErrorResponse(c, err)
	}
	return successResponse(c, s)
}

// Update modifies an existing service.
// PUT /api/v1/services/:id
func (h *ServiceHandler) Update(c echo.Context) error {
	var req service.UpdateServiceReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	s, err := h.svc.Update(c.Request().Context(), c.Param("id"), req)
	if err != nil {
		return serviceErrorResponse(c, err)
	}

	c.Logger().Infof("audit: service updated id=%s", c.Param("id"))

	return successResponse(c, s)
}

// Delete archives a service (soft delete).
// DELETE /api/v1/services/:id
func (h *ServiceHandler) Delete(c echo.Context) error {
	id := c.Param("id")

	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return serviceErrorResponse(c, err)
	}

	c.Logger().Infof("audit: service archived id=%s", id)

	return c.NoContent(204)
}
