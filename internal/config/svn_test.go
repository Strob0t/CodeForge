package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
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

// S10-D review: svn contacts private and loopback hosts only when the
// operator lists them (svn.allowed_private_hosts, YAML, or
// CODEFORGE_SVN_ALLOWED_PRIVATE_HOSTS, comma-separated).
func TestSVNAllowedPrivateHosts(t *testing.T) {
	if hosts := Defaults().SVN.AllowedPrivateHosts; len(hosts) != 0 {
		t.Fatalf("default svn.allowed_private_hosts = %v, want none", hosts)
	}
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	if err := os.WriteFile(path, []byte("svn:\n  allowed_private_hosts:\n    - svn.lan\n    - 10.20.0.0/16\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatal(err)
	}
	if want := []string{"svn.lan", "10.20.0.0/16"}; !slices.Equal(cfg.SVN.AllowedPrivateHosts, want) {
		t.Fatalf("YAML svn.allowed_private_hosts = %v, want %v", cfg.SVN.AllowedPrivateHosts, want)
	}
	t.Setenv("CODEFORGE_SVN_ALLOWED_PRIVATE_HOSTS", "svn.corp, 192.168.7.7")
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
	if want := []string{"svn.corp", "192.168.7.7"}; !slices.Equal(cfg.SVN.AllowedPrivateHosts, want) {
		t.Fatalf("env svn.allowed_private_hosts = %v, want %v", cfg.SVN.AllowedPrivateHosts, want)
	}

	cfg = Defaults()
	cfg.Auth.JWTSecret = strongTestSecret
	cfg.SVN.AllowedPrivateHosts = []string{"https://svn.lan"}
	if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "svn.allowed_private_hosts") {
		t.Fatalf("validate() = %v, want an error naming svn.allowed_private_hosts", err)
	}
}
