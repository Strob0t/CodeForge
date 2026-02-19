package secrets_test

import (
	"errors"
	"strings"
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

func TestVault_Redacted(t *testing.T) {
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{
			"API_KEY": "sk-abcdef123456",
			"SHORT":   "ab",
		}, nil
	})

	// Long secret: shows first 2 chars + ****
	got := v.Redacted("API_KEY")
	if got != "sk****" {
		t.Errorf("expected 'sk****', got %q", got)
	}

	// Short secret (<=4 chars): fully masked
	got = v.Redacted("SHORT")
	if got != "****" {
		t.Errorf("expected '****', got %q", got)
	}

	// Missing key: empty string
	got = v.Redacted("MISSING")
	if got != "" {
		t.Errorf("expected empty string for missing key, got %q", got)
	}
}

func TestVault_RedactString(t *testing.T) {
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{
			"DB_PASSWORD":  "supersecret123",
			"API_TOKEN":    "tok_live_abcdef",
			"SHORT_SECRET": "ab", // too short to redact (< 4 chars)
		}, nil
	})

	input := "Connected to DB with password supersecret123 and token tok_live_abcdef"
	got := v.RedactString(input)

	if strings.Contains(got, "supersecret123") {
		t.Errorf("DB password was not redacted in %q", got)
	}
	if strings.Contains(got, "tok_live_abcdef") {
		t.Errorf("API token was not redacted in %q", got)
	}
	if !strings.Contains(got, "su****") {
		t.Errorf("expected masked DB password, got %q", got)
	}
	if !strings.Contains(got, "to****") {
		t.Errorf("expected masked API token, got %q", got)
	}
}

func TestVault_RedactStringNoSecrets(t *testing.T) {
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{"KEY": "value123"}, nil
	})

	input := "This string has no secrets"
	got := v.RedactString(input)
	if got != input {
		t.Errorf("expected unchanged string, got %q", got)
	}
}

func TestVault_Keys(t *testing.T) {
	v, _ := secrets.NewVault(func() (map[string]string, error) {
		return map[string]string{"A": "1", "B": "2"}, nil
	})

	keys := v.Keys()
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	keySet := map[string]bool{}
	for _, k := range keys {
		keySet[k] = true
	}
	if !keySet["A"] || !keySet["B"] {
		t.Errorf("expected keys A and B, got %v", keys)
	}
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
