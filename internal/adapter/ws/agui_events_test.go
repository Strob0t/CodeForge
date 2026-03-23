package ws_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/event"
)

func TestAGUIGoalProposalEventMarshal(t *testing.T) {
	ev := event.AGUIGoalProposalEvent{
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
	var got struct {
		RunID      string `json:"run_id"`
		ProposalID string `json:"proposal_id"`
		Kind       string `json:"kind"`
		Action     string `json:"action"`
		Title      string `json:"title"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.RunID != "run-123" {
		t.Errorf("run_id = %v, want run-123", got.RunID)
	}
	if got.ProposalID != "prop-456" {
		t.Errorf("proposal_id = %v, want prop-456", got.ProposalID)
	}
	if got.Kind != "requirement" {
		t.Errorf("kind = %v, want requirement", got.Kind)
	}
	if got.Action != "create" {
		t.Errorf("action = %v, want create", got.Action)
	}
	if got.Title != "User can search products" {
		t.Errorf("title = %v, want User can search products", got.Title)
	}
}

func TestAGUIGoalProposalConstant(t *testing.T) {
	if event.AGUIGoalProposal != "agui.goal_proposal" {
		t.Errorf("AGUIGoalProposal = %q, want agui.goal_proposal", event.AGUIGoalProposal)
	}
}

// TestAGUIEventTypeConstants verifies all AG-UI event type string constants.
func TestAGUIEventTypeConstants(t *testing.T) {
	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"RunStarted", event.AGUIRunStarted, "agui.run_started"},
		{"RunFinished", event.AGUIRunFinished, "agui.run_finished"},
		{"TextMessage", event.AGUITextMessage, "agui.text_message"},
		{"ToolCall", event.AGUIToolCall, "agui.tool_call"},
		{"ToolResult", event.AGUIToolResult, "agui.tool_result"},
		{"StateDelta", event.AGUIStateDelta, "agui.state_delta"},
		{"StepStarted", event.AGUIStepStarted, "agui.step_started"},
		{"StepFinished", event.AGUIStepFinished, "agui.step_finished"},
		{"PermissionRequest", event.AGUIPermissionRequest, "agui.permission_request"},
		{"GoalProposal", event.AGUIGoalProposal, "agui.goal_proposal"},
		{"ActionSuggestion", event.AGUIActionSuggestion, "agui.action_suggestion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("constant = %q, want %q", tt.got, tt.expected)
			}
		})
	}
}

// TestAGUIEvents_JSONRoundTrip verifies that every AG-UI event type can be
// marshaled to JSON and unmarshaled back with all fields preserved.
func TestAGUIEvents_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name       string
		original   func() interface{}     // factory returns a pointer
		expectedKs map[string]interface{} // key -> expected value in the raw JSON map
		omittedKs  []string               // keys expected absent due to omitempty + zero
	}{
		{
			name: "RunStarted",
			original: func() interface{} {
				return &event.AGUIRunStartedEvent{
					RunID:     "run-001",
					ThreadID:  "thread-abc",
					AgentName: "assistant",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":     "run-001",
				"thread_id":  "thread-abc",
				"agent_name": "assistant",
			},
		},
		{
			name: "RunStarted_OmitEmpty",
			original: func() interface{} {
				return &event.AGUIRunStartedEvent{
					RunID: "run-002",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id": "run-002",
			},
			omittedKs: []string{"thread_id", "agent_name"},
		},
		{
			name: "RunFinished",
			original: func() interface{} {
				return &event.AGUIRunFinishedEvent{
					RunID:     "run-010",
					Status:    "completed",
					Error:     "something went wrong",
					Model:     "gpt-4o",
					CostUSD:   0.0042,
					TokensIn:  1500,
					TokensOut: 320,
					Steps:     7,
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":     "run-010",
				"status":     "completed",
				"error":      "something went wrong",
				"model":      "gpt-4o",
				"cost_usd":   0.0042,
				"tokens_in":  float64(1500),
				"tokens_out": float64(320),
				"steps":      float64(7),
			},
		},
		{
			name: "RunFinished_OmitEmpty",
			original: func() interface{} {
				return &event.AGUIRunFinishedEvent{
					RunID:  "run-011",
					Status: "failed",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id": "run-011",
				"status": "failed",
			},
			omittedKs: []string{"error", "model", "cost_usd", "tokens_in", "tokens_out", "steps"},
		},
		{
			name: "TextMessage",
			original: func() interface{} {
				return &event.AGUITextMessageEvent{
					RunID:   "run-020",
					Role:    "assistant",
					Content: "Hello, I will help you refactor this module.",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-020",
				"role":    "assistant",
				"content": "Hello, I will help you refactor this module.",
			},
		},
		{
			name: "ToolCall",
			original: func() interface{} {
				return &event.AGUIToolCallEvent{
					RunID:  "run-030",
					CallID: "call-xyz",
					Name:   "Read",
					Args:   `{"path":"/src/main.go"}`,
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-030",
				"call_id": "call-xyz",
				"name":    "Read",
				"args":    `{"path":"/src/main.go"}`,
			},
		},
		{
			name: "ToolResult",
			original: func() interface{} {
				return &event.AGUIToolResultEvent{
					RunID:  "run-040",
					CallID: "call-res-1",
					Result: `{"lines":42}`,
					Error:  "permission denied",
					Diff:   json.RawMessage(`{"op":"add","path":"/foo","value":"bar"}`),
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-040",
				"call_id": "call-res-1",
				"result":  `{"lines":42}`,
				"error":   "permission denied",
			},
		},
		{
			name: "ToolResult_OmitEmpty",
			original: func() interface{} {
				return &event.AGUIToolResultEvent{
					RunID:  "run-041",
					CallID: "call-res-2",
					Result: `{"ok":true}`,
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-041",
				"call_id": "call-res-2",
				"result":  `{"ok":true}`,
			},
			omittedKs: []string{"error", "diff"},
		},
		{
			name: "StateDelta",
			original: func() interface{} {
				return &event.AGUIStateDeltaEvent{
					RunID: "run-050",
					Delta: `[{"op":"replace","path":"/cost","value":0.01}]`,
				}
			},
			expectedKs: map[string]interface{}{
				"run_id": "run-050",
				"delta":  `[{"op":"replace","path":"/cost","value":0.01}]`,
			},
		},
		{
			name: "StepStarted",
			original: func() interface{} {
				return &event.AGUIStepStartedEvent{
					RunID:  "run-060",
					StepID: "step-1",
					Name:   "analyze_code",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-060",
				"step_id": "step-1",
				"name":    "analyze_code",
			},
		},
		{
			name: "StepFinished",
			original: func() interface{} {
				return &event.AGUIStepFinishedEvent{
					RunID:  "run-070",
					StepID: "step-2",
					Status: "completed",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-070",
				"step_id": "step-2",
				"status":  "completed",
			},
		},
		{
			name: "PermissionRequest",
			original: func() interface{} {
				return &event.AGUIPermissionRequestEvent{
					RunID:   "run-080",
					CallID:  "call-perm-1",
					Tool:    "Bash",
					Command: "rm -rf /tmp/build",
					Path:    "/tmp/build",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-080",
				"call_id": "call-perm-1",
				"tool":    "Bash",
				"command": "rm -rf /tmp/build",
				"path":    "/tmp/build",
			},
		},
		{
			name: "PermissionRequest_OmitEmpty",
			original: func() interface{} {
				return &event.AGUIPermissionRequestEvent{
					RunID:  "run-081",
					CallID: "call-perm-2",
					Tool:   "Read",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":  "run-081",
				"call_id": "call-perm-2",
				"tool":    "Read",
			},
			omittedKs: []string{"command", "path"},
		},
		{
			name: "ActionSuggestion",
			original: func() interface{} {
				return &event.AGUIActionSuggestionEvent{
					RunID:  "run-090",
					Label:  "Run tests",
					Action: "run_tool",
					Value:  "Bash",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id": "run-090",
				"label":  "Run tests",
				"action": "run_tool",
				"value":  "Bash",
			},
		},
		{
			name: "GoalProposal",
			original: func() interface{} {
				return &event.AGUIGoalProposalEvent{
					RunID:      "run-100",
					ProposalID: "prop-abc",
					Action:     "create",
					Kind:       "requirement",
					Title:      "Implement search",
					Content:    "Full-text search for products",
					Priority:   85,
					GoalID:     "goal-42",
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":      "run-100",
				"proposal_id": "prop-abc",
				"action":      "create",
				"kind":        "requirement",
				"title":       "Implement search",
				"content":     "Full-text search for products",
				"priority":    float64(85),
				"goal_id":     "goal-42",
			},
		},
		{
			name: "GoalProposal_OmitEmpty",
			original: func() interface{} {
				return &event.AGUIGoalProposalEvent{
					RunID:      "run-101",
					ProposalID: "prop-def",
					Action:     "update",
					Kind:       "task",
					Title:      "Fix bug",
					Content:    "Null pointer",
					Priority:   50,
				}
			},
			expectedKs: map[string]interface{}{
				"run_id":      "run-101",
				"proposal_id": "prop-def",
				"action":      "update",
				"kind":        "task",
				"title":       "Fix bug",
				"content":     "Null pointer",
				"priority":    float64(50),
			},
			omittedKs: []string{"goal_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.original()

			// Marshal to JSON.
			data, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			// Verify valid JSON and check key names + values.
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("json.Unmarshal to map failed: %v", err)
			}

			for key, want := range tt.expectedKs {
				got, ok := raw[key]
				if !ok {
					t.Errorf("JSON key %q missing from output", key)
					continue
				}
				// json.RawMessage unmarshals to interface{} differently;
				// for the diff field we just check presence was already done.
				if key == "diff" {
					continue
				}
				if got != want {
					t.Errorf("JSON key %q = %v (%T), want %v (%T)", key, got, got, want, want)
				}
			}

			// Verify omitempty keys are absent.
			for _, key := range tt.omittedKs {
				if _, ok := raw[key]; ok {
					t.Errorf("JSON key %q should be omitted for zero value, but present", key)
				}
			}

			// Unmarshal back into the same type and compare.
			roundTripped := reflect.New(reflect.TypeOf(original).Elem()).Interface()
			if err := json.Unmarshal(data, roundTripped); err != nil {
				t.Fatalf("json.Unmarshal round-trip failed: %v", err)
			}

			if !reflect.DeepEqual(original, roundTripped) {
				t.Errorf("round-trip mismatch:\n  original:    %+v\n  roundTripped: %+v", original, roundTripped)
			}
		})
	}
}

// TestAGUIToolResultEvent_DiffRawJSON verifies that the Diff field (json.RawMessage)
// preserves arbitrary JSON through marshal/unmarshal.
func TestAGUIToolResultEvent_DiffRawJSON(t *testing.T) {
	diffPayload := `{"op":"replace","path":"/content","value":"new"}`
	ev := event.AGUIToolResultEvent{
		RunID:  "run-diff",
		CallID: "call-diff",
		Result: "ok",
		Diff:   json.RawMessage(diffPayload),
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got event.AGUIToolResultEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if string(got.Diff) != diffPayload {
		t.Errorf("Diff = %s, want %s", string(got.Diff), diffPayload)
	}
}

// TestAGUIEvents_EmptyStructs verifies that zero-valued structs marshal
// without error and produce valid JSON.
func TestAGUIEvents_EmptyStructs(t *testing.T) {
	zeros := []struct {
		name  string
		event interface{}
	}{
		{"RunStarted_zero", event.AGUIRunStartedEvent{}},
		{"RunFinished_zero", event.AGUIRunFinishedEvent{}},
		{"TextMessage_zero", event.AGUITextMessageEvent{}},
		{"ToolCall_zero", event.AGUIToolCallEvent{}},
		{"ToolResult_zero", event.AGUIToolResultEvent{}},
		{"StateDelta_zero", event.AGUIStateDeltaEvent{}},
		{"StepStarted_zero", event.AGUIStepStartedEvent{}},
		{"StepFinished_zero", event.AGUIStepFinishedEvent{}},
		{"PermissionRequest_zero", event.AGUIPermissionRequestEvent{}},
		{"ActionSuggestion_zero", event.AGUIActionSuggestionEvent{}},
		{"GoalProposal_zero", event.AGUIGoalProposalEvent{}},
	}

	for _, tt := range zeros {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.event)
			if err != nil {
				t.Fatalf("marshal zero-value struct: %v", err)
			}
			var raw map[string]interface{}
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("unmarshal zero-value JSON: %v", err)
			}
		})
	}
}
