package api

import (
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/api/handler"
	"github.com/whg517/fleet/internal/api/middleware"
	"github.com/whg517/fleet/internal/domain/audit"
	"github.com/whg517/fleet/internal/domain/auth"
	"github.com/whg517/fleet/internal/domain/cluster"
	"github.com/whg517/fleet/internal/domain/rbac"
	"github.com/whg517/fleet/internal/infra/config"
	"github.com/whg517/fleet/internal/infra/secrets"
	entclient "github.com/whg517/fleet/internal/store/ent"
)

// Deps holds shared dependencies for route registration.
type Deps struct {
	DBDriver    *entsql.Driver
	EntClient   *entclient.Client
	RedisClient *redis.Client
	Config      *config.Config
	Logger      *zap.Logger
	RBACService rbac.Service
}

// RegisterRoutes sets up all HTTP routes on the Echo instance.
func RegisterRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client) {
	registerRoutes(e, dbDriver, redisClient, nil, nil, nil)
}

// RegisterRoutesWithConfig sets up routes with full configuration (audit, cluster management, etc.)
func RegisterRoutesWithConfig(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, cfg *config.Config, logger *zap.Logger) {
	registerRoutes(e, dbDriver, redisClient, cfg, logger, nil)
}

func registerRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, cfg *config.Config, logger *zap.Logger, rbacSvc rbac.Service) {
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

	// RBAC middleware for protected resource endpoints
	var rbacMW echo.MiddlewareFunc
	if rbacSvc != nil && logger != nil {
		rbacMW = middleware.RBACMiddleware(rbacSvc, logger)
	}

	// Cluster & Environment management
	if cfg != nil && logger != nil {
		dek, err := secrets.ParseDEK(cfg.Security.DEK)
		if err != nil || len(dek) != 32 {
			logger.Fatal("invalid or missing DEK: FLEET_SECURITY_DEK must be set to a valid 64-char hex string (32 bytes)",
				zap.Error(err),
			)
		}

		store := cluster.NewEntStore(entClient)
		clusterSvc := cluster.NewService(store, dek, logger)
		clusterH := handler.NewClusterHandler(clusterSvc)

		// Apply RBAC middleware if available, otherwise allow all
		var groupMW []echo.MiddlewareFunc
		if rbacMW != nil {
			groupMW = append(groupMW, rbacMW)
		}

		clusters := v1.Group("/clusters", groupMW...)
		clusters.POST("", clusterH.Create)
		clusters.GET("", clusterH.List)
		clusters.GET("/:id", clusterH.Get)
		clusters.PUT("/:id", clusterH.Update)
		clusters.DELETE("/:id", clusterH.Delete)
		clusters.POST("/:id/test", clusterH.TestConnection)
		clusters.POST("/:id/environments", clusterH.CreateEnvironment)
		clusters.GET("/:id/environments", clusterH.ListEnvironments)

		// Global environment list
		if rbacMW != nil {
			v1.GET("/environments", clusterH.ListAllEnvironments, rbacMW)
		} else {
			v1.GET("/environments", clusterH.ListAllEnvironments)
		}
	}

	// RBAC management endpoints
	if rbacSvc != nil && logger != nil {
		rbacH := handler.NewRBACHandler(rbacSvc, logger)
		var rbacGroupMW []echo.MiddlewareFunc
		if rbacMW != nil {
			rbacGroupMW = append(rbacGroupMW, rbacMW)
		}
		rbacGroup := v1.Group("/rbac", rbacGroupMW...)
		rbacGroup.GET("/roles", rbacH.ListRoles)
		rbacGroup.GET("/users/:id/roles", rbacH.GetUserRoles)
		rbacGroup.PUT("/users/:id/roles", rbacH.AssignUserRoles)
		rbacGroup.GET("/permissions", rbacH.GetPermissions)
		rbacGroup.POST("/users/:id/disable", rbacH.DisableUser)
		rbacGroup.POST("/users/:id/enable", rbacH.EnableUser)
	}
}

// RegisterRoutesWithDeps sets up all HTTP routes using the full dependency set.
// This is the preferred entry point when auth and other services are available.
// It registers audit, cluster, auth, and RBAC routes.
func RegisterRoutesWithDeps(e *echo.Echo, deps Deps) {
	// Register core routes (health + audit + cluster + RBAC).
	registerRoutes(e, deps.DBDriver, deps.RedisClient, deps.Config, deps.Logger, deps.RBACService)

	// Auth service
	sessionMgr := auth.NewSessionManager(deps.Config.JWT, deps.RedisClient)
	authSvc := auth.NewService(
		deps.Config.OIDC,
		deps.Config.JWT,
		deps.EntClient,
		deps.RedisClient,
		deps.Logger,
	)

	// Auth handler group (public endpoints, no token required)
	authH := handler.NewAuthHandler(authSvc, deps.Config.JWT, deps.Logger)
	authGroup := e.Group("/api/v1/auth")
	authGroup.GET("/login", authH.Login)
	authGroup.GET("/callback", authH.Callback)
	authGroup.POST("/token", authH.ExchangeToken)
	authGroup.POST("/refresh", authH.Refresh)
	authGroup.POST("/logout", authH.Logout)

	// Protected auth endpoint
	authGroup.GET("/me", authH.Me, middleware.AuthMiddleware(sessionMgr, deps.Logger))
}
