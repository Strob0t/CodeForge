package tenantctx_test

import (
	"context"
	"testing"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// A queue message's tenant header scopes its handler (KI-64). It applies
// where no tenant was set explicitly: a tenant set with WithTenant (an HTTP
// request, or the message payload's tenant) overrides it.
func TestMessageTenant(t *testing.T) {
	const header, payload = "hhhhhhhh-0000-4000-8000-000000000001", "pppppppp-0000-4000-8000-000000000002"
	bg := context.Background()

	tests := []struct {
		name         string
		ctx          context.Context
		want         string
		wantLookup   bool
		wantExplicit bool
	}{
		{"none", bg, tenantctx.DefaultTenantID, false, false},
		{"message tenant", tenantctx.WithMessageTenant(bg, header), header, true, false},
		{"explicit over message tenant", tenantctx.WithTenant(tenantctx.WithMessageTenant(bg, header), payload), payload, true, true},
		{"empty message tenant", tenantctx.WithMessageTenant(bg, ""), tenantctx.DefaultTenantID, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantctx.FromContext(tt.ctx); got != tt.want {
				t.Errorf("FromContext = %q, want %q", got, tt.want)
			}
			got, ok := tenantctx.Lookup(tt.ctx)
			if ok != tt.wantLookup || (ok && got != tt.want) {
				t.Errorf("Lookup = %q, %v; want %q, %v", got, ok, tt.want, tt.wantLookup)
			}
			if _, ok := tenantctx.Explicit(tt.ctx); ok != tt.wantExplicit {
				t.Errorf("Explicit ok = %v, want %v", ok, tt.wantExplicit)
			}
		})
	}
}
