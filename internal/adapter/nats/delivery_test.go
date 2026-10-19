package nats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// fakeMsg is a jetstream.Msg that records how it was settled. Methods the
// delivery code must not call are left to the embedded nil interface.
type fakeMsg struct {
	jetstream.Msg

	subject      string
	data         []byte
	headers      nats.Header
	numDelivered uint64
	metadataErr  error

	mu        sync.Mutex
	settled   []string
	progress  int
	nakDelays []time.Duration
}

func (m *fakeMsg) Subject() string      { return m.subject }
func (m *fakeMsg) Data() []byte         { return m.data }
func (m *fakeMsg) Headers() nats.Header { return m.headers }

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	if m.metadataErr != nil {
		return nil, m.metadataErr
	}
	return &jetstream.MsgMetadata{NumDelivered: m.numDelivered}, nil
}

func (m *fakeMsg) record(s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settled = append(m.settled, s)
	return nil
}

func (m *fakeMsg) Ack() error  { return m.record("ack") }
func (m *fakeMsg) Term() error { return m.record("term") }
func (m *fakeMsg) NakWithDelay(d time.Duration) error {
	m.mu.Lock()
	m.nakDelays = append(m.nakDelays, d)
	m.mu.Unlock()
	return m.record("nak")
}

func (m *fakeMsg) InProgress() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress++
	return nil
}

func (m *fakeMsg) settlements() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.settled...)
}

func (m *fakeMsg) progressCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.progress
}

// fakeJS records published messages and answers with ack/err.
type fakeJS struct {
	jetstream.JetStream

	ack       jetstream.PubAck
	err       error
	published []*nats.Msg
}

func (j *fakeJS) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	j.published = append(j.published, msg)
	if j.err != nil {
		return nil, j.err
	}
	ack := j.ack
	return &ack, nil
}

func newTestQueue(js *fakeJS) *Queue {
	return &Queue{js: js, ackWait: defaultAckWait, maxInProgress: defaultMaxInProgress, clock: time.Now}
}

func failingHandler(context.Context, string, []byte) error { return errAlwaysFail }

func assertSettled(t *testing.T, msg *fakeMsg, want ...string) {
	t.Helper()
	got := msg.settlements()
	if len(got) != len(want) {
		t.Fatalf("settled %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("settled %v, want %v", got, want)
		}
	}
}

// TestMoveToDLQ_DropsPublishControlHeaders: the DLQ copy must not carry the
// original Nats-Msg-Id (JetStream would drop it as a duplicate of the original
// within the dedup window) or other publish-control headers; the original ID is
// kept for tracing and the original message's headers are left untouched.
func TestMoveToDLQ_DropsPublishControlHeaders(t *testing.T) {
	js := &fakeJS{ack: jetstream.PubAck{Stream: streamName, Sequence: 7}}
	q := newTestQueue(js)
	msg := &fakeMsg{
		subject: "tasks.agent.x",
		data:    []byte("not-json"),
		headers: nats.Header{
			"Nats-Msg-Id":          []string{"orig-1"},
			"Nats-Expected-Stream": []string{"OTHER"},
			headerRequestID:        []string{"req-1"},
		},
		numDelivered: 1,
	}

	q.handleMessage(context.Background(), msg, failingHandler)

	if len(js.published) != 1 {
		t.Fatalf("published %d DLQ copies, want 1", len(js.published))
	}
	copyHdr := js.published[0].Header
	if js.published[0].Subject != "tasks.agent.x.dlq" {
		t.Errorf("DLQ subject = %q", js.published[0].Subject)
	}
	if v := copyHdr.Get("Nats-Msg-Id"); v != "" {
		t.Errorf("DLQ copy Nats-Msg-Id = %q, want none", v)
	}
	if v := copyHdr.Get("Nats-Expected-Stream"); v != "" {
		t.Errorf("DLQ copy Nats-Expected-Stream = %q, want none", v)
	}
	if v := copyHdr.Get(headerRequestID); v != "req-1" {
		t.Errorf("DLQ copy %s = %q, want req-1", headerRequestID, v)
	}
	if v := copyHdr.Get(headerOriginalMsgID); v != "orig-1" {
		t.Errorf("DLQ copy %s = %q, want orig-1", headerOriginalMsgID, v)
	}
	if v := msg.headers.Get("Nats-Msg-Id"); v != "orig-1" {
		t.Errorf("original Nats-Msg-Id changed to %q", v)
	}
	assertSettled(t, msg, "term")
}

