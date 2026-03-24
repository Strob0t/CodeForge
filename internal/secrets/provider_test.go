package secrets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/secrets"
)

func TestEnvProvider_Get(t *testing.T) {
	t.Setenv("CF_TEST_PROVIDER_SECRET", "hello")

	p := secrets.EnvProvider{}
	got, err := p.Get("CF_TEST_PROVIDER_SECRET")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Fatalf("expected 'hello', got %q", got)
	}
}

func TestEnvProvider_GetMissing(t *testing.T) {
	p := secrets.EnvProvider{}
	_, err := p.Get("CF_DEFINITELY_NOT_SET_12345")
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestFileProvider_Get(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "litellm-master-key")
	if err := os.WriteFile(secretFile, []byte("sk-prod-abc123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	fp := &secrets.FileProvider{Dir: dir}
	got, err := fp.Get("LITELLM_MASTER_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sk-prod-abc123" {
		t.Fatalf("expected 'sk-prod-abc123', got %q", got)
	}
}

func TestFileProvider_FallbackToEnv(t *testing.T) {
	dir := t.TempDir() // empty directory, no secret files
	t.Setenv("LITELLM_MASTER_KEY", "sk-from-env")

	fp := &secrets.FileProvider{Dir: dir}
	got, err := fp.Get("LITELLM_MASTER_KEY")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "sk-from-env" {
		t.Fatalf("expected 'sk-from-env', got %q", got)
	}
}

func TestFileProvider_BothMissing(t *testing.T) {
	dir := t.TempDir()
	fp := &secrets.FileProvider{Dir: dir}
	_, err := fp.Get("CF_NONEXISTENT_SECRET_XYZ")
	if err == nil {
		t.Fatal("expected error when both file and env var are missing")
	}
}

func TestAuto_ReturnsEnvProvider(t *testing.T) {
	// /run/secrets does not exist in test environments,
	// so Auto should return EnvProvider.
	p := secrets.Auto()
	if _, ok := p.(secrets.EnvProvider); !ok {
		t.Fatalf("expected EnvProvider, got %T", p)
	}
}
