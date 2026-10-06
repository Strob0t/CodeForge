package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/logger"
)

// testStreamMaxBytes matches the production default, so tests never change
// the limit of a shared CODEFORGE stream.
const testStreamMaxBytes = 10 << 30

// testConnect connects to NATS or skips the test if NATS_URL is not set.
func testConnect(t *testing.T) *Queue {
	t.Helper()

	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("requires NATS_URL")
	}

	q, err := Connect(context.Background(), url, testStreamMaxBytes)
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
// accepts as any valid JSON. Durables no longer expire on inactivity, so the
// durable consumer of the subject is deleted before and after the test: every
// run starts from a fresh durable and the shared server does not accumulate them.
func uniqueSubject(t *testing.T) string {
	t.Helper()
	// Use test name to avoid collisions between parallel tests.
	subject := "tasks.agent.test." + t.Name()
	deleteDurable(t, subject)
	t.Cleanup(func() { deleteDurable(t, subject) })
	return subject
}

// deleteDurable removes the Go durable consumer of subject, if it exists. It
// uses its own plain connection so it can run from t.Cleanup.
func deleteDurable(t *testing.T, subject string) {
	t.Helper()
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Skip("requires NATS_URL")
	}
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("admin jetstream: %v", err)
	}
	name := sanitizeConsumerName("codeforge-go-", subject)
	err = js.DeleteConsumer(context.Background(), streamName, name)
	if err != nil && !errors.Is(err, jetstream.ErrConsumerNotFound) && !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Fatalf("delete durable %s: %v", name, err)
	}
}

// durableInfo returns the server-side state of the Go durable consumer of subject.
func durableInfo(t *testing.T, q *Queue, subject string) *jetstream.ConsumerInfo {
	t.Helper()
	cons, err := q.js.Consumer(context.Background(), streamName, sanitizeConsumerName("codeforge-go-", subject))
	if err != nil {
		t.Fatalf("lookup durable for %s: %v", subject, err)
	}
	info, err := cons.Info(context.Background())
	if err != nil {
		t.Fatalf("durable info for %s: %v", subject, err)
	}
	return info
}

// recorder counts how often each message body was handled.
type recorder struct {
	mu     sync.Mutex
	counts map[string]int
	seen   chan string
}

func newRecorder() *recorder {
	return &recorder{counts: map[string]int{}, seen: make(chan string, 100)}
}

func (r *recorder) handle(_ context.Context, _ string, data []byte) error {
	r.mu.Lock()
	r.counts[string(data)]++
	r.mu.Unlock()
	r.seen <- string(data)
	return nil
}

// waitFor blocks until every body in want was handled at least once.
func (r *recorder) waitFor(t *testing.T, want ...string) {
	t.Helper()
	missing := map[string]bool{}
	for _, w := range want {
		missing[w] = true
	}
	r.mu.Lock()
	for w := range missing {
		if r.counts[w] > 0 {
			delete(missing, w)
		}
	}
	r.mu.Unlock()
	deadline := time.After(10 * time.Second)
	for len(missing) > 0 {
		select {
		case got := <-r.seen:
			delete(missing, got)
		case <-deadline:
			t.Fatalf("timed out waiting for %v", missing)
		}
	}
}

// snapshot returns a copy of the delivery counts.
func (r *recorder) snapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]int, len(r.counts))
	for k, v := range r.counts {
		out[k] = v
	}
	return out
}

// waitForNoPullRequests blocks until the server has dropped every pull request
// on the durable for subject, i.e. it has seen the subscriber go away. Until
// then a message can still be delivered to the stopped subscriber; JetStream
// redelivers it only after AckWait (90 s), as after any process exit.
func waitForNoPullRequests(t *testing.T, q *Queue, subject string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for durableInfo(t, q, subject).NumWaiting > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("durable for %s still has pull requests after its subscriber stopped", subject)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func publishRaw(t *testing.T, q *Queue, subject string, bodies ...string) {
	t.Helper()
	for _, b := range bodies {
		if err := q.Publish(context.Background(), subject, []byte(b)); err != nil {
			t.Fatalf("Publish %s: %v", b, err)
		}
	}
}

// settle gives JetStream time to deliver anything it still intends to deliver.
func settle() { time.Sleep(1500 * time.Millisecond) }

func assertCounts(t *testing.T, got, want map[string]int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("delivered %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("delivered %v, want %v", got, want)
		}
	}
}

