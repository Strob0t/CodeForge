package nats

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/logger"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// testConnect connects to NATS or skips the test if NATS_URL is not set.
func testConnect(t *testing.T) *Queue {
	t.Helper()

	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("requires NATS_URL")
	}

	q, err := Connect(context.Background(), url)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		if err := q.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return q
}

// uniqueSubject returns a test subject under the "tasks.agent." prefix
// which the CODEFORGE stream captures (tasks.>) and the validator
// accepts as any valid JSON.
func uniqueSubject(t *testing.T) string {
	t.Helper()
	// Use test name to avoid collisions between parallel tests.
	return "tasks.agent.test." + t.Name()
}

func TestQueue_PublishSubscribe(t *testing.T) {
	q := testConnect(t)
	subject := uniqueSubject(t)

	type payload struct {
		Msg string `json:"msg"`
	}
	want := payload{Msg: "hello-nats"}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var (
		mu       sync.Mutex
		received *payload
		done     = make(chan struct{})
		once     sync.Once
	)

	stop, err := q.Subscribe(context.Background(), subject, func(_ context.Context, subj string, d []byte) error {
		var got payload
		if err := json.Unmarshal(d, &got); err != nil {
			return err
		}
		mu.Lock()
		received = &got
		mu.Unlock()
		once.Do(func() { close(done) })
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	if err := q.Publish(context.Background(), subject, data); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("handler was not called")
	}
	if received.Msg != want.Msg {
		t.Errorf("got %q, want %q", received.Msg, want.Msg)
	}
}

func TestQueue_RequestIDPropagation(t *testing.T) {
	q := testConnect(t)
	subject := uniqueSubject(t)

	const wantReqID = "req-abc-123"
	data := []byte(`{"ok":true}`)

	var (
		mu       sync.Mutex
		gotReqID string
		done     = make(chan struct{})
		once     sync.Once
	)

	stop, err := q.Subscribe(context.Background(), subject, func(ctx context.Context, _ string, _ []byte) error {
		mu.Lock()
		gotReqID = logger.RequestID(ctx)
		mu.Unlock()
		once.Do(func() { close(done) })
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	// Publish with a request ID in the context.
	ctx := logger.WithRequestID(context.Background(), wantReqID)
	if err := q.Publish(ctx, subject, data); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message")
	}

	mu.Lock()
	defer mu.Unlock()

	if gotReqID != wantReqID {
		t.Errorf("request ID = %q, want %q", gotReqID, wantReqID)
	}
}

func TestQueue_DLQ(t *testing.T) {
	q := testConnect(t)
	ctx := context.Background()

	// Use tasks.created — the validator requires TaskCreatedPayload structure.
	// Publishing invalid JSON triggers immediate DLQ via validation failure.
	subject := messagequeue.SubjectTaskCreated
	dlqSubject := subject + ".dlq"

	// Subscribe to the main subject so the consumer processes the message.
	// Validation rejects the invalid JSON before the handler is called,
	// but old messages from prior runs may also arrive, so we simply
	// accept and ack everything here.
	mainStop, err := q.Subscribe(ctx, subject, func(_ context.Context, _ string, _ []byte) error {
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe main: %v", err)
	}
	defer mainStop()

	// Subscribe to the DLQ using a raw JetStream consumer so the invalid
	// payload is not run through the validator a second time.
	// DeliverPolicy: New ensures we only see messages published after this point.
	dlqConsumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		FilterSubject: dlqSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("create DLQ consumer: %v", err)
	}

	var (
		dlqData []byte
		dlqDone = make(chan struct{})
		dlqOnce sync.Once
	)
	dlqSub, err := dlqConsumer.Consume(func(msg jetstream.Msg) {
		dlqOnce.Do(func() {
			dlqData = msg.Data()
			close(dlqDone)
		})
		_ = msg.Ack()
	})
	if err != nil {
		t.Fatalf("consume DLQ: %v", err)
	}
	defer dlqSub.Stop()

	// Publish invalid JSON — not valid JSON at all, so Validate() rejects it.
	if err := q.Publish(ctx, subject, []byte("not-json")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-dlqDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for DLQ message")
	}

	if string(dlqData) != "not-json" {
		t.Errorf("DLQ data = %q, want %q", string(dlqData), "not-json")
	}
}

func TestQueue_DLQ_RetryExhaustion(t *testing.T) {
	q := testConnect(t)
	ctx := context.Background()

	// Use a subject under tasks.agent.* — validator accepts any valid JSON.
	subject := uniqueSubject(t)
	dlqSubject := subject + ".dlq"

	// Subscribe to the DLQ using a raw JetStream consumer to avoid the
	// DLQ message being re-validated by Queue.Subscribe.
	// DeliverPolicy: New ensures we only see messages from this test run.
	dlqConsumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		FilterSubject: dlqSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("create DLQ consumer: %v", err)
	}

	var (
		dlqData []byte
		dlqDone = make(chan struct{})
		dlqOnce sync.Once
	)
	dlqSub, err := dlqConsumer.Consume(func(msg jetstream.Msg) {
		dlqOnce.Do(func() {
			dlqData = msg.Data()
			close(dlqDone)
		})
		_ = msg.Ack()
	})
	if err != nil {
		t.Fatalf("consume DLQ: %v", err)
	}
	defer dlqSub.Stop()

	// Subscribe with a handler that always fails.
	mainStop, err := q.Subscribe(ctx, subject, func(_ context.Context, _ string, _ []byte) error {
		return errAlwaysFail
	})
	if err != nil {
		t.Fatalf("Subscribe main: %v", err)
	}
	defer mainStop()

	// Publish directly via the underlying JetStream so we can set the
	// Retry-Count header to maxRetries, simulating an already-exhausted message.
	// The handler fails, retryCount(hdrs) returns 3 (>= maxRetries), so
	// moveToDLQ fires immediately.
	msg := &nats.Msg{
		Subject: subject,
		Data:    []byte(`{"exhausted":true}`),
		Header:  nats.Header{},
	}
	msg.Header.Set(headerRetryCount, "3")

	if _, err := q.js.PublishMsg(ctx, msg); err != nil {
		t.Fatalf("PublishMsg: %v", err)
	}

	select {
	case <-dlqDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for DLQ message after retry exhaustion")
	}

	if string(dlqData) != `{"exhausted":true}` {
		t.Errorf("DLQ data = %q, want %q", string(dlqData), `{"exhausted":true}`)
	}
}

func TestQueue_KeyValue(t *testing.T) {
	q := testConnect(t)

	bucket := "test-kv-" + t.Name()
	ctx := context.Background()
	ttl := 30 * time.Second

	kv, err := q.KeyValue(ctx, bucket, ttl)
	if err != nil {
		t.Fatalf("KeyValue: %v", err)
	}

	// Put a key.
	_, err = kv.Put(ctx, "greeting", []byte("hello"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get the key.
	entry, err := kv.Get(ctx, "greeting")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(entry.Value()) != "hello" {
		t.Errorf("value = %q, want %q", string(entry.Value()), "hello")
	}

	// Update the key.
	_, err = kv.Put(ctx, "greeting", []byte("world"))
	if err != nil {
		t.Fatalf("Put update: %v", err)
	}
	entry, err = kv.Get(ctx, "greeting")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if string(entry.Value()) != "world" {
		t.Errorf("updated value = %q, want %q", string(entry.Value()), "world")
	}

	// Delete the key.
	if err := kv.Delete(ctx, "greeting"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete should fail.
	_, err = kv.Get(ctx, "greeting")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestQueue_IsConnected(t *testing.T) {
	q := testConnect(t)

	if !q.IsConnected() {
		t.Error("IsConnected() = false after Connect, want true")
	}
}

// errAlwaysFail is a sentinel error used by handlers that should always fail.
var errAlwaysFail = errSentinel("handler always fails")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
