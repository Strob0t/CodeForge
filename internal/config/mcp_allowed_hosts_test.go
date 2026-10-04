package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// KI-100: only the platform operator allows MCP servers on private
// addresses, with mcp.allowed_private_hosts (YAML) or
// CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS (comma-separated).

func TestMCPAllowedPrivateHosts_DefaultEmpty(t *testing.T) {
	if hosts := Defaults().MCP.AllowedPrivateHosts; len(hosts) != 0 {
		t.Fatalf("default allowed_private_hosts = %v, want none", hosts)
	}
}

func TestMCPAllowedPrivateHosts_Layering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	yaml := "mcp:\n  allowed_private_hosts:\n    - docs-mcp\n    - 10.20.0.0/16\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}
	if want := []string{"docs-mcp", "10.20.0.0/16"}; !slices.Equal(cfg.MCP.AllowedPrivateHosts, want) {
		t.Fatalf("YAML allowed_private_hosts = %v, want %v", cfg.MCP.AllowedPrivateHosts, want)
	}

	t.Setenv("CODEFORGE_MCP_ALLOWED_PRIVATE_HOSTS", " tools.internal , fd12::/16,,192.168.7.7")
	loadEnv(&cfg)
	if want := []string{"tools.internal", "fd12::/16", "192.168.7.7"}; !slices.Equal(cfg.MCP.AllowedPrivateHosts, want) {
		t.Fatalf("env allowed_private_hosts = %v, want %v (env replaces YAML)", cfg.MCP.AllowedPrivateHosts, want)
	}
}

// TestMCPUseProxy_Layering (KI-100 review): MCP connections may go through
// the proxy of the environment, opt-in by the operator.
func TestMCPUseProxy_Layering(t *testing.T) {
	if Defaults().MCP.UseProxy {
		t.Fatal("mcp.use_proxy is on by default")
	}
	path := filepath.Join(t.TempDir(), "codeforge.yaml")
	if err := os.WriteFile(path, []byte("mcp:\n  use_proxy: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	if err := loadYAML(&cfg, path); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}
	if !cfg.MCP.UseProxy {
		t.Fatal("YAML use_proxy: true was not read")
	}
	t.Setenv("CODEFORGE_MCP_USE_PROXY", "false")
	loadEnv(&cfg)
	if cfg.MCP.UseProxy {
		t.Fatal("CODEFORGE_MCP_USE_PROXY=false did not override YAML")
	}
}

func TestValidate_MCPAllowedPrivateHosts(t *testing.T) {
	for name, tt := range map[string]struct {
		hosts   []string
		wantErr bool
	}{
		"none":            {nil, false},
		"names and cidrs": {[]string{"docs-mcp", "10.0.0.0/8", "fd00::/8", "192.168.1.5"}, false},
		"host with port":  {[]string{"docs-mcp:6280"}, true},
		"url":             {[]string{"http://docs-mcp"}, true},
		"wildcard":        {[]string{"*.internal"}, true},
		"bad cidr":        {[]string{"10.0.0.0/40"}, true},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Auth.JWTSecret = strongTestSecret
			cfg.MCP.AllowedPrivateHosts = tt.hosts
			err := validate(&cfg)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validate() = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "mcp.allowed_private_hosts") {
				t.Fatalf("error %q should name mcp.allowed_private_hosts", err)
			}
		})
	}
}
