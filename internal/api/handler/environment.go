package handler

import (
	"strconv"

	"github.com/labstack/echo/v4"
)

// ListAllEnvironments returns all environments across clusters.
// GET /api/v1/environments
// This is a placeholder — the full implementation will need a store query
// that spans all clusters. For now it's a stub that returns a paginated empty list.
func (h *ClusterHandler) ListAllEnvironments(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if pageSize < 1 {
		pageSize = 20
	}

	// TODO: Implement cross-cluster environment query
	return paginatedResponse(c, []any{}, page, pageSize, 0)
}
