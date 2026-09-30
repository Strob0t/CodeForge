package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// strongTestSecret passes the production JWT checks (length and entropy).
const strongTestSecret = "Xk9mL2pR7wQn4tFvB8jC3hGdE5sAyU0zABCDEFGH"

func writeTestSecret(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// clearSecretEnv unsets KEY and KEY_FILE so the host environment (CI, a
// developer shell) cannot leak into a test.
func clearSecretEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	t.Setenv(key+"_FILE", "")
}

func loadFromNoYAML(t *testing.T) (*Config, error) {
	t.Helper()
	return LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
}

func TestLoadFrom_SecretFiles(t *testing.T) {
	tests := []struct {
		key     string
		content string
		want    string
		get     func(*Config) string
	}{
		{"CODEFORGE_AUTH_JWT_SECRET", strongTestSecret + "\n", strongTestSecret, func(c *Config) string { return c.Auth.JWTSecret }},
		{"CODEFORGE_INTERNAL_KEY", "internal-key-from-file\n", "internal-key-from-file", func(c *Config) string { return c.InternalKey }},
		{"DATABASE_URL", "postgresql://cf:p4ss@postgres:5432/cf?sslmode=require\n", "postgresql://cf:p4ss@postgres:5432/cf?sslmode=require", func(c *Config) string { return c.Postgres.DSN }},
		{"NATS_URL", "nats://user:pass@nats:4222\n", "nats://user:pass@nats:4222", func(c *Config) string { return c.NATS.URL }},
		{"LITELLM_MASTER_KEY", "sk-from-file\r\n", "sk-from-file", func(c *Config) string { return c.LiteLLM.MasterKey }},
		{"CODEFORGE_AUTH_ADMIN_PASS", "Adm1n-from-file!\n", "Adm1n-from-file!", func(c *Config) string { return c.Auth.DefaultAdminPass }},
		{"CODEFORGE_WEBHOOK_GITHUB_SECRET", "gh-hook\n", "gh-hook", func(c *Config) string { return c.Webhook.GitHubSecret }},
		{"CODEFORGE_WEBHOOK_GITLAB_TOKEN", "gl-hook\n", "gl-hook", func(c *Config) string { return c.Webhook.GitLabToken }},
		{"CODEFORGE_WEBHOOK_PLANE_SECRET", "plane-hook\n", "plane-hook", func(c *Config) string { return c.Webhook.PlaneSecret }},
		{"CODEFORGE_NOTIFICATION_SLACK_WEBHOOK_URL", "https://hooks.slack.test/x\n", "https://hooks.slack.test/x", func(c *Config) string { return c.Notification.SlackWebhookURL }},
		{"CODEFORGE_NOTIFICATION_DISCORD_WEBHOOK_URL", "https://discord.test/x\n", "https://discord.test/x", func(c *Config) string { return c.Notification.DiscordWebhookURL }},
		{"GITHUB_CLIENT_SECRET", "gh-oauth\n", "gh-oauth", func(c *Config) string { return c.GitHub.ClientSecret }},
		{"CODEFORGE_SMTP_PASSWORD", "smtp-pass\n", "smtp-pass", func(c *Config) string { return c.Notification.SMTPPassword }},
		{"CODEFORGE_PLANE_API_TOKEN", "plane-token\n", "plane-token", func(c *Config) string { return c.Plane.APIToken }},
		{"CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET", "llm-enc-from-file\n", "llm-enc-from-file", func(c *Config) string { return c.Auth.LLMKeyEncryptionSecret }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			clearSecretEnv(t, tt.key)
			t.Setenv(tt.key+"_FILE", writeTestSecret(t, tt.content))

			cfg, err := loadFromNoYAML(t)
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			if got := tt.get(cfg); got != tt.want {
				t.Fatalf("%s from %s_FILE: got %q, want %q", tt.key, tt.key, got, tt.want)
			}
		})
	}
}

func TestLoadFrom_SecretFileA2AKeys(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"comma separated", "key-one, key-two ,,\n", []string{"key-one", "key-two"}},
		{"one key per line", "key-one\nkey-two\n", []string{"key-one", "key-two"}},
		{"CRLF lines and blank lines", "key-one\r\n\r\nkey-two\r\n", []string{"key-one", "key-two"}},
		{"mixed commas and lines", "key-one, key-two\n key-three \n\nkey-four,\n", []string{"key-one", "key-two", "key-three", "key-four"}},
		{"single key", "only-key", []string{"only-key"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearSecretEnv(t, "CODEFORGE_A2A_API_KEYS")
			t.Setenv("CODEFORGE_A2A_API_KEYS_FILE", writeTestSecret(t, tt.content))

			cfg, err := loadFromNoYAML(t)
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			if !slices.Equal(cfg.A2A.APIKeys, tt.want) {
				t.Fatalf("A2A API keys: got %q, want %q", cfg.A2A.APIKeys, tt.want)
			}
		})
	}
}

func TestLoadEnv_A2AKeysSplitOnNewlines(t *testing.T) {
	t.Setenv("CODEFORGE_A2A_API_KEYS", "key-one\nkey-two,key-three")
	cfg := Defaults()
	loadEnv(&cfg)
	if want := []string{"key-one", "key-two", "key-three"}; !slices.Equal(cfg.A2A.APIKeys, want) {
		t.Fatalf("A2A API keys: got %q, want %q", cfg.A2A.APIKeys, want)
	}
}

