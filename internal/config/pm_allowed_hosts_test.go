package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// KI-85 review: only the platform operator lets PM syncs (the GitLab
// provider) reach private addresses, with pm.allowed_private_hosts (YAML) or
// CODEFORGE_PM_ALLOWED_PRIVATE_HOSTS (comma-separated), apart from the MCP
// allowlist.

func TestPMAllowedPrivateHosts_DefaultEmpty(t *testing.T) {
	if hosts := Defaults().PM.AllowedPrivateHosts; len(hosts) != 0 {
		t.Fatalf("default pm.allowed_private_hosts = %v, want none", hosts)
	}
}

func TestPMAllowedPrivateHosts_Layering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	yaml := "pm:\n  allowed_private_hosts:\n    - gitlab.corp.internal\n    - 10.20.0.0/16\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}
	if want := []string{"gitlab.corp.internal", "10.20.0.0/16"}; !slices.Equal(cfg.PM.AllowedPrivateHosts, want) {
		t.Fatalf("YAML pm.allowed_private_hosts = %v, want %v", cfg.PM.AllowedPrivateHosts, want)
	}
	if len(cfg.MCP.AllowedPrivateHosts) != 0 {
		t.Fatalf("pm.allowed_private_hosts reached mcp.allowed_private_hosts: %v", cfg.MCP.AllowedPrivateHosts)
	}

	t.Setenv("CODEFORGE_PM_ALLOWED_PRIVATE_HOSTS", " gitlab.lan , fd12::/16,,192.168.7.7")
	loadEnv(&cfg)
	if want := []string{"gitlab.lan", "fd12::/16", "192.168.7.7"}; !slices.Equal(cfg.PM.AllowedPrivateHosts, want) {
		t.Fatalf("env pm.allowed_private_hosts = %v, want %v (env replaces YAML)", cfg.PM.AllowedPrivateHosts, want)
	}
}

func TestValidate_PMAllowedPrivateHosts(t *testing.T) {
	for name, tt := range map[string]struct {
		hosts   []string
		wantErr bool
	}{
		"none":            {nil, false},
		"names and cidrs": {[]string{"gitlab.corp.internal", "10.0.0.0/8", "fd00::/8", "192.168.1.5", "127.0.0.1"}, false},
		"host with port":  {[]string{"gitlab.corp.internal:443"}, true},
		"url":             {[]string{"https://gitlab.corp.internal"}, true},
		"wildcard":        {[]string{"*.internal"}, true},
		"bad cidr":        {[]string{"10.0.0.0/40"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.PM.AllowedPrivateHosts = tt.hosts
			err := validate(&cfg)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validate() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "pm.allowed_private_hosts") {
				t.Fatalf("error %q should name pm.allowed_private_hosts", err)
			}
		})
	}
}
