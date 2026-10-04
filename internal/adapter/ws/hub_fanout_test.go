package ws

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

const (
	testTenantA = "11111111-1111-1111-1111-111111111111"
	testTenantB = "22222222-2222-2222-2222-222222222222"
)

// newTestHub creates a hub with its own ticket store.
func newTestHub(opts ...Option) *Hub {
	return NewHub("", NewTicketStore(time.Minute), opts...)
}

// startTestHub serves hub.HandleWS on an httptest server.
func startTestHub(t *testing.T, hub *Hub) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	t.Cleanup(srv.Close)
	return srv
}

// dialTenant opens a WebSocket connection with a ticket issued for tenant.
func dialTenant(t *testing.T, hub *Hub, srv *httptest.Server, tenant string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ticket := hub.tickets.Issue("user-"+tenant[:4], tenant)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?ticket=" + ticket
	c, resp, err := websocket.Dial(ctx, url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial tenant %s: %v", tenant, err)
	}
	c.SetReadLimit(-1)
	t.Cleanup(func() { _ = c.CloseNow() })
	return c
}

// waitForConnections blocks until the hub has registered n connections.
func waitForConnections(t *testing.T, hub *Hub, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for hub.ConnectionCount() != n {
		if time.Now().After(deadline) {
			t.Fatalf("hub has %d connections, want %d", hub.ConnectionCount(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// readTimeout bounds how long a test waits for an expected message.
const readTimeout = 2 * time.Second

// readMessage reads one hub message or fails after readTimeout.
func readMessage(t *testing.T, c *websocket.Conn) Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}
	return msg
}

// expectNoMessage asserts that c receives nothing within wait. The read
// context expiry closes c, so call it last for a given connection.
func expectNoMessage(t *testing.T, c *websocket.Conn, wait time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err == nil {
		t.Fatalf("unexpected message: %s", data)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("read failed with %v, want deadline exceeded", err)
	}
}

func TestHubBroadcastEvent_DeliversOnlyToEventTenant(t *testing.T) {
	hub := newTestHub()
	srv := startTestHub(t, hub)
	clientA := dialTenant(t, hub, srv, testTenantA)
	clientB := dialTenant(t, hub, srv, testTenantB)
	waitForConnections(t, hub, 2)

	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), testTenantA), "run.status", map[string]string{"run_id": "run-a"})
	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), testTenantB), "run.status", map[string]string{"run_id": "run-b"})

	gotA := readMessage(t, clientA)
	if !strings.Contains(string(gotA.Payload), "run-a") {
		t.Fatalf("tenant A received %s, want its own event run-a", gotA.Payload)
	}
	gotB := readMessage(t, clientB)
	if !strings.Contains(string(gotB.Payload), "run-b") {
		t.Fatalf("tenant B received %s, want its own event run-b (tenant A event leaked)", gotB.Payload)
	}
	expectNoMessage(t, clientA, 200*time.Millisecond)
	expectNoMessage(t, clientB, 200*time.Millisecond)
}

func TestHubBroadcastEvent_WithoutTenantIsDropped(t *testing.T) {
	hub := newTestHub()
	srv := startTestHub(t, hub)
	clientA := dialTenant(t, hub, srv, testTenantA)
	clientB := dialTenant(t, hub, srv, testTenantB)
	waitForConnections(t, hub, 2)

	hub.BroadcastEvent(context.Background(), "run.status", map[string]string{"run_id": "no-tenant"})
	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), ""), "run.status", map[string]string{"run_id": "empty-tenant"})

	expectNoMessage(t, clientA, 300*time.Millisecond)
	expectNoMessage(t, clientB, 300*time.Millisecond)
}

func TestHubBroadcastGlobal_ReachesEveryTenant(t *testing.T) {
	hub := newTestHub()
	srv := startTestHub(t, hub)
	clientA := dialTenant(t, hub, srv, testTenantA)
	clientB := dialTenant(t, hub, srv, testTenantB)
	waitForConnections(t, hub, 2)

	hub.BroadcastGlobal(context.Background(), "model.health", map[string]string{"model": "m"})

	for name, c := range map[string]*websocket.Conn{"A": clientA, "B": clientB} {
		if msg := readMessage(t, c); msg.Type != "model.health" {
			t.Fatalf("tenant %s got %q, want model.health", name, msg.Type)
		}
	}
}

func TestHubBroadcastEvent_StalledClientDoesNotBlockOthers(t *testing.T) {
	hub := newTestHub(WithSendQueueSize(8), WithWriteTimeout(250*time.Millisecond))
	srv := startTestHub(t, hub)
	stalled := dialTenant(t, hub, srv, testTenantA) // never reads
	_ = stalled
	healthy := dialTenant(t, hub, srv, testTenantA)
	waitForConnections(t, hub, 2)

	ctx := tenantctx.WithTenant(context.Background(), testTenantA)
	payload := map[string]string{"blob": strings.Repeat("x", 256*1024)}
	deadline := time.Now().Add(15 * time.Second)
	for i := 0; hub.ConnectionCount() > 1; i++ {
		if time.Now().After(deadline) {
			t.Fatalf("stalled client was not dropped after %d broadcasts", i)
		}
		done := make(chan struct{})
		go func() {
			hub.BroadcastEvent(ctx, "task.output", payload)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("broadcast %d blocked on the stalled client", i)
		}
		if msg := readMessage(t, healthy); msg.Type != "task.output" {
			t.Fatalf("healthy client got %q, want task.output", msg.Type)
		}
	}

	// The healthy client keeps receiving after the stalled one is gone.
	hub.BroadcastEvent(ctx, "run.status", map[string]string{"run_id": "after-drop"})
	for {
		msg := readMessage(t, healthy)
		if msg.Type == "run.status" {
			break
		}
	}
}

