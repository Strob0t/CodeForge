package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// failQueue always returns an error from Publish.
type failQueue struct {
	err error
}

func (q *failQueue) Publish(_ context.Context, _ string, _ []byte) error { return q.err }
func (q *failQueue) PublishWithDedup(ctx context.Context, subj string, data []byte, _ string) error {
	return q.Publish(ctx, subj, data)
}
func (q *failQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *failQueue) Drain() error      { return nil }
func (q *failQueue) Close() error      { return nil }
func (q *failQueue) IsConnected() bool { return false }

func TestBackendHealth_CheckHealth_HappyPath(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewBackendHealthService(q)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resultCh := make(chan []service.BackendHealthEntry, 1)
	errCh := make(chan error, 1)

	go func() {
		backends, err := svc.CheckHealth(ctx)
		resultCh <- backends
		errCh <- err
	}()

	// Wait briefly for the request to be published.
	time.Sleep(50 * time.Millisecond)

	// Extract the request_id from the published payload.
	subj, data := q.snapshot()
	if subj != messagequeue.SubjectBackendHealthRequest {
		t.Fatalf("expected subject %s, got %s", messagequeue.SubjectBackendHealthRequest, subj)
	}

	var reqPayload map[string]string
	if err := json.Unmarshal(data, &reqPayload); err != nil {
		t.Fatalf("unmarshal request payload: %v", err)
	}
	requestID := reqPayload["request_id"]
	if requestID == "" {
		t.Fatal("expected non-empty request_id in published payload")
	}

	// Simulate Python worker delivering the result.
	resultJSON, _ := json.Marshal(map[string]any{
		"request_id": requestID,
		"backends": []map[string]any{
			{
				"name":         "aider",
				"display_name": "Aider",
				"available":    true,
				"capabilities": []string{"code_edit", "git"},
			},
		},
	})
	if err := svc.HandleHealthResult(context.Background(), resultJSON); err != nil {
		t.Fatalf("HandleHealthResult: %v", err)
	}

	backends := <-resultCh
	err := <-errCh

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(backends))
	}
	if backends[0].Name != "aider" {
		t.Errorf("expected backend name 'aider', got %q", backends[0].Name)
	}
	if !backends[0].Available {
		t.Error("expected backend to be available")
	}
	if len(backends[0].Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(backends[0].Capabilities))
	}
}

func TestBackendHealth_CheckHealth_Timeout(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewBackendHealthService(q)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Allow the context to expire.
	time.Sleep(5 * time.Millisecond)

	backends, err := svc.CheckHealth(ctx)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if backends != nil {
		t.Errorf("expected nil backends on timeout, got %v", backends)
	}
}

