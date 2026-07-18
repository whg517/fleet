package api

import (
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/api/handler"
)

// RegisterRoutes sets up all HTTP routes on the Echo instance.
func RegisterRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client) {
	v1 := e.Group("/api/v1")

	healthH := handler.NewHealthHandler(dbDriver, redisClient)
	v1.GET("/health", healthH.Liveness)
	v1.GET("/health/ready", healthH.Readiness)
}