// TestMoveToDLQ_DuplicateAckIsNotACopy: a PubAck with Duplicate=true stored no
// new message, so the original must not be settled as dead-lettered.
func TestMoveToDLQ_DuplicateAckIsNotACopy(t *testing.T) {
	q := newTestQueue(&fakeJS{ack: jetstream.PubAck{Stream: streamName, Duplicate: true}})
	msg := &fakeMsg{subject: "tasks.agent.x", data: []byte(`{"a":1}`), numDelivered: maxDeliver}

	q.handleMessage(context.Background(), msg, failingHandler)

	assertSettled(t, msg, "nak")
}

// TestMoveToDLQ_PublishErrorKeepsMessage: without a DLQ copy the original is NAK'd.
func TestMoveToDLQ_PublishErrorKeepsMessage(t *testing.T) {
	q := newTestQueue(&fakeJS{err: errors.New("no stream")})
	msg := &fakeMsg{subject: "tasks.agent.x", data: []byte(`{"a":1}`), numDelivered: maxDeliver}

	q.handleMessage(context.Background(), msg, failingHandler)

	assertSettled(t, msg, "nak")
}

// TestHandleMessage_RetryAndDeadLetterByDeliveryCount covers the attempt
// boundaries and fails safe when the delivery count cannot be read.
func TestHandleMessage_RetryAndDeadLetterByDeliveryCount(t *testing.T) {
	tests := []struct {
		name         string
		numDelivered uint64
		metadataErr  error
		wantDLQ      bool
		wantSettled  string
	}{
		{"first attempt retries", 1, nil, false, "nak"},
		{"attempt before last retries", maxDeliver - 1, nil, false, "nak"},
		{"last attempt dead-letters", maxDeliver, nil, true, "ack"},
		{"beyond last attempt dead-letters", maxDeliver + 1, nil, true, "ack"},
		{"unknown delivery count dead-letters", 0, errors.New("not a JetStream message"), true, "ack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := &fakeJS{ack: jetstream.PubAck{Stream: streamName, Sequence: 1}}
			q := newTestQueue(js)
			msg := &fakeMsg{subject: "tasks.agent.x", data: []byte(`{"a":1}`), numDelivered: tt.numDelivered, metadataErr: tt.metadataErr}

			q.handleMessage(context.Background(), msg, failingHandler)

			if gotDLQ := len(js.published) == 1; gotDLQ != tt.wantDLQ {
				t.Errorf("dead-lettered = %v, want %v", gotDLQ, tt.wantDLQ)
			}
			assertSettled(t, msg, tt.wantSettled)
		})
	}
}

// fakeClock is real time shifted by an offset the test advances, so a limit of
// minutes is crossed without waiting for it.
type fakeClock struct {
	mu     sync.Mutex
	offset time.Duration
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().Add(c.offset)
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.offset += d
}

