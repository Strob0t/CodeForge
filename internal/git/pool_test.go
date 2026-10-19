package git

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
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

// S10-D review: one tenant whose workspaces block git (until the checks'
// deadline) must not hold every slot of the shared pool: each tenant gets
// at most limit-1 slots, so the others keep at least one.
func TestPoolKeepsASlotForOtherTenants(t *testing.T) {
	const limit = 3
	pool := NewPool(limit)
	tenantA := tenantctx.WithTenant(context.Background(), "aaaaaaaa-0000-4000-8000-000000000001")
	tenantB := tenantctx.WithTenant(context.Background(), "bbbbbbbb-0000-4000-8000-000000000002")

	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAll := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAll()
	var runningA, maxA atomic.Int32
	started := make(chan struct{}, limit)
	for range limit {
		go func() {
			_ = pool.Run(tenantA, func() error {
				cur := runningA.Add(1)
				for old := maxA.Load(); cur > old && !maxA.CompareAndSwap(old, cur); old = maxA.Load() {
				}
				started <- struct{}{}
				<-release
				runningA.Add(-1)
				return nil
			})
		}()
	}
	for range limit - 1 {
		<-started
	}
	select { // the last of A's jobs would have started by now without a cap
	case <-started:
		t.Fatalf("tenant A holds all %d slots", limit)
	case <-time.After(200 * time.Millisecond):
	}

	ctx, cancel := context.WithTimeout(tenantB, 5*time.Second)
	defer cancel()
	ranB := false
	if err := pool.Run(ctx, func() error { ranB = true; return nil }); err != nil || !ranB {
		t.Fatalf("tenant B got no slot while tenant A held its share: %v", err)
	}
	if got := maxA.Load(); got != limit-1 {
		t.Fatalf("tenant A ran %d at once, want %d", got, limit-1)
	}
	releaseAll()
	<-started // A's last job runs once a slot of its share is free
}
