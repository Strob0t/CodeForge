package ristretto

import (
	"context"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20) // 1 MB
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	if cache == nil {
		t.Fatal("New() returned nil")
	}
	if cache.c == nil {
		t.Error("cache.c is nil")
	}
}

func TestGetMissing(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	data, ok, getErr := cache.Get(ctx, "nonexistent")
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}
	if ok {
		t.Error("Get() ok = true, want false for missing key")
	}
	if data != nil {
		t.Errorf("Get() data = %v, want nil", data)
	}
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	value := []byte("test-value")

	if setErr := cache.Set(ctx, "key1", value, time.Minute); setErr != nil {
		t.Fatalf("Set() error = %v", setErr)
	}

	// Ristretto uses buffered writes; wait for the item to be processed.
	cache.c.Wait()

	data, ok, getErr := cache.Get(ctx, "key1")
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}
	if !ok {
		t.Error("Get() ok = false, want true")
	}
	if string(data) != "test-value" {
		t.Errorf("Get() data = %q, want %q", string(data), "test-value")
	}
}

func TestSetOverwrite(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	if setErr := cache.Set(ctx, "key", []byte("v1"), time.Minute); setErr != nil {
		t.Fatalf("Set() v1 error = %v", setErr)
	}
	cache.c.Wait()

	if setErr := cache.Set(ctx, "key", []byte("v2"), time.Minute); setErr != nil {
		t.Fatalf("Set() v2 error = %v", setErr)
	}
	cache.c.Wait()

	data, ok, getErr := cache.Get(ctx, "key")
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}
	if !ok {
		t.Error("Get() ok = false, want true")
	}
	if string(data) != "v2" {
		t.Errorf("Get() data = %q, want %q", string(data), "v2")
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	if setErr := cache.Set(ctx, "key", []byte("value"), time.Minute); setErr != nil {
		t.Fatalf("Set() error = %v", setErr)
	}
	cache.c.Wait()

	if delErr := cache.Delete(ctx, "key"); delErr != nil {
		t.Fatalf("Delete() error = %v", delErr)
	}

	// After deletion, Get should return not found.
	data, ok, getErr := cache.Get(ctx, "key")
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}
	if ok {
		t.Error("Get() ok = true, want false after Delete")
	}
	if data != nil {
		t.Errorf("Get() data = %v, want nil after Delete", data)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	// Delete of nonexistent key should succeed (no error).
	if delErr := cache.Delete(ctx, "nonexistent"); delErr != nil {
		t.Errorf("Delete() error = %v, want nil for nonexistent key", delErr)
	}
}

func TestSetEmptyValue(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()

	if setErr := cache.Set(ctx, "empty", []byte{}, time.Minute); setErr != nil {
		t.Fatalf("Set() error = %v", setErr)
	}
	cache.c.Wait()

	// Ristretto may or may not store zero-cost items, so we just verify no error.
	_, _, getErr := cache.Get(ctx, "empty")
	if getErr != nil {
		t.Fatalf("Get() error = %v", getErr)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Close should not panic.
	cache.Close()
}

func TestSetReturnsNilError(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	// Set always returns nil for ristretto.
	if setErr := cache.Set(ctx, "key", []byte("val"), time.Second); setErr != nil {
		t.Errorf("Set() error = %v, want nil", setErr)
	}
}

func TestMultipleKeys(t *testing.T) {
	t.Parallel()

	cache, err := New(1 << 20)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	keys := map[string]string{
		"k1": "value1",
		"k2": "value2",
		"k3": "value3",
	}

	for k, v := range keys {
		if setErr := cache.Set(ctx, k, []byte(v), time.Minute); setErr != nil {
			t.Fatalf("Set(%q) error = %v", k, setErr)
		}
	}
	cache.c.Wait()

	for k, want := range keys {
		data, ok, getErr := cache.Get(ctx, k)
		if getErr != nil {
			t.Fatalf("Get(%q) error = %v", k, getErr)
		}
		if !ok {
			t.Errorf("Get(%q) ok = false, want true", k)
			continue
		}
		if string(data) != want {
			t.Errorf("Get(%q) = %q, want %q", k, string(data), want)
		}
	}
}
