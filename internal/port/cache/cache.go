// Package cache defines the port interface for caching.
package cache

import (
	"context"
	"time"
)

// Cache is the port interface for key-value caching.
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
