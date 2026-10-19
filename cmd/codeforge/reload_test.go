package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/secrets"
)

// captureLog routes slog to a buffer for the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type reloadFixture struct {
	running  *config.Config
	flags    config.CLIFlags
	yamlPath string
	keyFile  string
	vault    *secrets.Vault
}

// startForReload loads the config like main() (YAML, env, *_FILE secrets)
// with the LiteLLM master key in a secret file, and builds the vault.
func startForReload(t *testing.T) *reloadFixture {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	for _, key := range []string{"NATS_URL", "DATABASE_URL", "LITELLM_MASTER_KEY", "CODEFORGE_LOG_LEVEL", "CODEFORGE_AUTH_JWT_SECRET"} {
		t.Setenv(key, "")
		t.Setenv(key+"_FILE", "")
	}
	dir := t.TempDir()
	f := &reloadFixture{yamlPath: filepath.Join(dir, "codeforge.yaml"), keyFile: filepath.Join(dir, "litellm-master-key")}
	writeFile(t, f.yamlPath, "logging:\n  level: info\n")
	writeFile(t, f.keyFile, "sk-old-key-0123456789\n")
	t.Setenv("LITELLM_MASTER_KEY_FILE", f.keyFile)
	t.Setenv("NATS_URL", "nats://olduser:oldpass-123@nats-a:4222")
	f.flags = config.CLIFlags{ConfigPath: &f.yamlPath}

	running, _, err := config.LoadWithCLI(f.flags)
	if err != nil {
		t.Fatalf("LoadWithCLI: %v", err)
	}
	f.running = running
	f.vault, err = secrets.NewVault(secrets.EnvLoader("LITELLM_MASTER_KEY"))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	return f
}

// TestReloadOnSIGHUP (KI-61): SIGHUP reloads the secrets vault (the LiteLLM
// master key the client reads on every request) and names every other
// changed setting as needing a restart, without printing any value.
func TestReloadOnSIGHUP(t *testing.T) {
	f := startForReload(t)
	logs := captureLog(t)

	writeFile(t, f.keyFile, "sk-new-key-0123456789\n")
	writeFile(t, f.yamlPath, "logging:\n  level: debug\n")
	t.Setenv("NATS_URL", "nats://newuser:newpass-456@nats-b:4222")

	reloadOnSIGHUP(f.running, f.flags, f.vault)

	if got := f.vault.Get("LITELLM_MASTER_KEY"); got != "sk-new-key-0123456789" {
		t.Fatalf("vault key = %q, want the rotated key", got)
	}
	out := logs.String()
	for _, want := range []string{"logging.level", "nats.url", "restart"} {
		if !strings.Contains(out, want) {
			t.Errorf("reload log should name %q:\n%s", want, out)
		}
	}
	for _, leak := range []string{"oldpass-123", "newpass-456", "olduser", "newuser", "sk-old-key", "sk-new-key", "debug"} {
		if strings.Contains(out, leak) {
			t.Errorf("reload log leaks %q:\n%s", leak, out)
		}
	}
	if strings.Contains(out, "litellm.master_key") {
		t.Errorf("the master key is applied through the vault, not a restart setting:\n%s", out)
	}
	if f.running.Logging.Level != "info" {
		t.Errorf("the running config must not change: level = %q", f.running.Logging.Level)
	}
}

func TestReloadOnSIGHUP_NothingChanged(t *testing.T) {
	f := startForReload(t)
	logs := captureLog(t)

	reloadOnSIGHUP(f.running, f.flags, f.vault)

	out := logs.String()
	if strings.Contains(out, "level=WARN") || strings.Contains(out, "level=ERROR") {
		t.Fatalf("an unchanged config needs no restart:\n%s", out)
	}
	if !strings.Contains(out, "secrets reloaded") {
		t.Fatalf("the vault reload should be logged:\n%s", out)
	}
}

func TestReloadOnSIGHUP_InvalidConfigKeepsRunning(t *testing.T) {
	f := startForReload(t)
	logs := captureLog(t)

	writeFile(t, f.yamlPath, "rate:\n  burst: 0\n")
	writeFile(t, f.keyFile, "sk-new-key-0123456789\n")

	reloadOnSIGHUP(f.running, f.flags, f.vault)

	out := logs.String()
	if !strings.Contains(out, "level=ERROR") || !strings.Contains(out, "rate.burst") {
		t.Fatalf("an invalid config file should be reported:\n%s", out)
	}
	if got := f.vault.Get("LITELLM_MASTER_KEY"); got != "sk-new-key-0123456789" {
		t.Fatalf("the secrets are reloaded even when the config file is invalid: %q", got)
	}
}

func TestReloadOnSIGHUP_MasterKeyFromYAMLNeedsRestart(t *testing.T) {
	f := startForReload(t)
	t.Setenv("LITELLM_MASTER_KEY_FILE", "")
	writeFile(t, f.yamlPath, "litellm:\n  master_key: sk-yaml-key-one\n")
	running, _, err := config.LoadWithCLI(f.flags)
	if err != nil {
		t.Fatalf("LoadWithCLI: %v", err)
	}
	vault, err := secrets.NewVault(secrets.EnvLoader("LITELLM_MASTER_KEY"))
	if err != nil {
		t.Fatalf("NewVault: %v", err)
	}
	logs := captureLog(t)

	writeFile(t, f.yamlPath, "litellm:\n  master_key: sk-yaml-key-two\n")
	reloadOnSIGHUP(running, f.flags, vault)

	out := logs.String()
	if !strings.Contains(out, "litellm.master_key") {
		t.Fatalf("a master key from the YAML file is not in the vault and needs a restart:\n%s", out)
	}
	if strings.Contains(out, "sk-yaml-key") {
		t.Fatalf("reload log leaks the key:\n%s", out)
	}
}
