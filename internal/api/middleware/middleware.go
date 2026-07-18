package middleware

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/infra/config"
)

// Setup configures the middleware chain on the Echo instance.
// Order: Recover → RequestID → CORS → Logging
func Setup(e *echo.Echo, cfg *config.Config, logger *zap.Logger) {
	e.Use(echomw.Recover())

	e.Use(echomw.RequestID())

	// CORS: origins from config (server.allowed_origins)
	e.Use(echomw.CORSWithConfig(echomw.CORSConfig{
		AllowOrigins: cfg.Server.AllowedOrigins,
		AllowMethods: []string{echo.GET, echo.POST, echo.PUT, echo.PATCH, echo.DELETE, echo.OPTIONS},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization, echo.HeaderXRequestID},
	}))

	e.Use(RequestLogger(logger))
}

// RequestLogger returns a Zap-based request logging middleware.
func RequestLogger(logger *zap.Logger) echo.MiddlewareFunc {
	return echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogMethod:  true,
		LogLatency: true,
		LogValuesFunc: func(c echo.Context, v echomw.RequestLoggerValues) error {
			logger.Info("http request",
				zap.String("method", v.Method),
				zap.String("uri", v.URI),
				zap.Int("status", v.Status),
				zap.Duration("latency", v.Latency),
				zap.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)),
			)
			return nil
		},
	})
}
