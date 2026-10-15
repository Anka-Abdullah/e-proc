// Package cache wraps Redis with the caching, idempotency and session helpers
// the platform uses.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss reports that a key was absent. Callers treat it as a cache miss, not
// as a failure.
var ErrMiss = errors.New("cache: miss")

// Client is the shared Redis handle plus the helpers built on top of it.
type Client struct {
	rdb *redis.Client
}

// New connects to Redis and verifies the connection with a PING.
func New(ctx context.Context, addr, password string, database int) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           database,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

// Raw exposes the underlying client for the rare direct use.
func (c *Client) Raw() *redis.Client { return c.rdb }

// Close releases the connection pool.
func (c *Client) Close() error { return c.rdb.Close() }

// GetJSON reads a key into dest. It returns ErrMiss when the key is absent.
func (c *Client) GetJSON(ctx context.Context, key string, dest any) error {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return fmt.Errorf("cache get %s: %w", key, err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("cache decode %s: %w", key, err)
	}
	return nil
}

// SetJSON stores value as JSON under key for ttl. A ttl of 0 means no expiry.
func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache encode %s: %w", key, err)
	}
	if err := c.rdb.Set(ctx, key, raw, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}
	return nil
}

// Delete removes keys; it is used to invalidate cached reads after a write.
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.rdb.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// ClaimIdempotency records an idempotency key exactly once. The boolean reports
// whether the caller won the race; false means the request was already handled.
func (c *Client) ClaimIdempotency(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := c.rdb.SetNX(ctx, "idem:"+key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency claim %s: %w", key, err)
	}
	return ok, nil
}

// StoreSession writes a session token with its TTL.
func (c *Client) StoreSession(ctx context.Context, token, subject string, ttl time.Duration) error {
	return c.rdb.Set(ctx, "session:"+token, subject, ttl).Err()
}

// SessionSubject returns the subject bound to a session token.
func (c *Client) SessionSubject(ctx context.Context, token string) (string, error) {
	subject, err := c.rdb.Get(ctx, "session:"+token).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	return subject, err
}

// RevokeSession deletes a session token.
func (c *Client) RevokeSession(ctx context.Context, token string) error {
	return c.rdb.Del(ctx, "session:"+token).Err()
}

// IncrWithTTL counts requests inside a window: the backbone of rate limiting.
func (c *Client) IncrWithTTL(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := c.rdb.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("rate limit incr %s: %w", key, err)
	}
	return count.Val(), nil
}
