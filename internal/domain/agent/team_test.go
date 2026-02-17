package agent_test

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/agent"
)

func TestCreateTeamRequestValidation(t *testing.T) {
	valid := agent.CreateTeamRequest{
		ProjectID: "p1",
		Name:      "Test Team",
		Protocol:  "sequential",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestCreateTeamRequestMissingProjectID(t *testing.T) {
	req := agent.CreateTeamRequest{
		Name: "Team",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for missing project_id")
	}
}

func TestCreateTeamRequestMissingName(t *testing.T) {
	req := agent.CreateTeamRequest{
		ProjectID: "p1",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCreateTeamRequestNoMembers(t *testing.T) {
	req := agent.CreateTeamRequest{
		ProjectID: "p1",
		Name:      "Team",
		Members:   []agent.CreateMemberRequest{},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for empty members")
	}
}

func TestCreateTeamRequestInvalidRole(t *testing.T) {
	req := agent.CreateTeamRequest{
		ProjectID: "p1",
		Name:      "Team",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: "invalid_role"},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for invalid role")
	}
}

func TestCreateTeamRequestDuplicateAgent(t *testing.T) {
	req := agent.CreateTeamRequest{
		ProjectID: "p1",
		Name:      "Team",
		Members: []agent.CreateMemberRequest{
			{AgentID: "a1", Role: agent.RoleCoder},
			{AgentID: "a1", Role: agent.RoleReviewer},
		},
	}
	if err := req.Validate(); err == nil {
		t.Fatal("expected error for duplicate agent_id")
	}
}

func TestValidTeamRole(t *testing.T) {
	validRoles := []string{"coder", "reviewer", "tester", "documenter", "planner"}
	for _, r := range validRoles {
		if !agent.ValidTeamRole(r) {
			t.Errorf("expected %q to be valid", r)
		}
	}

	if agent.ValidTeamRole("unknown") {
		t.Error("expected 'unknown' to be invalid")
	}
}

func TestTeamStatusIsTerminal(t *testing.T) {
	if !agent.TeamStatusCompleted.IsTerminal() {
		t.Error("expected completed to be terminal")
	}
	if !agent.TeamStatusFailed.IsTerminal() {
		t.Error("expected failed to be terminal")
	}
	if agent.TeamStatusActive.IsTerminal() {
		t.Error("expected active to not be terminal")
	}
	if agent.TeamStatusInitializing.IsTerminal() {
		t.Error("expected initializing to not be terminal")
	}
}