// TestQueue_Subscribe_DurableConfig checks the durable a fresh subscription
// creates: it survives inactivity and starts at new messages (KI-18).
func TestQueue_Subscribe_DurableConfig(t *testing.T) {
	q := testConnect(t)
	subject := uniqueSubject(t)

	stop, err := q.Subscribe(context.Background(), subject, newRecorder().handle)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	cfg := durableInfo(t, q, subject).Config
	if cfg.InactiveThreshold != 0 {
		t.Errorf("InactiveThreshold = %v, want 0 (durables must not expire)", cfg.InactiveThreshold)
	}
	if cfg.DeliverPolicy != jetstream.DeliverNewPolicy {
		t.Errorf("DeliverPolicy = %v, want DeliverNew on first creation", cfg.DeliverPolicy)
	}
	if cfg.AckPolicy != jetstream.AckExplicitPolicy {
		t.Errorf("AckPolicy = %v, want explicit", cfg.AckPolicy)
	}
	if cfg.MaxDeliver != maxDeliver {
		t.Errorf("MaxDeliver = %d, want %d", cfg.MaxDeliver, maxDeliver)
	}
	if cfg.AckWait != q.ackWait {
		t.Errorf("AckWait = %v, want %v", cfg.AckWait, q.ackWait)
	}
	if cfg.DeliverGroup != "" {
		t.Errorf("DeliverGroup = %q, want empty (pull consumers are shared by name)", cfg.DeliverGroup)
	}
}

// TestQueue_RestartDoesNotReplayOrLose simulates a Go Core restart: history
// from before the durable existed is not delivered, messages already handled
// are not delivered again, and messages published while the process was down
// are delivered after it re-attaches (KI-18).
func TestQueue_RestartDoesNotReplayOrLose(t *testing.T) {
	subject := uniqueSubject(t)
	publisher := testConnect(t)
	publishRaw(t, publisher, subject, `{"m":"before-first-start"}`)

	first := testConnect(t)
	rec1 := newRecorder()
	stop1, err := first.Subscribe(context.Background(), subject, rec1.handle)
	if err != nil {
		t.Fatalf("Subscribe first: %v", err)
	}
	publishRaw(t, publisher, subject, `{"m":"m1"}`)
	rec1.waitFor(t, `{"m":"m1"}`)
	settle()
	stop1()
	if err := first.Close(); err != nil {
		t.Fatalf("Close first: %v", err)
	}
	waitForNoPullRequests(t, publisher, subject)

	publishRaw(t, publisher, subject, `{"m":"while-down"}`)

	second := testConnect(t)
	rec2 := newRecorder()
	stop2, err := second.Subscribe(context.Background(), subject, rec2.handle)
	if err != nil {
		t.Fatalf("Subscribe second: %v", err)
	}
	defer stop2()
	publishRaw(t, publisher, subject, `{"m":"m2"}`)
	rec2.waitFor(t, `{"m":"while-down"}`, `{"m":"m2"}`)
	settle()

	assertCounts(t, rec1.snapshot(), map[string]int{`{"m":"m1"}`: 1})
	assertCounts(t, rec2.snapshot(), map[string]int{`{"m":"while-down"}`: 1, `{"m":"m2"}`: 1})
}

// TestQueue_RecreatedDurableDoesNotReplay deletes the durable externally (as a
// server-side inactivity expiry or an operator would) and subscribes again:
// the recreated durable must not replay the subject's history (KI-18).
func TestQueue_RecreatedDurableDoesNotReplay(t *testing.T) {
	q := testConnect(t)
	subject := uniqueSubject(t)
	rec := newRecorder()

	stop1, err := q.Subscribe(context.Background(), subject, rec.handle)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	publishRaw(t, q, subject, `{"m":"m1"}`)
	rec.waitFor(t, `{"m":"m1"}`)
	settle()
	stop1()

	deleteDurable(t, subject)

	stop2, err := q.Subscribe(context.Background(), subject, rec.handle)
	if err != nil {
		t.Fatalf("Subscribe after deletion: %v", err)
	}
	defer stop2()
	publishRaw(t, q, subject, `{"m":"m2"}`)
	rec.waitFor(t, `{"m":"m2"}`)
	settle()

	assertCounts(t, rec.snapshot(), map[string]int{`{"m":"m1"}`: 1, `{"m":"m2"}`: 1})
}