// --- Fake sockets for deterministic delivery tests ---

// recordingSocket accepts every write immediately.
type recordingSocket struct {
	got chan []byte
}

func newRecordingSocket() *recordingSocket {
	return &recordingSocket{got: make(chan []byte, 1024)}
}

func (s *recordingSocket) Write(_ context.Context, _ websocket.MessageType, p []byte) error {
	s.got <- p
	return nil
}

func (s *recordingSocket) CloseNow() error { return nil }

// stalledSocket blocks every write until the write context ends, like a
// client whose TCP window stays closed.
type stalledSocket struct {
	closeOnce sync.Once
	closed    chan struct{}
}

func newStalledSocket() *stalledSocket {
	return &stalledSocket{closed: make(chan struct{})}
}

func (s *stalledSocket) Write(ctx context.Context, _ websocket.MessageType, _ []byte) error {
	<-ctx.Done()
	return ctx.Err()
}

func (s *stalledSocket) CloseNow() error {
	s.closeOnce.Do(func() { close(s.closed) })
	return nil
}

func waitClosed(t *testing.T, s *stalledSocket, timeout time.Duration) {
	t.Helper()
	select {
	case <-s.closed:
	case <-time.After(timeout):
		t.Fatal("stalled client was not closed")
	}
}

func expectDelivered(t *testing.T, s *recordingSocket, want string) {
	t.Helper()
	select {
	case p := <-s.got:
		if !strings.Contains(string(p), want) {
			t.Fatalf("delivered %s, want %s", p, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("message %s not delivered", want)
	}
}

func TestHubDeliver_FullQueueDropsOnlyTheStalledClient(t *testing.T) {
	const queueSize = 4
	hub := newTestHub(WithSendQueueSize(queueSize), WithWriteTimeout(time.Hour))
	stalled := newStalledSocket()
	healthy := newRecordingSocket()
	hub.register(context.Background(), stalled, testTenantA, "stalled")
	hub.register(context.Background(), healthy, testTenantA, "healthy")
	ctx := tenantctx.WithTenant(context.Background(), testTenantA)

	// One message blocks in the stalled writer, queueSize fill its queue and
	// the next one overflows it.
	for i := 0; i < queueSize+2; i++ {
		start := time.Now()
		hub.BroadcastEvent(ctx, "task.output", map[string]int{"seq": i})
		if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
			t.Fatalf("broadcast %d took %s", i, elapsed)
		}
		expectDelivered(t, healthy, `"seq":`)
	}

	waitClosed(t, stalled, 2*time.Second)
	if got := hub.ConnectionCount(); got != 1 {
		t.Fatalf("hub has %d connections, want 1 (only the healthy client)", got)
	}
	hub.BroadcastEvent(ctx, "run.status", map[string]string{"run_id": "after-drop"})
	expectDelivered(t, healthy, "after-drop")
}

func TestHubDeliver_WriteTimeoutDropsTheStalledClient(t *testing.T) {
	hub := newTestHub(WithWriteTimeout(50 * time.Millisecond))
	stalled := newStalledSocket()
	healthy := newRecordingSocket()
	hub.register(context.Background(), stalled, testTenantA, "stalled")
	hub.register(context.Background(), healthy, testTenantA, "healthy")
	ctx := tenantctx.WithTenant(context.Background(), testTenantA)

	hub.BroadcastEvent(ctx, "task.output", map[string]string{"line": "one"})
	expectDelivered(t, healthy, "one")

	waitClosed(t, stalled, 2*time.Second)
	waitForConnections(t, hub, 1)
	hub.BroadcastEvent(ctx, "task.output", map[string]string{"line": "two"})
	expectDelivered(t, healthy, "two")
}

func TestHubBroadcastToTenant_EmptyTenantReachesNobody(t *testing.T) {
	hub := newTestHub()
	sock := newRecordingSocket()
	hub.register(context.Background(), sock, testTenantA, "user")

	hub.BroadcastToTenant("", Message{Type: "run.status", Payload: json.RawMessage(`{}`)})
	hub.BroadcastToTenant(testTenantB, Message{Type: "run.status", Payload: json.RawMessage(`{}`)})

	select {
	case p := <-sock.got:
		t.Fatalf("unexpected delivery: %s", p)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHubRemove_StopsWriter(t *testing.T) {
	hub := newTestHub()
	sock := newRecordingSocket()
	c := hub.register(context.Background(), sock, testTenantA, "user")

	if !hub.remove(c) {
		t.Fatal("remove() = false for a registered connection")
	}
	if hub.remove(c) {
		t.Fatal("second remove() = true, want false")
	}
	select {
	case <-c.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("connection context not cancelled on remove")
	}
	hub.BroadcastEvent(tenantctx.WithTenant(context.Background(), testTenantA), "run.status", map[string]string{})
	select {
	case p := <-sock.got:
		t.Fatalf("removed client received %s", p)
	case <-time.After(100 * time.Millisecond):
	}
}
