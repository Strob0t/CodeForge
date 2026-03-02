package task

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActiveWorkItemJSONSerialization(t *testing.T) {
	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	item := ActiveWorkItem{
		TaskID:     "task-1",
		TaskTitle:  "Implement auth",
		TaskStatus: StatusRunning,
		ProjectID:  "proj-1",
		AgentID:    "agent-1",
		AgentName:  "CodeBot",
		AgentMode:  "coder",
		RunID:      "run-1",
		StepCount:  5,
		CostUSD:    0.42,
		StartedAt:  now,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal ActiveWorkItem: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	requiredFields := []string{
		"task_id", "task_title", "task_status", "project_id",
		"agent_id", "agent_name", "agent_mode", "run_id",
		"step_count", "cost_usd", "started_at",
	}
	for _, field := range requiredFields {
		if _, ok := m[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}

	if m["task_id"] != "task-1" {
		t.Errorf("task_id = %v, want task-1", m["task_id"])
	}
	if m["agent_name"] != "CodeBot" {
		t.Errorf("agent_name = %v, want CodeBot", m["agent_name"])
	}
	if m["task_status"] != "running" {
		t.Errorf("task_status = %v, want running", m["task_status"])
	}
}

func TestActiveWorkItemOmitsEmptyOptionalFields(t *testing.T) {
	item := ActiveWorkItem{
		TaskID:     "task-1",
		TaskTitle:  "Fix bug",
		TaskStatus: StatusQueued,
		ProjectID:  "proj-1",
		AgentID:    "agent-1",
		AgentName:  "TestBot",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Optional fields with omitempty should be absent when zero
	if _, ok := m["run_id"]; ok {
		t.Error("run_id should be omitted when empty")
	}
	if _, ok := m["agent_mode"]; ok {
		t.Error("agent_mode should be omitted when empty")
	}
}

func TestClaimResultJSONSerialization(t *testing.T) {
	tests := []struct {
		name    string
		result  ClaimResult
		claimed bool
		reason  string
	}{
		{
			name: "claimed successfully",
			result: ClaimResult{
				Task: &Task{
					ID:        "task-1",
					ProjectID: "proj-1",
					Title:     "Do work",
					Status:    StatusQueued,
				},
				Claimed: true,
			},
			claimed: true,
		},
		{
			name: "already claimed",
			result: ClaimResult{
				Claimed: false,
				Reason:  "already claimed by agent CodeBot",
			},
			claimed: false,
			reason:  "already claimed by agent CodeBot",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("marshal ClaimResult: %v", err)
			}

			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal to map: %v", err)
			}

			if m["claimed"] != tt.claimed {
				t.Errorf("claimed = %v, want %v", m["claimed"], tt.claimed)
			}

			if tt.reason != "" {
				if m["reason"] != tt.reason {
					t.Errorf("reason = %v, want %v", m["reason"], tt.reason)
				}
			}
		})
	}
}

func TestActiveWorkItemZeroValues(t *testing.T) {
	var item ActiveWorkItem
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal zero ActiveWorkItem: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Core fields must be present even with zero values
	for _, field := range []string{"task_id", "task_title", "task_status", "project_id", "agent_id", "agent_name"} {
		if _, ok := m[field]; !ok {
			t.Errorf("missing JSON field %q on zero-value struct", field)
		}
	}
}
