package middleware_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/middleware"
)

// mockKV is an in-memory mock of jetstream.KeyValue for testing.
type mockKV struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMockKV() *mockKV {
	return &mockKV{data: make(map[string][]byte)}
}

func (m *mockKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return &mockEntry{key: key, value: v}, nil
}

func (m *mockKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return 1, nil
}

// Implement remaining jetstream.KeyValue interface methods as no-ops.
func (m *mockKV) Bucket() string { return "test" }
func (m *mockKV) Create(_ context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, nil
}
func (m *mockKV) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	return 0, nil
}
func (m *mockKV) PutString(_ context.Context, _, _ string) (uint64, error)             { return 0, nil }
func (m *mockKV) Delete(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error { return nil }
func (m *mockKV) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error  { return nil }
func (m *mockKV) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	return nil, nil
}
func (m *mockKV) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) { return nil, nil }
func (m *mockKV) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return nil, nil
}
func (m *mockKV) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	return nil, nil
}
func (m *mockKV) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, nil
}
func (m *mockKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}
func (m *mockKV) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}
func (m *mockKV) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}
func (m *mockKV) Status(_ context.Context) (jetstream.KeyValueStatus, error)      { return nil, nil }
func (m *mockKV) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error { return nil }

// mockEntry implements jetstream.KeyValueEntry.
type mockEntry struct {
	key   string
	value []byte
}

func (e *mockEntry) Bucket() string                  { return "test" }
func (e *mockEntry) Key() string                     { return e.key }
func (e *mockEntry) Value() []byte                   { return e.value }
func (e *mockEntry) Revision() uint64                { return 1 }
func (e *mockEntry) Created() time.Time              { return time.Time{} }
func (e *mockEntry) Delta() uint64                   { return 0 }
func (e *mockEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }

func makeTestHandler(counter *int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*counter++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"call":%d}`, *counter)
	})
}

func TestIdempotency_NoHeader(t *testing.T) {
	counter := 0
	kv := newMockKV()
	handler := middleware.Idempotency(kv)(makeTestHandler(&counter))

	req := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if counter != 1 {
		t.Fatalf("expected 1 call, got %d", counter)
	}
}

func TestIdempotency_FirstRequestStoresResponse(t *testing.T) {
	counter := 0
	kv := newMockKV()
	handler := middleware.Idempotency(kv)(makeTestHandler(&counter))

	req := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	req.Header.Set("Idempotency-Key", "key-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if counter != 1 {
		t.Fatalf("expected 1 call, got %d", counter)
	}
	// Verify stored in KV
	kv.mu.Lock()
	_, ok := kv.data["key-1"]
	kv.mu.Unlock()
	if !ok {
		t.Fatal("expected key-1 in KV store")
	}
}

func TestIdempotency_SecondRequestReplays(t *testing.T) {
	counter := 0
	kv := newMockKV()
	handler := middleware.Idempotency(kv)(makeTestHandler(&counter))

	// First request
	req1 := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	req1.Header.Set("Idempotency-Key", "key-2")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	// Second request with same key
	req2 := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	req2.Header.Set("Idempotency-Key", "key-2")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if counter != 1 {
		t.Fatalf("expected handler called once, got %d", counter)
	}
	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec2.Code)
	}
}

func TestIdempotency_GETIgnored(t *testing.T) {
	counter := 0
	kv := newMockKV()
	handler := middleware.Idempotency(kv)(makeTestHandler(&counter))

	req := httptest.NewRequest(http.MethodGet, "/test", http.NoBody)
	req.Header.Set("Idempotency-Key", "key-get")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if counter != 1 {
		t.Fatalf("expected handler called, got %d", counter)
	}
}

func TestIdempotency_DifferentKeys(t *testing.T) {
	counter := 0
	kv := newMockKV()
	handler := middleware.Idempotency(kv)(makeTestHandler(&counter))

	// Request with key-a
	req1 := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	req1.Header.Set("Idempotency-Key", "key-a")
	handler.ServeHTTP(httptest.NewRecorder(), req1)

	// Request with key-b
	req2 := httptest.NewRequest(http.MethodPost, "/test", http.NoBody)
	req2.Header.Set("Idempotency-Key", "key-b")
	handler.ServeHTTP(httptest.NewRecorder(), req2)

	if counter != 2 {
		t.Fatalf("expected 2 calls, got %d", counter)
	}
}
