package api

import (
	"context"
	"fmt"

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
	"github.com/whg517/fleet/internal/domain/service"
	sysdomain "github.com/whg517/fleet/internal/domain/system"
	"github.com/whg517/fleet/internal/domain/template"
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
	registerRoutes(e, dbDriver, redisClient, nil, nil, nil, nil)
}

// RegisterRoutesWithConfig sets up routes with full configuration (audit, cluster management, etc.)
func RegisterRoutesWithConfig(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, cfg *config.Config, logger *zap.Logger) {
	registerRoutes(e, dbDriver, redisClient, cfg, logger, nil, nil)
}

func registerRoutes(e *echo.Echo, dbDriver *entsql.Driver, redisClient *redis.Client, cfg *config.Config, logger *zap.Logger, rbacSvc rbac.Service, sessionMgr *auth.SessionManager) {
	entClient := entclient.NewClient(entclient.Driver(dbDriver))

	// Create audit service and middleware
	auditSvc := audit.NewService(entClient, logger)
	auditMW := middleware.AuditMiddleware(auditSvc, logger)

	// --- Public group: health endpoints (no auth, no audit) ---
	public := e.Group("/api/v1")
	healthH := handler.NewHealthHandler(dbDriver, redisClient)
	public.GET("/health", healthH.Liveness)
	public.GET("/health/ready", healthH.Readiness)

	// --- Protected group: auth required ---
	var authMW echo.MiddlewareFunc
	if sessionMgr != nil && logger != nil {
		authMW = middleware.AuthMiddleware(sessionMgr, logger)
	}

	// Build middleware chain for protected routes: audit + auth
	protectedMW := []echo.MiddlewareFunc{auditMW}
	if authMW != nil {
		protectedMW = append(protectedMW, authMW)
	}

	v1 := e.Group("/api/v1", protectedMW...)

	// Audit log endpoints (protected)
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

		// Apply RBAC middleware if available
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

		// Service catalog management
		svcStore := service.NewEntStore(entClient)
		serviceSvc := service.NewService(svcStore, logger)
		serviceH := handler.NewServiceHandler(serviceSvc)

		var svcMW []echo.MiddlewareFunc
		if rbacMW != nil {
			svcMW = append(svcMW, rbacMW)
		}
		services := v1.Group("/services", svcMW...)
		services.POST("", serviceH.Create)
		services.GET("", serviceH.List)
		services.GET("/:id", serviceH.Get)
		services.PUT("/:id", serviceH.Update)
		services.DELETE("/:id", serviceH.Delete)

		// Template management
		tmplStore := template.NewEntStore(entClient)
		tmplSvc := template.NewService(tmplStore, logger)
		tmplH := handler.NewTemplateHandler(tmplSvc)

		var tmplMW []echo.MiddlewareFunc
		if rbacMW != nil {
			tmplMW = append(tmplMW, rbacMW)
		}
		templates := v1.Group("/templates", tmplMW...)
		templates.POST("", tmplH.Create)
		templates.GET("", tmplH.List)
		templates.GET("/:id", tmplH.Get)
		templates.PUT("/:id", tmplH.Update)
		templates.DELETE("/:id", tmplH.Delete)
		templates.POST("/:id/versions", tmplH.PublishVersion)
		templates.GET("/:id/versions", tmplH.ListVersions)
		templates.POST("/:id/versions/:ver/archive", tmplH.ArchiveVersion)

		// System settings management
		sysStore := sysdomain.NewEntStore(entClient)
		healthChecker := &infraHealthChecker{dbDriver: dbDriver, redisClient: redisClient}
		sysSvc := sysdomain.NewService(sysStore, healthChecker, dek, logger)
		sysH := handler.NewSystemHandler(sysSvc)

		// Health-check is public (no auth required)
		public.GET("/system/health-check", sysH.HealthCheck)

		// Settings reads: any authenticated user
		var sysReadMW []echo.MiddlewareFunc
		if rbacMW != nil {
			sysReadMW = append(sysReadMW, rbacMW)
		}
		sysGroup := v1.Group("/system", sysReadMW...)
		sysGroup.GET("/settings", sysH.ListSettings)
		sysGroup.GET("/settings/:key", sysH.GetSetting)

		// Settings writes: admin-only
		sysAdminMW := []echo.MiddlewareFunc{middleware.RequireRole("admin", logger)}
		if rbacMW != nil {
			sysAdminMW = append([]echo.MiddlewareFunc{rbacMW}, sysAdminMW...)
		}
		sysAdmin := v1.Group("/system", sysAdminMW...)
		sysAdmin.PUT("/settings/:key", sysH.SetSetting)
		sysAdmin.DELETE("/settings/:key", sysH.DeleteSetting)
	}

	// RBAC management endpoints (protected + admin-only for mutations)
	if rbacSvc != nil && logger != nil {
		rbacH := handler.NewRBACHandler(rbacSvc, logger)

		// Read endpoints: any authenticated user with a valid role
		var readMW []echo.MiddlewareFunc
		if rbacMW != nil {
			readMW = append(readMW, rbacMW)
		}
		rbacRead := v1.Group("/rbac", readMW...)
		rbacRead.GET("/roles", rbacH.ListRoles)
		rbacRead.GET("/users/:id/roles", rbacH.GetUserRoles)
		rbacRead.GET("/permissions", rbacH.GetPermissions)

		// Write endpoints: admin-only
		adminMW := []echo.MiddlewareFunc{middleware.RequireRole("admin", logger)}
		if rbacMW != nil {
			adminMW = append([]echo.MiddlewareFunc{rbacMW}, adminMW...)
		}
		rbacAdmin := v1.Group("/rbac", adminMW...)
		rbacAdmin.PUT("/users/:id/roles", rbacH.AssignUserRoles)
		rbacAdmin.POST("/users/:id/disable", rbacH.DisableUser)
		rbacAdmin.POST("/users/:id/enable", rbacH.EnableUser)
	}
}

