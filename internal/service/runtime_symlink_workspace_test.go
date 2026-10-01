package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
	"github.com/Strob0t/CodeForge/internal/service"
)

// A project workspace behind a symlink: Bash writes through the physical
// directory, so a redirection to the real path (or a ../<real name>/ path)
// is the workspace's file and must meet path_deny (S6-E follow-up c).
func TestToolCall_RedirectionInSymlinkedWorkspace(t *testing.T) {
	base := t.TempDir()
	realWS := filepath.Join(base, "real-ws")
	if err := os.Mkdir(realWS, 0o750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link-ws")
	if err := os.Symlink(realWS, link); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		command string
		want    string
	}{
		{"real path", "echo x > " + filepath.Join(resolved, ".env"), "deny"},
		{"physical parent", "echo x > ../" + filepath.Base(resolved) + "/.env", "deny"},
		{"workspace path", "echo x > " + filepath.Join(link, ".env"), "deny"},
		{"other file", "echo x > out.log", "allow"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, store, queue, _ := newRuntimeTestEnvWithPolicy(service.NewPolicyService("trusted-mount-autonomous", nil))
			store.mu.Lock()
			store.projects[0].WorkspacePath = link
			store.runs = append(store.runs, run.Run{
				ID: "run-ln", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
				PolicyProfile: "trusted-mount-autonomous", Status: run.StatusRunning, StartedAt: time.Now(),
			})
			store.mu.Unlock()

			if err := svc.HandleToolCallRequest(context.Background(), &messagequeue.ToolCallRequestPayload{
				RunID: "run-ln", CallID: "call-ln", Tool: "Bash", Command: tt.command,
			}); err != nil {
				t.Fatalf("HandleToolCallRequest: %v", err)
			}
			if resp := toolCallResponse(t, queue, "call-ln"); resp.Decision != tt.want {
				t.Fatalf("%q in %s -> %s (%s), want %s", tt.command, link, resp.Decision, resp.Reason, tt.want)
			}
		})
	}
}
