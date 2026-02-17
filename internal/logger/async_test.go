package logger

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// recordingHandler collects slog.Records for test assertions.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
	delay   time.Duration // optional per-record processing delay
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(_ context.Context, rec slog.Record) error { //nolint:gocritic // slog.Handler interface requires value receiver
	if h.delay > 0 {
		time.Sleep(h.delay)
	}
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

func TestAsyncHandler_BasicWrite(t *testing.T) {
	inner := &recordingHandler{}
	ah := NewAsyncHandler(inner, 100, 1)

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if err := ah.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	ah.Close()

	if got := inner.count(); got != 1 {
		t.Fatalf("expected 1 record, got %d", got)
	}
}

func TestAsyncHandler_ConcurrentWrites(t *testing.T) {
	const goroutines = 100
	const perGoroutine = 100
	total := goroutines * perGoroutine

	inner := &recordingHandler{}
	ah := NewAsyncHandler(inner, 10000, 4)

	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				rec := slog.NewRecord(time.Now(), slog.LevelInfo, "concurrent", 0)
				_ = ah.Handle(context.Background(), rec)
			}
		}()
	}
	wg.Wait()
	ah.Close()

	if got := inner.count(); got != total {
		t.Fatalf("expected %d records, got %d", total, got)
	}
}

func TestAsyncHandler_ChannelFullDrops(t *testing.T) {
	// Use a slow inner handler with a tiny channel to force drops.
	inner := &recordingHandler{delay: 10 * time.Millisecond}
	ah := NewAsyncHandler(inner, 1, 1)

	// Rapidly enqueue more records than the channel can hold.
	for range 50 {
		rec := slog.NewRecord(time.Now(), slog.LevelInfo, "flood", 0)
		_ = ah.Handle(context.Background(), rec)
	}

	ah.Close()

	dropped := ah.DroppedCount()
	if dropped == 0 {
		t.Fatal("expected some records to be dropped, got 0")
	}
	t.Logf("dropped %d out of 50 records", dropped)
}

func TestAsyncHandler_CloseFlushesRemaining(t *testing.T) {
	inner := &recordingHandler{}
	ah := NewAsyncHandler(inner, 1000, 2)

	const total = 200
	for range total {
		rec := slog.NewRecord(time.Now(), slog.LevelInfo, "flush-test", 0)
		_ = ah.Handle(context.Background(), rec)
	}

	// Close should block until all enqueued records are drained.
	ah.Close()

	if got := inner.count(); got != total {
		t.Fatalf("expected %d records after close, got %d", total, got)
	}
}
