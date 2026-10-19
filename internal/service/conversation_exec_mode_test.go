package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/service"
)

type convExecModeEnv struct {
	svc    *service.ConversationService
	store  *convMockStore
	queue  *captureQueue
	bc     *mockBroadcaster
	convID string
}

func newConvExecModeEnv(t *testing.T, projectConfig map[string]string) *convExecModeEnv {
	t.Helper()
	store := &convMockStore{}
	store.projects = []project.Project{
		{ID: "proj-1", Name: "test", WorkspacePath: t.TempDir(), Config: projectConfig},
	}
	q := &captureQueue{}
	bc := &mockBroadcaster{}
	svc := service.NewConversationService(store, bc, "gpt-4o", service.NewModeService())
	svc.SetQueue(q)
	svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})

	conv, err := svc.Create(context.Background(), conversation.CreateRequest{ProjectID: "proj-1", Title: "exec mode"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return &convExecModeEnv{svc: svc, store: store, queue: q, bc: bc, convID: conv.ID}
}

// dispatchers are the two public entry points into the agentic dispatch path.
var agenticDispatchers = []struct {
	name     string
	dispatch func(ctx context.Context, svc *service.ConversationService, convID string) error
}{
	{
		name: "SendMessageAgentic",
		dispatch: func(ctx context.Context, svc *service.ConversationService, convID string) error {
			return svc.SendMessageAgentic(ctx, convID, &conversation.SendMessageRequest{Content: "run the tests"})
		},
	},
	{
		name: "SendMessageAgenticWithMode",
		dispatch: func(ctx context.Context, svc *service.ConversationService, convID string) error {
			return svc.SendMessageAgenticWithMode(ctx, convID, "run the tests", "coder")
		},
	},
}

func TestAgenticConversation_RejectsExecModesWithoutIsolation(t *testing.T) {
	tests := []struct {
		name          string
		projectConfig map[string]string
		wantErr       error
	}{
		{name: "project sandbox", projectConfig: map[string]string{"execution_mode": "sandbox"}, wantErr: run.ErrExecModeUnavailable},
		{name: "project hybrid", projectConfig: map[string]string{"execution_mode": "hybrid"}, wantErr: run.ErrExecModeUnavailable},
		{name: "mixed case fails closed", projectConfig: map[string]string{"execution_mode": "HYBRID"}, wantErr: domain.ErrValidation},
		{name: "whitespace fails closed", projectConfig: map[string]string{"execution_mode": "sandbox\n"}, wantErr: domain.ErrValidation},
		{name: "unknown value fails closed", projectConfig: map[string]string{"execution_mode": "container"}, wantErr: domain.ErrValidation},
	}
	for _, d := range agenticDispatchers {
		for _, tc := range tests {
			t.Run(d.name+"/"+tc.name, func(t *testing.T) {
				env := newConvExecModeEnv(t, tc.projectConfig)
				eventsBefore := env.bc.eventCount()

				err := d.dispatch(context.Background(), env.svc, env.convID)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("dispatch error = %v, want %v", err, tc.wantErr)
				}
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("dispatch error = %v, want it to wrap domain.ErrValidation (HTTP 400)", err)
				}
				if _, data := env.queue.snapshot(); len(data) != 0 {
					t.Fatalf("expected no conversation run start to be published, got %s", data)
				}
				if n := len(env.store.messages); n != 0 {
					t.Fatalf("expected the rejected user message not to be stored, got %d messages", n)
				}
				if n := env.bc.eventCount() - eventsBefore; n != 0 {
					t.Fatalf("expected no run events to be broadcast, got %d", n)
				}
			})
		}
	}
}

func TestAgenticConversation_DispatchesInMountMode(t *testing.T) {
	tests := []struct {
		name          string
		projectConfig map[string]string
	}{
		{name: "no project config"},
		{name: "empty value", projectConfig: map[string]string{"execution_mode": ""}},
		{name: "explicit mount", projectConfig: map[string]string{"execution_mode": "mount"}},
	}
	for _, d := range agenticDispatchers {
		for _, tc := range tests {
			t.Run(d.name+"/"+tc.name, func(t *testing.T) {
				env := newConvExecModeEnv(t, tc.projectConfig)

				if err := d.dispatch(context.Background(), env.svc, env.convID); err != nil {
					t.Fatalf("dispatch: %v", err)
				}
				if _, data := env.queue.snapshot(); len(data) == 0 {
					t.Fatal("expected a conversation run start to be published")
				}
			})
		}
	}
}

// The simple (non-agentic) chat path runs no tools, so the exec mode does not apply.
func TestSimpleConversation_IgnoresExecMode(t *testing.T) {
	env := newConvExecModeEnv(t, map[string]string{"execution_mode": "sandbox"})

	if _, err := env.svc.SendMessage(context.Background(), env.convID, &conversation.SendMessageRequest{Content: "hello"}); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if _, data := env.queue.snapshot(); len(data) == 0 {
		t.Fatal("expected a conversation run start to be published")
	}
}
