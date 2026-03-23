package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// convRunCompleteMockStore extends convMockStore with tracking for stored tool messages.
type convRunCompleteMockStore struct {
	convMockStore
	mu2          sync.Mutex
	toolMessages []conversation.Message
}

func (m *convRunCompleteMockStore) CreateToolMessages(_ context.Context, convID string, msgs []conversation.Message) error {
	m.mu2.Lock()
	defer m.mu2.Unlock()
	for i := range msgs {
		msgs[i].ConversationID = convID
		m.toolMessages = append(m.toolMessages, msgs[i])
	}
	return nil
}

func (m *convRunCompleteMockStore) CreateMessage(_ context.Context, msg *conversation.Message) (*conversation.Message, error) {
	m.mu2.Lock()
	defer m.mu2.Unlock()
	msg.ID = fmt.Sprintf("msg-%d", len(m.messages)+len(m.toolMessages)+1)
	m.messages = append(m.messages, *msg)
	return msg, nil
}

// newConvRunCompleteEnv builds a ConversationService with mocks suitable for
// HandleConversationRunComplete tests.
func newConvRunCompleteEnv() (*service.ConversationService, *convRunCompleteMockStore, *runtimeMockBroadcaster) {
	store := &convRunCompleteMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"},
	}
	bc := &runtimeMockBroadcaster{}
	modes := service.NewModeService()
	svc := service.NewConversationService(store, bc, "gpt-4o", modes)
	return svc, store, bc
}

// makeRunCompletePayload creates a JSON payload for HandleConversationRunComplete.
func makeRunCompletePayload(p *messagequeue.ConversationRunCompletePayload) []byte {
	data, _ := json.Marshal(p) //nolint:errcheck // test helper
	return data
}

// --- TestHandleConversationRunComplete_BasicStorage ---

func TestHandleConversationRunComplete_BasicStorage(t *testing.T) {
	tests := []struct {
		name             string
		payload          messagequeue.ConversationRunCompletePayload
		wantMessages     int
		wantToolMessages int
	}{
		{
			name: "completed with assistant content stores message",
			payload: messagequeue.ConversationRunCompletePayload{
				RunID:            "run-basic-1",
				ConversationID:   "conv-1",
				AssistantContent: "Hello, I helped you!",
				Status:           "completed",
				CostUSD:          0.01,
				TokensIn:         100,
				TokensOut:        50,
				Model:            "gpt-4o",
			},
			wantMessages:     1,
			wantToolMessages: 0,
		},
		{
			name: "completed with tool messages stores them",
			payload: messagequeue.ConversationRunCompletePayload{
				RunID:          "run-basic-2",
				ConversationID: "conv-1",
				Status:         "completed",
				ToolMessages: []messagequeue.ConversationMessagePayload{
					{Role: "assistant", Content: "Let me read the file."},
					{Role: "tool", Content: "file contents here", ToolCallID: "tc-1", Name: "Read"},
				},
			},
			wantMessages:     1, // empty assistant content but status=completed stores message
			wantToolMessages: 2,
		},
		{
			name: "completed with both assistant and tool messages",
			payload: messagequeue.ConversationRunCompletePayload{
				RunID:            "run-basic-3",
				ConversationID:   "conv-1",
				AssistantContent: "Done!",
				Status:           "completed",
				ToolMessages: []messagequeue.ConversationMessagePayload{
					{Role: "tool", Content: "output", ToolCallID: "tc-2", Name: "Bash"},
				},
			},
			wantMessages:     1,
			wantToolMessages: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _ := newConvRunCompleteEnv()
			ctx := context.Background()

			data := makeRunCompletePayload(&tc.payload)
			if err := svc.HandleConversationRunComplete(ctx, "", data); err != nil {
				t.Fatalf("HandleConversationRunComplete: %v", err)
			}

			store.mu2.Lock()
			gotMessages := len(store.messages)
			gotToolMessages := len(store.toolMessages)
			store.mu2.Unlock()

			if gotMessages != tc.wantMessages {
				t.Errorf("expected %d stored messages, got %d", tc.wantMessages, gotMessages)
			}
			if gotToolMessages != tc.wantToolMessages {
				t.Errorf("expected %d stored tool messages, got %d", tc.wantToolMessages, gotToolMessages)
			}
		})
	}
}

// --- TestHandleConversationRunComplete_Idempotency ---

// Note: Application-level idempotency (processedRuns map) was removed.
// Dedup is now handled entirely by NATS JetStream Nats-Msg-Id headers,
// which prevents the same completion message from being delivered twice.
// This also fixes a bug where follow-up messages in the same conversation
// were silently dropped because RunID == ConversationID (not unique per message).

// --- TestHandleConversationRunComplete_FailedStatus ---

