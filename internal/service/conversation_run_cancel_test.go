package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Conversation runs reuse the conversation ID as run ID. A stop marks the
// conversation run cancelled so that tool calls of the stopped run are
// rejected; the next run of the conversation must clear the mark (KI-24).

// convStopEnv wires a ConversationService and a RuntimeService to one store,
// as main.go does.
type convStopEnv struct {
	conv      *service.ConversationService
	runtime   *service.RuntimeService
	responses *runtimeMockQueue // tool call responses of the runtime
	starts    *runtimeMockQueue // run starts of the conversation service (nil with a custom queue)
	convID    string
	store     *convMockStore
	hub       *runtimeMockBroadcaster // broadcasts of the conversation service
}

// newConvStopEnv builds the environment; convQueue is the queue the
// conversation service publishes run starts to (nil = a working queue).
func newConvStopEnv(t *testing.T, projectConfig map[string]string, convQueue messagequeue.Queue) *convStopEnv {
	t.Helper()
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "stop", WorkspacePath: t.TempDir(), Config: projectConfig},
	}
	responses := &runtimeMockQueue{}
	rt := service.NewRuntimeService(store, responses, &runtimeMockBroadcaster{}, &runtimeMockEventStore{},
		service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{})

	var starts *runtimeMockQueue
	if convQueue == nil {
		starts = &runtimeMockQueue{}
		convQueue = starts
	}
	hub := &runtimeMockBroadcaster{}
	conv := service.NewConversationService(store, hub, "gpt-4o", service.NewModeService())
	conv.SetQueue(convQueue)
	conv.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})
	conv.SetRunTracker(rt)

	c, err := conv.Create(context.Background(), conversation.CreateRequest{ProjectID: "proj-1", Title: "stop"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return &convStopEnv{conv: conv, runtime: rt, responses: responses, starts: starts, convID: c.ID, store: store, hub: hub}
}

// lastTurn returns the turn ID of the latest run start of the conversation.
func (e *convStopEnv) lastTurn(t *testing.T) string {
	t.Helper()
	msg, ok := e.starts.lastMessage(messagequeue.SubjectConversationRunStart)
	if !ok {
		t.Fatal("no conversation run start published")
	}
	var start messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(msg.Data, &start); err != nil {
		t.Fatalf("unmarshal run start: %v", err)
	}
	return start.TurnID
}

// toolCall sends a Read tool call for the conversation's run, as a worker
// that does not report the run's turn, and returns the runtime's response.
func (e *convStopEnv) toolCall(t *testing.T, callID string) messagequeue.ToolCallResponsePayload {
	t.Helper()
	return e.toolCallInTurn(t, callID, "")
}

// toolCallInTurn sends a Read tool call of the run started with turnID.
func (e *convStopEnv) toolCallInTurn(t *testing.T, callID, turnID string) messagequeue.ToolCallResponsePayload {
	t.Helper()
	req := messagequeue.ToolCallRequestPayload{RunID: e.convID, CallID: callID, Tool: "Read", Path: "main.go", TurnID: turnID}
	if err := e.runtime.HandleToolCallRequest(context.Background(), &req); err != nil {
		t.Fatalf("HandleToolCallRequest(%s): %v", callID, err)
	}
	e.responses.mu.Lock()
	defer e.responses.mu.Unlock()
	for i := len(e.responses.messages) - 1; i >= 0; i-- {
		msg := e.responses.messages[i]
		if msg.Subject != messagequeue.SubjectRunToolCallResponse {
			continue
		}
		var resp messagequeue.ToolCallResponsePayload
		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			t.Fatalf("unmarshal tool call response: %v", err)
		}
		if resp.CallID == callID {
			return resp
		}
	}
	t.Fatalf("no tool call response for %s", callID)
	return messagequeue.ToolCallResponsePayload{}
}

// conversationRunStarters are the entry points that start a run of an
// existing conversation.
var conversationRunStarters = []struct {
	name    string
	agentic bool
	start   func(ctx context.Context, svc *service.ConversationService, convID string) error
}{
	{
		name:    "SendMessageAgentic",
		agentic: true,
		start: func(ctx context.Context, svc *service.ConversationService, convID string) error {
			return svc.SendMessageAgentic(ctx, convID, &conversation.SendMessageRequest{Content: "next task"})
		},
	},
	{
		name:    "SendMessageAgenticWithMode",
		agentic: true,
		start: func(ctx context.Context, svc *service.ConversationService, convID string) error {
			return svc.SendMessageAgenticWithMode(ctx, convID, "next task", "coder")
		},
	},
	{
		name: "SendMessage",
		start: func(ctx context.Context, svc *service.ConversationService, convID string) error {
			_, err := svc.SendMessage(ctx, convID, &conversation.SendMessageRequest{Content: "next question"})
			return err
		},
	},
}

func TestConversationStop_NextRunAllowsToolCallsAgain(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			env := newConvStopEnv(t, nil, nil)

			if resp := env.toolCall(t, "call-before-stop"); resp.Decision != "allow" {
				t.Fatalf("before the stop: decision %q (%s), want allow", resp.Decision, resp.Reason)
			}

			env.runtime.MarkConversationRunCancelled(env.convID)
			resp := env.toolCall(t, "call-of-stopped-run")
			if resp.Decision != "deny" || resp.Reason != "conversation run cancelled" {
				t.Fatalf("tool call of the stopped run: decision %q (%s), want deny (conversation run cancelled)", resp.Decision, resp.Reason)
			}

			if err := starter.start(context.Background(), env.conv, env.convID); err != nil {
				t.Fatalf("start next run: %v", err)
			}
			if resp := env.toolCall(t, "call-of-next-run"); resp.Decision != "allow" {
				t.Fatalf("tool call of the next run: decision %q (%s), want allow", resp.Decision, resp.Reason)
			}
		})
	}
}

