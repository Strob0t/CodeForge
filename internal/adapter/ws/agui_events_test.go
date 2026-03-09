package ws_test

import (
	"encoding/json"
	"testing"

	"github.com/Strob0t/CodeForge/internal/adapter/ws"
)

func TestAGUIGoalProposalEventMarshal(t *testing.T) {
	ev := ws.AGUIGoalProposalEvent{
		RunID:      "run-123",
		ProposalID: "prop-456",
		Action:     "create",
		Kind:       "requirement",
		Title:      "User can search products",
		Content:    "A search function...",
		Priority:   90,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["run_id"] != "run-123" {
		t.Errorf("run_id = %v, want run-123", got["run_id"])
	}
	if got["proposal_id"] != "prop-456" {
		t.Errorf("proposal_id = %v, want prop-456", got["proposal_id"])
	}
	if got["kind"] != "requirement" {
		t.Errorf("kind = %v, want requirement", got["kind"])
	}
	if got["action"] != "create" {
		t.Errorf("action = %v, want create", got["action"])
	}
	if got["title"] != "User can search products" {
		t.Errorf("title = %v, want User can search products", got["title"])
	}
}

func TestAGUIGoalProposalConstant(t *testing.T) {
	if ws.AGUIGoalProposal != "agui.goal_proposal" {
		t.Errorf("AGUIGoalProposal = %q, want agui.goal_proposal", ws.AGUIGoalProposal)
	}
}
