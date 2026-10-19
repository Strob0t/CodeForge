// Package git provides shared utilities for git CLI operations.
package git

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/semaphore"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// Pool limits concurrent git CLI operations using a weighted semaphore.
// All git exec calls across providers and services should go through a shared Pool
// to prevent resource exhaustion when multiple projects need simultaneous git ops.
//
// One tenant holds at most limit-1 slots (the tenant of the context;
// tenantctx's default tenant without one): a tenant whose workspaces make
// git block until its deadline leaves the others at least one slot (S10-D
// review). A pool of one slot has no reserve.
type Pool struct {
	sem       *semaphore.Weighted
	perTenant int64

	mu      sync.Mutex
	tenants map[string]*tenantSlots
}

// tenantSlots are the slots one tenant holds; users counts the Run calls
// that reference them (the entry is dropped at zero).
type tenantSlots struct {
	sem   *semaphore.Weighted
	users int
}

// NewPool creates a Pool that allows at most limit concurrent git operations.
func NewPool(limit int) *Pool {
	if limit < 1 {
		limit = 1
	}
	return &Pool{
		sem:       semaphore.NewWeighted(int64(limit)),
		perTenant: int64(max(1, limit-1)),
		tenants:   make(map[string]*tenantSlots),
	}
}

// Run acquires a slot, runs fn, and releases the slot.
// Blocks if all slots are busy. Returns ctx.Err() if the context
// is cancelled while waiting for a slot.
// If the pool is nil, fn is executed directly without concurrency control.
func (p *Pool) Run(ctx context.Context, fn func() error) error {
	if p == nil || p.sem == nil {
		return fn()
	}
	tenant := tenantctx.FromContext(ctx)
	slots := p.tenantSlots(tenant)
	defer p.dropTenantSlots(tenant)
	if err := slots.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("acquire tenant semaphore: %w", err)
	}
	defer slots.Release(1)
	if err := p.sem.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("acquire semaphore: %w", err)
	}
	defer p.sem.Release(1)
	return fn()
}

// tenantSlots returns the semaphore of tenant and counts one more user.
func (p *Pool) tenantSlots(tenant string) *semaphore.Weighted {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tenants == nil {
		p.tenants = make(map[string]*tenantSlots)
	}
	t := p.tenants[tenant]
	if t == nil {
		t = &tenantSlots{sem: semaphore.NewWeighted(max(1, p.perTenant))}
		p.tenants[tenant] = t
	}
	t.users++
	return t.sem
}

// dropTenantSlots counts one user of tenant's semaphore less.
func (p *Pool) dropTenantSlots(tenant string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := p.tenants[tenant]
	if t == nil {
		return
	}
	t.users--
	if t.users <= 0 {
		delete(p.tenants, tenant)
	}
}
