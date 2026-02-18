package openspec_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/Strob0t/CodeForge/internal/adapter/openspec"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

func newProvider(t *testing.T) specprovider.Provider {
	t.Helper()
	p, err := specprovider.New("openspec", nil)
	if err != nil {
		t.Fatalf("expected openspec provider to be registered: %v", err)
	}
	return p
}

func TestRegistration(t *testing.T) {
	p := newProvider(t)
	if p.Name() != "openspec" {
		t.Fatalf("expected name 'openspec', got %q", p.Name())
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
	if err := os.Mkdir(filepath.Join(dir, "openspec"), 0o755); err != nil {
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
	specDir := filepath.Join(dir, "openspec")
	if err := os.Mkdir(specDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create YAML with title
	yamlContent := []byte("title: My Feature\nversion: 1.0\n")
	if err := os.WriteFile(filepath.Join(specDir, "feature.yaml"), yamlContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create JSON file (no YAML title)
	jsonContent := []byte(`{"name": "test"}`)
	if err := os.WriteFile(filepath.Join(specDir, "api.json"), jsonContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Create nested file
	nested := filepath.Join(specDir, "sub")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.yml"), []byte("title: Deep Spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Non-spec file should be ignored
	if err := os.WriteFile(filepath.Join(specDir, "README.md"), []byte("# ignore\n"), 0o644); err != nil {
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

	// Check that YAML title extraction works.
	titles := map[string]bool{}
	for _, s := range specs {
		titles[s.Title] = true
	}
	if !titles["My Feature"] {
		t.Error("expected 'My Feature' title from feature.yaml")
	}
	if !titles["Deep Spec"] {
		t.Error("expected 'Deep Spec' title from deep.yml")
	}
}

func TestListSpecs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "openspec"), 0o755); err != nil {
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
	specDir := filepath.Join(dir, "openspec")
	if err := os.Mkdir(specDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("title: Test\nversion: 1\n")
	if err := os.WriteFile(filepath.Join(specDir, "spec.yaml"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	data, err := p.ReadSpec(context.Background(), dir, "openspec/spec.yaml")
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
	_, err := p.ReadSpec(context.Background(), dir, "openspec/nonexistent.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
