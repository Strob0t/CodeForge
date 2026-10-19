package tenantctx_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

func TestLookup(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		want   string
		wantOK bool
	}{
		{"no tenant", context.Background(), "", false},
		{"empty tenant", tenantctx.WithTenant(context.Background(), ""), "", false},
		{"default tenant set explicitly", tenantctx.WithTenant(context.Background(), tenantctx.DefaultTenantID), tenantctx.DefaultTenantID, true},
		{"tenant set", tenantctx.WithTenant(context.Background(), "t-1"), "t-1", true},
		{"innermost tenant wins", tenantctx.WithTenant(tenantctx.WithTenant(context.Background(), "t-1"), "t-2"), "t-2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tenantctx.Lookup(tt.ctx)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("Lookup() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestFromContextFallsBackToDefault(t *testing.T) {
	if got := tenantctx.FromContext(context.Background()); got != tenantctx.DefaultTenantID {
		t.Fatalf("FromContext() = %q, want default tenant", got)
	}
}
