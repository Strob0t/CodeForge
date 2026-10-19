package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/tenant"
)

type fakeToolUIDStore struct {
	mu        sync.Mutex
	next      int
	allocated map[string]int
	calls     int
	err       error
}

func (f *fakeToolUIDStore) AllocateToolUID(_ context.Context, tenantID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	if uid, ok := f.allocated[tenantID]; ok {
		return uid, nil
	}
	if f.allocated == nil {
		f.allocated = map[string]int{}
	}
	f.allocated[tenantID] = f.next
	f.next++
	return f.allocated[tenantID], nil
}

func (f *fakeToolUIDStore) AdvanceToolUIDSequence(_ context.Context, _ int) (bool, error) {
	return false, nil
}

func TestToolUIDForAllocatesOnceAndCaches(t *testing.T) {
	store := &fakeToolUIDStore{next: tenant.ToolUIDMin}
	svc := NewToolUIDService(store, true)
	ctx := context.Background()

	var wg sync.WaitGroup
	uids := make([]int, 20)
	for i := range uids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid, err := svc.ToolUIDFor(ctx, "tenant-a")
			if err != nil {
				t.Error(err)
			}
			uids[i] = uid
		}(i)
	}
	wg.Wait()
	for _, uid := range uids {
		if uid != tenant.ToolUIDMin {
			t.Fatalf("uid = %d, want %d", uid, tenant.ToolUIDMin)
		}
	}
	calls := store.calls
	if _, err := svc.ToolUIDFor(ctx, "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if store.calls != calls {
		t.Fatalf("a cached UID went to the store again")
	}
	other, err := svc.ToolUIDFor(ctx, "tenant-b")
	if err != nil || other != tenant.ToolUIDMin+1 {
		t.Fatalf("tenant-b: uid %d, err %v", other, err)
	}
}

func TestToolUIDForErrors(t *testing.T) {
	ctx := context.Background()
	exhausted := &fakeToolUIDStore{err: fmt.Errorf("allocate: %w", tenant.ErrToolUIDRangeExhausted)}
	if _, err := NewToolUIDService(exhausted, true).ToolUIDFor(ctx, "t"); !errors.Is(err, tenant.ErrToolUIDRangeExhausted) {
		t.Fatalf("exhausted: err = %v", err)
	}
	if _, err := NewToolUIDService(exhausted, true).ToolUIDFor(ctx, ""); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("no tenant: err = %v", err)
	}
	for _, uid := range []int{0, 10002, tenant.SystemToolUID, tenant.ToolUIDMax + 1} {
		store := &fakeToolUIDStore{next: uid}
		if _, err := NewToolUIDService(store, true).ToolUIDFor(ctx, "t"); err == nil {
			t.Fatalf("uid %d outside the range was accepted", uid)
		}
	}
}

func TestPayloadToolUIDIsZeroWithToolACLsOff(t *testing.T) {
	store := &fakeToolUIDStore{next: tenant.ToolUIDMin}
	for _, svc := range []*ToolUIDService{nil, NewToolUIDService(store, false)} {
		uid, err := svc.PayloadToolUID(context.Background(), "tenant-a")
		if uid != 0 || err != nil {
			t.Fatalf("off: uid %d, err %v", uid, err)
		}
	}
	if store.calls != 0 {
		t.Fatalf("off allocated a UID")
	}
	uid, err := NewToolUIDService(store, true).PayloadToolUID(context.Background(), "tenant-a")
	if uid != tenant.ToolUIDMin || err != nil {
		t.Fatalf("required: uid %d, err %v", uid, err)
	}
}
