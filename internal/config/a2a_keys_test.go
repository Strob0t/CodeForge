package config

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const testTenant = "11111111-2222-3333-4444-555555555555"

// A2A API keys map to the tenant their callers act in (KI-15): an entry
// "<tenant-uuid>:<key>" maps its key to that tenant, any other entry is a
// key of the default tenant.
func TestA2A_ParsedAPIKeys(t *testing.T) {
	keys, err := (&A2A{APIKeys: []string{"plain-key", testTenant + ":tenant-key", "not-a-uuid:colon-key"}}).ParsedAPIKeys()
	if err != nil {
		t.Fatalf("ParsedAPIKeys: %v", err)
	}
	want := []A2AAPIKey{
		{Key: "plain-key", TenantID: tenantctx.DefaultTenantID},
		{Key: "tenant-key", TenantID: testTenant},
		{Key: "not-a-uuid:colon-key", TenantID: tenantctx.DefaultTenantID},
	}
	if len(keys) != len(want) {
		t.Fatalf("keys = %+v, want %+v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("key %d = %+v, want %+v", i, keys[i], want[i])
		}
	}

	for _, bad := range [][]string{
		{""},
		{"  "},
		{testTenant + ":"},
		{"same-key", testTenant + ":same-key"},
	} {
		if _, err := (&A2A{APIKeys: bad}).ParsedAPIKeys(); err == nil {
			t.Errorf("ParsedAPIKeys(%q) succeeded, want an error", bad)
		}
	}
}

func TestValidate_A2AAPIKeys(t *testing.T) {
	cfg := Defaults()
	cfg.Auth.JWTSecret = strongTestSecret
	cfg.A2A.Enabled = true
	cfg.A2A.APIKeys = []string{testTenant + ":"}
	if err := validate(&cfg); err == nil || !strings.Contains(err.Error(), "a2a.api_keys") {
		t.Fatalf("validate() = %v, want an error naming a2a.api_keys", err)
	}
	// Keys of a disabled A2A server are not checked.
	cfg.A2A.Enabled = false
	if err := validate(&cfg); err != nil {
		t.Fatalf("validate() with A2A disabled = %v", err)
	}
}
