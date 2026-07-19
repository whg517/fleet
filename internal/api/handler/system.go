package handler

import (
	"github.com/labstack/echo/v4"

	"github.com/whg517/fleet/internal/domain/system"
)

type SystemHandler struct {
	svc system.Service
}

func NewSystemHandler(svc system.Service) *SystemHandler {
	return &SystemHandler{svc: svc}
}

func (h *SystemHandler) ListSettings(c echo.Context) error {
	settings, err := h.svc.List(c.Request().Context(), "", c.QueryParam("category"))
	if err != nil {
		return systemErrorResponse(c, err)
	}
	return successResponse(c, settings)
}

func (h *SystemHandler) GetSetting(c echo.Context) error {
	setting, err := h.svc.Get(c.Request().Context(), "", c.Param("key"))
	if err != nil {
		return systemErrorResponse(c, err)
	}
	return successResponse(c, setting)
}

func (h *SystemHandler) SetSetting(c echo.Context) error {
	var req system.SetSettingReq
	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]APIError{
			"error": {Code: "INVALID_INPUT", Message: "invalid request body"},
		})
	}
	setting, err := h.svc.Set(c.Request().Context(), "", c.Param("key"), req)
	if err != nil {
		return systemErrorResponse(c, err)
	}
	c.Logger().Infof("audit: system setting set key=%s", c.Param("key"))
	return successResponse(c, setting)
}

func (h *SystemHandler) DeleteSetting(c echo.Context) error {
	if err := h.svc.Delete(c.Request().Context(), "", c.Param("key")); err != nil {
		return systemErrorResponse(c, err)
	}
	c.Logger().Infof("audit: system setting deleted key=%s", c.Param("key"))
	return c.NoContent(204)
}

func (h *SystemHandler) HealthCheck(c echo.Context) error {
	result, err := h.svc.HealthCheck(c.Request().Context())
	if err != nil {
		return systemErrorResponse(c, err)
	}
	return successResponse(c, result)
}
