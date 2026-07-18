package api

import (
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/api/handler"
	"github.com/whg517/fleet/internal/api/middleware"
	"github.com/whg517/fleet/internal/domain/audit"
	"github.com/whg517/fleet/internal/store/ent"
)

// RegisterRoutes sets up all HTTP routes on the Echo instance.
func RegisterRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, logger *zap.Logger) {
	// Create Ent client from the existing driver
	entClient := ent.NewClient(ent.Driver(dbDriver))

	// Create audit service
	auditSvc := audit.NewService(entClient, logger)

	// Register audit middleware on the v1 API group
	auditMW := middleware.AuditMiddleware(auditSvc, logger)

	v1 := e.Group("/api/v1", auditMW)

	// Health endpoints (no audit middleware needed, but group-level MW applies)
	healthH := handler.NewHealthHandler(dbDriver, redisClient)
	v1.GET("/health", healthH.Liveness)
	v1.GET("/health/ready", healthH.Readiness)

	// Audit log endpoints
	auditH := handler.NewAuditHandler(auditSvc, logger)
	v1.GET("/audit-logs", auditH.List)
	v1.GET("/audit-logs/verify", auditH.Verify)
}
