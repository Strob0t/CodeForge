package plan

import "testing"

func TestDecomposeRequestValidate(t *testing.T) {
	tests := []struct {
		name    string
		req     DecomposeRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     DecomposeRequest{ProjectID: "p1", Feature: "Add user auth"},
			wantErr: false,
		},
		{
			name:    "missing project_id",
			req:     DecomposeRequest{Feature: "something"},
			wantErr: true,
		},
		{
			name:    "missing feature",
			req:     DecomposeRequest{ProjectID: "p1"},
			wantErr: true,
		},
		{
			name:    "both empty",
			req:     DecomposeRequest{},
			wantErr: true,
		},
		{
			name:    "with optional fields",
			req:     DecomposeRequest{ProjectID: "p1", Feature: "f", Context: "ctx", Model: "gpt-4o"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidOrchestratorMode(t *testing.T) {
	for _, m := range []string{"manual", "semi_auto", "full_auto"} {
		if !ValidOrchestratorMode(m) {
			t.Errorf("expected %q to be valid", m)
		}
	}
	for _, m := range []string{"", "invalid", "auto"} {
		if ValidOrchestratorMode(m) {
			t.Errorf("expected %q to be invalid", m)
		}
	}
}

func TestStrategyToProtocol(t *testing.T) {
	tests := []struct {
		strategy AgentStrategy
		want     Protocol
	}{
		{StrategySingle, ProtocolSequential},
		{StrategyPair, ProtocolPingPong},
		{StrategyTeam, ProtocolParallel},
		{AgentStrategy("unknown"), ProtocolSequential}, // default
	}
	for _, tt := range tests {
		got := StrategyToProtocol(tt.strategy)
		if got != tt.want {
			t.Errorf("StrategyToProtocol(%q) = %q, want %q", tt.strategy, got, tt.want)
		}
	}
}

func TestDecomposeResultValidate(t *testing.T) {
	tests := []struct {
		name    string
		result  DecomposeResult
		wantErr bool
	}{
		{
			name: "valid result",
			result: DecomposeResult{
				PlanName: "Auth Plan",
				Subtasks: []SubtaskDefinition{
					{Title: "Task 1", Prompt: "Do thing 1"},
					{Title: "Task 2", Prompt: "Do thing 2", DependsOn: []int{0}},
				},
			},
			wantErr: false,
		},
		{
			name:    "missing plan name",
			result:  DecomposeResult{Subtasks: []SubtaskDefinition{{Title: "T", Prompt: "P"}}},
			wantErr: true,
		},
		{
			name:    "no subtasks",
			result:  DecomposeResult{PlanName: "Plan"},
			wantErr: true,
		},
		{
			name: "subtask missing title",
			result: DecomposeResult{
				PlanName: "Plan",
				Subtasks: []SubtaskDefinition{{Prompt: "P"}},
			},
			wantErr: true,
		},
		{
			name: "subtask missing prompt",
			result: DecomposeResult{
				PlanName: "Plan",
				Subtasks: []SubtaskDefinition{{Title: "T"}},
			},
			wantErr: true,
		},
		{
			name: "invalid dependency index",
			result: DecomposeResult{
				PlanName: "Plan",
				Subtasks: []SubtaskDefinition{
					{Title: "T1", Prompt: "P1", DependsOn: []int{5}},
				},
			},
			wantErr: true,
		},
		{
			name: "self-referencing dependency",
			result: DecomposeResult{
				PlanName: "Plan",
				Subtasks: []SubtaskDefinition{
					{Title: "T1", Prompt: "P1", DependsOn: []int{0}},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.result.ValidateResult()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateResult() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
