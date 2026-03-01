package service

import (
	"log/slog"
	"sync"
)

// ---------------------------------------------------------------------------
// syncWaiter — generic correlation-ID-based waiter
// ---------------------------------------------------------------------------

// syncWaiter manages a set of channel-based waiters keyed by correlation ID.
type syncWaiter[T any] struct {
	mu      sync.Mutex
	waiters map[string]chan *T
	label   string // for logging
}

func newSyncWaiter[T any](label string) *syncWaiter[T] {
	return &syncWaiter[T]{
		waiters: make(map[string]chan *T),
		label:   label,
	}
}

// register creates a buffered channel for the given request ID.
func (w *syncWaiter[T]) register(requestID string) chan *T {
	ch := make(chan *T, 1)
	w.mu.Lock()
	w.waiters[requestID] = ch
	w.mu.Unlock()
	return ch
}

// unregister removes the waiter for the given request ID.
func (w *syncWaiter[T]) unregister(requestID string) {
	w.mu.Lock()
	delete(w.waiters, requestID)
	w.mu.Unlock()
}

// deliver sends a result to the waiting channel and removes the waiter.
// Returns false if no waiter was registered for the given ID.
func (w *syncWaiter[T]) deliver(requestID string, payload *T) bool {
	w.mu.Lock()
	ch, ok := w.waiters[requestID]
	if ok {
		delete(w.waiters, requestID)
	}
	w.mu.Unlock()

	if !ok {
		slog.Warn("no waiter for "+w.label+" result", "request_id", requestID)
		return false
	}

	ch <- payload
	return true
}
