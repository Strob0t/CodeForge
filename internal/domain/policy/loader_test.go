package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	content := `
name: custom-policy
mode: default
rules:
  - specifier:
      tool: Read
    decision: allow
  - specifier:
      tool: Bash
    decision: deny
termination:
  max_steps: 10
  timeout_seconds: 60
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "custom-policy" {
		t.Errorf("expected name 'custom-policy', got %q", p.Name)
	}
	if p.Mode != ModeDefault {
		t.Errorf("expected mode 'default', got %q", p.Mode)
	}
	if len(p.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(p.Rules))
	}
	if p.Termination.MaxSteps != 10 {
		t.Errorf("expected max_steps 10, got %d", p.Termination.MaxSteps)
	}
}

func TestLoadFromFileInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{not yaml}}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected 'parse' in error, got: %v", err)
	}
}

func TestLoadFromFileValidationError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	content := `
mode: default
rules:
  - specifier:
      tool: Read
    decision: allow
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected validation error (missing name)")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' in error, got: %v", err)
	}
}

func TestLoadFromFileMissing(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/policy.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFromDirectory(t *testing.T) {
	dir := t.TempDir()

	for i, name := range []string{"a.yaml", "b.yml"} {
		content := []byte("name: policy-" + string(rune('a'+i)) + "\nmode: default\n")
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Non-YAML file should be skipped.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatal(err)
	}

	profiles, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestLoadFromDirectoryMissing(t *testing.T) {
	profiles, err := LoadFromDirectory("/nonexistent/dir")
	if err != nil {
		t.Fatalf("missing directory should not error, got: %v", err)
	}
	if profiles != nil {
		t.Fatalf("expected nil for missing directory, got %v", profiles)
	}
}

func TestLoadFromDirectoryEmpty(t *testing.T) {
	dir := t.TempDir()
	profiles, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profiles != nil {
		t.Fatalf("expected nil for empty directory, got %v", profiles)
	}
}
