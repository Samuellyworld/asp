// redis client
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/trading-bot/go-bot/internal/config"
)

type RedisClient struct {
	client *redis.Client
}

func NewRedisClient(cfg config.RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     10,
		MinIdleConns: 2,
	})

	return &RedisClient{client: client}, nil
}

func (c *RedisClient) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	_, err := c.client.Ping(ctx).Result()
	return time.Since(start), err
}

func (c *RedisClient) Close() error {
	return c.client.Close()
}

func (c *RedisClient) Client() *redis.Client {
	return c.client
}

func (c *RedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.client.Set(ctx, key, value, expiration).Err()
}

func (c *RedisClient) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, key).Result()
}

func (c *RedisClient) Del(ctx context.Context, keys ...string) error {
	return c.client.Del(ctx, keys...).Err()
}

func (c *RedisClient) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

func (c *RedisClient) Expire(ctx context.Context, key string, expiration time.Duration) error {
	return c.client.Expire(ctx, key, expiration).Err()
}

func (c *RedisClient) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, key).Result()
}

// rate limiting helper
func (c *RedisClient) RateLimit(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	count, err := c.Incr(ctx, key)
	if err != nil {
		return false, fmt.Errorf("failed to increment rate limit counter: %w", err)
	}

	if count == 1 {
		if err := c.Expire(ctx, key, window); err != nil {
			return false, fmt.Errorf("failed to set rate limit expiry: %w", err)
		}
	}

	return count <= limit, nil
}
