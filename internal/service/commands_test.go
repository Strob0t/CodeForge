package service

import (
	"context"
	"testing"
)

func TestCommandService_ListCommands_ReturnsAllBuiltins(t *testing.T) {
	svc := NewCommandService()
	cmds := svc.ListCommands(context.Background())

	const expectedCount = 8
	if len(cmds) != expectedCount {
		t.Fatalf("expected %d commands, got %d", expectedCount, len(cmds))
	}
}

func TestCommandService_ListCommands_IDs(t *testing.T) {
	svc := NewCommandService()
	cmds := svc.ListCommands(context.Background())

	expectedIDs := []string{"/compact", "/rewind", "/clear", "/diff", "/cost", "/help", "/mode", "/model"}
	ids := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		ids[c.ID] = true
	}

	for _, id := range expectedIDs {
		if !ids[id] {
			t.Errorf("expected command %q not found", id)
		}
	}
}

func TestCommandService_ListCommands_Categories(t *testing.T) {
	svc := NewCommandService()
	cmds := svc.ListCommands(context.Background())

	chatCount := 0
	agentCount := 0
	for _, c := range cmds {
		switch c.Category {
		case "chat":
			chatCount++
		case "agent":
			agentCount++
		default:
			t.Errorf("unexpected category %q for command %q", c.Category, c.ID)
		}
	}

	if chatCount != 6 {
		t.Errorf("expected 6 chat commands, got %d", chatCount)
	}
	if agentCount != 2 {
		t.Errorf("expected 2 agent commands, got %d", agentCount)
	}
}

func TestCommandService_ListCommands_AgentCommandsHaveArgs(t *testing.T) {
	svc := NewCommandService()
	cmds := svc.ListCommands(context.Background())

	for _, c := range cmds {
		if c.Category == "agent" && c.Args == "" {
			t.Errorf("agent command %q should have args", c.ID)
		}
	}
}

func TestCommandService_ListCommands_AllFieldsPopulated(t *testing.T) {
	svc := NewCommandService()
	cmds := svc.ListCommands(context.Background())

	for _, c := range cmds {
		if c.ID == "" {
			t.Error("command has empty ID")
		}
		if c.Label == "" {
			t.Errorf("command %q has empty Label", c.ID)
		}
		if c.Category == "" {
			t.Errorf("command %q has empty Category", c.ID)
		}
		if c.Description == "" {
			t.Errorf("command %q has empty Description", c.ID)
		}
	}
}

func TestCommandService_ListCommands_ReturnsCopy(t *testing.T) {
	svc := NewCommandService()
	cmds1 := svc.ListCommands(context.Background())
	cmds2 := svc.ListCommands(context.Background())

	// Mutating the first result should not affect the second.
	cmds1[0].ID = "mutated"
	if cmds2[0].ID == "mutated" {
		t.Error("ListCommands should return a copy, not a reference to the internal slice")
	}
}
