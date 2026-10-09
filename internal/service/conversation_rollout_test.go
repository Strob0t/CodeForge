package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// Multi-rollout turns reset the workspace between rollouts (KI-195): they
// run only while no other run, task or conversation turn works on the
// project; otherwise the turn runs once and its output says why.
func TestSendMessageAgentic_MultiRolloutOnlyWithoutOtherWorkOnTheProject(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(ctx context.Context, s *convMockStore)
		wantRollout int
		wantSkipped bool
	}{
		{name: "idle project", setup: func(context.Context, *convMockStore) {}, wantRollout: 3},
		{name: "work of another project", setup: func(_ context.Context, s *convMockStore) {
			s.runs = append(s.runs, run.Run{ID: "r1", ProjectID: "proj-2", Status: run.StatusRunning})
		}, wantRollout: 3},
		{name: "ended run", setup: func(_ context.Context, s *convMockStore) {
			s.runs = append(s.runs, run.Run{ID: "r1", ProjectID: "proj-1", Status: run.StatusCompleted})
		}, wantRollout: 3},
		{name: "running run", setup: func(_ context.Context, s *convMockStore) {
			s.runs = append(s.runs, run.Run{ID: "r1", ProjectID: "proj-1", Status: run.StatusRunning})
		}, wantRollout: 1, wantSkipped: true},
		{name: "queued task", setup: func(_ context.Context, s *convMockStore) {
			s.tasks = append(s.tasks, task.Task{ID: "t1", ProjectID: "proj-1", Status: task.StatusQueued})
		}, wantRollout: 1, wantSkipped: true},
		{name: "active turn of another conversation", setup: func(ctx context.Context, s *convMockStore) {
			other, err := s.CreateConversation(ctx, &conversation.Conversation{ProjectID: "proj-1", Title: "other"})
			if err != nil {
				t.Fatalf("CreateConversation: %v", err)
			}
			if err := s.BeginConversationTurn(ctx, other.ID, "turn-other"); err != nil {
				t.Fatalf("BeginConversationTurn: %v", err)
			}
		}, wantRollout: 1, wantSkipped: true},
		{name: "store error fails closed", setup: func(_ context.Context, s *convMockStore) {
			s.activeWorkErr = errors.New("db down")
		}, wantRollout: 1, wantSkipped: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := &convMockStore{}
			store.projects = []project.Project{{ID: "proj-1", Name: "test", WorkspacePath: "/tmp/test"}}
			q := &captureQueue{}
			bc := &mockBroadcaster{}
			svc := service.NewConversationService(store, bc, "gpt-4o", service.NewModeService())
			svc.SetQueue(q)
			svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10, ConversationRolloutCount: 3})
			conv, err := svc.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "rollouts"})
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			tc.setup(ctx, store)

			// "prototyper" has autonomy 4: multi-rollout applies.
			if err := svc.SendMessageAgentic(ctx, conv.ID, &conversation.SendMessageRequest{Content: "build it", Mode: "prototyper"}); err != nil {
				t.Fatalf("SendMessageAgentic: %v", err)
			}

			_, data := q.snapshot()
			var payload messagequeue.ConversationRunStartPayload
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if payload.RolloutCount != tc.wantRollout {
				t.Errorf("rollout_count = %d, want %d", payload.RolloutCount, tc.wantRollout)
			}
			skipped := false
			for _, e := range bc.events {
				if msg, ok := e.payload.(event.AGUITextMessageEvent); ok && e.eventType == event.AGUITextMessage &&
					msg.RunID == conv.ID && strings.Contains(msg.Content, "Multi-rollout skipped") {
					skipped = true
				}
			}
			if skipped != tc.wantSkipped {
				t.Errorf("skip notice in the output = %v, want %v (events %+v)", skipped, tc.wantSkipped, bc.events)
			}
		})
	}
}
