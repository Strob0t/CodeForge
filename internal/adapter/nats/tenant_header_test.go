package nats

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const headerTestTenant = "aaaaaaaa-0000-4000-8000-000000000001"

// The tenant travels as a message header (KI-64): the publisher stamps the
// tenant of its context, and the handler of a message runs in the tenant of
// its header, so a subject needs no per-payload tenant plumbing to keep its
// events and queries in the right tenant.
func TestPublish_StampsTheTenantHeader(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"tenant in context", tenantctx.WithTenant(context.Background(), headerTestTenant), headerTestTenant},
		{"message tenant in context", tenantctx.WithMessageTenant(context.Background(), headerTestTenant), headerTestTenant},
		{"no tenant", context.Background(), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, publish := range []func(q *Queue) error{
				func(q *Queue) error { return q.Publish(tt.ctx, "runs.start", []byte(`{}`)) },
				func(q *Queue) error { return q.PublishWithDedup(tt.ctx, "runs.start", []byte(`{}`), "id-1") },
			} {
				js := &fakeJS{ack: jetstream.PubAck{Stream: streamName, Sequence: 1}}
				if err := publish(newTestQueue(js)); err != nil {
					t.Fatal(err)
				}
				if got := js.published[0].Header.Get(headerTenantID); got != tt.want {
					t.Errorf("tenant header = %q, want %q", got, tt.want)
				}
			}
		})
	}
}

func TestHandleMessage_RunsInTheHeaderTenant(t *testing.T) {
	tests := []struct {
		name    string
		headers nats.Header
		want    string
		wantOK  bool
	}{
		{"tenant header", nats.Header{headerTenantID: []string{headerTestTenant}}, headerTestTenant, true},
		{"no header", nil, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := &fakeJS{ack: jetstream.PubAck{Stream: streamName, Sequence: 1}}
			msg := &fakeMsg{subject: "tasks.agent.x", data: []byte(`{"a":1}`), headers: tt.headers, numDelivered: 1}
			var got string
			var ok, explicit bool
			newTestQueue(js).handleMessage(context.Background(), msg, func(ctx context.Context, _ string, _ []byte) error {
				got, ok = tenantctx.Lookup(ctx)
				_, explicit = tenantctx.Explicit(ctx)
				return nil
			})
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("handler tenant = %q, %v; want %q, %v", got, ok, tt.want, tt.wantOK)
			}
			if explicit {
				t.Fatal("the header tenant must not count as explicit: the payload tenant overrides it")
			}
			assertSettled(t, msg, "ack")
		})
	}
}
