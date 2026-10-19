package service

import (
	"context"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/config"
)

// TestAgentDispatch_SendsTheHeartbeatInterval: the worker reports a backend
// task alive at the configured interval (S2-F review, F11), 30 s by default.
func TestAgentDispatch_SendsTheHeartbeatInterval(t *testing.T) {
	for _, tt := range []struct {
		name string
		cfg  *config.Runtime
		want int
	}{
		{name: "no runtime config", cfg: nil, want: 30},
		{name: "configured", cfg: &config.Runtime{HeartbeatInterval: 20 * time.Second}, want: 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			probe := registerExecutionProbe(t)
			svc := NewAgentService(dispatchStore("/data/workspaces/proj-1"), &mockQueue{}, &mockBroadcaster{})
			if tt.cfg != nil {
				svc.SetRuntimeConfig(tt.cfg)
			}
			if err := svc.Dispatch(context.Background(), "agent-1", "task-1"); err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			got := probe.reset()
			if len(got) != 1 || got[0].HeartbeatSeconds != tt.want {
				t.Fatalf("executions = %+v, want one with heartbeat_seconds %d", got, tt.want)
			}
		})
	}
}
