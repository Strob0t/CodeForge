package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPolicyForAutonomy(t *testing.T) {
	tests := []struct {
		autonomy int
		expected string
	}{
		{1, "supervised-ask-all"},
		{2, "headless-safe-sandbox"},
		{3, "headless-safe-sandbox"},
		{4, "trusted-mount-autonomous"},
		{5, "trusted-mount-autonomous"},
		{0, "headless-safe-sandbox"},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("autonomy_%d", tc.autonomy), func(t *testing.T) {
			result := policyForAutonomy(tc.autonomy)
			if result != tc.expected {
				t.Errorf("policyForAutonomy(%d) = %q, want %q", tc.autonomy, result, tc.expected)
			}
		})
	}
}

func TestDetectStackSummary_IncludesFrameworks(t *testing.T) {
	dir := t.TempDir()

	// Create package.json with solid-js and tsconfig.json to trigger TS detection.
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
		"dependencies": {"solid-js": "^1.9.0"}
	}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write tsconfig.json: %v", err)
	}

	summary, err := detectStackSummary(dir)
	if err != nil {
		t.Fatalf("detectStackSummary returned error: %v", err)
	}
	if !strings.Contains(summary, "typescript") {
		t.Errorf("expected summary to contain 'typescript', got %q", summary)
	}
	if !strings.Contains(summary, "solidjs") {
		t.Errorf("expected summary to contain 'solidjs', got %q", summary)
	}
}

func TestDetectStackSummary_NoFrameworks(t *testing.T) {
	dir := t.TempDir()

	// Create go.mod without any known framework.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	summary, err := detectStackSummary(dir)
	if err != nil {
		t.Fatalf("detectStackSummary returned error: %v", err)
	}
	if summary != "go" {
		t.Errorf("expected summary to be 'go', got %q", summary)
	}
	// Should NOT contain parentheses when no frameworks.
	if strings.Contains(summary, "(") {
		t.Errorf("expected no parentheses for language without frameworks, got %q", summary)
	}
}

func TestDetectStackSummary_MultipleFrameworks(t *testing.T) {
	dir := t.TempDir()

	// Create package.json with both react and @nestjs/core, plus tsconfig.
	// Both react and nestjs are registered under the "typescript" framework detectors.
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
		"dependencies": {"react": "^18.0.0", "@nestjs/core": "^10.0.0"}
	}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write tsconfig.json: %v", err)
	}

	summary, err := detectStackSummary(dir)
	if err != nil {
		t.Fatalf("detectStackSummary returned error: %v", err)
	}
	if !strings.Contains(summary, "react") {
		t.Errorf("expected summary to contain 'react', got %q", summary)
	}
	if !strings.Contains(summary, "nestjs") {
		t.Errorf("expected summary to contain 'nestjs', got %q", summary)
	}
	// Should contain comma-separated frameworks in parentheses.
	if !strings.Contains(summary, "(") {
		t.Errorf("expected parentheses for frameworks, got %q", summary)
	}
}
