package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Strob0t/CodeForge/internal/config"
	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/conversation"
	"github.com/Strob0t/CodeForge/internal/domain/project"
	"github.com/Strob0t/CodeForge/internal/service"
)

// KI-193: an agentic conversation turn on a project without a workspace is
// refused (HTTP 400). Before, an explicit {"agentic": true} skipped the
// workspace check and the worker ran its file tools in its own directory.
func TestAgenticConversation_RejectsProjectWithoutWorkspace(t *testing.T) {
	for _, workspace := range []string{"", "   "} {
		for _, d := range agenticDispatchers {
			t.Run(d.name+"/"+workspace, func(t *testing.T) {
				store := &convMockStore{}
				store.projects = []project.Project{{ID: "proj-1", Name: "test", WorkspacePath: workspace}}
				q := &captureQueue{}
				bc := &mockBroadcaster{}
				svc := service.NewConversationService(store, bc, "gpt-4o", service.NewModeService())
				svc.SetQueue(q)
				svc.SetAgentConfig(&config.Agent{MaxLoopIterations: 10})
				conv, err := svc.Create(context.Background(), conversation.CreateRequest{ProjectID: "proj-1", Title: "no ws"})
				if err != nil {
					t.Fatalf("Create: %v", err)
				}

				// The explicit agentic flag still reaches the agentic path,
				// which refuses the turn instead of dropping the flag.
				agentic := true
				if !svc.IsAgentic(context.Background(), conv.ID, &conversation.SendMessageRequest{Content: "x", Agentic: &agentic}) {
					t.Fatal("an explicit agentic request must not fall back to plain chat silently")
				}

				err = d.dispatch(context.Background(), svc, conv.ID)
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("dispatch error = %v, want domain.ErrValidation (HTTP 400)", err)
				}
				if _, data := q.snapshot(); len(data) != 0 {
					t.Fatalf("expected no conversation run start to be published, got %s", data)
				}
				if n := len(store.messages); n != 0 {
					t.Fatalf("expected the rejected user message not to be stored, got %d messages", n)
				}
			})
		}
	}
}
