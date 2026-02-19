package speckit_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/Strob0t/CodeForge/internal/adapter/speckit"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

func newProvider(t *testing.T) specprovider.Provider {
	t.Helper()
	p, err := specprovider.New("speckit", nil)
	if err != nil {
		t.Fatalf("expected speckit provider to be registered: %v", err)
	}
	return p
}

func TestRegistration(t *testing.T) {
	p := newProvider(t)
	if p.Name() != "speckit" {
		t.Fatalf("expected name 'speckit', got %q", p.Name())
	}
	caps := p.Capabilities()
	if !caps.Read {
		t.Fatal("expected Read capability")
	}
	if caps.Write || caps.Sync {
		t.Fatal("expected Write=false, Sync=false")
	}
}

func TestDetect_Present(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".specify"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected Detect to return true")
	}
}

func TestDetect_Absent(t *testing.T) {
	dir := t.TempDir()

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected Detect to return false")
	}
}

func TestListSpecs(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, ".specify")
	if err := os.Mkdir(specDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create Markdown with H1 heading
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("# My Feature\n\nSome content.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create plan file without heading (fallback to filename)
	if err := os.WriteFile(filepath.Join(specDir, "plan.md"), []byte("This is a plan.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create nested file
	nested := filepath.Join(specDir, "tasks")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "task1.md"), []byte("# Task One\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-spec file should be ignored
	if err := os.WriteFile(filepath.Join(specDir, "notes.txt"), []byte("ignore\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	specs, err := p.ListSpecs(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}

	titles := map[string]bool{}
	for _, s := range specs {
		titles[s.Title] = true
		if s.Format != "speckit" {
			t.Errorf("expected format 'speckit', got %q", s.Format)
		}
	}
	if !titles["My Feature"] {
		t.Error("expected 'My Feature' title from spec.md")
	}
	if !titles["Task One"] {
		t.Error("expected 'Task One' title from task1.md")
	}
	if !titles["plan"] {
		t.Error("expected 'plan' fallback title from plan.md")
	}
}

func TestListSpecs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".specify"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	specs, err := p.ListSpecs(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected 0 specs, got %d", len(specs))
	}
}

func TestReadSpec_HappyPath(t *testing.T) {
	dir := t.TempDir()
	specDir := filepath.Join(dir, ".specify")
	if err := os.Mkdir(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Test\n\nSpec content.\n")
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	data, err := p.ReadSpec(context.Background(), dir, ".specify/spec.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("content mismatch: got %q", string(data))
	}
}

func TestReadSpec_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	p := newProvider(t)
	_, err := p.ReadSpec(context.Background(), dir, "../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestReadSpec_MissingFile(t *testing.T) {
	dir := t.TempDir()

	p := newProvider(t)
	_, err := p.ReadSpec(context.Background(), dir, ".specify/nonexistent.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
