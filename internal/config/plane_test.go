package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// S3-F security review S3: the operator's Plane token goes only to the
// operator's Plane, plane.base_url (default: the Plane cloud API).
func TestPlaneBaseURL(t *testing.T) {
	if got := Defaults().Plane.BaseURL; got != "https://api.plane.so" {
		t.Fatalf("default plane.base_url = %q, want the Plane cloud API", got)
	}

	t.Setenv("CODEFORGE_PLANE_BASE_URL", "https://plane.example.com")
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if cfg.Plane.BaseURL != "https://plane.example.com" {
		t.Fatalf("plane.base_url from env = %q", cfg.Plane.BaseURL)
	}

	for _, bad := range []string{"", "plane.example.com", "ftp://plane.example.com", "https://u:p@plane.example.com", "https://plane.example.com/?x=1"} {
		cfg := Defaults()
		cfg.AppEnv = "development"
		if err := ensureSecrets(&cfg); err != nil {
			t.Fatal(err)
		}
		cfg.Plane.BaseURL = bad
		if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "plane.base_url") {
			t.Errorf("validate with plane.base_url %q = %v, want an error naming plane.base_url", bad, err)
		}
	}
}
