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

	t.Run("MaxReconnects_Unlimited", func(t *testing.T) {
		// A bounded count closed the connection for good after an outage
		// longer than count x wait: runs never started again and
		// /health/ready stayed 503 (KI-213).
		if nopts.MaxReconnect != -1 {
			t.Errorf("MaxReconnect = %d, want -1 (reconnect until the process stops)", nopts.MaxReconnect)
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
