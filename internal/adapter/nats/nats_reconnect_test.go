package nats

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// --------------------------------------------------------------------------
// TestReconnectOpts_Comprehensive (FIX-013)
//
// Verifies that reconnectOpts() returns options that configure NATS for
// production-grade resilience: auto-reconnect, error reporting, and
// reasonable timeouts.
// --------------------------------------------------------------------------

func TestReconnectOpts_Comprehensive(t *testing.T) {
	opts := reconnectOpts()

	// Apply all options to a nats.Options struct for inspection.
	nopts := nats.GetDefaultOptions()
	for _, o := range opts {
		if err := o(&nopts); err != nil {
			t.Fatalf("applying option: %v", err)
		}
	}

	t.Run("MaxReconnects_Positive", func(t *testing.T) {
		if nopts.MaxReconnect <= 0 {
			t.Errorf("MaxReconnect = %d, want > 0 for auto-reconnect", nopts.MaxReconnect)
		}
	})

	t.Run("MaxReconnects_Reasonable", func(t *testing.T) {
		// At least 10 reconnect attempts (with 2s wait = 20s minimum recovery window).
		if nopts.MaxReconnect < 10 {
			t.Errorf("MaxReconnect = %d, want >= 10 for production resilience", nopts.MaxReconnect)
		}
	})

	t.Run("ReconnectWait_Positive", func(t *testing.T) {
		if nopts.ReconnectWait <= 0 {
			t.Errorf("ReconnectWait = %v, want > 0", nopts.ReconnectWait)
		}
	})

	t.Run("ReconnectWait_NotTooFast", func(t *testing.T) {
		// At least 1 second between reconnect attempts to avoid thundering herd.
		if nopts.ReconnectWait < 1*time.Second {
			t.Errorf("ReconnectWait = %v, want >= 1s to avoid thundering herd", nopts.ReconnectWait)
		}
	})

	t.Run("DisconnectHandler_Set", func(t *testing.T) {
		if nopts.DisconnectedErrCB == nil {
			t.Error("DisconnectErrHandler must be set for disconnect logging")
		}
	})

	t.Run("ReconnectHandler_Set", func(t *testing.T) {
		if nopts.ReconnectedCB == nil {
			t.Error("ReconnectHandler must be set for reconnect logging")
		}
	})

	t.Run("ErrorHandler_Set", func(t *testing.T) {
		if nopts.AsyncErrorCB == nil {
			t.Error("ErrorHandler must be set for async error reporting")
		}
	})

	t.Run("OptionCount", func(t *testing.T) {
		// reconnectOpts must return at least 3 options:
		// MaxReconnects, ReconnectWait, and at least one handler.
		if len(opts) < 3 {
			t.Errorf("reconnectOpts returned %d options, want >= 3", len(opts))
		}
	})
}

// TestReconnectOpts_TotalRecoveryWindow verifies the total recovery window
// (MaxReconnects * ReconnectWait) is sufficient for typical outages.
func TestReconnectOpts_TotalRecoveryWindow(t *testing.T) {
	opts := reconnectOpts()

	nopts := nats.GetDefaultOptions()
	for _, o := range opts {
		if err := o(&nopts); err != nil {
			t.Fatalf("applying option: %v", err)
		}
	}

	totalWindow := time.Duration(nopts.MaxReconnect) * nopts.ReconnectWait
	// Total recovery window should be at least 30 seconds for
	// typical container restarts/network blips.
	if totalWindow < 30*time.Second {
		t.Errorf("total recovery window = %v (MaxReconnect=%d * ReconnectWait=%v), want >= 30s",
			totalWindow, nopts.MaxReconnect, nopts.ReconnectWait)
	}
}

// TestSanitizeConsumerName verifies the consumer name builder.
func TestSanitizeConsumerName(t *testing.T) {
	tests := []struct {
		prefix  string
		subject string
		want    string
	}{
		{"codeforge-go-", "conversation.run.start", "codeforge-go-conversation-run-start"},
		{"codeforge-go-", "benchmark.>", "codeforge-go-benchmark-all"},
		{"codeforge-go-", "tasks.*", "codeforge-go-tasks-all"},
		{"codeforge-py-", "evaluation.run.start", "codeforge-py-evaluation-run-start"},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			got := sanitizeConsumerName(tt.prefix, tt.subject)
			if got != tt.want {
				t.Errorf("sanitizeConsumerName(%q, %q) = %q, want %q",
					tt.prefix, tt.subject, got, tt.want)
			}
		})
	}
}
