package handler

import (
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/cluster"
)

// ClusterHandler handles cluster and environment API endpoints.
type ClusterHandler struct {
	svc cluster.Service
}

// NewClusterHandler creates a ClusterHandler.
func NewClusterHandler(svc cluster.Service) *ClusterHandler {
	return &ClusterHandler{svc: svc}
}

// Create registers a new cluster.
// POST /api/v1/clusters
func (h *ClusterHandler) Create(c echo.Context) error {
	var req cluster.CreateClusterReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	cl, err := h.svc.Create(c.Request().Context(), req)
	if err != nil {
		return errorResponse(c, err)
	}

	// Audit log placeholder (Issue #16 will add proper audit middleware)
	c.Logger().Infof("audit: cluster created id=%s name=%s", cl.ID, cl.Name)

	return createdResponse(c, cl)
}

// List returns a paginated list of clusters.
// GET /api/v1/clusters
func (h *ClusterHandler) List(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))

	filter := cluster.ClusterFilter{
		OrgID:    c.QueryParam("org_id"),
		Status:   c.QueryParam("status"),
		Page:     page,
		PageSize: pageSize,
		Labels:   parseLabelFilter(c.QueryParam("labels")),
	}

	result, err := h.svc.List(c.Request().Context(), filter)
	if err != nil {
		return errorResponse(c, err)
	}

	return paginatedResponse(c, result.Clusters, result.Page, result.PageSize, result.Total)
}

// Get returns a single cluster by ID.
// GET /api/v1/clusters/:id
func (h *ClusterHandler) Get(c echo.Context) error {
	cl, err := h.svc.Get(c.Request().Context(), c.Param("id"))
	if err != nil {
		return errorResponse(c, err)
	}
	return successResponse(c, cl)
}

// Update modifies an existing cluster.
// PUT /api/v1/clusters/:id
func (h *ClusterHandler) Update(c echo.Context) error {
	var req cluster.UpdateClusterReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	cl, err := h.svc.Update(c.Request().Context(), c.Param("id"), req)
	if err != nil {
		return errorResponse(c, err)
	}

	c.Logger().Infof("audit: cluster updated id=%s", c.Param("id"))

	return successResponse(c, cl)
}

// Delete removes a cluster.
// DELETE /api/v1/clusters/:id
func (h *ClusterHandler) Delete(c echo.Context) error {
	id := c.Param("id")

	// Require confirmation via query param or header
	confirm := c.QueryParam("confirm")
	if confirm != "true" {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "CONFIRMATION_REQUIRED", Message: "pass ?confirm=true to confirm deletion"},
		})
	}

	if err := h.svc.Delete(c.Request().Context(), id); err != nil {
		return errorResponse(c, err)
	}

	c.Logger().Infof("audit: cluster deleted id=%s", id)

	return c.NoContent(204)
}

// TestConnection tests cluster connectivity.
// POST /api/v1/clusters/:id/test
func (h *ClusterHandler) TestConnection(c echo.Context) error {
	id := c.Param("id")

	if err := h.svc.TestConnection(c.Request().Context(), id); err != nil {
		return errorResponse(c, err)
	}

	return successResponse(c, map[string]string{
		"status": "ok",
		"message": "cluster connection verified",
	})
}

// CreateEnvironment creates an environment under a cluster.
// POST /api/v1/clusters/:id/environments
func (h *ClusterHandler) CreateEnvironment(c echo.Context) error {
	clusterID := c.Param("id")

	var req cluster.CreateEnvReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}

	env, err := h.svc.CreateEnvironment(c.Request().Context(), clusterID, req)
	if err != nil {
		return errorResponse(c, err)
	}

	c.Logger().Infof("audit: environment created id=%s cluster=%s name=%s", env.ID, clusterID, env.Name)

	return createdResponse(c, env)
}

// ListEnvironments returns environments for a cluster.
// GET /api/v1/clusters/:id/environments
func (h *ClusterHandler) ListEnvironments(c echo.Context) error {
	clusterID := c.Param("id")

	envs, err := h.svc.ListEnvironments(c.Request().Context(), clusterID)
	if err != nil {
		return errorResponse(c, err)
	}

	return successResponse(c, envs)
}

// parseLabelFilter parses a label filter string like "key1=val1,key2=val2".
func parseLabelFilter(s string) map[string]string {
	if s == "" {
		return nil
	}
	labels := make(map[string]string)
	parts := strings.Split(s, ",")
	for _, p := range parts {
		kv := strings.SplitN(p, "=", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			if k != "" {
				labels[k] = v
			}
		}
	}
	if len(labels) == 0 {
		return nil
	}
	return labels
}
