// Package git provides shared utilities for git CLI operations.
package git

import (
	"context"

	"golang.org/x/sync/semaphore"
)

// Pool limits concurrent git CLI operations using a weighted semaphore.
// All git exec calls across providers and services should go through a shared Pool
// to prevent resource exhaustion when multiple projects need simultaneous git ops.
type Pool struct {
	sem *semaphore.Weighted
}

// NewPool creates a Pool that allows at most limit concurrent git operations.
func NewPool(limit int) *Pool {
	if limit < 1 {
		limit = 1
	}
	return &Pool{sem: semaphore.NewWeighted(int64(limit))}
}

// Run acquires a slot, runs fn, and releases the slot.
// Blocks if all slots are busy. Returns ctx.Err() if the context
// is cancelled while waiting for a slot.
// If the pool is nil, fn is executed directly without concurrency control.
func (p *Pool) Run(ctx context.Context, fn func() error) error {
	if p == nil || p.sem == nil {
		return fn()
	}
	if err := p.sem.Acquire(ctx, 1); err != nil {
		return err
	}
	defer p.sem.Release(1)
	return fn()
}
