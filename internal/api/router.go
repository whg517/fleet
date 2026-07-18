package api

import (
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/api/handler"
	"github.com/whg517/fleet/internal/api/middleware"
	"github.com/whg517/fleet/internal/domain/audit"
	"github.com/whg517/fleet/internal/domain/cluster"
	"github.com/whg517/fleet/internal/infra/config"
	"github.com/whg517/fleet/internal/infra/secrets"
	entclient "github.com/whg517/fleet/internal/store/ent"
)

// RegisterRoutes sets up all HTTP routes on the Echo instance.
func RegisterRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client) {
	registerRoutes(e, dbDriver, redisClient, nil, nil)
}

// RegisterRoutesWithConfig sets up routes with full configuration (audit, cluster management, etc.)
func RegisterRoutesWithConfig(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, cfg *config.Config, logger *zap.Logger) {
	registerRoutes(e, dbDriver, redisClient, cfg, logger)
}

func registerRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, cfg *config.Config, logger *zap.Logger) {
	entClient := entclient.NewClient(entclient.Driver(dbDriver))

	// Create audit service and middleware
	auditSvc := audit.NewService(entClient, logger)
	auditMW := middleware.AuditMiddleware(auditSvc, logger)

	v1 := e.Group("/api/v1", auditMW)

	// Health endpoints
	healthH := handler.NewHealthHandler(dbDriver, redisClient)
	v1.GET("/health", healthH.Liveness)
	v1.GET("/health/ready", healthH.Readiness)

	// Audit log endpoints
	auditH := handler.NewAuditHandler(auditSvc, logger)
	v1.GET("/audit-logs", auditH.List)
	v1.GET("/audit-logs/verify", auditH.Verify)

	// Cluster & Environment management
	if cfg != nil && logger != nil {
		dek, err := secrets.ParseDEK(cfg.Security.DEK)
		if err != nil || len(dek) != 32 {
			logger.Fatal("FLEET_SECURITY_DEK must be set to a valid 32-byte hex string", zap.Error(err))
		}

		store := cluster.NewEntStore(entClient)
		clusterSvc := cluster.NewService(store, dek, logger)
		clusterH := handler.NewClusterHandler(clusterSvc)

		// Operator/admin middleware placeholder (RBAC in Issue #12)
		operatorMW := operatorGuard()

		clusters := v1.Group("/clusters")
		clusters.POST("", clusterH.Create, operatorMW)
		clusters.GET("", clusterH.List)
		clusters.GET("/:id", clusterH.Get)
		clusters.PUT("/:id", clusterH.Update, operatorMW)
		clusters.DELETE("/:id", clusterH.Delete, operatorMW)
		clusters.POST("/:id/test", clusterH.TestConnection, operatorMW)
		clusters.POST("/:id/environments", clusterH.CreateEnvironment, operatorMW)
		clusters.GET("/:id/environments", clusterH.ListEnvironments)

		// Global environment list
		v1.GET("/environments", clusterH.ListAllEnvironments)
	}
}

// operatorGuard is a placeholder middleware for operator/admin RBAC.
// Full RBAC will be implemented in Issue #12.
func operatorGuard() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// TODO(Issue #12): Implement proper RBAC check here
			// For now, allow all requests
			return next(c)
		}
	}
}
