package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/cluster"
	svcerrs "github.com/whg517/fleet/internal/domain/service"
	syserrs "github.com/whg517/fleet/internal/domain/system"
)

// APIError represents a structured error response.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// PaginationResponse represents pagination metadata.
type PaginationResponse struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// errorResponse maps domain errors to HTTP responses.
func errorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, cluster.ErrClusterNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "cluster not found"},
		})
	case errors.Is(err, cluster.ErrClusterAlreadyExists):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "CONFLICT", Message: "cluster already exists"},
		})
	case errors.Is(err, cluster.ErrInvalidKubeconfig):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_KUBECONFIG", Message: err.Error()},
		})
	case errors.Is(err, cluster.ErrClusterHasEnvironments):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "HAS_ENVIRONMENTS", Message: "cluster has active environments, cannot delete"},
		})
	case errors.Is(err, cluster.ErrEnvironmentAlreadyExists):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "CONFLICT", Message: "environment already exists for this cluster"},
		})
	case errors.Is(err, cluster.ErrEnvironmentNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "environment not found"},
		})
	case errors.Is(err, cluster.ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: err.Error()},
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]APIError{
			"error": {Code: "INTERNAL", Message: "internal server error"},
		})
	}
}

// serviceErrorResponse maps service domain errors to HTTP responses.
func serviceErrorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, svcerrs.ErrServiceNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "service not found"},
		})
	case errors.Is(err, svcerrs.ErrServiceAlreadyExists):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "CONFLICT", Message: "service already exists"},
		})
	case errors.Is(err, svcerrs.ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: err.Error()},
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]APIError{
			"error": {Code: "INTERNAL", Message: "internal server error"},
		})
	}
}

// systemErrorResponse maps system-setting domain errors to HTTP responses.
func systemErrorResponse(c echo.Context, err error) error {
	switch {
	case errors.Is(err, syserrs.ErrSettingNotFound):
		return c.JSON(http.StatusNotFound, map[string]APIError{
			"error": {Code: "NOT_FOUND", Message: "setting not found"},
		})
	case errors.Is(err, syserrs.ErrSettingAlreadyExists):
		return c.JSON(http.StatusConflict, map[string]APIError{
			"error": {Code: "CONFLICT", Message: "setting already exists"},
		})
	case errors.Is(err, syserrs.ErrInvalidInput):
		return c.JSON(http.StatusBadRequest, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: err.Error()},
		})
	default:
		return c.JSON(http.StatusInternalServerError, map[string]APIError{
			"error": {Code: "INTERNAL", Message: "internal server error"},
		})
	}
}

// successResponse returns a standard success envelope.
func successResponse(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, map[string]any{"data": data})
}

// createdResponse returns a 201 with the standard envelope.
func createdResponse(c echo.Context, data any) error {
	return c.JSON(http.StatusCreated, map[string]any{"data": data})
}

// paginatedResponse returns a list response with pagination metadata.
func paginatedResponse(c echo.Context, data any, page, pageSize, total int) error {
	return c.JSON(http.StatusOK, map[string]any{
		"data": data,
		"pagination": PaginationResponse{
			Page:     page,
			PageSize: pageSize,
			Total:    total,
		},
	})
}
