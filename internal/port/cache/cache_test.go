package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/cache"
)

// RunComplianceTests runs the standard compliance test suite against any Cache implementation.
func RunComplianceTests(t *testing.T, c cache.Cache) {
	t.Helper()
	ctx := context.Background()

	t.Run("SetAndGet", func(t *testing.T) {
		if err := c.Set(ctx, "compliance-key", []byte("compliance-val"), time.Minute); err != nil {
			t.Fatal(err)
		}
		val, found, err := c.Get(ctx, "compliance-key")
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatal("expected found after Set")
		}
		if string(val) != "compliance-val" {
			t.Fatalf("expected compliance-val, got %s", val)
		}
	})

	t.Run("GetMiss", func(t *testing.T) {
		_, found, err := c.Get(ctx, "nonexistent-key")
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Fatal("expected miss for nonexistent key")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		_ = c.Set(ctx, "del-key", []byte("del-val"), time.Minute)
		if err := c.Delete(ctx, "del-key"); err != nil {
			t.Fatal(err)
		}
		_, found, err := c.Get(ctx, "del-key")
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Fatal("expected miss after Delete")
		}
	})

	t.Run("DeleteNonexistent", func(t *testing.T) {
		if err := c.Delete(ctx, "never-existed"); err != nil {
			t.Fatal("Delete of nonexistent key should not error")
		}
	})

	t.Run("Overwrite", func(t *testing.T) {
		_ = c.Set(ctx, "ow-key", []byte("v1"), time.Minute)
		_ = c.Set(ctx, "ow-key", []byte("v2"), time.Minute)
		val, found, err := c.Get(ctx, "ow-key")
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatal("expected found after overwrite")
		}
		if string(val) != "v2" {
			t.Fatalf("expected v2 after overwrite, got %s", val)
		}
	})
}
