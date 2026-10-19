package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/run"
)

func TestExecModeCheckAvailable(t *testing.T) {
	tests := []struct {
		name        string
		mode        run.ExecMode
		unavailable bool
		invalid     bool
	}{
		{name: "empty (defaults to mount)", mode: ""},
		{name: "mount", mode: run.ExecModeMount},
		{name: "sandbox", mode: run.ExecModeSandbox, unavailable: true},
		{name: "hybrid", mode: run.ExecModeHybrid, unavailable: true},
		{name: "mixed case", mode: "Sandbox", invalid: true},
		{name: "whitespace", mode: " hybrid", invalid: true},
		{name: "unknown", mode: "docker", invalid: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.mode.CheckAvailable()
			switch {
			case tc.invalid:
				if !errors.Is(err, domain.ErrValidation) || errors.Is(err, run.ErrExecModeUnavailable) {
					t.Fatalf("CheckAvailable(%q) = %v, want a plain validation error", tc.mode, err)
				}
				return
			case !tc.unavailable:
				if err != nil {
					t.Fatalf("CheckAvailable(%q) = %v, want nil", tc.mode, err)
				}
				return
			}
			if !errors.Is(err, run.ErrExecModeUnavailable) {
				t.Fatalf("CheckAvailable(%q) = %v, want ErrExecModeUnavailable", tc.mode, err)
			}
			// ErrValidation makes the HTTP layer answer 400 instead of 500.
			if !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("CheckAvailable(%q) = %v, want it to wrap domain.ErrValidation", tc.mode, err)
			}
			want := string(tc.mode) + " execution mode is not available yet: tools would run without isolation (KI-13)"
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("CheckAvailable(%q) message = %q, want it to contain %q", tc.mode, err.Error(), want)
			}
		})
	}
}
