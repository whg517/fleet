package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/template"
)

// TemplateHandler handles template management API endpoints.
type TemplateHandler struct {
	svc template.Service
}

// NewTemplateHandler creates a TemplateHandler.
func NewTemplateHandler(svc template.Service) *TemplateHandler {
	return &TemplateHandler{svc: svc}
}

// Create registers a new template.
// POST /api/v1/templates
func (h *TemplateHandler) Create(c echo.Context) error {
	var req template.CreateTemplateReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	t, err := h.svc.Create(c.Request().Context(), req)
	if err != nil {
		return templateErrorResponse(c, err)
	}

	c.Logger().Infof("audit: template created id=%s name=%s type=%s", t.ID, t.Name, t.Type)

	return createdResponse(c, t)
}

// List returns a paginated list of templates.
// GET /api/v1/templates
func (h *TemplateHandler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	filter := template.TemplateFilter{
		OrgID:    c.QueryParam("org_id"),
		Type:     c.QueryParam("type"),
		Source:   c.QueryParam("source"),
		Status:   c.QueryParam("status"),
		Name:     c.QueryParam("name"),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := h.svc.List(c.Request().Context(), filter)
	if err != nil {
		return templateErrorResponse(c, err)
	}

	return paginatedResponse(c, result.Templates, result.Page, result.PageSize, result.Total)
}

// Get returns a single template by ID, including its active versions.
// GET /api/v1/templates/:id
func (h *TemplateHandler) Get(c echo.Context) error {
	t, versions, err := h.svc.GetWithVersions(c.Request().Context(), c.Param("id"))
	if err != nil {
		return templateErrorResponse(c, err)
	}

	return successResponse(c, map[string]any{
		"template": t,
		"versions": versions,
	})
}

// Update modifies an existing template.
// PUT /api/v1/templates/:id
func (h *TemplateHandler) Update(c echo.Context) error {
	var req template.UpdateTemplateReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	t, err := h.svc.Update(c.Request().Context(), c.Param("id"), req)
	if err != nil {
		return templateErrorResponse(c, err)
	}

	c.Logger().Infof("audit: template updated id=%s", c.Param("id"))

	return successResponse(c, t)
}

// Delete archives a template (soft delete).
// DELETE /api/v1/templates/:id
func (h *TemplateHandler) Delete(c echo.Context) error {
	id := c.Param("id")

	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return templateErrorResponse(c, err)
	}

	c.Logger().Infof("audit: template archived id=%s", id)

	return c.NoContent(204)
}

// PublishVersion publishes a new version for a template.
// POST /api/v1/templates/:id/versions
func (h *TemplateHandler) PublishVersion(c echo.Context) error {
	var req template.PublishVersionReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	v, err := h.svc.PublishVersion(c.Request().Context(), c.Param("id"), req)
	if err != nil {
		return templateErrorResponse(c, err)
	}

	c.Logger().Infof("audit: template version published template_id=%s version=%s", c.Param("id"), v.Version)

	return createdResponse(c, v)
}

// ListVersions returns a paginated list of versions for a template.
// GET /api/v1/templates/:id/versions
func (h *TemplateHandler) ListVersions(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	result, err := h.svc.ListVersions(c.Request().Context(), c.Param("id"), page, pageSize)
	if err != nil {
		return templateErrorResponse(c, err)
	}

	return paginatedResponse(c, result.Versions, result.Page, result.PageSize, result.Total)
}

// ArchiveVersion archives a specific version of a template.
// POST /api/v1/templates/:id/versions/:ver/archive
func (h *TemplateHandler) ArchiveVersion(c echo.Context) error {
	templateID := c.Param("id")
	version := c.Param("ver")

	if err := h.svc.ArchiveVersion(c.Request().Context(), templateID, version); err != nil {
		return templateErrorResponse(c, err)
	}

	c.Logger().Infof("audit: template version archived template_id=%s version=%s", templateID, version)

	return c.NoContent(204)
}
