package a2a

import (
	"context"
	"encoding/json"
	"testing"

	a2adomain "github.com/Strob0t/CodeForge/internal/domain/a2a"
)

func TestNewCardBuilder(t *testing.T) {
	t.Parallel()

	modes := []ModeInfo{
		{ID: "coder", Name: "Coder", Description: "Writes code"},
		{ID: "reviewer", Name: "Reviewer", Description: "Reviews code"},
	}
	builder := NewCardBuilder("https://codeforge.example.com", modes, "1.0.0")

	if builder == nil {
		t.Fatal("NewCardBuilder() returned nil")
	}
	if builder.baseURL != "https://codeforge.example.com" {
		t.Errorf("baseURL = %q, want %q", builder.baseURL, "https://codeforge.example.com")
	}
	if builder.version != "1.0.0" {
		t.Errorf("version = %q, want %q", builder.version, "1.0.0")
	}
	if len(builder.modes) != 2 {
		t.Errorf("len(modes) = %d, want 2", len(builder.modes))
	}
}

func TestCardBuilderCard(t *testing.T) {
	t.Parallel()

	modes := []ModeInfo{
		{ID: "coder", Name: "Coder", Description: "Writes code"},
		{ID: "reviewer", Name: "Reviewer", Description: "Reviews code"},
		{ID: "debugger", Name: "Debugger", Description: "Debugs issues"},
	}
	builder := NewCardBuilder("https://codeforge.example.com", modes, "2.0.0")

	card, err := builder.Card(context.Background())
	if err != nil {
		t.Fatalf("Card() error = %v", err)
	}
	if card == nil {
		t.Fatal("Card() returned nil")
	}

	if card.Name != "CodeForge" {
		t.Errorf("Name = %q, want %q", card.Name, "CodeForge")
	}
	if card.Description != "AI coding agent orchestration platform" {
		t.Errorf("Description = %q, want %q", card.Description, "AI coding agent orchestration platform")
	}
	if card.URL != "https://codeforge.example.com" {
		t.Errorf("URL = %q, want %q", card.URL, "https://codeforge.example.com")
	}
	if card.Version != "2.0.0" {
		t.Errorf("Version = %q, want %q", card.Version, "2.0.0")
	}
	if len(card.Skills) != 3 {
		t.Fatalf("len(Skills) = %d, want 3", len(card.Skills))
	}

	// Verify skills match modes.
	for i, skill := range card.Skills {
		if skill.ID != modes[i].ID {
			t.Errorf("Skills[%d].ID = %q, want %q", i, skill.ID, modes[i].ID)
		}
		if skill.Name != modes[i].Name {
			t.Errorf("Skills[%d].Name = %q, want %q", i, skill.Name, modes[i].Name)
		}
		if skill.Description != modes[i].Description {
			t.Errorf("Skills[%d].Description = %q, want %q", i, skill.Description, modes[i].Description)
		}
		if len(skill.Tags) != 1 || skill.Tags[0] != "codeforge" {
			t.Errorf("Skills[%d].Tags = %v, want [\"codeforge\"]", i, skill.Tags)
		}
	}

	// Verify capabilities.
	if card.Capabilities.Streaming {
		t.Error("Capabilities.Streaming = true, want false")
	}

	// Verify provider.
	if card.Provider == nil {
		t.Fatal("Provider is nil")
	}
	if card.Provider.Org != "CodeForge" {
		t.Errorf("Provider.Org = %q, want %q", card.Provider.Org, "CodeForge")
	}
}

func TestCardBuilderNoModes(t *testing.T) {
	t.Parallel()

	builder := NewCardBuilder("https://example.com", nil, "1.0.0")

	card, err := builder.Card(context.Background())
	if err != nil {
		t.Fatalf("Card() error = %v", err)
	}

	if len(card.Skills) != 0 {
		t.Errorf("len(Skills) = %d, want 0 for no modes", len(card.Skills))
	}
}

func TestCardBuilderSecurity(t *testing.T) {
	t.Parallel()

	builder := NewCardBuilder("https://example.com", nil, "1.0.0")
	card, err := builder.Card(context.Background())
	if err != nil {
		t.Fatalf("Card() error = %v", err)
	}

	if len(card.Security) != 1 {
		t.Fatalf("len(Security) = %d, want 1", len(card.Security))
	}

	// Verify the security requirement references "apiKey".
	if _, ok := card.Security[0]["apiKey"]; !ok {
		t.Error("Security[0] should contain 'apiKey' requirement")
	}

	// Verify SecuritySchemes has "apiKey".
	if card.SecuritySchemes == nil {
		t.Fatal("SecuritySchemes is nil")
	}
	if _, ok := card.SecuritySchemes["apiKey"]; !ok {
		t.Error("SecuritySchemes should contain 'apiKey'")
	}
}

