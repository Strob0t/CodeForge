package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// startWith writes yaml to a temp config file and loads it the way main()
// does (LoadWithCLI). It returns the running config and the flags.
func startWith(t *testing.T, yaml string, flags CLIFlags) (*Config, CLIFlags, string) {
	t.Helper()
	yamlPath := filepath.Join(t.TempDir(), "codeforge.yaml")
	writeYAML(t, yamlPath, yaml)
	flags.ConfigPath = &yamlPath
	cfg, _, err := LoadWithCLI(flags)
	if err != nil {
		t.Fatalf("LoadWithCLI: %v", err)
	}
	return cfg, flags, yamlPath
}

func writeYAML(t *testing.T, path, yaml string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
}

func changedSinceStart(t *testing.T, running *Config, flags CLIFlags) []string {
	t.Helper()
	changed, err := ChangedSinceStart(running, flags)
	if err != nil {
		t.Fatalf("ChangedSinceStart: %v", err)
	}
	return changed
}

// isolateEnv clears the variables the tests below change, so the host
// environment (e.g. a CI DATABASE_URL) does not leak in.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	for _, key := range []string{"CODEFORGE_LOG_LEVEL", "CODEFORGE_PORT", "CODEFORGE_AUTH_JWT_SECRET", "CODEFORGE_A2A_API_KEYS"} {
		clearSecretEnv(t, key)
	}
	for _, key := range []string{"NATS_URL", "DATABASE_URL", "LITELLM_MASTER_KEY"} {
		clearSecretEnv(t, key)
	}
}

// TestChangedSinceStart covers KI-61: SIGHUP reloads only the secrets vault,
// so the settings whose files or variables changed since startup are named
// (and need a restart). It replaces the ConfigHolder reload tests.
func TestChangedSinceStart(t *testing.T) {
	const initial = `
logging:
  level: "info"
rate:
  burst: 50
postgres:
  max_conns: 10
`
	tests := []struct {
		name   string
		yaml   string
		env    map[string]string
		want   []string
		detail string
	}{
		{name: "nothing changed", yaml: initial, want: nil},
		{
			name: "yaml values changed",
			yaml: `
logging:
  level: "debug"
rate:
  burst: 200
postgres:
  max_conns: 10
`,
			want: []string{"logging.level", "rate.burst"},
		},
		{
			name: "setting removed from yaml falls back to its default",
			yaml: `
logging:
  level: "info"
rate:
  burst: 50
`,
			want: []string{"postgres.max_conns"},
		},
		{
			name: "environment wins over yaml, as at startup",
			yaml: initial,
			env:  map[string]string{"CODEFORGE_LOG_LEVEL": "error"},
			want: []string{"logging.level"},
		},
		{
			name: "list and duration settings",
			yaml: initial + `
runtime:
  stale_check_interval: "7m"
`,
			env:  map[string]string{"CODEFORGE_A2A_API_KEYS": "key-one,key-two"},
			want: []string{"a2a.api_keys", "runtime.stale_check_interval"},
		},
		{
			name: "secrets are named like other settings",
			yaml: initial,
			env:  map[string]string{"NATS_URL": "nats://user:pass@nats-b:4222"},
			want: []string{"nats.url"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateEnv(t)
			running, flags, yamlPath := startWith(t, initial, CLIFlags{})
			writeYAML(t, yamlPath, tt.yaml)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := changedSinceStart(t, running, flags)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("changed settings = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChangedSinceStart_CLIFlagsApplyAgain(t *testing.T) {
	isolateEnv(t)
	port, level := "9999", "debug"
	running, flags, _ := startWith(t, "server:\n  port: \"8080\"\n", CLIFlags{Port: &port, LogLevel: &level})
	if running.Server.Port != "9999" {
		t.Fatalf("port = %q, want the flag", running.Server.Port)
	}
	if got := changedSinceStart(t, running, flags); len(got) != 0 {
		t.Fatalf("flags must be applied again, not reported as changes: %v", got)
	}
}

func TestChangedSinceStart_GeneratedJWTSecretIsNotAChange(t *testing.T) {
	isolateEnv(t)
	running, flags, _ := startWith(t, "auth:\n  enabled: true\n", CLIFlags{})
	if running.Auth.JWTSecret == "" {
		t.Fatal("startup must generate a JWT secret")
	}
	if got := changedSinceStart(t, running, flags); slices.Contains(got, "auth.jwt_secret") {
		t.Fatalf("a secret generated at startup is not a configuration change: %v", got)
	}
}

func TestChangedSinceStart_RemovedJWTSecretIsAChange(t *testing.T) {
	isolateEnv(t)
	t.Setenv("CODEFORGE_AUTH_JWT_SECRET", strongTestSecret)
	running, flags, _ := startWith(t, "", CLIFlags{})

	t.Setenv("CODEFORGE_AUTH_JWT_SECRET", "")
	if got := changedSinceStart(t, running, flags); !slices.Equal(got, []string{"auth.jwt_secret"}) {
		t.Fatalf("a restart would generate a new secret: changed = %v, want [auth.jwt_secret]", got)
	}
}

func TestChangedSinceStart_RotatedSecretFile(t *testing.T) {
	isolateEnv(t)
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "litellm-master-key")
	writeYAML(t, keyFile, "sk-first-0123456789\n")
	t.Setenv("LITELLM_MASTER_KEY_FILE", keyFile)
	running, flags, _ := startWith(t, "", CLIFlags{})

	writeYAML(t, keyFile, "sk-second-0123456789\n")
	if got := changedSinceStart(t, running, flags); !slices.Equal(got, []string{"litellm.master_key"}) {
		t.Fatalf("changed settings = %v, want [litellm.master_key]", got)
	}
}

func TestChangedSinceStart_InvalidConfigIsAnError(t *testing.T) {
	isolateEnv(t)
	running, flags, yamlPath := startWith(t, "rate:\n  burst: 50\n", CLIFlags{})
	writeYAML(t, yamlPath, "rate:\n  burst: 0\n")

	_, err := ChangedSinceStart(running, flags)
	if err == nil || !strings.Contains(err.Error(), "rate.burst") {
		t.Fatalf("err = %v, want the validation error", err)
	}
	if running.Rate.Burst != 50 {
		t.Fatalf("the running config must not change: burst = %d", running.Rate.Burst)
	}
}

func TestChangedSettings_NamesOnlyExportedYAMLKeys(t *testing.T) {
	a := Defaults()
	b := Defaults()
	b.Auth.JWTSecret = "a-new-secret"
	b.Postgres.DSN = "postgres://u:p@db/x"
	b.Agent.MaxContextTokens++
	b.Server.TrustedProxies = []string{"10.0.0.0/8"}

	got := changedSettings(&a, &b)
	want := []string{"agent.max_context_tokens", "auth.jwt_secret", "postgres.dsn", "server.trusted_proxies"}
	if !slices.Equal(got, want) {
		t.Fatalf("changedSettings = %v, want %v", got, want)
	}
	for _, name := range got {
		if strings.Contains(name, "secret-") || strings.Contains(name, "u:p") {
			t.Fatalf("names must not carry values: %q", name)
		}
	}
	if got := changedSettings(&a, &a); len(got) != 0 {
		t.Fatalf("identical configs: %v", got)
	}
}
