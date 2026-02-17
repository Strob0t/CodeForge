package logger

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
)

// Closer allows flushing and stopping the async handler.
type Closer interface {
	Close()
}

// nopCloser is a no-op Closer for synchronous mode.
type nopCloser struct{}

func (nopCloser) Close() {}

// AsyncHandler wraps an slog.Handler with a buffered channel and worker pool.
type AsyncHandler struct {
	inner   slog.Handler
	ch      chan slog.Record
	wg      *sync.WaitGroup
	dropped *atomic.Int64
}

// NewAsyncHandler creates an AsyncHandler with the given channel capacity and worker count.
func NewAsyncHandler(inner slog.Handler, chanSize, workers int) *AsyncHandler {
	h := &AsyncHandler{
		inner:   inner,
		ch:      make(chan slog.Record, chanSize),
		wg:      &sync.WaitGroup{},
		dropped: &atomic.Int64{},
	}
	for range workers {
		h.wg.Add(1)
		go h.drain()
	}
	return h
}

func (h *AsyncHandler) drain() {
	defer h.wg.Done()
	for rec := range h.ch {
		_ = h.inner.Handle(context.Background(), rec)
	}
}

// Enabled delegates to the inner handler.
func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle enqueues the record. Drops if the channel is full.
func (h *AsyncHandler) Handle(_ context.Context, rec slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	select {
	case h.ch <- rec:
	default:
		h.dropped.Add(1)
	}
	return nil
}

// WithAttrs returns a new AsyncHandler sharing the same channel but wrapping a new inner handler.
func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncHandler{
		inner:   h.inner.WithAttrs(attrs),
		ch:      h.ch,
		wg:      h.wg,
		dropped: h.dropped,
	}
}

// WithGroup returns a new AsyncHandler sharing the same channel but wrapping a new inner handler.
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	return &AsyncHandler{
		inner:   h.inner.WithGroup(name),
		ch:      h.ch,
		wg:      h.wg,
		dropped: h.dropped,
	}
}

// DroppedCount returns the number of dropped records.
func (h *AsyncHandler) DroppedCount() int64 {
	return h.dropped.Load()
}

// Close closes the channel and waits for all workers to drain.
func (h *AsyncHandler) Close() {
	close(h.ch)
	h.wg.Wait()
}