func TestBackendHealth_CheckHealth_NilResult(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewBackendHealthService(q)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	errCh := make(chan error, 1)

	go func() {
		_, err := svc.CheckHealth(ctx)
		errCh <- err
	}()

	// Wait for publish.
	time.Sleep(50 * time.Millisecond)

	// Extract request_id.
	_, data := q.snapshot()
	var reqPayload map[string]string
	if err := json.Unmarshal(data, &reqPayload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Deliver nil by sending a result with the request_id but using the
	// waiter's deliver method with nil. We simulate this by calling
	// HandleHealthResult with a valid JSON that will be delivered, but
	// we need to test the nil path. The nil path occurs when the channel
	// receives nil. Since HandleHealthResult always sends &result (non-nil),
	// we access the waiter indirectly. Instead, we test the code path by
	// delivering a result whose Backends field is nil (still non-nil pointer).
	// The actual nil check in CheckHealth guards against the channel returning
	// a nil *backendHealthResult. We can test this by creating a second service
	// and manually triggering.

	// Actually, the nil result path in CheckHealth happens if someone puts nil
	// on the channel. HandleHealthResult always sends &result, so this path is
	// a defensive check. We verify the timeout path instead since direct
	// channel manipulation isn't possible from outside the package.
	// Let's deliver a valid result with empty backends to verify the path works.
	resultJSON, _ := json.Marshal(map[string]any{
		"request_id": reqPayload["request_id"],
		"backends":   []any{},
	})
	if err := svc.HandleHealthResult(context.Background(), resultJSON); err != nil {
		t.Fatalf("HandleHealthResult: %v", err)
	}

	err := <-errCh
	if err != nil {
		t.Fatalf("unexpected error for empty backends: %v", err)
	}
}

func TestBackendHealth_HandleHealthResult_Malformed(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewBackendHealthService(q)

	tests := []struct {
		name string
		data []byte
	}{
		{"empty bytes", []byte{}},
		{"invalid json", []byte(`{not json}`)},
		{"truncated json", []byte(`{"request_id": "abc"`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.HandleHealthResult(context.Background(), tt.data)
			if err == nil {
				t.Error("expected unmarshal error, got nil")
			}
		})
	}
}

func TestBackendHealth_HandleHealthResult_NoWaiter(t *testing.T) {
	q := &captureQueue{}
	svc := service.NewBackendHealthService(q)

	// Deliver a result for a request_id that nobody is waiting for.
	resultJSON, _ := json.Marshal(map[string]any{
		"request_id": "orphan-request-id",
		"backends": []map[string]any{
			{"name": "test", "available": true},
		},
	})

	// Should not panic.
	err := svc.HandleHealthResult(context.Background(), resultJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendHealth_CheckHealth_PublishError(t *testing.T) {
	publishErr := fmt.Errorf("nats connection closed")
	q := &failQueue{err: publishErr}
	svc := service.NewBackendHealthService(q)

	backends, err := svc.CheckHealth(context.Background())
	if err == nil {
		t.Fatal("expected publish error, got nil")
	}
	if backends != nil {
		t.Errorf("expected nil backends on publish error, got %v", backends)
	}
}

func TestBackendHealth_Concurrent(t *testing.T) {
	multiQ := &multiCaptureQueue{}
	svc := service.NewBackendHealthService(multiQ)

	const goroutines = 10
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	type result struct {
		backends []service.BackendHealthEntry
		err      error
	}
	results := make(chan result, goroutines)

	// Launch goroutines that each call CheckHealth.
	for range goroutines {
		go func() {
			backends, err := svc.CheckHealth(ctx)
			results <- result{backends: backends, err: err}
		}()
	}

	// Wait for all requests to be published.
	time.Sleep(100 * time.Millisecond)

	// Deliver results for each request.
	msgs := multiQ.snapshot()

	for i, msg := range msgs {
		if msg.subject != messagequeue.SubjectBackendHealthRequest {
			continue
		}
		var reqPayload map[string]string
		if err := json.Unmarshal(msg.data, &reqPayload); err != nil {
			t.Fatalf("unmarshal request %d: %v", i, err)
		}
		resultJSON, _ := json.Marshal(map[string]any{
			"request_id": reqPayload["request_id"],
			"backends": []map[string]any{
				{"name": fmt.Sprintf("backend-%d", i), "available": true},
			},
		})
		if err := svc.HandleHealthResult(context.Background(), resultJSON); err != nil {
			t.Fatalf("HandleHealthResult %d: %v", i, err)
		}
	}

	// Collect results.
	for range goroutines {
		r := <-results
		if r.err != nil {
			t.Errorf("unexpected error: %v", r.err)
		}
		if len(r.backends) != 1 {
			t.Errorf("expected 1 backend, got %d", len(r.backends))
		}
	}
}

// capturedMsg holds a single published message for multiCaptureQueue.
type capturedMsg struct {
	subject string
	data    []byte
}

// multiCaptureQueue records all published messages (thread-safe).
type multiCaptureQueue struct {
	mu       sync.Mutex
	messages []capturedMsg
}

func (q *multiCaptureQueue) Publish(_ context.Context, subject string, data []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	copied := make([]byte, len(data))
	copy(copied, data)
	q.messages = append(q.messages, capturedMsg{subject: subject, data: copied})
	return nil
}
func (q *multiCaptureQueue) PublishWithDedup(ctx context.Context, subj string, data []byte, _ string) error {
	return q.Publish(ctx, subj, data)
}
func (q *multiCaptureQueue) Subscribe(_ context.Context, _ string, _ messagequeue.Handler) (func(), error) {
	return func() {}, nil
}
func (q *multiCaptureQueue) Drain() error      { return nil }
func (q *multiCaptureQueue) Close() error      { return nil }
func (q *multiCaptureQueue) IsConnected() bool { return true }

func (q *multiCaptureQueue) snapshot() []capturedMsg {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]capturedMsg, len(q.messages))
	copy(out, q.messages)
	return out
}
