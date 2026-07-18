package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/domain/audit"
)

// AuditHandler handles audit log HTTP endpoints.
type AuditHandler struct {
	auditSvc audit.Service
	logger   *zap.Logger
}

// NewAuditHandler creates an AuditHandler.
func NewAuditHandler(auditSvc audit.Service, logger *zap.Logger) *AuditHandler {
	return &AuditHandler{
		auditSvc: auditSvc,
		logger:   logger,
	}
}

// pagination is the standard pagination response envelope.
type pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// listResponse is the standard paginated list response.
type listResponse struct {
	Data       any        `json:"data"`
	Pagination pagination `json:"pagination"`
}

// List handles GET /api/v1/audit-logs.
// Query params: user_id, resource_type, action, start_time, end_time, page, page_size
func (h *AuditHandler) List(c echo.Context) error {
	filter := audit.AuditFilter{
		UserID:       c.QueryParam("user_id"),
		ResourceType: c.QueryParam("resource_type"),
		Action:       c.QueryParam("action"),
		Page:         parseIntDefault(c.QueryParam("page"), 1),
		PageSize:     parseIntDefault(c.QueryParam("page_size"), 20),
	}

	// Parse time range
	if s := c.QueryParam("start_time"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid start_time format, use RFC3339",
			})
		}
		filter.StartTime = &t
	}
	if s := c.QueryParam("end_time"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid end_time format, use RFC3339",
			})
		}
		filter.EndTime = &t
	}

	result, err := h.auditSvc.List(c.Request().Context(), filter)
	if err != nil {
		h.logger.Error("audit list failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to list audit logs",
		})
	}

	return c.JSON(http.StatusOK, listResponse{
		Data: result.Logs,
		Pagination: pagination{
			Page:     result.Page,
			PageSize: result.PageSize,
			Total:    result.Total,
		},
	})
}

// verifyResponse is the response for hash chain verification.
type verifyResponse struct {
	Valid bool                       `json:"valid"`
	Gaps  []audit.VerificationGap   `json:"gaps"`
}

// Verify handles GET /api/v1/audit-logs/verify.
func (h *AuditHandler) Verify(c echo.Context) error {
	valid, gaps, err := h.auditSvc.VerifyChain(c.Request().Context())
	if err != nil {
		h.logger.Error("audit verify failed", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to verify audit log chain",
		})
	}

	return c.JSON(http.StatusOK, verifyResponse{
		Valid: valid,
		Gaps:  gaps,
	})
}

// parseIntDefault parses an integer query param, falling back to def.
func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}
