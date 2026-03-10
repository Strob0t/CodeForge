package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvWriter_Set_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	w := NewEnvWriter(path)

	if err := w.Set("FOO", "bar"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := w.Get("FOO")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "bar" {
		t.Errorf("Get(FOO) = %q, want %q", got, "bar")
	}
}

func TestEnvWriter_Set_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=old\nBAR=keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewEnvWriter(path)
	if err := w.Set("FOO", "new"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, _ := w.Get("FOO")
	if got != "new" {
		t.Errorf("Get(FOO) = %q, want %q", got, "new")
	}
	bar, _ := w.Get("BAR")
	if bar != "keep" {
		t.Errorf("Get(BAR) = %q, want %q", bar, "keep")
	}
}

func TestEnvWriter_Set_PreservesComments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# This is a comment\nFOO=bar\n\n# Another comment\nBAZ=qux\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewEnvWriter(path)
	if err := w.Set("FOO", "updated"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, _ := os.ReadFile(path) //nolint:gosec // G304: test file path from t.TempDir()
	result := string(data)

	if got, _ := w.Get("FOO"); got != "updated" {
		t.Errorf("Get(FOO) = %q, want %q", got, "updated")
	}
	if got, _ := w.Get("BAZ"); got != "qux" {
		t.Errorf("Get(BAZ) = %q, want %q", got, "qux")
	}

	// Verify comments are preserved.
	if !contains(result, "# This is a comment") {
		t.Error("first comment was lost")
	}
	if !contains(result, "# Another comment") {
		t.Error("second comment was lost")
	}
}

func TestEnvWriter_Delete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\nBAZ=qux\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewEnvWriter(path)
	if err := w.Delete("FOO"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, _ := w.Get("FOO")
	if got != "" {
		t.Errorf("Get(FOO) after delete = %q, want empty", got)
	}
	baz, _ := w.Get("BAZ")
	if baz != "qux" {
		t.Errorf("Get(BAZ) = %q, want %q", baz, "qux")
	}
}

func TestEnvWriter_Delete_NonExistent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	w := NewEnvWriter(path)
	// Should not error when file doesn't exist.
	if err := w.Delete("NOPE"); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestEnvWriter_Has(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\nEMPTY=\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewEnvWriter(path)

	has, err := w.Has("FOO")
	if err != nil {
		t.Fatalf("Has(FOO): %v", err)
	}
	if !has {
		t.Error("Has(FOO) = false, want true")
	}

	has, err = w.Has("EMPTY")
	if err != nil {
		t.Fatalf("Has(EMPTY): %v", err)
	}
	if has {
		t.Error("Has(EMPTY) = true, want false (empty value)")
	}

	has, err = w.Has("MISSING")
	if err != nil {
		t.Fatalf("Has(MISSING): %v", err)
	}
	if has {
		t.Error("Has(MISSING) = true, want false")
	}
}

func TestEnvWriter_Get_MissingFile(t *testing.T) {
	w := NewEnvWriter("/nonexistent/.env")
	_, err := w.Get("FOO")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestEnvWriter_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewEnvWriter(path)
	if err := w.Set("BAZ", "qux"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify no temp files remain.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != ".env" {
			t.Errorf("temp file not cleaned up: %s", e.Name())
		}
	}
}

func TestEnvWriter_Set_AddToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewEnvWriter(path)
	if err := w.Set("NEW_KEY", "new_value"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, _ := w.Get("FOO")
	if got != "bar" {
		t.Errorf("Get(FOO) = %q, want %q", got, "bar")
	}
	got, _ = w.Get("NEW_KEY")
	if got != "new_value" {
		t.Errorf("Get(NEW_KEY) = %q, want %q", got, "new_value")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || s != "" && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
