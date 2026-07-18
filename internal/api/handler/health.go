package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	entsql "entgo.io/ent/dialect/sql"
)

// HealthHandler handles health-check endpoints.
type HealthHandler struct {
	redisClient *redis.Client
	dbDriver    *entsql.Driver
}

// NewHealthHandler creates a HealthHandler.
func NewHealthHandler(dbDriver *entsql.Driver, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{
		dbDriver:    dbDriver,
		redisClient: redisClient,
	}
}

// Liveness returns 200 if the process is running.
func (h *HealthHandler) Liveness(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Readiness checks DB and Redis connectivity.
func (h *HealthHandler) Readiness(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), 3*time.Second)
	defer cancel()

	// Check DB
	if h.dbDriver != nil {
		sqldb := h.dbDriver.DB()
		if err := sqldb.PingContext(ctx); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status":   "error",
				"database": "unreachable",
			})
		}
	}

	// Check Redis
	if h.redisClient != nil {
		if err := h.redisClient.Ping(ctx).Err(); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status": "error",
				"redis":  "unreachable",
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
