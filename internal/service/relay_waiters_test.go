package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/memory"
	"github.com/Strob0t/CodeForge/internal/domain/policy"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/tenantctx"
)

// KI-86: during a blue-green switch two Go Core replicas run. A worker
// result (shared durable consumer) or an HTTP decision can reach the replica
// that does not wait for it; it is relayed to the one that does.

// relayBus connects the queues of the replicas of a test, like core NATS.
type relayBus struct {
	mu      sync.Mutex
	servers map[string]func([]byte) []byte
	fail    error // every request fails with it
}

func newRelayBus() *relayBus { return &relayBus{servers: map[string]func([]byte) []byte{}} }

// relayQueue is one replica's queue: it records what it publishes and relays
// over the bus.
type relayQueue struct {
	noopQueue
	bus       *relayBus
	published chan publishedRelayMsg
}

type publishedRelayMsg struct {
	subject string
	data    []byte
}

func (b *relayBus) replica() *relayQueue {
	return &relayQueue{bus: b, published: make(chan publishedRelayMsg, 16)}
}

func (q *relayQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.published <- publishedRelayMsg{subject: subject, data: data}
	return nil
}

func (q *relayQueue) Serve(key string, handle func([]byte) []byte) (func(), error) {
	q.bus.mu.Lock()
	defer q.bus.mu.Unlock()
	q.bus.servers[key] = handle
	return func() {
		q.bus.mu.Lock()
		defer q.bus.mu.Unlock()
		delete(q.bus.servers, key)
	}, nil
}

func (q *relayQueue) Request(_ context.Context, key string, data []byte) ([]byte, error) {
	q.bus.mu.Lock()
	handle, fail := q.bus.servers[key], q.bus.fail
	q.bus.mu.Unlock()
	if fail != nil {
		return nil, fail
	}
	if handle == nil {
		return nil, nil
	}
	return handle(data), nil
}

