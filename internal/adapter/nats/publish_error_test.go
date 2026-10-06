package nats

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/resilience"
)

// A message the client or the server refused (too large, a 4xx API answer)
// does not count against NATS in the shared breaker; an unreachable server
// or a timeout does (KI-213).
func TestPublishError_RequestErrorsAreNeutral(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		neutral bool
	}{
		{"payload too large", nats.ErrMaxPayload, true},
		{"api 400", &jetstream.APIError{Code: 400, Description: "maximum message size exceeded"}, true},
		{"api 503", &jetstream.APIError{Code: 503, Description: "JetStream system temporarily unavailable"}, false},
		{"timeout", nats.ErrTimeout, false},
		{"no responders", nats.ErrNoResponders, false},
		{"connection closed", nats.ErrConnectionClosed, false},
		{"deadline", context.DeadlineExceeded, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publishError("tasks.agent.x", tt.err)
			if !errors.Is(got, tt.err) {
				t.Fatalf("publishError(%v) = %v, does not wrap it", tt.err, got)
			}
			if resilience.IsNeutral(got) != tt.neutral {
				t.Fatalf("neutral = %v, want %v", resilience.IsNeutral(got), tt.neutral)
			}
		})
	}
}
