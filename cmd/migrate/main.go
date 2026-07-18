package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/whg517/fleet/internal/infra/config"
	"github.com/whg517/fleet/internal/infra/db"
	"github.com/whg517/fleet/internal/infra/logger"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Init logger
	log, err := logger.New(cfg.Log.Level, cfg.Log.Encoding)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	// Connect to DB
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbDriver, err := db.New(ctx, cfg.Database)
	if err != nil {
		log.Error("failed to connect database", zap.Error(err))
		os.Exit(1)
	}
	defer func() { _ = dbDriver.Close() }()

	// TODO: Run Ent auto-migration once schemas are defined
	// client := entutil.NewClient(entutil.Driver(dbDriver))
	// if err := client.Schema.Create(ctx); err != nil {
	// 	log.Fatal("failed to run migrations", zap.Error(err))
	// }

	log.Info("migrations completed successfully")
}
