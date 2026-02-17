// Package natskv implements the cache port using NATS JetStream KV as L2 remote cache.
package natskv

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Cache wraps a NATS JetStream KeyValue store as an L2 cache.
type Cache struct {
	kv jetstream.KeyValue
}

// New creates a NATS KV-backed cache.
func New(kv jetstream.KeyValue) *Cache {
	return &Cache{kv: kv}
}

// Get retrieves a value from the NATS KV store.
func (c *Cache) Get(ctx context.Context, key string) (data []byte, ok bool, err error) {
	entry, err := c.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return entry.Value(), true, nil
}

// Set stores a value in the NATS KV store. TTL is managed at bucket level.
func (c *Cache) Set(ctx context.Context, key string, value []byte, _ time.Duration) error {
	_, err := c.kv.Put(ctx, key, value)
	return err
}

// Delete removes a value from the NATS KV store.
func (c *Cache) Delete(ctx context.Context, key string) error {
	err := c.kv.Delete(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil
	}
	return err
}
