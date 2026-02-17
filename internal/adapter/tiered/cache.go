// Package tiered implements a two-level (L1 + L2) cache adapter.
package tiered

import (
	"context"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/cache"
)

// Cache combines an L1 (in-process) and L2 (remote) cache.
// Get checks L1 first, then L2 (backfilling L1 on L2 hit).
// Set and Delete operate on both levels.
type Cache struct {
	l1       cache.Cache
	l2       cache.Cache
	l1Expire time.Duration
}

// New creates a tiered cache with the given L1 and L2 backends.
// l1Expire controls how long L2 backfill entries live in L1.
func New(l1, l2 cache.Cache, l1Expire time.Duration) *Cache {
	return &Cache{l1: l1, l2: l2, l1Expire: l1Expire}
}

// Get checks L1, then L2. On L2 hit, backfills L1.
func (c *Cache) Get(ctx context.Context, key string) (data []byte, ok bool, err error) {
	// L1
	val, found, err := c.l1.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if found {
		return val, true, nil
	}

	// L2
	val, found, err = c.l2.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if found {
		// Backfill L1
		_ = c.l1.Set(ctx, key, val, c.l1Expire)
		return val, true, nil
	}

	return nil, false, nil
}

// Set writes to both L1 and L2.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.l1.Set(ctx, key, value, ttl); err != nil {
		return err
	}
	return c.l2.Set(ctx, key, value, ttl)
}

// Delete removes from both L1 and L2.
func (c *Cache) Delete(ctx context.Context, key string) error {
	if err := c.l1.Delete(ctx, key); err != nil {
		return err
	}
	return c.l2.Delete(ctx, key)
}
