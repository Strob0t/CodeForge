package secrets_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/secrets"
)

func TestNewVault_InitialLoad(t *testing.T) {
	v, err := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{"KEY_A": "val_a", "KEY_B": "val_b"}, nil
	})
	if err != nil {
		t.Fatalf("NewVault failed: %v", err)
	}

	if got := v.Get("KEY_A"); got != "val_a" {
		t.Fatalf("expected 'val_a', got %q", got)
	}
	if got := v.Get("KEY_B"); got != "val_b" {
		t.Fatalf("expected 'val_b', got %q", got)
	}
}

func TestNewVault_LoaderError(t *testing.T) {
	_, err := secrets.NewVault(func() (map[string]string, error) {
		return nil, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected error from failing loader")
	}
}

func TestVault_GetMissingKey(t *testing.T) {
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{"EXIST": "yes"}, nil
	})
	if got := v.Get("MISSING"); got != "" {
		t.Fatalf("expected empty string for missing key, got %q", got)
	}
}

func TestVault_Reload(t *testing.T) {
	callCount := 0
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		callCount++
		if callCount == 1 {
			return map[string]string{"TOKEN": "old"}, nil
		}
		return map[string]string{"TOKEN": "new"}, nil
	})

	if got := v.Get("TOKEN"); got != "old" {
		t.Fatalf("expected 'old', got %q", got)
	}

	if err := v.Reload(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}

	if got := v.Get("TOKEN"); got != "new" {
		t.Fatalf("expected 'new' after reload, got %q", got)
	}
}

func TestVault_ReloadErrorPreservesValues(t *testing.T) {
	callCount := 0
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		callCount++
		if callCount == 1 {
			return map[string]string{"KEY": "original"}, nil
		}
		return nil, errors.New("vault unavailable")
	})

	if err := v.Reload(); err == nil {
		t.Fatal("expected reload error")
	}

	// Original values must be preserved.
	if got := v.Get("KEY"); got != "original" {
		t.Fatalf("expected 'original' after failed reload, got %q", got)
	}
}

func TestVault_ConcurrentAccess(t *testing.T) {
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{"K": "V"}, nil
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = v.Get("K")
		}()
		go func() {
			defer wg.Done()
			_ = v.Reload()
		}()
	}
	wg.Wait()
}

func TestEnvLoader(t *testing.T) {
	t.Setenv("CF_TEST_SECRET", "mysecret")
	loader := secrets.EnvLoader("CF_TEST_SECRET", "CF_MISSING_SECRET")

	vals, err := loader()
	if err != nil {
		t.Fatalf("EnvLoader failed: %v", err)
	}
	if vals["CF_TEST_SECRET"] != "mysecret" {
		t.Fatalf("expected 'mysecret', got %q", vals["CF_TEST_SECRET"])
	}
	if _, ok := vals["CF_MISSING_SECRET"]; ok {
		t.Fatal("expected missing env var to be omitted")
	}
}