func TestModeInfoFields(t *testing.T) {
	t.Parallel()

	mi := ModeInfo{
		ID:          "architect",
		Name:        "Architect",
		Description: "Designs system architecture",
	}

	if mi.ID != "architect" {
		t.Errorf("ID = %q, want %q", mi.ID, "architect")
	}
	if mi.Name != "Architect" {
		t.Errorf("Name = %q, want %q", mi.Name, "Architect")
	}
	if mi.Description != "Designs system architecture" {
		t.Errorf("Description = %q, want %q", mi.Description, "Designs system architecture")
	}
}

// --- Domain conversion tests ---

func TestSdkToDomainTask(t *testing.T) {
	t.Parallel()

	dt := a2adomain.NewA2ATask("task-1")
	dt.State = a2adomain.TaskStateWorking
	dt.Direction = a2adomain.DirectionInbound

	if dt.ID != "task-1" {
		t.Errorf("ID = %q, want %q", dt.ID, "task-1")
	}
	if dt.State != a2adomain.TaskStateWorking {
		t.Errorf("State = %q, want %q", dt.State, a2adomain.TaskStateWorking)
	}
}

func TestDomainToSDKTask(t *testing.T) {
	t.Parallel()

	dt := &a2adomain.A2ATask{
		ID:           "test-task",
		ContextID:    "ctx-1",
		State:        a2adomain.TaskStateCompleted,
		Direction:    a2adomain.DirectionOutbound,
		ErrorMessage: "",
		History:      []byte("[]"),
		Artifacts:    []byte("[]"),
	}

	sdkTask := domainToSDKTask(dt)

	if string(sdkTask.ID) != "test-task" {
		t.Errorf("ID = %q, want %q", sdkTask.ID, "test-task")
	}
	if sdkTask.ContextID != "ctx-1" {
		t.Errorf("ContextID = %q, want %q", sdkTask.ContextID, "ctx-1")
	}
	if string(sdkTask.Status.State) != "completed" {
		t.Errorf("Status.State = %q, want %q", sdkTask.Status.State, "completed")
	}
	if sdkTask.Status.Message != nil {
		t.Error("Status.Message should be nil when no error message")
	}
}

func TestDomainToSDKTaskWithError(t *testing.T) {
	t.Parallel()

	dt := &a2adomain.A2ATask{
		ID:           "err-task",
		State:        a2adomain.TaskStateFailed,
		Direction:    a2adomain.DirectionInbound,
		ErrorMessage: "something went wrong",
		History:      []byte("[]"),
		Artifacts:    []byte("[]"),
	}

	sdkTask := domainToSDKTask(dt)

	if sdkTask.Status.Message == nil {
		t.Fatal("Status.Message should not be nil when error message is set")
	}
}

func TestDomainToSDKTaskWithHistory(t *testing.T) {
	t.Parallel()

	// History needs to be valid JSON with more than 2 bytes to be parsed.
	history := `[{"role":"user","parts":[{"text":"hello"}]}]`

	dt := &a2adomain.A2ATask{
		ID:        "hist-task",
		State:     a2adomain.TaskStateCompleted,
		Direction: a2adomain.DirectionInbound,
		History:   []byte(history),
		Artifacts: []byte("[]"),
	}

	sdkTask := domainToSDKTask(dt)

	// History should be populated (if unmarshal succeeds).
	// The exact content depends on the SDK types; just verify no panic.
	_ = sdkTask.History
}

func TestSdkToDomainTaskConversion(t *testing.T) {
	t.Parallel()

	dt := &a2adomain.A2ATask{
		ID:           "roundtrip-task",
		ContextID:    "ctx-rt",
		State:        a2adomain.TaskStateWorking,
		Direction:    a2adomain.DirectionInbound,
		ErrorMessage: "partial failure",
		History:      []byte(`[{"role":"user","parts":[{"text":"test"}]}]`),
		Artifacts:    []byte(`[{"parts":[{"text":"output"}]}]`),
	}

	sdkTask := domainToSDKTask(dt)
	roundTrip := sdkToDomainTask(sdkTask, "inbound")

	if roundTrip.ID != dt.ID {
		t.Errorf("ID = %q, want %q", roundTrip.ID, dt.ID)
	}
	if roundTrip.ContextID != dt.ContextID {
		t.Errorf("ContextID = %q, want %q", roundTrip.ContextID, dt.ContextID)
	}
	if roundTrip.Direction != a2adomain.DirectionInbound {
		t.Errorf("Direction = %q, want %q", roundTrip.Direction, a2adomain.DirectionInbound)
	}
}

