package orchestration

import (
	"testing"
)

func TestHandoffMessageValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		msg     HandoffMessage
		wantErr string
	}{
		{
			name: "valid handoff",
			msg: HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "agent-2",
				Context:       "Please review the code changes",
			},
			wantErr: "",
		},
		{
			name: "valid handoff with all fields",
			msg: HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "agent-2",
				TargetModeID:  "reviewer",
				Context:       "Review code",
				Artifacts:     []string{"file1.go", "file2.go"},
				PlanID:        "plan-1",
				StepID:        "step-3",
				Metadata:      map[string]string{"priority": "high"},
			},
			wantErr: "",
		},
		{
			name: "missing source_agent_id",
			msg: HandoffMessage{
				TargetAgentID: "agent-2",
				Context:       "Do something",
			},
			wantErr: "source_agent_id is required",
		},
		{
			name: "missing target_agent_id",
			msg: HandoffMessage{
				SourceAgentID: "agent-1",
				Context:       "Do something",
			},
			wantErr: "target_agent_id is required",
		},
		{
			name: "missing context",
			msg: HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "agent-2",
			},
			wantErr: "context is required",
		},
		{
			name: "empty source_agent_id",
			msg: HandoffMessage{
				SourceAgentID: "",
				TargetAgentID: "agent-2",
				Context:       "Do something",
			},
			wantErr: "source_agent_id is required",
		},
		{
			name: "empty target_agent_id",
			msg: HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "",
				Context:       "Do something",
			},
			wantErr: "target_agent_id is required",
		},
		{
			name: "empty context",
			msg: HandoffMessage{
				SourceAgentID: "agent-1",
				TargetAgentID: "agent-2",
				Context:       "",
			},
			wantErr: "context is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.msg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() = nil, want error containing %q", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("Validate() = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestHandoffMessageValidateOrder(t *testing.T) {
	t.Parallel()

	// When all three fields are missing, source_agent_id should be reported first.
	msg := HandoffMessage{}
	err := msg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if err.Error() != "source_agent_id is required" {
		t.Errorf("Validate() = %q, want first validation to be source_agent_id", err.Error())
	}
}

func TestHandoffMessageOptionalFields(t *testing.T) {
	t.Parallel()

	msg := HandoffMessage{
		SourceAgentID: "a",
		TargetAgentID: "b",
		Context:       "c",
	}

	if err := msg.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	// Optional fields should have zero values.
	if msg.TargetModeID != "" {
		t.Errorf("TargetModeID = %q, want empty", msg.TargetModeID)
	}
	if msg.Artifacts != nil {
		t.Errorf("Artifacts = %v, want nil", msg.Artifacts)
	}
	if msg.PlanID != "" {
		t.Errorf("PlanID = %q, want empty", msg.PlanID)
	}
	if msg.StepID != "" {
		t.Errorf("StepID = %q, want empty", msg.StepID)
	}
	if msg.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", msg.Metadata)
	}
	if msg.Trust != nil {
		t.Errorf("Trust = %v, want nil", msg.Trust)
	}
}

func TestReviewDecisionFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		decision ReviewDecision
	}{
		{
			name: "needs review",
			decision: ReviewDecision{
				NeedsReview:        true,
				Confidence:         0.85,
				Reason:             "destructive file operation detected",
				SuggestedReviewers: []string{"user-1", "user-2"},
			},
		},
		{
			name: "no review needed",
			decision: ReviewDecision{
				NeedsReview: false,
				Confidence:  0.95,
				Reason:      "read-only operation",
			},
		},
		{
			name: "zero confidence",
			decision: ReviewDecision{
				NeedsReview: true,
				Confidence:  0.0,
				Reason:      "unknown operation",
			},
		},
		{
			name: "empty reviewers",
			decision: ReviewDecision{
				NeedsReview:        true,
				Confidence:         0.5,
				Reason:             "moderate risk",
				SuggestedReviewers: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := tt.decision
			if d.Reason == "" {
				t.Error("Reason should not be empty in test fixtures")
			}
			// Confidence should be between 0 and 1.
			if d.Confidence < 0 || d.Confidence > 1 {
				t.Errorf("Confidence = %f, should be between 0 and 1", d.Confidence)
			}
		})
	}
}
