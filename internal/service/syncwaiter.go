package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// ---------------------------------------------------------------------------
// syncWaiter — generic correlation-ID-based waiter
// ---------------------------------------------------------------------------

// syncWaiter manages a set of channel-based waiters keyed by correlation ID.
// Results come from shared durable consumers and may reach another Go Core
// replica than the one waiting: a waiter also serves its relay key, and a
// result without a local waiter is relayed to the replica that waits (KI-86).
type syncWaiter[T any] struct {
	mu      sync.Mutex
	waiters map[string]chan *T
	stops   map[string]func() // relay keys served, by request ID
	label   string            // for logging and the relay key
}

func newSyncWaiter[T any](label string) *syncWaiter[T] {
	return &syncWaiter[T]{
		waiters: make(map[string]chan *T),
		stops:   make(map[string]func()),
		label:   label,
	}
}

// relayKey is the relay key of a request's waiter.
func (w *syncWaiter[T]) relayKey(requestID string) string {
	return "result:" + w.label + ":" + requestID
}

// relayTaken is the relay answer of a waiter that took a result.
var relayTaken = []byte("taken")

// register creates a buffered channel for the given request ID and, with a
// relay (nil: this replica only), serves its relay key until unregister.
func (w *syncWaiter[T]) register(requestID string, relay messagequeue.Relay) chan *T {
	ch := make(chan *T, 1)
	w.mu.Lock()
	w.waiters[requestID] = ch
	w.mu.Unlock()
	if relay == nil {
		return ch
	}
	stop, err := relay.Serve(w.relayKey(requestID), func(data []byte) []byte {
		var payload T
		if err := json.Unmarshal(data, &payload); err != nil {
			slog.Warn("relayed "+w.label+" result unreadable", "request_id", requestID, "error", err)
			return nil
		}
		if !w.deliverLocal(requestID, &payload) {
			return nil
		}
		return relayTaken
	})
	if err != nil {
		slog.Warn("waiting for a "+w.label+" result on this replica only", "request_id", requestID, "error", err)
		return ch
	}
	w.mu.Lock()
	w.stops[requestID] = stop
	w.mu.Unlock()
	return ch
}

// unregister removes the waiter for the given request ID.
func (w *syncWaiter[T]) unregister(requestID string) {
	w.mu.Lock()
	delete(w.waiters, requestID)
	stop := w.stops[requestID]
	delete(w.stops, requestID)
	w.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// deliver hands a result to its waiter: on this replica, else through the
// relay (nil: none) to the replica that waits. Returns false if no waiter
// took it (none waits, it was taken already, or the relay failed).
func (w *syncWaiter[T]) deliver(ctx context.Context, relay messagequeue.Relay, requestID string, payload *T) bool {
	if w.deliverLocal(requestID, payload) {
		return true
	}
	if relay != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			slog.Error("relay "+w.label+" result", "request_id", requestID, "error", err)
			return false
		}
		answer, err := relay.Request(ctx, w.relayKey(requestID), data)
		if err != nil {
			slog.Warn("relay "+w.label+" result failed, its waiter times out", "request_id", requestID, "error", err)
			return false
		}
		if answer != nil {
			return true
		}
	}
	slog.Warn("no waiter for "+w.label+" result", "request_id", requestID)
	return false
}

// deliverLocal hands a result to this replica's waiter and removes it.
func (w *syncWaiter[T]) deliverLocal(requestID string, payload *T) bool {
	w.mu.Lock()
	ch, ok := w.waiters[requestID]
	if ok {
		delete(w.waiters, requestID)
	}
	w.mu.Unlock()
	if !ok {
		return false
	}
	ch <- payload
	return true
}
