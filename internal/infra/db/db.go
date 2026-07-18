package db

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/whg517/fleet/internal/infra/config"

	// PostgreSQL driver.
	_ "github.com/lib/pq"
)

// New creates an Ent SQL client connected to PostgreSQL using the given config.
func New(ctx context.Context, cfg config.DatabaseConfig) (*sql.Driver, error) {
	drv, err := sql.Open(dialect.Postgres, cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	// Configure connection pool.
	db := drv.DB()
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(0)

	if err := db.PingContext(ctx); err != nil {
		_ = drv.Close()
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}

	return drv, nil
}
