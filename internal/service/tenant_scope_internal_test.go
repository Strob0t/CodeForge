package service

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	scopeHeaderTenant  = "hhhhhhhh-0000-4000-8000-000000000001"
	scopePayloadTenant = "pppppppp-0000-4000-8000-000000000002"
	scopeRequestTenant = "rrrrrrrr-0000-4000-8000-000000000003"
)

// The payload tenant keeps precedence over the message's header tenant
// (KI-64): a message whose payload names its tenant is handled in it, a
// message without one in its header tenant, and a request's own tenant is
// never replaced.
func TestWithPayloadTenant_Precedence(t *testing.T) {
	bg := context.Background()
	fromHeader := tenantctx.WithMessageTenant(bg, scopeHeaderTenant)
	tests := []struct {
		name    string
		ctx     context.Context
		payload string
		want    string
	}{
		{"payload over header", fromHeader, scopePayloadTenant, scopePayloadTenant},
		{"header without payload tenant", fromHeader, "", scopeHeaderTenant},
		{"request over payload", tenantctx.WithTenant(bg, scopeRequestTenant), scopePayloadTenant, scopeRequestTenant},
		{"payload only", bg, scopePayloadTenant, scopePayloadTenant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tenantctx.Lookup(withPayloadTenant(tt.ctx, tt.payload))
			if !ok || got != tt.want {
				t.Fatalf("tenant = %q, %v; want %q", got, ok, tt.want)
			}
		})
	}
}

// outgoingTenant is the tenant stamped on outgoing payloads: the tenant of
// ctx; a missing one is logged loudly (it used to fall back silently).
func TestOutgoingTenant(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	if got := outgoingTenant(tenantctx.WithTenant(context.Background(), scopeRequestTenant), "runs.start"); got != scopeRequestTenant {
		t.Fatalf("outgoingTenant = %q, want the context tenant", got)
	}
	if got := outgoingTenant(tenantctx.WithMessageTenant(context.Background(), scopeHeaderTenant), "runs.start"); got != scopeHeaderTenant {
		t.Fatalf("outgoingTenant = %q, want the message tenant", got)
	}
	if logs.Len() != 0 {
		t.Fatalf("a present tenant must not be logged: %s", logs.String())
	}

	if got := outgoingTenant(context.Background(), "runs.start"); got != tenantctx.DefaultTenantID {
		t.Fatalf("outgoingTenant = %q, want the default tenant", got)
	}
	if !strings.Contains(logs.String(), "level=ERROR") || !strings.Contains(logs.String(), "runs.start") {
		t.Fatalf("a missing tenant must be logged as an error naming the message: %s", logs.String())
	}
}
