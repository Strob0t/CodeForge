package markdownspec_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/Strob0t/CodeForge/internal/adapter/markdownspec"
	"github.com/Strob0t/CodeForge/internal/port/specprovider"
)

func newProvider(t *testing.T) specprovider.Provider {
	t.Helper()
	p, err := specprovider.New("markdown", nil)
	if err != nil {
		t.Fatalf("expected markdown provider to be registered: %v", err)
	}
	return p
}

func TestRegistration(t *testing.T) {
	p := newProvider(t)
	if p.Name() != "markdown" {
		t.Fatalf("expected name 'markdown', got %q", p.Name())
	}
	caps := p.Capabilities()
	if !caps.Read {
		t.Fatal("expected Read capability")
	}
	if !caps.Write {
		t.Fatal("expected Write=true")
	}
	if caps.Sync {
		t.Fatal("expected Sync=false")
	}
}

func TestDetect_UpperCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("# Roadmap\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected Detect to return true for ROADMAP.md")
	}
}

func TestDetect_LowerCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "roadmap.md"), []byte("# Roadmap\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	found, err := p.Detect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected Detect to return true for roadmap.md")
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
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	specs, err := p.ListSpecs(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Path != "ROADMAP.md" {
		t.Fatalf("expected path 'ROADMAP.md', got %q", specs[0].Path)
	}
	if specs[0].Format != "markdown" {
		t.Fatalf("expected format 'markdown', got %q", specs[0].Format)
	}
}

func TestListSpecs_BothFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "roadmap.md"), []byte("# Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	specs, err := p.ListSpecs(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
}

func TestReadSpec(t *testing.T) {
	dir := t.TempDir()
	content := []byte("# My Roadmap\n\n## Phase 1\n")
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	p := newProvider(t)
	data, err := p.ReadSpec(context.Background(), dir, "ROADMAP.md")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("content mismatch: got %q", string(data))
	}
}
