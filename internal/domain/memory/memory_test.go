package memory

import (
	"testing"
	"time"
)

func TestCreateRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     CreateRequest
		wantErr string
	}{
		{
			name: "valid request",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "The user prefers functional style",
				Kind:       KindObservation,
				Importance: 0.7,
			},
			wantErr: "",
		},
		{
			name: "valid with all fields",
			req: CreateRequest{
				TenantID:   "tenant-1",
				ProjectID:  "proj-1",
				AgentID:    "agent-1",
				RunID:      "run-1",
				Content:    "Decided to use Go for the service",
				Kind:       KindDecision,
				Importance: 1.0,
				Metadata:   map[string]string{"source": "code_review"},
			},
			wantErr: "",
		},
		{
			name: "missing project_id",
			req: CreateRequest{
				Content:    "content",
				Kind:       KindObservation,
				Importance: 0.5,
			},
			wantErr: "project_id is required",
		},
		{
			name: "missing content",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Kind:       KindObservation,
				Importance: 0.5,
			},
			wantErr: "content is required",
		},
		{
			name: "invalid kind",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "content",
				Kind:       Kind("invalid"),
				Importance: 0.5,
			},
			wantErr: "invalid kind: must be observation, decision, error, or insight",
		},
		{
			name: "empty kind",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "content",
				Kind:       Kind(""),
				Importance: 0.5,
			},
			wantErr: "invalid kind: must be observation, decision, error, or insight",
		},
		{
			name: "importance below zero",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "content",
				Kind:       KindObservation,
				Importance: -0.1,
			},
			wantErr: "importance must be between 0 and 1",
		},
		{
			name: "importance above one",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "content",
				Kind:       KindObservation,
				Importance: 1.1,
			},
			wantErr: "importance must be between 0 and 1",
		},
		{
			name: "importance at zero boundary",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "content",
				Kind:       KindObservation,
				Importance: 0.0,
			},
			wantErr: "",
		},
		{
			name: "importance at one boundary",
			req: CreateRequest{
				ProjectID:  "proj-1",
				Content:    "content",
				Kind:       KindObservation,
				Importance: 1.0,
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() = nil, want error %q", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("Validate() = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestKindConstants(t *testing.T) {
	t.Parallel()

	if KindObservation != "observation" {
		t.Errorf("KindObservation = %q, want %q", KindObservation, "observation")
	}
	if KindDecision != "decision" {
		t.Errorf("KindDecision = %q, want %q", KindDecision, "decision")
	}
	if KindError != "error" {
		t.Errorf("KindError = %q, want %q", KindError, "error")
	}
	if KindInsight != "insight" {
		t.Errorf("KindInsight = %q, want %q", KindInsight, "insight")
	}
}

func TestValidKinds(t *testing.T) {
	t.Parallel()

	if len(ValidKinds) != 4 {
		t.Errorf("len(ValidKinds) = %d, want 4", len(ValidKinds))
	}

	expected := []Kind{KindObservation, KindDecision, KindError, KindInsight}
	for i, k := range ValidKinds {
		if k != expected[i] {
			t.Errorf("ValidKinds[%d] = %q, want %q", i, k, expected[i])
		}
	}
}

func TestDefaultTenantID(t *testing.T) {
	t.Parallel()

	if DefaultTenantID != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("DefaultTenantID = %q, want UUID zero", DefaultTenantID)
	}
}

func TestMemoryFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	m := Memory{
		ID:         "mem-1",
		TenantID:   "tenant-1",
		ProjectID:  "proj-1",
		AgentID:    "agent-1",
		RunID:      "run-1",
		Content:    "Test memory content",
		Kind:       KindInsight,
		Importance: 0.8,
		Embedding:  []byte{0x01, 0x02, 0x03},
		Metadata:   map[string]string{"key": "value"},
		CreatedAt:  now,
	}

	if m.ID != "mem-1" {
		t.Errorf("ID = %q, want %q", m.ID, "mem-1")
	}
	if m.Kind != KindInsight {
		t.Errorf("Kind = %q, want %q", m.Kind, KindInsight)
	}
	if m.Importance != 0.8 {
		t.Errorf("Importance = %f, want 0.8", m.Importance)
	}
	if len(m.Embedding) != 3 {
		t.Errorf("len(Embedding) = %d, want 3", len(m.Embedding))
	}
}

func TestScoredMemory(t *testing.T) {
	t.Parallel()

	sm := ScoredMemory{
		Memory: Memory{
			ID:         "mem-1",
			Content:    "scored memory",
			Kind:       KindObservation,
			Importance: 0.5,
		},
		Score: 0.92,
	}

	if sm.Score != 0.92 {
		t.Errorf("Score = %f, want 0.92", sm.Score)
	}
	if sm.Content != "scored memory" {
		t.Errorf("Content = %q, want %q", sm.Content, "scored memory")
	}
}

func TestRecallRequest(t *testing.T) {
	t.Parallel()

	r := RecallRequest{
		RequestID: "req-1",
		TenantID:  "tenant-1",
		ProjectID: "proj-1",
		Query:     "how to test Go code",
		TopK:      10,
		Kind:      KindDecision,
	}

	if r.TopK != 10 {
		t.Errorf("TopK = %d, want 10", r.TopK)
	}
	if r.Kind != KindDecision {
		t.Errorf("Kind = %q, want %q", r.Kind, KindDecision)
	}
}

func TestRecallResult(t *testing.T) {
	t.Parallel()

	rr := RecallResult{
		RequestID: "req-1",
		ProjectID: "proj-1",
		Query:     "Go testing",
		Results: []ScoredResult{
			{ID: "mem-1", Content: "Use table-driven tests", Kind: "insight", Score: 0.95},
			{ID: "mem-2", Content: "Run go test with -race", Kind: "observation", Score: 0.85},
		},
	}

	if len(rr.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(rr.Results))
	}
	if rr.Results[0].Score != 0.95 {
		t.Errorf("Results[0].Score = %f, want 0.95", rr.Results[0].Score)
	}
	if rr.Error != "" {
		t.Errorf("Error = %q, want empty", rr.Error)
	}
}

func TestRecallResultWithError(t *testing.T) {
	t.Parallel()

	rr := RecallResult{
		RequestID: "req-2",
		ProjectID: "proj-1",
		Query:     "broken query",
		Error:     "embedding service unavailable",
	}

	if rr.Error == "" {
		t.Error("Error should not be empty")
	}
	if len(rr.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0 for error result", len(rr.Results))
	}
}