// failingQueue rejects every publish.
type failingQueue struct{ runtimeMockQueue }

func (q *failingQueue) Publish(context.Context, string, []byte) error {
	return errors.New("nats unavailable")
}

func (q *failingQueue) PublishWithDedup(context.Context, string, []byte, string) error {
	return errors.New("nats unavailable")
}

// TestConversationStop_NoNextRunKeepsToolCallsRejected: when the next run does
// not start, the stopped run's tool calls stay rejected.
func TestConversationStop_NoNextRunKeepsToolCallsRejected(t *testing.T) {
	tests := []struct {
		name          string
		projectConfig map[string]string
		convQueue     messagequeue.Queue
		agenticOnly   bool
	}{
		{name: "run start rejected before dispatch", projectConfig: map[string]string{"execution_mode": "sandbox"}, agenticOnly: true},
		{name: "run start publish fails", convQueue: &failingQueue{}},
	}
	for _, tc := range tests {
		for _, starter := range conversationRunStarters {
			if tc.agenticOnly && !starter.agentic {
				continue
			}
			t.Run(tc.name+"/"+starter.name, func(t *testing.T) {
				env := newConvStopEnv(t, tc.projectConfig, tc.convQueue)
				env.runtime.MarkConversationRunCancelled(env.convID)

				if err := starter.start(context.Background(), env.conv, env.convID); err == nil {
					t.Fatal("expected the next run not to start")
				}
				resp := env.toolCall(t, "call-of-stopped-run")
				if resp.Decision != "deny" || resp.Reason != "conversation run cancelled" {
					t.Fatalf("decision %q (%s), want deny (conversation run cancelled)", resp.Decision, resp.Reason)
				}
			})
		}
	}
}

// TestConversationStop_ConcurrentStopsStartsAndToolCalls runs stops, run
// starts and tool calls of one conversation concurrently (race detector), then
// checks that the last stop and the last start still decide.
func TestConversationStop_ConcurrentStopsStartsAndToolCalls(t *testing.T) {
	env := newConvStopEnv(t, nil, nil)
	const workers = 8
	const rounds = 25

	var wg sync.WaitGroup
	errs := make(chan error, workers*rounds)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				switch (w + i) % 3 {
				case 0:
					env.runtime.MarkConversationRunCancelled(env.convID)
				case 1:
					// A run start; refused while another run is active.
					turn := fmt.Sprintf("turn-%d-%d", w, i)
					if env.runtime.BeginConversationRun(context.Background(), env.convID, turn) == nil {
						env.runtime.ConversationRunDispatched(env.convID, turn)
					}
				default:
					req := messagequeue.ToolCallRequestPayload{
						RunID: env.convID, CallID: fmt.Sprintf("call-%d-%d", w, i), Tool: "Read", Path: "main.go",
					}
					if err := env.runtime.HandleToolCallRequest(context.Background(), &req); err != nil {
						errs <- err
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("HandleToolCallRequest: %v", err)
	}

	env.runtime.MarkConversationRunCancelled(env.convID)
	if resp := env.toolCall(t, "call-after-last-stop"); resp.Decision != "deny" {
		t.Fatalf("after the last stop: decision %q, want deny", resp.Decision)
	}
	if err := env.runtime.BeginConversationRun(context.Background(), env.convID, "turn-last"); err != nil {
		t.Fatalf("last start: %v", err)
	}
	env.runtime.ConversationRunDispatched(env.convID, "turn-last")
	if resp := env.toolCall(t, "call-after-last-start"); resp.Decision != "allow" {
		t.Fatalf("after the last start: decision %q (%s), want allow", resp.Decision, resp.Reason)
	}
}

// TestConversationStop_LateCallsOfTheStoppedRunStayDenied: conversation runs
// reuse the conversation ID as run ID; the turn ID of each run start tells
// the runs apart, so a call the stopped run makes after the next run started
// is still denied, while the next run's calls are allowed (review finding 11).
func TestConversationStop_LateCallsOfTheStoppedRunStayDenied(t *testing.T) {
	for _, starter := range conversationRunStarters {
		t.Run(starter.name, func(t *testing.T) {
			env := newConvStopEnv(t, nil, nil)
			ctx := context.Background()

			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("start first run: %v", err)
			}
			first := env.lastTurn(t)
			if first == "" {
				t.Fatal("the run start carries no turn ID")
			}
			if resp := env.toolCallInTurn(t, "call-first", first); resp.Decision != "allow" {
				t.Fatalf("call of the running run: %s (%s), want allow", resp.Decision, resp.Reason)
			}

			env.runtime.MarkConversationRunCancelled(env.convID)
			if err := starter.start(ctx, env.conv, env.convID); err != nil {
				t.Fatalf("start next run: %v", err)
			}
			next := env.lastTurn(t)
			if next == "" || next == first {
				t.Fatalf("next run turn = %q, want a new turn (first %q)", next, first)
			}

			if resp := env.toolCallInTurn(t, "call-late", first); resp.Decision != "deny" || resp.Reason != "conversation run ended" {
				t.Errorf("late call of the stopped run: %s (%s), want deny (conversation run ended)", resp.Decision, resp.Reason)
			}
			if resp := env.toolCallInTurn(t, "call-next", next); resp.Decision != "allow" {
				t.Errorf("call of the next run: %s (%s), want allow", resp.Decision, resp.Reason)
			}
			// A worker that does not report turns keeps the previous behaviour.
			if resp := env.toolCall(t, "call-no-turn"); resp.Decision != "allow" {
				t.Errorf("call without turn: %s (%s), want allow", resp.Decision, resp.Reason)
			}
		})
	}
}
