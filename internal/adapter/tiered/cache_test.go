package tiered_test

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/adapter/tiered"
)

// memCache is a simple in-memory cache for testing.
type memCache struct {
	data map[string][]byte
}

func newMemCache() *memCache {
	return &memCache{data: make(map[string][]byte)}
}

func (m *memCache) Get(_ context.Context, key string) (data []byte, ok bool, err error) {
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *memCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.data[key] = value
	return nil
}

func (m *memCache) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func TestTiered_L1Hit(t *testing.T) {
	l1 := newMemCache()
	l2 := newMemCache()
	c := tiered.New(l1, l2, 5*time.Minute)
	ctx := context.Background()

	// Set only in L1
	l1.data["key1"] = []byte("val1")

	val, found, err := c.Get(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected L1 hit")
	}
	if string(val) != "val1" {
		t.Fatalf("expected val1, got %s", val)
	}
}

func TestTiered_L2HitWithBackfill(t *testing.T) {
	l1 := newMemCache()
	l2 := newMemCache()
	c := tiered.New(l1, l2, 5*time.Minute)
	ctx := context.Background()

	// Set only in L2
	l2.data["key2"] = []byte("val2")

	val, found, err := c.Get(ctx, "key2")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected L2 hit")
	}
	if string(val) != "val2" {
		t.Fatalf("expected val2, got %s", val)
	}

	// Verify backfill into L1
	l1Val, ok := l1.data["key2"]
	if !ok {
		t.Fatal("expected L1 backfill")
	}
	if string(l1Val) != "val2" {
		t.Fatalf("expected backfilled val2, got %s", l1Val)
	}
}

func TestTiered_Miss(t *testing.T) {
	l1 := newMemCache()
	l2 := newMemCache()
	c := tiered.New(l1, l2, 5*time.Minute)
	ctx := context.Background()

	_, found, err := c.Get(ctx, "missing")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected miss")
	}
}

func TestTiered_SetBoth(t *testing.T) {
	l1 := newMemCache()
	l2 := newMemCache()
	c := tiered.New(l1, l2, 5*time.Minute)
	ctx := context.Background()

	if err := c.Set(ctx, "key3", []byte("val3"), time.Minute); err != nil {
		t.Fatal(err)
	}

	if _, ok := l1.data["key3"]; !ok {
		t.Fatal("expected key3 in L1")
	}
	if _, ok := l2.data["key3"]; !ok {
		t.Fatal("expected key3 in L2")
	}
}

func TestTiered_DeleteBoth(t *testing.T) {
	l1 := newMemCache()
	l2 := newMemCache()
	c := tiered.New(l1, l2, 5*time.Minute)
	ctx := context.Background()

	l1.data["key4"] = []byte("val4")
	l2.data["key4"] = []byte("val4")

	if err := c.Delete(ctx, "key4"); err != nil {
		t.Fatal(err)
	}

	if _, ok := l1.data["key4"]; ok {
		t.Fatal("expected key4 deleted from L1")
	}
	if _, ok := l2.data["key4"]; ok {
		t.Fatal("expected key4 deleted from L2")
	}
}
