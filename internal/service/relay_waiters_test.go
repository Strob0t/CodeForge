package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
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

// recordingRelay records what a replica relays; the waiter takes it.
type recordingRelay struct {
	noopQueue
	mu   sync.Mutex
	sent [][]byte
}

func (r *recordingRelay) Serve(string, func([]byte) []byte) (func(), error) { return func() {}, nil }

func (r *recordingRelay) Request(_ context.Context, _ string, data []byte) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, data)
	return relayTaken, nil
}

func (r *recordingRelay) last(t *testing.T) []byte {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.sent) == 0 {
		t.Fatal("nothing relayed")
	}
	return r.sent[len(r.sent)-1]
}

// S7-F review: a relayed result is one core NATS message and must fit the
// payload limit as the worker's message did. json.Marshal escapes <, > and &
// to six bytes each, so code full of them grew up to sixfold.
func TestSyncWaiter_RelayDoesNotEscapeHTML(t *testing.T) {
	code := strings.Repeat("if a < b && c > d {} ", 2000)
	payload := &messagequeue.RetrievalSearchResultPayload{
		RequestID: "req-1",
		Results:   []messagequeue.RetrievalSearchHitPayload{{Filepath: "a.go", Content: code}},
	}
	relay := &recordingRelay{}
	w := newSyncWaiter[messagequeue.RetrievalSearchResultPayload]("search")
	if !w.deliver(context.Background(), relay, "req-1", payload) {
		t.Fatal("not delivered")
	}
	sent := relay.last(t)
	if escaped := `\u00`; strings.Contains(string(sent), escaped+"3c") || strings.Contains(string(sent), escaped+"26") {
		t.Fatalf("the relayed result escapes HTML characters: %.200s", sent)
	}
	if len(sent) > len(code)+512 {
		t.Fatalf("relayed %d bytes for %d bytes of code", len(sent), len(code))
	}
	var back messagequeue.RetrievalSearchResultPayload
	if err := json.Unmarshal(sent, &back); err != nil || len(back.Results) != 1 || back.Results[0].Content != code {
		t.Fatalf("the relayed result does not decode to the result: %v", err)
	}
}

// The worker's message itself is relayed when the subscriber has it.
func TestSyncWaiter_RelaysTheOriginalMessage(t *testing.T) {
	raw := []byte(`{ "request_id": "req-1", "results": [{"filepath": "a.go", "content": "<&>"}], "extra": 1 }`)
	var payload messagequeue.RetrievalSearchResultPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	relay := &recordingRelay{}
	w := newSyncWaiter[messagequeue.RetrievalSearchResultPayload]("search")
	if !w.deliverMessage(context.Background(), relay, "req-1", &payload, raw) {
		t.Fatal("not delivered")
	}
	if got := relay.last(t); !bytes.Equal(got, raw) {
		t.Fatalf("relayed %s, want the original message", got)
	}
}

// subscribedQueue keeps the handlers its subscribers registered.
type subscribedQueue struct {
	recordingRelay
	handlers map[string]messagequeue.Handler
}

func (q *subscribedQueue) Subscribe(_ context.Context, subject string, h messagequeue.Handler) (func(), error) {
	if q.handlers == nil {
		q.handlers = map[string]messagequeue.Handler{}
	}
	q.handlers[subject] = h
	return func() {}, nil
}

// Every result subscriber of a syncWaiter relays the worker's message as it
// came.
func TestResultSubscribers_RelayTheOriginalMessage(t *testing.T) {
	ctx := context.Background()
	retrieval := func(q *subscribedQueue) error {
		_, err := NewRetrievalService(nil, q, nil, &config.Orchestrator{}, &config.Limits{}).StartSubscribers(ctx)
		return err
	}
	tests := []struct {
		subject string
		start   func(q *subscribedQueue) error
	}{
		{messagequeue.SubjectRetrievalSearchResult, retrieval},
		{messagequeue.SubjectSubAgentSearchResult, retrieval},
		{messagequeue.SubjectGraphSearchResult, func(q *subscribedQueue) error {
			_, err := NewGraphService(nil, q, nil, &config.Orchestrator{}, &config.Limits{}).StartSubscribers(ctx)
			return err
		}},
		{messagequeue.SubjectMemoryRecallResult, func(q *subscribedQueue) error {
			_, err := NewMemoryService(nil, q).StartSubscribers(ctx)
			return err
		}},
		{messagequeue.SubjectContextRerankResult, func(q *subscribedQueue) error {
			svc := NewContextOptimizerService(nil, &config.Orchestrator{}, &config.Limits{})
			svc.SetQueue(q)
			_, err := svc.StartSubscribers(ctx)
			return err
		}},
		{messagequeue.SubjectConversationTestResult, func(q *subscribedQueue) error {
			_, err := NewAutoAgentService(nil, nil, q, nil).StartTestResultSubscriber(ctx)
			return err
		}},
	}
	raw := []byte(`{ "request_id": "req-1", "query": "<&>" }`)
	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			q := &subscribedQueue{}
			if err := tt.start(q); err != nil {
				t.Fatal(err)
			}
			handle := q.handlers[tt.subject]
			if handle == nil {
				t.Fatalf("no subscriber for %s", tt.subject)
			}
			if err := handle(ctx, tt.subject, raw); err != nil {
				t.Fatal(err)
			}
			if got := q.last(t); !bytes.Equal(got, raw) {
				t.Fatalf("relayed %s, want the original message", got)
			}
		})
	}

	// The backend health result arrives as bytes as well.
	q := &subscribedQueue{}
	if err := NewBackendHealthService(q).HandleHealthResult(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if got := q.last(t); !bytes.Equal(got, raw) {
		t.Fatalf("backend health: relayed %s, want the original message", got)
	}
}
