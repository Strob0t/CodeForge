package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain/agent"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/domain/task"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// The worker's heartbeat interval is the Go Core's runtime.heartbeat_interval
// (S2-F review, F11). The worker beat every 30 s whatever was configured,
// while the watchdog judged silence by the configured interval: a
// heartbeat_timeout below 30 s ended every healthy run. Go sends the
// interval with every start; the lost-worker threshold never assumes a
// shorter interval than an older worker's 30 s.

func TestLostWorkerAfter(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Runtime
		want time.Duration
	}{
		{name: "no config", cfg: nil, want: 0},
		{name: "no heartbeat timeout disables the check", cfg: &config.Runtime{HeartbeatInterval: 30 * time.Second}, want: 0},
		{name: "default interval", cfg: &config.Runtime{HeartbeatTimeout: 2 * time.Minute}, want: 3 * time.Minute},
		{name: "longer interval", cfg: &config.Runtime{HeartbeatTimeout: 5 * time.Minute, HeartbeatInterval: time.Minute}, want: 7 * time.Minute},
		{
			name: "a shorter interval keeps an older worker's 30 s",
			cfg:  &config.Runtime{HeartbeatTimeout: 20 * time.Second, HeartbeatInterval: 5 * time.Second},
			want: 80 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := service.LostWorkerAfter(tt.cfg); got != tt.want {
				t.Fatalf("LostWorkerAfter = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRunStart_CarriesTheHeartbeatInterval(t *testing.T) {
	for _, tt := range []struct {
		name     string
		interval time.Duration
		want     int
	}{
		{name: "default", want: 30},
		{name: "configured", interval: 10 * time.Second, want: 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &runtimeMockStore{
				projects: []project.Project{{ID: "proj-1", WorkspacePath: "/tmp/ws"}},
				agents:   []agent.Agent{{ID: "agent-1", ProjectID: "proj-1", Backend: "aider", Status: agent.StatusIdle}},
				tasks:    []task.Task{{ID: "task-1", ProjectID: "proj-1", Prompt: "p", Status: task.StatusPending}},
			}
			queue := &runtimeMockQueue{}
			svc := service.NewRuntimeService(store, queue, &runtimeMockBroadcaster{}, &runtimeMockEventStore{},
				service.NewPolicyService("headless-safe-sandbox", nil), &config.Runtime{HeartbeatInterval: tt.interval})
			if _, err := svc.StartRun(context.Background(), &run.StartRequest{TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1"}); err != nil {
				t.Fatalf("StartRun: %v", err)
			}
			msg, ok := queue.lastMessage(messagequeue.SubjectRunStart)
			if !ok {
				t.Fatal("no runs.start message published")
			}
			var payload messagequeue.RunStartPayload
			if err := json.Unmarshal(msg.Data, &payload); err != nil {
				t.Fatalf("unmarshal runs.start: %v", err)
			}
			if payload.HeartbeatSeconds != tt.want {
				t.Fatalf("heartbeat_seconds = %d, want %d", payload.HeartbeatSeconds, tt.want)
			}
		})
	}
}

func TestConversationRunStart_CarriesTheHeartbeatInterval(t *testing.T) {
	store := &convMockStore{}
	store.projects = []project.Project{{ID: "proj-1", Name: "p", WorkspacePath: t.TempDir()}}
	queue := &runtimeMockQueue{}
	conv := service.NewConversationService(store, &runtimeMockBroadcaster{}, "gpt-4o", service.NewModeService())
	conv.SetQueue(queue)
	conv.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})
	conv.SetRuntimeConfig(&config.Runtime{HeartbeatInterval: 15 * time.Second})
	ctx := context.Background()
	c, err := conv.Create(ctx, conversation.CreateRequest{ProjectID: "proj-1", Title: "t"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := conv.SendMessageAgentic(ctx, c.ID, &conversation.SendMessageRequest{Content: "do it"}); err != nil {
		t.Fatalf("SendMessageAgentic: %v", err)
	}
	msg, ok := queue.lastMessage(messagequeue.SubjectConversationRunStart)
	if !ok {
		t.Fatal("no conversation.run.start published")
	}
	var payload messagequeue.ConversationRunStartPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.HeartbeatSeconds != 15 {
		t.Fatalf("heartbeat_seconds = %d, want 15", payload.HeartbeatSeconds)
	}
}
