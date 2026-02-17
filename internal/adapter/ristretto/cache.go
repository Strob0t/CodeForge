// Package ristretto implements the cache port using dgraph-io/ristretto as L1 in-process cache.
package ristretto

import (
	"context"
	"time"

	"github.com/dgraph-io/ristretto/v2"
)

// Cache wraps a ristretto cache as an in-process L1 cache.
type Cache struct {
	c *ristretto.Cache[string, []byte]
}

// New creates a ristretto-backed cache. maxCostBytes is the maximum total
// size of cached values in bytes.
func New(maxCostBytes int64) (*Cache, error) {
	c, err := ristretto.NewCache(&ristretto.Config[string, []byte]{
		NumCounters: maxCostBytes / 100 * 10, // ~10x expected items
		MaxCost:     maxCostBytes,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &Cache{c: c}, nil
}

// Get retrieves a value from the cache.
func (c *Cache) Get(_ context.Context, key string) (data []byte, ok bool, err error) {
	val, found := c.c.Get(key)
	if !found {
		return nil, false, nil
	}
	return val, true, nil
}

// Set stores a value in the cache with the given TTL.
func (c *Cache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.c.SetWithTTL(key, value, int64(len(value)), ttl)
	return nil
}

// Delete removes a value from the cache.
func (c *Cache) Delete(_ context.Context, key string) error {
	c.c.Del(key)
	return nil
}

// Close shuts down the cache and releases resources.
func (c *Cache) Close() {
	c.c.Close()
}
