package config

import (
	"strings"
	"testing"
)

// S3 follow-up 1d: runtime.default_test_command and default_lint_command are
// checked at load time with the rules of the project config keys (the worker
// would refuse them at every gate otherwise).
func TestValidate_DefaultGateCommands(t *testing.T) {
	tests := []struct {
		name    string
		test    string
		lint    string
		wantErr string
	}{
		{name: "unset"},
		{name: "allowed", test: "go test ./...", lint: "golangci-lint run ./..."},
		{name: "quoted arguments", test: "pytest -k 'not slow'", lint: "make lint"},
		{name: "test executable not allowed", test: "echo ok", wantErr: "runtime.default_test_command"},
		{name: "lint executable not allowed", lint: "/usr/bin/ruff check .", wantErr: "runtime.default_lint_command"},
		{name: "unparseable test command", test: "pytest 'unterminated", wantErr: "runtime.default_test_command: invalid command"},
		{name: "shell", lint: "sh -c 'ruff check .'", wantErr: "runtime.default_lint_command"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.AppEnv = "development"
			if err := ensureSecrets(&cfg); err != nil {
				t.Fatal(err)
			}
			cfg.Runtime.DefaultTestCommand, cfg.Runtime.DefaultLintCommand = tc.test, tc.lint
			err := validate(&cfg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate = %v, want an error containing %q", err, tc.wantErr)
			}
		})
	}
}
