package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/event"
	"github.com/Strob0t/CodeForge/internal/domain/run"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// KI-126: a failed gate check keeps its output. The worker bounds each output
// to agent.tool_output_max_chars, which the gate request carries, and the
// run.qualitygate.failed event records the output of the checks that failed.

func TestEnterQualityGate_SendsToolOutputMaxChars(t *testing.T) {
	env, _ := newWatchdogEnv(time.Minute)
	env.svc.SetToolOutputMaxChars(1234)
	env.addRun("run-limit", "headless-safe-sandbox", run.StatusRunning, run.DeliverModeNone)

	completeRun(t, env, "run-limit")

	msg, ok := env.queue.lastMessage(messagequeue.SubjectQualityGateRequest)
	if !ok {
		t.Fatal("no gate request")
	}
	var req messagequeue.QualityGateRequestPayload
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		t.Fatal(err)
	}
	if req.ToolOutputMaxChars != 1234 {
		t.Fatalf("tool_output_max_chars = %d, want 1234", req.ToolOutputMaxChars)
	}
}

func TestQualityGate_FailedEventKeepsTheOutputOfFailedChecks(t *testing.T) {
	passed, failed := true, false
	env := newGateOutcomeEnv(defaultGateConfig())
	setStoredRun(env.store.runtimeMockStore, &run.Run{
		ID: "run-gate", TaskID: "task-1", AgentID: "agent-1", ProjectID: "proj-1",
		PolicyProfile: "headless-safe-sandbox", Status: run.StatusQualityGate,
	})
	result := messagequeue.QualityGateResultPayload{
		RunID:       "run-gate",
		TestsPassed: &failed,
		TestOutput:  "exit code 1\nFAILED test_a.py::test_a - assert 1 == 2",
		LintPassed:  &passed,
		LintOutput:  "All checks passed!",
	}
	if err := env.svc.HandleQualityGateResult(context.Background(), &result); err != nil {
		t.Fatalf("HandleQualityGateResult: %v", err)
	}

	var payload map[string]string
	for i := range env.events.events {
		if env.events.events[i].Type == event.TypeQualityGateFailed {
			if err := json.Unmarshal(env.events.events[i].Payload, &payload); err != nil {
				t.Fatal(err)
			}
		}
	}
	if payload == nil {
		t.Fatal("no run.qualitygate.failed event")
	}
	if payload["test_output"] != result.TestOutput {
		t.Errorf("test_output = %q, want %q", payload["test_output"], result.TestOutput)
	}
	if _, ok := payload["lint_output"]; ok {
		t.Errorf("lint_output = %q, want none: the lint check passed", payload["lint_output"])
	}
	if payload["error"] != "quality gate failed: tests failed" {
		t.Errorf("error = %q", payload["error"])
	}
}
