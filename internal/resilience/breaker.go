// Package resilience provides reliability patterns for external service calls.
package resilience

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when the circuit breaker is open and rejecting calls.
var ErrCircuitOpen = errors.New("circuit breaker is open")

type state int

const (
	stateClosed state = iota
	stateOpen
	stateHalfOpen
)

// Breaker implements a circuit breaker pattern for protecting external calls.
// It tracks consecutive failures and opens the circuit when a threshold is reached,
// preventing further calls until a timeout elapses.
type Breaker struct {
	mu          sync.Mutex
	state       state
	failures    int
	maxFailures int
	timeout     time.Duration
	openedAt    time.Time
	now         func() time.Time // for testing
}

// NewBreaker creates a circuit breaker that opens after maxFailures consecutive
// failures and stays open for the given timeout before transitioning to half-open.
func NewBreaker(maxFailures int, timeout time.Duration) *Breaker {
	return &Breaker{
		maxFailures: maxFailures,
		timeout:     timeout,
		now:         time.Now,
	}
}

// Execute runs fn if the circuit is closed or half-open.
// Returns ErrCircuitOpen if the circuit is open.
func (b *Breaker) Execute(fn func() error) error {
	if !b.allowRequest() {
		return ErrCircuitOpen
	}

	err := fn()

	b.mu.Lock()
	defer b.mu.Unlock()

	if err != nil {
		b.onFailure()
		return err
	}

	b.onSuccess()
	return nil
}

func (b *Breaker) allowRequest() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateClosed:
		return true
	case stateOpen:
		if b.now().Sub(b.openedAt) >= b.timeout {
			b.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		return true
	}
	return false
}

// onFailure must be called with b.mu held.
func (b *Breaker) onFailure() {
	b.failures++
	if b.state == stateHalfOpen || b.failures >= b.maxFailures {
		b.state = stateOpen
		b.openedAt = b.now()
	}
}

// onSuccess must be called with b.mu held.
func (b *Breaker) onSuccess() {
	b.failures = 0
	b.state = stateClosed
}