// RegisterRoutesWithDeps sets up all HTTP routes using the full dependency set.
// This is the preferred entry point when auth and other services are available.
// It registers audit, cluster, auth, and RBAC routes.
func RegisterRoutesWithDeps(e *echo.Echo, deps Deps) {
	// Create session manager first so it can be passed to registerRoutes.
	sessionMgr := auth.NewSessionManager(deps.Config.JWT, deps.RedisClient)

	// Register core routes (health + audit + cluster + RBAC).
	// Auth middleware is applied to the protected v1 group inside registerRoutes.
	registerRoutes(e, deps.DBDriver, deps.RedisClient, deps.Config, deps.Logger, deps.RBACService, sessionMgr)

	// Auth service
	authSvc := auth.NewService(
		deps.Config.OIDC,
		deps.Config.JWT,
		deps.EntClient,
		deps.RedisClient,
		deps.Logger,
	)

	// Auth handler group (public endpoints, no token required)
	authH := handler.NewAuthHandler(authSvc, deps.Config.JWT, deps.Config.Server, deps.Logger)
	authGroup := e.Group("/api/v1/auth")
	authGroup.GET("/login", authH.Login)
	authGroup.GET("/callback", authH.Callback)
	authGroup.POST("/token", authH.ExchangeToken)
	authGroup.POST("/refresh", authH.Refresh)
	authGroup.POST("/logout", authH.Logout)

	// Protected auth endpoint
	authGroup.GET("/me", authH.Me, middleware.AuthMiddleware(sessionMgr, deps.Logger))
}

// infraHealthChecker adapts DB and Redis clients to the system.HealthChecker interface.
type infraHealthChecker struct {
	dbDriver    *entsql.Driver
	redisClient *redis.Client
}

func (h *infraHealthChecker) PingDB(ctx context.Context) error {
	if h.dbDriver == nil {
		return fmt.Errorf("db driver not configured")
	}
	return h.dbDriver.DB().PingContext(ctx)
}

func (h *infraHealthChecker) PingRedis(ctx context.Context) error {
	if h.redisClient == nil {
		return fmt.Errorf("redis client not configured")
	}
	return h.redisClient.Ping(ctx).Err()
}
