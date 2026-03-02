package agent_test

import (
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
)

func TestInboxMessage_ValidMessage(t *testing.T) {
	msg := agent.InboxMessage{
		AgentID:   "agent-1",
		FromAgent: "agent-2",
		Content:   "Please review the implementation",
		Priority:  1,
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("expected no error for valid message, got: %v", err)
	}
}

func TestInboxMessage_MissingAgentID(t *testing.T) {
	msg := agent.InboxMessage{
		FromAgent: "agent-2",
		Content:   "some content",
	}
	err := msg.Validate()
	if err == nil {
		t.Fatal("expected error for missing agent_id")
	}
	if err.Error() != "agent_id required" {
		t.Errorf("expected 'agent_id required', got: %s", err.Error())
	}
}

func TestInboxMessage_MissingContent(t *testing.T) {
	msg := agent.InboxMessage{
		AgentID:   "agent-1",
		FromAgent: "agent-2",
		Content:   "",
	}
	err := msg.Validate()
	if err == nil {
		t.Fatal("expected error for missing content")
	}
	if err.Error() != "content required" {
		t.Errorf("expected 'content required', got: %s", err.Error())
	}
}

func TestInboxMessage_NegativePriority(t *testing.T) {
	msg := agent.InboxMessage{
		AgentID:   "agent-1",
		FromAgent: "agent-2",
		Content:   "some content",
		Priority:  -1,
	}
	err := msg.Validate()
	if err == nil {
		t.Fatal("expected error for negative priority")
	}
	if err.Error() != "priority must be >= 0" {
		t.Errorf("expected 'priority must be >= 0', got: %s", err.Error())
	}
}

func TestInboxMessage_Defaults(t *testing.T) {
	var msg agent.InboxMessage

	if msg.Read {
		t.Error("expected Read to default to false")
	}
	if msg.Priority != 0 {
		t.Errorf("expected Priority to default to 0, got %d", msg.Priority)
	}
	if !msg.CreatedAt.IsZero() {
		t.Errorf("expected CreatedAt to be zero, got %v", msg.CreatedAt)
	}
}

func TestInboxMessage_ZeroPriorityIsValid(t *testing.T) {
	msg := agent.InboxMessage{
		AgentID: "agent-1",
		Content: "zero priority message",
	}
	if err := msg.Validate(); err != nil {
		t.Fatalf("expected no error for zero priority, got: %v", err)
	}
}

func TestInboxMessage_WhitespaceOnlyContent(t *testing.T) {
	msg := agent.InboxMessage{
		AgentID: "agent-1",
		Content: "   ",
	}
	err := msg.Validate()
	if err == nil {
		t.Fatal("expected error for whitespace-only content")
	}
}

func TestAgent_IdentityFieldDefaults(t *testing.T) {
	a := agent.Agent{}

	if a.TotalRuns != 0 {
		t.Errorf("expected TotalRuns=0, got %d", a.TotalRuns)
	}
	if a.TotalCost != 0.0 {
		t.Errorf("expected TotalCost=0.0, got %f", a.TotalCost)
	}
	if a.SuccessRate != 0.0 {
		t.Errorf("expected SuccessRate=0.0, got %f", a.SuccessRate)
	}
	if a.State != nil {
		t.Errorf("expected State=nil, got %v", a.State)
	}
	if a.Capabilities != nil {
		t.Errorf("expected Capabilities=nil, got %v", a.Capabilities)
	}
	if a.LastActiveAt != nil {
		t.Errorf("expected LastActiveAt=nil, got %v", a.LastActiveAt)
	}
}

func TestAgent_StateMapOperations(t *testing.T) {
	a := agent.Agent{
		State: map[string]string{
			"last_decision": "use_retry",
			"context_key":   "abc123",
		},
	}
	if a.State["last_decision"] != "use_retry" {
		t.Errorf("expected 'use_retry', got %q", a.State["last_decision"])
	}
	if a.State["context_key"] != "abc123" {
		t.Errorf("expected 'abc123', got %q", a.State["context_key"])
	}

	// Mutate and verify
	a.State["new_key"] = "new_value"
	if len(a.State) != 3 {
		t.Errorf("expected 3 state entries, got %d", len(a.State))
	}
}

func TestAgent_SuccessRateCalculation(t *testing.T) {
	// Verify success rate after simulated sequence of runs:
	// 3 successes out of 4 runs = 0.75
	a := agent.Agent{
		TotalRuns:   4,
		SuccessRate: 0.75,
	}
	if a.SuccessRate < 0.0 || a.SuccessRate > 1.0 {
		t.Errorf("SuccessRate out of range [0,1]: %f", a.SuccessRate)
	}

	// After another success: 4/5 = 0.80
	newRate := (a.SuccessRate*float64(a.TotalRuns) + 1.0) / float64(a.TotalRuns+1)
	if newRate < 0.79 || newRate > 0.81 {
		t.Errorf("expected ~0.80 after 4th success in 5 runs, got %f", newRate)
	}

	// After a failure: 4/6 = 0.6667
	failRate := (newRate * 5.0) / 6.0
	if failRate < 0.66 || failRate > 0.67 {
		t.Errorf("expected ~0.6667 after failure, got %f", failRate)
	}
}

func TestAgent_LastActiveAtTracking(t *testing.T) {
	now := time.Now()
	a := agent.Agent{
		LastActiveAt: &now,
	}
	if a.LastActiveAt == nil {
		t.Fatal("expected LastActiveAt to be set")
	}
	if a.LastActiveAt.After(time.Now()) {
		t.Error("LastActiveAt should not be in the future")
	}
}

func TestAgent_Capabilities(t *testing.T) {
	a := agent.Agent{
		Capabilities: []string{"code", "review", "test"},
	}
	if len(a.Capabilities) != 3 {
		t.Fatalf("expected 3 capabilities, got %d", len(a.Capabilities))
	}
	if a.Capabilities[0] != "code" {
		t.Errorf("expected first capability 'code', got %q", a.Capabilities[0])
	}
}