// waitForProgress polls until msg has at least n in-progress acks. The
// deadline only bounds a broken implementation; a slow machine just waits longer.
func waitForProgress(t *testing.T, msg *fakeMsg, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for msg.progressCount() < n {
		if time.Now().After(deadline) {
			t.Fatalf("in-progress acks = %d, want at least %d", msg.progressCount(), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestKeepInProgress_FastHandlerSendsNothing: a handler that returns before the
// first interval must not cause any in-progress ack.
func TestKeepInProgress_FastHandlerSendsNothing(t *testing.T) {
	msg := &fakeMsg{}
	stop := keepInProgress(msg, time.Hour, time.Hour, time.Now)
	stop()
	time.Sleep(20 * time.Millisecond)

	if n := msg.progressCount(); n != 0 {
		t.Fatalf("in-progress acks = %d, want 0", n)
	}
}

// TestKeepInProgress_ReportsWhileRunningAndStops: progress is reported every
// interval while the handler runs and never after stop returns.
func TestKeepInProgress_ReportsWhileRunningAndStops(t *testing.T) {
	msg := &fakeMsg{}
	stop := keepInProgress(msg, 5*time.Millisecond, time.Hour, time.Now)
	waitForProgress(t, msg, 3)
	stop()
	n := msg.progressCount()
	time.Sleep(40 * time.Millisecond) // eight intervals

	if after := msg.progressCount(); after != n {
		t.Fatalf("in-progress acks after stop: %d, want %d", after, n)
	}
}

// TestKeepInProgress_StopsAtTheLimit: a hung handler is reported in progress
// only up to the limit, so JetStream redelivers it afterwards. A report that
// read the clock before it was advanced may still complete; nothing follows it.
func TestKeepInProgress_StopsAtTheLimit(t *testing.T) {
	clock := &fakeClock{}
	msg := &fakeMsg{subject: "tasks.agent.x"}
	stop := keepInProgress(msg, 5*time.Millisecond, time.Minute, clock.now)
	defer stop()
	waitForProgress(t, msg, 2)

	clock.advance(time.Minute + time.Millisecond)
	n := msg.progressCount()
	time.Sleep(60 * time.Millisecond) // twelve intervals

	if after := msg.progressCount(); after > n+1 {
		t.Fatalf("in-progress acks continued after the limit: %d -> %d", n, after)
	}
}

// TestInProgressLimit: the in-progress limit covers the longest legitimate
// handler (the HITL approval wait) plus a margin and never drops below the default.
func TestInProgressLimit(t *testing.T) {
	tests := []struct {
		name       string
		maxHandler time.Duration
		want       time.Duration
	}{
		{"unset keeps the default", 0, defaultMaxInProgress},
		{"negative keeps the default", -time.Second, defaultMaxInProgress},
		{"default approval wait keeps the default", 60 * time.Second, defaultMaxInProgress},
		{"wait that just fits keeps the default", defaultMaxInProgress - inProgressMargin, defaultMaxInProgress},
		{"wait one second longer raises the limit", defaultMaxInProgress - inProgressMargin + time.Second, defaultMaxInProgress + time.Second},
		{"30 minute approval wait", 30 * time.Minute, 30*time.Minute + inProgressMargin},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inProgressLimit(tt.maxHandler); got != tt.want {
				t.Fatalf("inProgressLimit(%v) = %v, want %v", tt.maxHandler, got, tt.want)
			}
		})
	}
}

// TestHandleMessage_InProgressLimitFollowsMaxHandlerDuration: with a HITL
// approval timeout longer than the default limit, a handler waiting for the
// approval keeps its message in progress instead of it being redelivered and
// handled twice; a handler hung beyond the raised limit is still released.
func TestHandleMessage_InProgressLimitFollowsMaxHandlerDuration(t *testing.T) {
	tests := []struct {
		name         string
		maxHandler   time.Duration // 0: SetMaxHandlerDuration not called
		elapsed      time.Duration
		wantProgress bool
	}{
		{"default limit releases a handler after ten minutes", 0, defaultMaxInProgress + time.Minute, false},
		{"30 minute approval wait is still in progress after 20 minutes", 30 * time.Minute, 20 * time.Minute, true},
		{"30 minute approval wait releases a handler hung beyond it", 30 * time.Minute, 30*time.Minute + inProgressMargin + time.Minute, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{}
			q := newTestQueue(&fakeJS{})
			q.ackWait = 15 * time.Millisecond // report every 5ms
			q.clock = clock.now
			if tt.maxHandler > 0 {
				q.SetMaxHandlerDuration(tt.maxHandler)
			}
			msg := &fakeMsg{subject: "tasks.agent.x", data: []byte(`{"a":1}`), numDelivered: 1}

			handler := func(context.Context, string, []byte) error {
				waitForProgress(t, msg, 1)
				clock.advance(tt.elapsed)
				n := msg.progressCount()
				if tt.wantProgress {
					waitForProgress(t, msg, n+3)
					return nil
				}
				time.Sleep(60 * time.Millisecond) // twelve intervals
				if after := msg.progressCount(); after > n+1 {
					t.Errorf("in-progress acks continued after the limit: %d -> %d", n, after)
				}
				return nil
			}
			q.handleMessage(context.Background(), msg, handler)

			assertSettled(t, msg, "ack")
		})
	}
}

// TestHandleMessage_RetryAfterDelaysTheRedelivery (S2-G fix 2, 3): a handler
// that cannot handle a message yet (a handoff claimed by a process that may
// have died) asks for its redelivery after a delay; the last delivery is
// still dead-lettered.
func TestHandleMessage_RetryAfterDelaysTheRedelivery(t *testing.T) {
	later := func(context.Context, string, []byte) error {
		return messagequeue.RetryAfter(errors.New("in progress elsewhere"), 7*time.Minute)
	}
	js := &fakeJS{ack: jetstream.PubAck{Stream: streamName, Sequence: 1}}
	q := newTestQueue(js)
	msg := &fakeMsg{subject: "handoff.request", data: []byte(`{"a":1}`), numDelivered: 1}

	q.handleMessage(context.Background(), msg, later)

	assertSettled(t, msg, "nak")
	if len(msg.nakDelays) != 1 || msg.nakDelays[0] != 7*time.Minute {
		t.Fatalf("nak delays = %v, want 7m", msg.nakDelays)
	}

	last := &fakeMsg{subject: "handoff.request", data: []byte(`{"a":1}`), numDelivered: maxDeliver}
	q.handleMessage(context.Background(), last, later)
	if len(js.published) != 1 {
		t.Fatalf("the last delivery was not dead-lettered: %d copies", len(js.published))
	}
}