func TestHandleConversationRunComplete_FailedStatus(t *testing.T) {
	tests := []struct {
		name           string
		status         string
		wantWSStatus   string
		wantErrorField string
	}{
		{
			name:           "failed status broadcasts failed",
			status:         "failed",
			wantWSStatus:   "failed",
			wantErrorField: "LLM call failed",
		},
		{
			name:           "error status broadcasts failed",
			status:         "error",
			wantWSStatus:   "failed",
			wantErrorField: "timeout",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, bc := newConvRunCompleteEnv()
			ctx := context.Background()

			payload := messagequeue.ConversationRunCompletePayload{
				RunID:          "run-fail-" + tc.status,
				ConversationID: "conv-1",
				Status:         tc.status,
				Error:          tc.wantErrorField,
			}
			_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

			bc.mu.Lock()
			defer bc.mu.Unlock()

			found := false
			for _, ev := range bc.events {
				if ev.EventType != event.AGUIRunFinished {
					continue
				}
				finEv, ok := ev.Data.(event.AGUIRunFinishedEvent)
				if !ok {
					continue
				}
				found = true
				if finEv.Status != tc.wantWSStatus {
					t.Errorf("expected WS status %q, got %q", tc.wantWSStatus, finEv.Status)
				}
				if finEv.Error != tc.wantErrorField {
					t.Errorf("expected WS error %q, got %q", tc.wantErrorField, finEv.Error)
				}
			}
			if !found {
				t.Error("expected AGUIRunFinished broadcast event")
			}
		})
	}
}

// --- TestHandleConversationRunComplete_EmptyContent ---

func TestHandleConversationRunComplete_EmptyContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		status   string
		wantMsgs int
	}{
		{
			name:     "empty content with completed stores message",
			content:  "",
			status:   "completed",
			wantMsgs: 1, // empty content + completed => stores
		},
		{
			name:     "empty content with non-completed skips message",
			content:  "",
			status:   "failed",
			wantMsgs: 0, // empty content + non-completed => skips
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, _ := newConvRunCompleteEnv()
			ctx := context.Background()

			payload := messagequeue.ConversationRunCompletePayload{
				RunID:            "run-empty-" + tc.status,
				ConversationID:   "conv-1",
				AssistantContent: tc.content,
				Status:           tc.status,
			}
			_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

			store.mu2.Lock()
			gotMsgs := len(store.messages)
			store.mu2.Unlock()

			if gotMsgs != tc.wantMsgs {
				t.Errorf("expected %d messages, got %d", tc.wantMsgs, gotMsgs)
			}
		})
	}
}

// --- TestHandleConversationRunComplete_Waiters ---

func TestHandleConversationRunComplete_Waiters(t *testing.T) {
	t.Run("registered waiter receives result", func(t *testing.T) {
		svc, _, _ := newConvRunCompleteEnv()
		ctx := context.Background()

		resultCh := make(chan service.CompletionResult, 1)

		// Start waiting in a goroutine.
		go func() {
			result, err := svc.WaitForCompletion(ctx, "conv-wait-1")
			if err != nil {
				resultCh <- service.CompletionResult{Status: "error", Error: err.Error()}
				return
			}
			resultCh <- result
		}()

		// Give the goroutine time to register.
		time.Sleep(50 * time.Millisecond)

		// Complete the run.
		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-wait-1",
			ConversationID:   "conv-wait-1",
			AssistantContent: "done",
			Status:           "completed",
			CostUSD:          0.05,
		}
		_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))

		select {
		case result := <-resultCh:
			if result.Status != "completed" {
				t.Errorf("expected status 'completed', got %q", result.Status)
			}
			if result.CostUSD != 0.05 {
				t.Errorf("expected cost 0.05, got %f", result.CostUSD)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("waiter did not receive result within 5s")
		}
	})

	t.Run("no waiter does not panic", func(t *testing.T) {
		svc, _, _ := newConvRunCompleteEnv()
		ctx := context.Background()

		// No waiter registered -- should complete without panic.
		payload := messagequeue.ConversationRunCompletePayload{
			RunID:            "run-no-waiter",
			ConversationID:   "conv-no-waiter",
			AssistantContent: "done",
			Status:           "completed",
		}
		err := svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// --- TestHandleConversationRunComplete_Concurrent ---

func TestHandleConversationRunComplete_Concurrent(t *testing.T) {
	svc, _, _ := newConvRunCompleteEnv()
	ctx := context.Background()

	var wg sync.WaitGroup
	const n = 100

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			payload := messagequeue.ConversationRunCompletePayload{
				RunID:            fmt.Sprintf("run-conc-%d", idx),
				ConversationID:   "conv-conc",
				AssistantContent: fmt.Sprintf("result %d", idx),
				Status:           "completed",
			}
			_ = svc.HandleConversationRunComplete(ctx, "", makeRunCompletePayload(&payload))
		}(i)
	}

	wg.Wait()
	// If we get here without a data race (run with -race), the test passes.
}

// --- TestWaitForCompletion_ContextCancelled ---

func TestWaitForCompletion_ContextCancelled(t *testing.T) {
	svc, _, _ := newConvRunCompleteEnv()
	ctx, cancel := context.WithCancel(context.Background())

	resultCh := make(chan error, 1)
	go func() {
		_, err := svc.WaitForCompletion(ctx, "conv-cancel")
		resultCh <- err
	}()

	// Give time for registration.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-resultCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitForCompletion did not return within 5s after context cancel")
	}
}
