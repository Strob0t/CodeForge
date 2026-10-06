package config

import (
	"os"
	"path/filepath"
	"testing"
)

// KI-189 (R3-14): local (file://) SVN repositories are an operator decision
// (svn.allow_file_urls), off by default.
func TestSVNAllowFileURLs(t *testing.T) {
	if Defaults().SVN.AllowFileURLs {
		t.Fatal("svn.allow_file_urls is on by default")
	}

	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	if err := os.WriteFile(path, []byte("svn:\n  allow_file_urls: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if !cfg.SVN.AllowFileURLs {
		t.Fatal("svn.allow_file_urls: true in YAML was not read")
	}

	t.Setenv("CODEFORGE_SVN_ALLOW_FILE_URLS", "false")
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
	if cfg.SVN.AllowFileURLs {
		t.Fatal("CODEFORGE_SVN_ALLOW_FILE_URLS=false did not override the YAML value")
	}
}
