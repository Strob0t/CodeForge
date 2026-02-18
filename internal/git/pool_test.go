package git

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolLimitsConcurrency(t *testing.T) {
	const limit = 3
	const workers = 10
	pool := NewPool(limit)

	var running atomic.Int32
	var maxSeen atomic.Int32

	ctx := context.Background()
	done := make(chan struct{}, workers)

	for range workers {
		go func() {
			defer func() { done <- struct{}{} }()
			err := pool.Run(ctx, func() error {
				cur := running.Add(1)
				// Record high-water mark
				for {
					old := maxSeen.Load()
					if cur <= old || maxSeen.CompareAndSwap(old, cur) {
						break
					}
				}
				time.Sleep(10 * time.Millisecond)
				running.Add(-1)
				return nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}

	for range workers {
		<-done
	}

	if m := maxSeen.Load(); m > limit {
		t.Errorf("max concurrent = %d, want <= %d", m, limit)
	}
}

func TestPoolContextCancellation(t *testing.T) {
	pool := NewPool(1)
	ctx := context.Background()

	// Fill the single slot
	occupied := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = pool.Run(ctx, func() error {
			close(occupied)
			<-release
			return nil
		})
	}()
	<-occupied

	// Try to acquire with a cancelled context
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()

	err := pool.Run(cancelCtx, func() error {
		t.Error("fn should not have been called")
		return nil
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}

	close(release)
}

func TestPoolAllowsWithinLimit(t *testing.T) {
	pool := NewPool(5)
	ctx := context.Background()

	for i := range 5 {
		err := pool.Run(ctx, func() error { return nil })
		if err != nil {
			t.Errorf("iteration %d: unexpected error: %v", i, err)
		}
	}
}

func TestPoolClampMinLimit(t *testing.T) {
	pool := NewPool(0)
	ctx := context.Background()

	err := pool.Run(ctx, func() error { return nil })
	if err != nil {
		t.Errorf("unexpected error with limit=0 (should clamp to 1): %v", err)
	}
}
