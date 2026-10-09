package resilience

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// The shared breakers guard the service, not single requests: a request the
// service rejected (4xx, a payload too large) or the caller gave up on is no
// outage, so one user's bad requests cannot block everyone (KI-213).
func TestBreaker_RequestErrorsDoNotOpenIt(t *testing.T) {
	tests := []struct {
		name string
		err  error
		open bool
	}{
		{"service error", errTest, true},
		{"timeout", fmt.Errorf("request: %w", context.DeadlineExceeded), true},
		{"rejected request", Neutral(errors.New("litellm API error 400: bad model")), false},
		{"wrapped rejected request", fmt.Errorf("add model: %w", Neutral(errors.New("422"))), false},
		{"caller cancelled", fmt.Errorf("request: %w", context.Canceled), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBreaker(3, time.Minute)
			for range 5 {
				err := b.Execute(func() error { return tt.err })
				if errors.Is(err, ErrCircuitOpen) {
					break
				}
				if !errors.Is(err, tt.err) {
					t.Fatalf("Execute = %v, want it to wrap %v", err, tt.err)
				}
			}
			err := b.Execute(func() error { return nil })
			if gotOpen := errors.Is(err, ErrCircuitOpen); gotOpen != tt.open {
				t.Fatalf("open = %v after five %q, want %v", gotOpen, tt.name, tt.open)
			}
		})
	}
}

// A neutral error between failures neither resets nor adds to the count.
func TestBreaker_NeutralErrorKeepsTheCount(t *testing.T) {
	b := NewBreaker(2, time.Minute)
	_ = b.Execute(func() error { return errTest })
	_ = b.Execute(func() error { return Neutral(errTest) })
	_ = b.Execute(func() error { return errTest })
	if err := b.Execute(func() error { return nil }); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("Execute = %v, want ErrCircuitOpen after two service errors", err)
	}
}

func TestNeutral(t *testing.T) {
	if Neutral(nil) != nil {
		t.Fatal("Neutral(nil) must be nil")
	}
	err := Neutral(errTest)
	if !errors.Is(err, errTest) || err.Error() != errTest.Error() {
		t.Fatalf("Neutral changed the error: %v", err)
	}
	if !IsNeutral(fmt.Errorf("wrapped: %w", err)) || IsNeutral(errTest) {
		t.Fatal("IsNeutral does not follow the wrap chain")
	}
}

// An error after the caller's context ended (cancelled, or its deadline such
// as the API route timeout passed) is the caller's, not the service's; a
// timeout of the call itself while the caller still waits is an outage.
func TestBreaker_ExecuteContextCountsOnlyTheCallsOwnTimeout(t *testing.T) {
	expired, cancelExpired := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancelExpired()
	<-expired.Done()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		open bool
	}{
		{"caller deadline passed", expired, fmt.Errorf("http request: %w", context.DeadlineExceeded), false},
		{"caller deadline passed, any error", expired, errTest, false},
		{"caller cancelled", cancelled, fmt.Errorf("http request: %w", context.Canceled), false},
		{"transport timeout while the caller waits", context.Background(), fmt.Errorf("Client.Timeout exceeded: %w", context.DeadlineExceeded), true},
		{"service error while the caller waits", context.Background(), errTest, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBreaker(3, time.Minute)
			for range 5 {
				err := b.ExecuteContext(tt.ctx, func() error { return tt.err })
				if errors.Is(err, ErrCircuitOpen) {
					break
				}
				if !errors.Is(err, tt.err) {
					t.Fatalf("ExecuteContext = %v, want it to wrap %v", err, tt.err)
				}
			}
			err := b.Execute(func() error { return nil })
			if gotOpen := errors.Is(err, ErrCircuitOpen); gotOpen != tt.open {
				t.Fatalf("open = %v after five %q, want %v", gotOpen, tt.name, tt.open)
			}
		})
	}
}
