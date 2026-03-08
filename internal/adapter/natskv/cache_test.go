package natskv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// mockKVEntry implements jetstream.KeyValueEntry for testing.
type mockKVEntry struct {
	key   string
	value []byte
}

func (e *mockKVEntry) Key() string                     { return e.key }
func (e *mockKVEntry) Value() []byte                   { return e.value }
func (e *mockKVEntry) Revision() uint64                { return 1 }
func (e *mockKVEntry) Created() time.Time              { return time.Now() }
func (e *mockKVEntry) Delta() uint64                   { return 0 }
func (e *mockKVEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }
func (e *mockKVEntry) Bucket() string                  { return "test" }

// mockKV implements jetstream.KeyValue for testing.
type mockKV struct {
	data map[string][]byte
}

func newMockKV() *mockKV {
	return &mockKV{data: make(map[string][]byte)}
}

func (m *mockKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	val, ok := m.data[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return &mockKVEntry{key: key, value: val}, nil
}

func (m *mockKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	m.data[key] = value
	return 1, nil
}

func (m *mockKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	if _, ok := m.data[key]; !ok {
		return jetstream.ErrKeyNotFound
	}
	delete(m.data, key)
	return nil
}

// Stub methods to satisfy the jetstream.KeyValue interface.
func (m *mockKV) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) Create(_ context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	return 0, errors.New("not implemented")
}
func (m *mockKV) Update(_ context.Context, _ string, _ []byte, _ uint64) (uint64, error) {
	return 0, errors.New("not implemented")
}
func (m *mockKV) PutString(_ context.Context, _, _ string) (uint64, error) {
	return 0, errors.New("not implemented")
}
func (m *mockKV) Purge(_ context.Context, _ string, _ ...jetstream.KVDeleteOpt) error {
	return errors.New("not implemented")
}
func (m *mockKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) Bucket() string { return "test" }
func (m *mockKV) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	return errors.New("not implemented")
}
func (m *mockKV) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return nil, errors.New("not implemented")
}
func (m *mockKV) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	return nil, errors.New("not implemented")
}

func TestNew(t *testing.T) {
	t.Parallel()

	kv := newMockKV()
	cache := New(kv)

	if cache == nil {
		t.Fatal("New() returned nil")
	}
	if cache.kv != kv {
		t.Error("cache.kv not set correctly")
	}
}

func TestGetMissing(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	data, ok, err := cache.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if ok {
		t.Error("Get() ok = true, want false for missing key")
	}
	if data != nil {
		t.Errorf("Get() data = %v, want nil", data)
	}
}

func TestSetAndGet(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	value := []byte("hello world")
	if err := cache.Set(ctx, "key1", value, time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	data, ok, err := cache.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Error("Get() ok = false, want true")
	}
	if string(data) != "hello world" {
		t.Errorf("Get() data = %q, want %q", string(data), "hello world")
	}
}

func TestSetOverwrite(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	if err := cache.Set(ctx, "key", []byte("v1"), time.Minute); err != nil {
		t.Fatalf("Set() v1 error = %v", err)
	}
	if err := cache.Set(ctx, "key", []byte("v2"), time.Minute); err != nil {
		t.Fatalf("Set() v2 error = %v", err)
	}

	data, ok, err := cache.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Error("Get() ok = false, want true")
	}
	if string(data) != "v2" {
		t.Errorf("Get() data = %q, want %q", string(data), "v2")
	}
}

func TestDelete(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	if err := cache.Set(ctx, "key", []byte("value"), time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := cache.Delete(ctx, "key"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	data, ok, err := cache.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok {
		t.Error("Get() ok = true, want false after Delete")
	}
	if data != nil {
		t.Errorf("Get() data = %v, want nil after Delete", data)
	}
}

func TestDeleteNonexistent(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	// Delete of nonexistent key should return nil (not error).
	err := cache.Delete(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Delete() error = %v, want nil for nonexistent key", err)
	}
}

func TestSetEmptyValue(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	if err := cache.Set(ctx, "empty", []byte{}, time.Minute); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	data, ok, err := cache.Get(ctx, "empty")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Error("Get() ok = false, want true for empty value")
	}
	if len(data) != 0 {
		t.Errorf("Get() data = %v, want empty slice", data)
	}
}

func TestMultipleKeys(t *testing.T) {
	t.Parallel()

	cache := New(newMockKV())
	ctx := context.Background()

	keys := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	for k, v := range keys {
		if err := cache.Set(ctx, k, []byte(v), time.Minute); err != nil {
			t.Fatalf("Set(%q) error = %v", k, err)
		}
	}

	for k, want := range keys {
		data, ok, err := cache.Get(ctx, k)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", k, err)
		}
		if !ok {
			t.Errorf("Get(%q) ok = false, want true", k)
		}
		if string(data) != want {
			t.Errorf("Get(%q) = %q, want %q", k, string(data), want)
		}
	}
}
