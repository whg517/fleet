package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/whg517/fleet/internal/infra/config"
)

// New creates a Redis client using the given config and pings the server.
func New(ctx context.Context, cfg config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to ping redis: %w", err)
	}

	return client, nil
}