func TestLoadFrom_SecretEnvVersusFile(t *testing.T) {
	const key = "CODEFORGE_INTERNAL_KEY"
	tests := []struct {
		name    string
		env     string
		file    string // file content; "" = KEY_FILE unset
		want    string
		wantErr string
	}{
		{name: "neither set keeps default", want: ""},
		{name: "env only", env: "from-env", want: "from-env"},
		{name: "file only", file: "from-file\n", want: "from-file"},
		{name: "both set is rejected", env: "from-env", file: "from-file\n", wantErr: "both CODEFORGE_INTERNAL_KEY and CODEFORGE_INTERNAL_KEY_FILE are set"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearSecretEnv(t, key)
			t.Setenv(key, tt.env)
			if tt.file != "" {
				t.Setenv(key+"_FILE", writeTestSecret(t, tt.file))
			}

			cfg, err := loadFromNoYAML(t)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			if cfg.InternalKey != tt.want {
				t.Fatalf("internal key: got %q, want %q", cfg.InternalKey, tt.want)
			}
		})
	}
}

func TestLoadFrom_SecretFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr string
	}{
		{"missing file", func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, "CODEFORGE_AUTH_JWT_SECRET_FILE"},
		{"directory instead of file", func(t *testing.T) string { return t.TempDir() }, "CODEFORGE_AUTH_JWT_SECRET_FILE"},
		{"empty file", func(t *testing.T) string { return writeTestSecret(t, "") }, "is empty"},
		{"newline-only file", func(t *testing.T) string { return writeTestSecret(t, "\n") }, "is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearSecretEnv(t, "CODEFORGE_AUTH_JWT_SECRET")
			t.Setenv("CODEFORGE_AUTH_JWT_SECRET_FILE", tt.path(t))

			_, err := loadFromNoYAML(t)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestLoadWithCLI_SecretFilesProduction mirrors the production compose: every
// secret comes from a mounted file and the strict production checks pass.
func TestLoadWithCLI_SecretFilesProduction(t *testing.T) {
	secretsDir := t.TempDir()
	files := map[string]string{
		"CODEFORGE_AUTH_JWT_SECRET": strongTestSecret + "\n",
		"CODEFORGE_INTERNAL_KEY":    "0f1e2d3c4b5a69788796a5b4c3d2e1f0\n",
		"DATABASE_URL":              "postgresql://codeforge:0a1b2c3d@postgres:5432/codeforge?sslmode=require\n",
		"NATS_URL":                  "nats://4e5f:6a7b@nats:4222\n",
		"LITELLM_MASTER_KEY":        "sk-0a1b2c3d4e5f\n",
	}
	for key, content := range files {
		clearSecretEnv(t, key)
		path := filepath.Join(secretsDir, strings.ToLower(key))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key+"_FILE", path)
	}
	t.Setenv("APP_ENV", "production")
	t.Setenv("CODEFORGE_AUTH_ADMIN_PASS", "")
	t.Setenv("CODEFORGE_AUTH_ADMIN_PASS_FILE", "")
	t.Setenv("CODEFORGE_CONFIG_FILE", filepath.Join(t.TempDir(), "absent.yaml"))

	cfg, _, err := LoadWithCLI(CLIFlags{})
	if err != nil {
		t.Fatalf("LoadWithCLI: %v", err)
	}
	for key, content := range files {
		want := strings.TrimSpace(content)
		var got string
		switch key {
		case "CODEFORGE_AUTH_JWT_SECRET":
			got = cfg.Auth.JWTSecret
		case "CODEFORGE_INTERNAL_KEY":
			got = cfg.InternalKey
		case "DATABASE_URL":
			got = cfg.Postgres.DSN
		case "NATS_URL":
			got = cfg.NATS.URL
		case "LITELLM_MASTER_KEY":
			got = cfg.LiteLLM.MasterKey
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", key, got, want)
		}
	}
}

// TestLoadFrom_FileSuffixOnlyForSecrets documents that *_FILE is an opt-in for
// secret settings, not a generic indirection for every variable.
func TestLoadFrom_FileSuffixOnlyForSecrets(t *testing.T) {
	t.Setenv("CODEFORGE_PORT", "")
	t.Setenv("CODEFORGE_PORT_FILE", writeTestSecret(t, "9999\n"))

	cfg, err := loadFromNoYAML(t)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Server.Port != Defaults().Server.Port {
		t.Fatalf("CODEFORGE_PORT_FILE must be ignored, got port %q", cfg.Server.Port)
	}
}

func TestLoadEnv_LLMKeyEncryptionSecret(t *testing.T) {
	clearSecretEnv(t, "CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET")
	t.Setenv("CODEFORGE_AUTH_LLM_KEY_ENCRYPTION_SECRET", "llm-enc-from-env")

	cfg, err := loadFromNoYAML(t)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Auth.LLMKeyEncryptionSecret != "llm-enc-from-env" {
		t.Fatalf("got %q, want %q", cfg.Auth.LLMKeyEncryptionSecret, "llm-enc-from-env")
	}
}