func TestNewTaskStoreAdapter(t *testing.T) {
	t.Parallel()

	adapter := NewTaskStoreAdapter(nil)
	if adapter == nil {
		t.Fatal("NewTaskStoreAdapter() returned nil")
	}
}

func TestNewExecutor(t *testing.T) {
	t.Parallel()

	exec := NewExecutor(nil, nil, nil, []string{"coder", "reviewer"})
	if exec == nil {
		t.Fatal("NewExecutor() returned nil")
	}
	if len(exec.modes) != 2 {
		t.Errorf("len(modes) = %d, want 2", len(exec.modes))
	}
}

func TestDomainTaskValidStates(t *testing.T) {
	t.Parallel()

	validStates := []a2adomain.TaskState{
		a2adomain.TaskStateSubmitted,
		a2adomain.TaskStateWorking,
		a2adomain.TaskStateCompleted,
		a2adomain.TaskStateFailed,
		a2adomain.TaskStateCanceled,
		a2adomain.TaskStateRejected,
		a2adomain.TaskStateInputRequired,
		a2adomain.TaskStateAuthRequired,
	}

	for _, state := range validStates {
		if !a2adomain.IsValidState(state) {
			t.Errorf("IsValidState(%q) = false, want true", state)
		}
	}

	if a2adomain.IsValidState(a2adomain.TaskState("bogus")) {
		t.Error("IsValidState(\"bogus\") = true, want false")
	}
}

func TestDomainTaskValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		task    a2adomain.A2ATask
		wantErr bool
	}{
		{
			name: "valid task",
			task: a2adomain.A2ATask{
				ID:        "t-1",
				State:     a2adomain.TaskStateSubmitted,
				Direction: a2adomain.DirectionInbound,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			task: a2adomain.A2ATask{
				State:     a2adomain.TaskStateWorking,
				Direction: a2adomain.DirectionInbound,
			},
			wantErr: true,
		},
		{
			name: "invalid state",
			task: a2adomain.A2ATask{
				ID:        "t-2",
				State:     a2adomain.TaskState("invalid"),
				Direction: a2adomain.DirectionInbound,
			},
			wantErr: true,
		},
		{
			name: "invalid direction",
			task: a2adomain.A2ATask{
				ID:        "t-3",
				State:     a2adomain.TaskStateWorking,
				Direction: a2adomain.Direction("sideways"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.task.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewA2ATaskDefaults(t *testing.T) {
	t.Parallel()

	task := a2adomain.NewA2ATask("my-task")

	if task.ID != "my-task" {
		t.Errorf("ID = %q, want %q", task.ID, "my-task")
	}
	if task.State != a2adomain.TaskStateSubmitted {
		t.Errorf("State = %q, want %q", task.State, a2adomain.TaskStateSubmitted)
	}
	if task.Direction != a2adomain.DirectionInbound {
		t.Errorf("Direction = %q, want %q", task.Direction, a2adomain.DirectionInbound)
	}
	if task.TrustOrigin != "a2a" {
		t.Errorf("TrustOrigin = %q, want %q", task.TrustOrigin, "a2a")
	}
	if task.TrustLevel != "untrusted" {
		t.Errorf("TrustLevel = %q, want %q", task.TrustLevel, "untrusted")
	}
	if task.Version != 1 {
		t.Errorf("Version = %d, want 1", task.Version)
	}
	if task.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
	if string(task.History) != "[]" {
		t.Errorf("History = %q, want %q", string(task.History), "[]")
	}
	if string(task.Artifacts) != "[]" {
		t.Errorf("Artifacts = %q, want %q", string(task.Artifacts), "[]")
	}
	if task.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if task.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}

	// Validate should pass for default task.
	if err := task.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil for default task", err)
	}
}

func TestDomainTaskJSONRoundTrip(t *testing.T) {
	t.Parallel()

	task := a2adomain.NewA2ATask("json-rt")
	task.State = a2adomain.TaskStateWorking
	task.SkillID = "coder"

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded a2adomain.A2ATask
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != task.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, task.ID)
	}
	if decoded.State != task.State {
		t.Errorf("State = %q, want %q", decoded.State, task.State)
	}
	if decoded.SkillID != task.SkillID {
		t.Errorf("SkillID = %q, want %q", decoded.SkillID, task.SkillID)
	}
}
