package nats

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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

	mu       sync.Mutex
	settled  []string
	progress int
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

func (m *fakeMsg) Ack() error                         { return m.record("ack") }
func (m *fakeMsg) Term() error                        { return m.record("term") }
func (m *fakeMsg) NakWithDelay(_ time.Duration) error { return m.record("nak") }

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
	return &Queue{js: js, ackWait: defaultAckWait}
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

// TestKeepInProgress_FastHandlerSendsNothing: a handler that returns before the
// first interval must not cause any in-progress ack.
func TestKeepInProgress_FastHandlerSendsNothing(t *testing.T) {
	msg := &fakeMsg{}
	stop := keepInProgress(msg, 20*time.Millisecond, time.Minute)
	stop()
	time.Sleep(60 * time.Millisecond)

	if n := msg.progressCount(); n != 0 {
		t.Fatalf("in-progress acks = %d, want 0", n)
	}
}

// TestKeepInProgress_ReportsWhileRunningAndStops: progress is reported every
// interval while the handler runs and never after stop returns.
func TestKeepInProgress_ReportsWhileRunningAndStops(t *testing.T) {
	msg := &fakeMsg{}
	stop := keepInProgress(msg, 10*time.Millisecond, time.Minute)
	time.Sleep(55 * time.Millisecond)
	stop()
	n := msg.progressCount()
	time.Sleep(40 * time.Millisecond)

	if n < 3 {
		t.Fatalf("in-progress acks = %d, want at least 3", n)
	}
	if after := msg.progressCount(); after != n {
		t.Fatalf("in-progress acks after stop: %d, want %d", after, n)
	}
}

// TestKeepInProgress_StopsAtTheLimit: a hung handler is reported in progress
// only up to the limit, so JetStream redelivers it afterwards.
func TestKeepInProgress_StopsAtTheLimit(t *testing.T) {
	msg := &fakeMsg{subject: "tasks.agent.x"}
	stop := keepInProgress(msg, 10*time.Millisecond, 35*time.Millisecond)
	defer stop()
	time.Sleep(150 * time.Millisecond)

	n := msg.progressCount()
	if n < 2 || n > 4 {
		t.Fatalf("in-progress acks = %d, want 2..4 before the 35ms limit", n)
	}
	time.Sleep(50 * time.Millisecond)
	if after := msg.progressCount(); after != n {
		t.Fatalf("in-progress acks continued after the limit: %d -> %d", n, after)
	}
}
