package secrets_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/secrets"
)

const fileEnvTestKey = "CF_TEST_FILE_ENV_SECRET"

func writeSecretFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLookupFileEnv(t *testing.T) {
	dir := t.TempDir()
	plain := writeSecretFile(t, dir, "plain", "s3cret-value")
	withLF := writeSecretFile(t, dir, "lf", "s3cret-value\n")
	withCRLF := writeSecretFile(t, dir, "crlf", "s3cret-value\r\n")
	padded := writeSecretFile(t, dir, "padded", "  s3cret-value \t\n")
	inner := writeSecretFile(t, dir, "inner", "pass word/with+special=chars\n")
	empty := writeSecretFile(t, dir, "empty", "")
	blank := writeSecretFile(t, dir, "blank", " \n\t\n")

	tests := []struct {
		name      string
		env       string // value of KEY ("" = unset)
		file      string // value of KEY_FILE ("" = unset)
		wantValue string
		wantOK    bool
		wantErr   string // substring; "" = no error
	}{
		{name: "neither set", wantOK: false},
		{name: "only env set is not a file secret", env: "from-env", wantOK: false},
		{name: "file without newline", file: plain, wantValue: "s3cret-value", wantOK: true},
		{name: "trailing LF trimmed", file: withLF, wantValue: "s3cret-value", wantOK: true},
		{name: "trailing CRLF trimmed", file: withCRLF, wantValue: "s3cret-value", wantOK: true},
		{name: "surrounding whitespace trimmed", file: padded, wantValue: "s3cret-value", wantOK: true},
		{name: "inner spaces and URL-special chars kept", file: inner, wantValue: "pass word/with+special=chars", wantOK: true},
		{name: "env and file both set is ambiguous", env: "from-env", file: plain, wantErr: "both " + fileEnvTestKey + " and " + fileEnvTestKey + "_FILE are set"},
		{name: "missing file", file: filepath.Join(dir, "does-not-exist"), wantErr: fileEnvTestKey + "_FILE"},
		{name: "path is a directory", file: dir, wantErr: fileEnvTestKey + "_FILE"},
		{name: "empty file", file: empty, wantErr: "is empty"},
		{name: "whitespace-only file", file: blank, wantErr: "is empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(fileEnvTestKey, tt.env)
			t.Setenv(fileEnvTestKey+secrets.FileEnvSuffix, tt.file)

			got, ok, err := secrets.LookupFileEnv(fileEnvTestKey)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got value %q", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err)
				}
				if ok || got != "" {
					t.Fatalf("expected no value on error, got ok=%v value=%q", ok, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok != tt.wantOK || got != tt.wantValue {
				t.Fatalf("got (%q, %v), want (%q, %v)", got, ok, tt.wantValue, tt.wantOK)
			}
		})
	}
}

func TestLookupFileEnv_RelativePath(t *testing.T) {
	dir := t.TempDir()
	writeSecretFile(t, dir, "relative-secret", "rel-value\n")
	t.Chdir(dir)
	t.Setenv(fileEnvTestKey, "")
	t.Setenv(fileEnvTestKey+secrets.FileEnvSuffix, "relative-secret")

	got, ok, err := secrets.LookupFileEnv(fileEnvTestKey)
	if err != nil || !ok || got != "rel-value" {
		t.Fatalf("got (%q, %v, %v), want (\"rel-value\", true, nil)", got, ok, err)
	}
}

func TestLookupFileEnv_ErrorDoesNotLeakEnvValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(fileEnvTestKey, "super-secret-env-value")
	t.Setenv(fileEnvTestKey+secrets.FileEnvSuffix, writeSecretFile(t, dir, "s", "super-secret-file-value"))

	_, _, err := secrets.LookupFileEnv(fileEnvTestKey)
	if err == nil {
		t.Fatal("expected error when both are set")
	}
	if strings.Contains(err.Error(), "super-secret") {
		t.Fatalf("error leaks a secret value: %q", err)
	}
}

func TestEnvLoader_ReadsFileEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeSecretFile(t, dir, "litellm-master-key", "sk-from-file\n")
	t.Setenv(fileEnvTestKey, "")
	t.Setenv(fileEnvTestKey+secrets.FileEnvSuffix, path)

	vals, err := secrets.EnvLoader(fileEnvTestKey)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals[fileEnvTestKey] != "sk-from-file" {
		t.Fatalf("expected value from file, got %q", vals[fileEnvTestKey])
	}

	// A rotated secret file is picked up by the next load (SIGHUP reload).
	writeSecretFile(t, dir, "litellm-master-key", "sk-rotated\n")
	vals, err = secrets.EnvLoader(fileEnvTestKey)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals[fileEnvTestKey] != "sk-rotated" {
		t.Fatalf("expected rotated value, got %q", vals[fileEnvTestKey])
	}
}

func TestEnvLoader_PlainEnvStillWorks(t *testing.T) {
	t.Setenv(fileEnvTestKey, "sk-from-env")
	t.Setenv(fileEnvTestKey+secrets.FileEnvSuffix, "")

	vals, err := secrets.EnvLoader(fileEnvTestKey)()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals[fileEnvTestKey] != "sk-from-env" {
		t.Fatalf("expected env value, got %q", vals[fileEnvTestKey])
	}
}

func TestEnvLoader_FileEnvErrorPropagates(t *testing.T) {
	t.Setenv(fileEnvTestKey, "")
	t.Setenv(fileEnvTestKey+secrets.FileEnvSuffix, filepath.Join(t.TempDir(), "missing"))

	if _, err := secrets.EnvLoader(fileEnvTestKey)(); err == nil {
		t.Fatal("expected error for a missing secret file")
	}
}