func (q *relayQueue) nextPublished(t *testing.T, subject string) []byte {
	t.Helper()
	for {
		select {
		case msg := <-q.published:
			if msg.subject == subject {
				return msg.data
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("nothing published on %s", subject)
		}
	}
}

func TestSyncWaiter_ResultReachesTheWaitingReplica(t *testing.T) {
	ctx := context.Background()
	bus := newRelayBus()
	waiting, other := newSyncWaiter[memory.RecallResult]("memory-recall"), newSyncWaiter[memory.RecallResult]("memory-recall")
	relayA, relayB := bus.replica(), bus.replica()

	ch := waiting.register("req-1", relayA)
	if !other.deliver(ctx, relayB, "req-1", &memory.RecallResult{RequestID: "req-1", ProjectID: "p"}) {
		t.Fatal("the other replica did not hand the result over")
	}
	select {
	case got := <-ch:
		if got.ProjectID != "p" {
			t.Fatalf("result = %+v", got)
		}
	default:
		t.Fatal("the waiter got nothing")
	}
	// A redelivery finds no waiter: the result was taken.
	if other.deliver(ctx, relayB, "req-1", &memory.RecallResult{RequestID: "req-1"}) {
		t.Fatal("a redelivered result was handed over again")
	}
	waiting.unregister("req-1")
	if len(bus.servers) != 0 {
		t.Fatalf("unregister left relay keys: %v", bus.servers)
	}
	if other.deliver(ctx, relayB, "req-1", &memory.RecallResult{RequestID: "req-1"}) {
		t.Fatal("a result without waiter was handed over")
	}
}

func TestSyncWaiter_LocalWaiterAndNoRelay(t *testing.T) {
	ctx := context.Background()
	w := newSyncWaiter[memory.RecallResult]("memory-recall")
	ch := w.register("req-1", nil)
	if !w.deliver(ctx, nil, "req-1", &memory.RecallResult{RequestID: "req-1"}) || len(ch) != 1 {
		t.Fatal("the local waiter did not get its result")
	}
	if w.deliver(ctx, nil, "req-2", &memory.RecallResult{RequestID: "req-2"}) {
		t.Fatal("a result without waiter was handed over")
	}
}

func TestSyncWaiter_RelayFailureIsNotAHandover(t *testing.T) {
	bus := newRelayBus()
	bus.fail = errors.New("nats: connection closed")
	w := newSyncWaiter[memory.RecallResult]("memory-recall")
	if w.deliver(context.Background(), bus.replica(), "req-1", &memory.RecallResult{RequestID: "req-1"}) {
		t.Fatal("deliver reported a handover although the relay failed")
	}
}

func TestMemoryRecall_ResultOnTheOtherReplica(t *testing.T) {
	bus := newRelayBus()
	qA, qB := bus.replica(), bus.replica()
	a, b := NewMemoryService(nil, qA), NewMemoryService(nil, qB)
	ctx := context.Background()

	type outcome struct {
		res *memory.RecallResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := a.RecallSync(ctx, &memory.RecallRequest{ProjectID: "proj-1", Query: "q"})
		done <- outcome{res, err}
	}()
	var req memory.RecallRequest
	if err := json.Unmarshal(qA.nextPublished(t, messagequeue.SubjectMemoryRecall), &req); err != nil {
		t.Fatal(err)
	}
	b.HandleRecallResult(ctx, &memory.RecallResult{RequestID: req.RequestID, ProjectID: "proj-1"})
	select {
	case got := <-done:
		if got.err != nil || got.res.ProjectID != "proj-1" {
			t.Fatalf("Recall = %+v, %v", got.res, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Recall still waits: the result taken by the other replica was lost")
	}
}

func TestWorkspaceTest_ResultOnTheOtherReplica(t *testing.T) {
	bus := newRelayBus()
	qA, qB := bus.replica(), bus.replica()
	a, b := NewAutoAgentService(nil, nil, qA, nil), NewAutoAgentService(nil, nil, qB, nil)
	a.testTimeout, a.testWaitMargin = time.Second, time.Second
	ctx := context.Background()

	type outcome struct {
		res *messagequeue.WorkspaceTestResultPayload
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := a.requestWorkspaceTest(ctx, &messagequeue.WorkspaceTestRequestPayload{RequestID: "req-1", TestFile: "test_x.py"})
		done <- outcome{res, err}
	}()
	qA.nextPublished(t, messagequeue.SubjectConversationTestRequest)
	res := &messagequeue.WorkspaceTestResultPayload{RequestID: "req-1", Passed: passedPtr(true), Output: "1 passed"}
	if err := b.HandleWorkspaceTestResult(ctx, res); err != nil {
		t.Fatalf("HandleWorkspaceTestResult: %v", err)
	}
	if err := b.HandleWorkspaceTestResult(ctx, res); err != nil { // delivered at least once
		t.Fatalf("HandleWorkspaceTestResult (redelivery): %v", err)
	}
	got := <-done
	if got.err != nil || got.res.Output != "1 passed" {
		t.Fatalf("requestWorkspaceTest = %+v, %v; want the result the other replica took", got.res, got.err)
	}
}

// The HITL approval waits in the replica that handles the tool call; the
// approval page and the decision may reach the other one.
func TestApproval_DecidedOnTheOtherReplica(t *testing.T) {
	bus := newRelayBus()
	waiting, _ := newHITLTestService(30)
	other, _ := newHITLTestService(30)
	waiting.queue, other.queue = bus.replica(), bus.replica()
	owner := tenantctx.WithTenant(context.Background(), approvalTenantA)
	stranger := tenantctx.WithTenant(context.Background(), approvalTenantB)

	decided := waitPending(t, waiting, approvalTenantA, "run-1", "call-1")

	req, err := other.PendingApproval(owner, "run-1", "call-1")
	if err != nil || req.Tool != "Bash" || req.Command != "make" {
		t.Fatalf("PendingApproval on the other replica = %+v, %v; want the waiting call", req, err)
	}
	if _, err := other.PendingApproval(stranger, "run-1", "call-1"); err == nil {
		t.Fatal("another tenant saw the pending approval through the relay")
	}
	if other.ResolveApproval(stranger, "run-1", "call-1", "allow") {
		t.Fatal("another tenant resolved the approval through the relay")
	}
	if other.ResolveApproval(owner, "run-1", "call-1", "maybe") {
		t.Fatal("an invalid decision was relayed")
	}
	if !other.ResolveApproval(owner, "run-1", "call-1", "allow") {
		t.Fatal("the decision did not reach the waiting replica")
	}
	select {
	case d := <-decided:
		if d != policy.DecisionAllow {
			t.Fatalf("decision = %q, want allow", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the wait did not end")
	}
	if other.ResolveApproval(owner, "run-1", "call-1", "deny") {
		t.Fatal("a decided approval was resolved again")
	}
	if _, err := other.PendingApproval(owner, "run-1", "call-1"); err == nil {
		t.Fatal("a decided approval is still pending")
	}
}

func TestApproval_RelayFailureIsNotFound(t *testing.T) {
	bus := newRelayBus()
	bus.fail = errors.New("nats: timeout")
	svc, _ := newHITLTestService(30)
	svc.queue = bus.replica()
	owner := tenantctx.WithTenant(context.Background(), approvalTenantA)
	if svc.ResolveApproval(owner, "run-1", "call-1", "allow") {
		t.Fatal("resolved through a failing relay")
	}
	if _, err := svc.PendingApproval(owner, "run-1", "call-1"); err == nil {
		t.Fatal("found through a failing relay")
	}
}

// The auto-agent waits for its conversation run in the replica that runs
// it; the run's completion may be taken by the other one.
func TestCompletionWaiter_CompletionOnTheOtherReplica(t *testing.T) {
	bus := newRelayBus()
	a := &ConversationService{queue: bus.replica(), completionWaiters: map[string]chan CompletionResult{}}
	b := &ConversationService{queue: bus.replica(), completionWaiters: map[string]chan CompletionResult{}}

	w, err := a.ExpectCompletion("conv-1")
	if err != nil {
		t.Fatal(err)
	}
	b.notifyCompletionWaiter(context.Background(), "conv-1", CompletionResult{Status: "completed", CostUSD: 0.5})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got, err := w.Wait(ctx)
	if err != nil || got.Status != "completed" || got.CostUSD != 0.5 {
		t.Fatalf("Wait = %+v, %v; want the completion the other replica took", got, err)
	}
	w.Close()
	if len(bus.servers) != 0 {
		t.Fatalf("Close left relay keys: %v", bus.servers)
	}
}

// The relay keys of different waits never meet: an approval key is not a
// completion key, a request ID of one service is not another's.
func TestRelayKeys_AreDistinct(t *testing.T) {
	keys := []string{
		approvalRelayKey(approvalKey(approvalTenantA, "x", "y")),
		completionRelayKey("x"),
		newSyncWaiter[memory.RecallResult]("memory-recall").relayKey("x"),
		newSyncWaiter[memory.RecallResult]("search").relayKey("x"),
	}
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Fatalf("relay key %q used twice", k)
		}
		seen[k] = true
	}
}
