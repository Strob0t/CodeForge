package autospec_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/Strob0t/CodeForge/internal/adapter/autospec"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

func newProvider(t *testing.T) specprovider.Provider {
	t.Helper()
	p, err := specprovider.New("autospec", nil)
	if err != nil {
		t.Fatalf("expected autospec provider to be registered: %v", err)
	}
	return p
}

func TestRegistration(t *testing.T) {
	p := newProvider(t)
	if p.Name() != "autospec" {
		t.Fatalf("expected name 'autospec', got %q", p.Name())
	}
	caps := p.Capabilities()
	if !caps.Read {
		t.Fatal("expected Read capability")
	}
	if caps.Write || caps.Sync {
		t.Fatal("expected Write=false, Sync=false")
	}
}

func TestDetect_WithSpecYAML(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := os.Mkdir(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "spec.yaml"), []byte("title: Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected Detect to return true for specs/spec.yaml")
	}
}

func TestDetect_WithSpecYML(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := os.Mkdir(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specsDir, "spec.yml"), []byte("title: Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected Detect to return true for specs/spec.yml")
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

func TestDetect_DirWithoutSpecFile(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := os.Mkdir(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// specs/ dir exists but no spec.yaml or spec.yml
	if err := os.WriteFile(filepath.Join(specsDir, "other.yaml"), []byte("title: Other\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected Detect to return false without spec.yaml")
	}
}

func TestListSpecs(t *testing.T) {
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := os.Mkdir(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create YAML with title
	if err := os.WriteFile(filepath.Join(specsDir, "spec.yaml"), []byte("title: Main Spec\nversion: 1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create plan.yml with title
	if err := os.WriteFile(filepath.Join(specsDir, "plan.yml"), []byte("title: Project Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create YAML without title (fallback to filename)
	if err := os.WriteFile(filepath.Join(specsDir, "tasks.yaml"), []byte("items:\n  - do stuff\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-YAML file should be ignored
	if err := os.WriteFile(filepath.Join(specsDir, "notes.md"), []byte("# ignore\n"), 0o644); err != nil {
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
		if s.Format != "autospec" {
			t.Errorf("expected format 'autospec', got %q", s.Format)
		}
	}
	if !titles["Main Spec"] {
		t.Error("expected 'Main Spec' title from spec.yaml")
	}
	if !titles["Project Plan"] {
		t.Error("expected 'Project Plan' title from plan.yml")
	}
	if !titles["tasks"] {
		t.Error("expected 'tasks' fallback title from tasks.yaml")
	}
}

func TestListSpecs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "specs"), 0o755); err != nil {
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
	specsDir := filepath.Join(dir, "specs")
	if err := os.Mkdir(specsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("title: Test\nversion: 1\n")
	if err := os.WriteFile(filepath.Join(specsDir, "spec.yaml"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	data, err := p.ReadSpec(context.Background(), dir, "specs/spec.yaml")
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
	_, err := p.ReadSpec(context.Background(), dir, "specs/nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
