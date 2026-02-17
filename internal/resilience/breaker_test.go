package resilience

import (
	"errors"
	"testing"
	"time"
)

var errTest = errors.New("service unavailable")

func TestClosedStateAllowsCalls(t *testing.T) {
	b := NewBreaker(3, time.Second)
	called := false
	err := b.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called")
	}
}

func TestOpensAfterMaxFailures(t *testing.T) {
	b := NewBreaker(3, time.Second)

	for i := 0; i < 3; i++ {
		_ = b.Execute(func() error { return errTest })
	}

	err := b.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestTransitionsToHalfOpenAfterTimeout(t *testing.T) {
	now := time.Now()
	b := NewBreaker(2, time.Second)
	b.now = func() time.Time { return now }

	// Trip the breaker
	for i := 0; i < 2; i++ {
		_ = b.Execute(func() error { return errTest })
	}

	// Still open
	err := b.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Advance past timeout
	now = now.Add(2 * time.Second)

	// Should be half-open — allows one call
	called := false
	err = b.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error in half-open, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called in half-open")
	}

	// Success should close the circuit
	b.mu.Lock()
	if b.state != stateClosed {
		t.Fatalf("expected state closed after half-open success, got %d", b.state)
	}
	b.mu.Unlock()
}

func TestHalfOpenFailureReopens(t *testing.T) {
	now := time.Now()
	b := NewBreaker(2, time.Second)
	b.now = func() time.Time { return now }

	// Trip the breaker
	for i := 0; i < 2; i++ {
		_ = b.Execute(func() error { return errTest })
	}

	// Advance past timeout to reach half-open
	now = now.Add(2 * time.Second)

	// Fail in half-open → should reopen
	_ = b.Execute(func() error { return errTest })

	b.mu.Lock()
	if b.state != stateOpen {
		t.Fatalf("expected state open after half-open failure, got %d", b.state)
	}
	b.mu.Unlock()

	// Calls should be rejected
	err := b.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen after reopen, got %v", err)
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	b := NewBreaker(3, time.Second)

	// Two failures
	_ = b.Execute(func() error { return errTest })
	_ = b.Execute(func() error { return errTest })

	// One success resets
	_ = b.Execute(func() error { return nil })

	// Two more failures should not trip (only 2, need 3)
	_ = b.Execute(func() error { return errTest })
	_ = b.Execute(func() error { return errTest })

	// Still closed
	called := false
	err := b.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !called {
		t.Fatal("expected fn to be called")
	}
}
