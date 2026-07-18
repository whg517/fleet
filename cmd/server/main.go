package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/api"
	"github.com/whg517/fleet/internal/api/middleware"
	"github.com/whg517/fleet/internal/infra/config"
	"github.com/whg517/fleet/internal/infra/db"
	fleetredis "github.com/whg517/fleet/internal/infra/redis"
	"github.com/whg517/fleet/internal/infra/logger"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	if err := run(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath string) error {
	// 1. Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// 2. Init logger
	log, err := logger.New(cfg.Log.Level, cfg.Log.Encoding)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	log.Info("starting fleet server",
		zap.Int("port", cfg.Server.Port),
	)

	// 3. Connect to PostgreSQL
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dbDriver, err := db.New(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer func() { _ = dbDriver.Close() }()
	log.Info("database connected")

	// 4. Connect to Redis
	redisClient, err := fleetredis.New(ctx, cfg.Redis)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = redisClient.Close() }()
	log.Info("redis connected")

	// 5. Create Echo + middleware chain
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true

	middleware.Setup(e, cfg, log)

	// 6. Register routes
	api.RegisterRoutes(e, dbDriver, redisClient, log)

	// 7. Configure server timeouts
	e.Server.Addr = fmt.Sprintf(":%d", cfg.Server.Port)
	e.Server.ReadTimeout = cfg.Server.ReadTimeout
	e.Server.WriteTimeout = cfg.Server.WriteTimeout

	// 8. Start server in goroutine — errors piped via channel
	errCh := make(chan error, 1)
	go func() {
		log.Info("http server listening", zap.String("addr", e.Server.Addr))
		if err := e.Start(e.Server.Addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	// 9. Wait for signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		log.Info("received shutdown signal", zap.String("signal", sig.String()))
	case err := <-errCh:
		log.Error("server error", zap.Error(err))
	}

	// 10. Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := e.Shutdown(shutdownCtx); err != nil {
		log.Error("server shutdown error", zap.Error(err))
	}

	log.Info("server stopped")
	return nil
}