// TestQueue_Subscribe_UpgradesLegacyDurable re-attaches to a durable created by
// an earlier release (deliver-all, 5 min inactivity expiry, deliver group).
// The start policy cannot be changed in place and must be kept, the expiry
// must be removed (KI-18).
func TestQueue_Subscribe_UpgradesLegacyDurable(t *testing.T) {
	q := testConnect(t)
	subject := uniqueSubject(t)
	name := sanitizeConsumerName("codeforge-go-", subject)

	_, err := q.js.CreateConsumer(context.Background(), streamName, jetstream.ConsumerConfig{
		Name:              name,
		Durable:           name,
		FilterSubject:     subject,
		AckPolicy:         jetstream.AckExplicitPolicy,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		DeliverGroup:      "codeforge-go",
		AckWait:           90 * time.Second,
		MaxDeliver:        maxDeliver,
		MaxAckPending:     100,
		InactiveThreshold: 5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("create legacy durable: %v", err)
	}

	stop, err := q.Subscribe(context.Background(), subject, newRecorder().handle)
	if err != nil {
		t.Fatalf("Subscribe onto legacy durable: %v", err)
	}
	defer stop()

	cfg := durableInfo(t, q, subject).Config
	if cfg.InactiveThreshold != 0 {
		t.Errorf("InactiveThreshold = %v, want 0 after re-attach", cfg.InactiveThreshold)
	}
	if cfg.DeliverPolicy != jetstream.DeliverAllPolicy {
		t.Errorf("DeliverPolicy = %v, want the existing DeliverAll to be kept", cfg.DeliverPolicy)
	}
}

// TestQueue_DLQMonitorDurableDoesNotExpire checks that the DLQ monitor, whose
// recreation replayed every dead letter of the retention window, is a durable
// without inactivity expiry (KI-18).
func TestQueue_DLQMonitorDurableDoesNotExpire(t *testing.T) {
	q := testConnect(t)
	cons, err := q.js.Consumer(context.Background(), streamName, dlqMonitorName)
	if err != nil {
		t.Fatalf("lookup DLQ monitor: %v", err)
	}
	cfg := cons.CachedInfo().Config
	if cfg.InactiveThreshold != 0 {
		t.Errorf("DLQ monitor InactiveThreshold = %v, want 0", cfg.InactiveThreshold)
	}
}

// TestQueue_TwoConsumersNoDoubleDelivery runs two Queue instances (two Go Core
// processes) on one subject with handlers slower than AckWait. Every message
// must be handled exactly once: the instances share the durable, and the
// in-progress heartbeat stops JetStream from redelivering a message that is
// still being handled (KI-18).
func TestQueue_TwoConsumersNoDoubleDelivery(t *testing.T) {
	subject := uniqueSubject(t)
	const ackWait = 2 * time.Second
	const handlerTime = 5 * time.Second // > 2 x AckWait

	var (
		mu     sync.Mutex
		counts = map[string]int{}
		wg     sync.WaitGroup
	)
	const total = 6
	wg.Add(total)
	slow := func(_ context.Context, _ string, data []byte) error {
		mu.Lock()
		counts[string(data)]++
		first := counts[string(data)] == 1
		mu.Unlock()
		time.Sleep(handlerTime)
		if first {
			wg.Done()
		}
		return nil
	}

	for i := range 2 {
		q := testConnect(t)
		q.ackWait = ackWait
		stop, err := q.Subscribe(context.Background(), subject, slow)
		if err != nil {
			t.Fatalf("Subscribe instance %d: %v", i, err)
		}
		t.Cleanup(stop)
	}

	publisher := testConnect(t)
	for i := range total {
		publishRaw(t, publisher, subject, fmt.Sprintf(`{"n":%d}`, i))
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for all messages")
	}
	// Leave room for a redelivery that would follow an expired AckWait.
	time.Sleep(2 * ackWait)

	mu.Lock()
	defer mu.Unlock()
	if len(counts) != total {
		t.Fatalf("handled %d distinct messages, want %d: %v", len(counts), total, counts)
	}
	for body, n := range counts {
		if n != 1 {
			t.Errorf("message %s handled %d times, want exactly once", body, n)
		}
	}
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

	// Invalid JSON fails validation on every subject, so a test subject is
	// enough and the production durable of a real subject is left alone.
	subject := uniqueSubject(t)
	dlqSubject := subject + ".dlq"

	// Validation rejects the invalid JSON before the handler is called.
	var handled atomic.Int32
	mainStop, err := q.Subscribe(ctx, subject, func(_ context.Context, _ string, _ []byte) error {
		handled.Add(1)
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
	if n := handled.Load(); n != 0 {
		t.Errorf("handler called %d times for an invalid payload, want 0", n)
	}
}

// TestQueue_DLQ_KeepsMessagesPublishedWithMsgID: a message published with a
// Nats-Msg-Id must still reach the DLQ. Copying that header made JetStream
// drop the copy as a duplicate of the original, which was then terminated.
func TestQueue_DLQ_KeepsMessagesPublishedWithMsgID(t *testing.T) {
	q := testConnect(t)
	ctx := context.Background()
	subject := uniqueSubject(t)

	dlqConsumer, err := q.js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		FilterSubject: subject + ".dlq",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatalf("create DLQ consumer: %v", err)
	}
	dlq := make(chan jetstream.Msg, 1)
	dlqSub, err := dlqConsumer.Consume(func(msg jetstream.Msg) {
		_ = msg.Ack()
		select {
		case dlq <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("consume DLQ: %v", err)
	}
	defer dlqSub.Stop()

	stop, err := q.Subscribe(ctx, subject, func(context.Context, string, []byte) error { return nil })
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	msgID := fmt.Sprintf("dedup-%d", time.Now().UnixNano())
	if err := q.PublishWithDedup(ctx, subject, []byte("not-json"), msgID); err != nil {
		t.Fatalf("PublishWithDedup: %v", err)
	}

	select {
	case got := <-dlq:
		if string(got.Data()) != "not-json" {
			t.Errorf("DLQ data = %q", got.Data())
		}
		if v := got.Headers().Get(headerOriginalMsgID); v != msgID {
			t.Errorf("DLQ %s = %q, want %q", headerOriginalMsgID, v, msgID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("message published with a Nats-Msg-Id never reached the DLQ")
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

	// Subscribe with a handler that always fails. Retries are counted by
	// JetStream (NumDelivered); the last allowed delivery goes to the DLQ.
	var attempts atomic.Int32
	mainStop, err := q.Subscribe(ctx, subject, func(_ context.Context, _ string, _ []byte) error {
		attempts.Add(1)
		return errAlwaysFail
	})
	if err != nil {
		t.Fatalf("Subscribe main: %v", err)
	}
	defer mainStop()

	if err := q.Publish(ctx, subject, []byte(`{"exhausted":true}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-dlqDone:
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for DLQ message after retry exhaustion")
	}

	if string(dlqData) != `{"exhausted":true}` {
		t.Errorf("DLQ data = %q, want %q", string(dlqData), `{"exhausted":true}`)
	}
	settle()
	if n := attempts.Load(); n != maxDeliver {
		t.Errorf("handler attempts = %d, want %d (maxDeliver)", n, maxDeliver)
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

func TestReconnectOpts(t *testing.T) {
	opts := reconnectOpts()
	if len(opts) != 5 {
		t.Fatalf("expected 5 reconnect options, got %d", len(opts))
	}

	// Verify MaxReconnects and ReconnectWait are wired by applying the
	// options to a nats.Options struct.
	nopts := nats.GetDefaultOptions()
	for _, o := range opts {
		if err := o(&nopts); err != nil {
			t.Fatalf("applying option: %v", err)
		}
	}

	if nopts.MaxReconnect != -1 {
		t.Errorf("MaxReconnect = %d, want -1 (unlimited)", nopts.MaxReconnect)
	}
	if nopts.ReconnectWait != 2*time.Second {
		t.Errorf("ReconnectWait = %v, want 2s", nopts.ReconnectWait)
	}
	if nopts.DisconnectedErrCB == nil {
		t.Error("DisconnectErrHandler not set")
	}
	if nopts.ReconnectedCB == nil {
		t.Error("ReconnectHandler not set")
	}
	if nopts.AsyncErrorCB == nil {
		t.Error("ErrorHandler not set")
	}
}

// errAlwaysFail is a sentinel error used by handlers that should always fail.
var errAlwaysFail = errSentinel("handler always fails")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
