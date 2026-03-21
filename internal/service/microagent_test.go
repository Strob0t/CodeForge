package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Strob0t/CodeForge/internal/domain"
	"github.com/Strob0t/CodeForge/internal/domain/microagent"
)

// ---------------------------------------------------------------------------
// Fake store for MicroagentService CRUD tests
// ---------------------------------------------------------------------------

// fakeMicroagentStore embeds mockStore for full database.Store interface
// satisfaction and overrides the five microagent methods with in-memory logic.
type fakeMicroagentStore struct {
	mockStore
	agents map[string]*microagent.Microagent
	nextID int
}

func newFakeMicroagentStore() *fakeMicroagentStore {
	return &fakeMicroagentStore{
		agents: make(map[string]*microagent.Microagent),
	}
}

func (f *fakeMicroagentStore) CreateMicroagent(_ context.Context, m *microagent.Microagent) error {
	f.nextID++
	m.ID = "ma-" + strings.Repeat("0", 3) + string(rune('0'+f.nextID))
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	cp := *m
	f.agents[m.ID] = &cp
	return nil
}

func (f *fakeMicroagentStore) GetMicroagent(_ context.Context, id string) (*microagent.Microagent, error) {
	m, ok := f.agents[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *m
	return &cp, nil
}

func (f *fakeMicroagentStore) ListMicroagents(_ context.Context, projectID string) ([]microagent.Microagent, error) {
	var result []microagent.Microagent
	for _, m := range f.agents {
		if m.ProjectID == projectID || m.ProjectID == "" {
			result = append(result, *m)
		}
	}
	return result, nil
}

func (f *fakeMicroagentStore) UpdateMicroagent(_ context.Context, m *microagent.Microagent) error {
	if _, ok := f.agents[m.ID]; !ok {
		return domain.ErrNotFound
	}
	m.UpdatedAt = time.Now()
	cp := *m
	f.agents[m.ID] = &cp
	return nil
}

func (f *fakeMicroagentStore) DeleteMicroagent(_ context.Context, id string) error {
	if _, ok := f.agents[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.agents, id)
	return nil
}

// ---------------------------------------------------------------------------
// Trigger-matching tests (from Task 1)
// ---------------------------------------------------------------------------

func TestMatchesTrigger_ReDoSProtection(t *testing.T) {
	t.Parallel()

	// Classic ReDoS pattern: catastrophic backtracking on non-matching input.
	// Go's regexp uses a linear-time engine so it won't hang, but we still
	// verify the function completes quickly with our safety bounds in place.
	pattern := "(a+)+b"
	input := strings.Repeat("a", 10_000) // long non-matching input

	done := make(chan bool, 1)
	go func() {
		matchesTrigger(pattern, input)
		done <- true
	}()

	select {
	case <-done:
		// completed in time
	case <-time.After(5 * time.Second):
		t.Fatal("matchesTrigger did not complete within 5s -- possible ReDoS")
	}
}

func TestMatchesTrigger_InvalidRegex(t *testing.T) {
	t.Parallel()

	// Invalid regex pattern (unclosed bracket) must return false, not panic.
	got := matchesTrigger("[invalid", "hello world")
	if got {
		t.Error("matchesTrigger([invalid, ...) = true, want false")
	}
}

func TestMatchesTrigger_ValidSubstring(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"docker", "How do I use Docker?", true}, // case-insensitive
		{"python", "Working with Python files", true},
		{"rust", "Go programming guide", false},
		{"", "anything", true}, // empty pattern always matches via Contains; blocked by Validate()
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			t.Parallel()
			got := matchesTrigger(tt.pattern, tt.text)
			if got != tt.want {
				t.Errorf("matchesTrigger(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestMatchesTrigger_ValidRegex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		text    string
		want    bool
	}{
		{
			name:    "caret prefix matches",
			pattern: "^hello",
			text:    "hello world",
			want:    true,
		},
		{
			name:    "caret prefix no match",
			pattern: "^hello",
			text:    "say hello",
			want:    false,
		},
		{
			name:    "paren prefix matches",
			pattern: "(error|warning)",
			text:    "found an error in code",
			want:    true,
		},
		{
			name:    "paren prefix no match",
			pattern: "(error|warning)",
			text:    "all good",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := matchesTrigger(tt.pattern, tt.text)
			if got != tt.want {
				t.Errorf("matchesTrigger(%q, %q) = %v, want %v", tt.pattern, tt.text, got, tt.want)
			}
		})
	}
}

func TestMatchesTrigger_PatternTooLong(t *testing.T) {
	t.Parallel()

	// Pattern exceeding MaxTriggerPatternLength must be rejected.
	longPattern := "^" + strings.Repeat("a", microagent.MaxTriggerPatternLength+1)
	got := matchesTrigger(longPattern, "aaa")
	if got {
		t.Error("matchesTrigger with oversized pattern = true, want false")
	}
}

func TestMatchesTrigger_InputTruncation(t *testing.T) {
	t.Parallel()

	// Pattern that matches text only beyond the 10K truncation boundary.
	// The match target "MARKER" is placed past the limit.
	input := strings.Repeat("x", maxTriggerInputLength) + "MARKER"
	got := matchesTrigger("^.*MARKER", input)
	if got {
		t.Error("matchesTrigger should not find MARKER beyond truncation limit")
	}

	// Same pattern with MARKER within the limit.
	shortInput := strings.Repeat("x", 100) + "MARKER"
	got2 := matchesTrigger("(MARKER)", shortInput)
	if !got2 {
		t.Error("matchesTrigger should find MARKER within truncation limit")
	}
}

func TestCreateRequest_Validate_RegexPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pattern string
		wantErr string
	}{
		{
			name:    "valid substring pattern",
			pattern: "docker",
			wantErr: "",
		},
		{
			name:    "valid caret regex",
			pattern: "^test_.*\\.py$",
			wantErr: "",
		},
		{
			name:    "valid paren regex",
			pattern: "(error|warning|critical)",
			wantErr: "",
		},
		{
			name:    "invalid regex - unclosed bracket",
			pattern: "^[invalid",
			wantErr: "invalid trigger_pattern regex:",
		},
		{
			name:    "invalid regex - bad repetition",
			pattern: "(abc",
			wantErr: "invalid trigger_pattern regex:",
		},
		{
			name:    "pattern too long",
			pattern: strings.Repeat("a", microagent.MaxTriggerPatternLength+1),
			wantErr: "trigger_pattern exceeds maximum length of 512",
		},
		{
			name:    "pattern at max length (valid)",
			pattern: strings.Repeat("a", microagent.MaxTriggerPatternLength),
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := microagent.CreateRequest{
				Name:           "test-agent",
				Type:           microagent.TypeKnowledge,
				TriggerPattern: tt.pattern,
				Prompt:         "test prompt",
			}
			err := req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() = nil, want error containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Validate() = %q, want error containing %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CRUD tests (F18-D2)
// ---------------------------------------------------------------------------

func TestMicroagentService_Create_HappyPath(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	req := &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "docker-helper",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "docker",
		Description:    "Helps with Docker questions",
		Prompt:         "You are a Docker specialist.",
	}

	got, err := svc.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if got.Name != "docker-helper" {
		t.Errorf("Name = %q, want %q", got.Name, "docker-helper")
	}
	if got.ProjectID != "proj-1" {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, "proj-1")
	}
	if got.Type != microagent.TypeKnowledge {
		t.Errorf("Type = %q, want %q", got.Type, microagent.TypeKnowledge)
	}
	if got.TriggerPattern != "docker" {
		t.Errorf("TriggerPattern = %q, want %q", got.TriggerPattern, "docker")
	}
	if got.Prompt != "You are a Docker specialist." {
		t.Errorf("Prompt = %q, want %q", got.Prompt, "You are a Docker specialist.")
	}
	if !got.Enabled {
		t.Error("expected Enabled = true for new microagent")
	}
}

func TestMicroagentService_Create_ValidationError(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	tests := []struct {
		name    string
		req     *microagent.CreateRequest
		wantErr string
	}{
		{
			name: "empty name",
			req: &microagent.CreateRequest{
				Name:           "",
				Type:           microagent.TypeKnowledge,
				TriggerPattern: "docker",
				Prompt:         "test prompt",
			},
			wantErr: "name is required",
		},
		{
			name: "empty trigger pattern",
			req: &microagent.CreateRequest{
				Name:           "test",
				Type:           microagent.TypeKnowledge,
				TriggerPattern: "",
				Prompt:         "test prompt",
			},
			wantErr: "trigger_pattern is required",
		},
		{
			name: "empty prompt",
			req: &microagent.CreateRequest{
				Name:           "test",
				Type:           microagent.TypeKnowledge,
				TriggerPattern: "docker",
				Prompt:         "",
			},
			wantErr: "prompt is required",
		},
		{
			name: "invalid type",
			req: &microagent.CreateRequest{
				Name:           "test",
				Type:           "bogus",
				TriggerPattern: "docker",
				Prompt:         "test prompt",
			},
			wantErr: "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.Create(context.Background(), tt.req)
			if err == nil {
				t.Fatalf("Create() = %v, want error containing %q", got, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Create() error = %q, want containing %q", err.Error(), tt.wantErr)
			}
			if got != nil {
				t.Errorf("Create() returned non-nil microagent on error: %+v", got)
			}
		})
	}

	// Verify nothing was persisted.
	all, err := store.ListMicroagents(context.Background(), "")
	if err != nil {
		t.Fatalf("ListMicroagents() error = %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 persisted microagents after validation errors, got %d", len(all))
	}
}

func TestMicroagentService_Get_Found(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	created, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "auth-helper",
		Type:           microagent.TypeRepo,
		TriggerPattern: "auth",
		Prompt:         "You handle auth.",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("Get().ID = %q, want %q", got.ID, created.ID)
	}
	if got.Name != "auth-helper" {
		t.Errorf("Get().Name = %q, want %q", got.Name, "auth-helper")
	}
}

func TestMicroagentService_Get_NotFound(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	_, err := svc.Get(context.Background(), "nonexistent-id")
	if err == nil {
		t.Fatal("Get() expected error for nonexistent ID, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() error = %q, want containing %q", err.Error(), "not found")
	}
}

func TestMicroagentService_List(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	// Create two agents for proj-1 and one for proj-2.
	for _, req := range []*microagent.CreateRequest{
		{ProjectID: "proj-1", Name: "a1", Type: microagent.TypeKnowledge, TriggerPattern: "go", Prompt: "p1"},
		{ProjectID: "proj-1", Name: "a2", Type: microagent.TypeTask, TriggerPattern: "rust", Prompt: "p2"},
		{ProjectID: "proj-2", Name: "a3", Type: microagent.TypeRepo, TriggerPattern: "python", Prompt: "p3"},
	} {
		if _, err := svc.Create(context.Background(), req); err != nil {
			t.Fatalf("Create(%s) error = %v", req.Name, err)
		}
	}

	got, err := svc.List(context.Background(), "proj-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("List(proj-1) returned %d agents, want 2", len(got))
	}

	got2, err := svc.List(context.Background(), "proj-2")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got2) != 1 {
		t.Errorf("List(proj-2) returned %d agents, want 1", len(got2))
	}
}

func TestMicroagentService_Update_PartialFields(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	created, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "original-name",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "docker",
		Description:    "original desc",
		Prompt:         "original prompt",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update only the Name; other fields should remain unchanged.
	updated, err := svc.Update(context.Background(), created.ID, microagent.UpdateRequest{
		Name: "new-name",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.Name != "new-name" {
		t.Errorf("Name = %q, want %q", updated.Name, "new-name")
	}
	if updated.TriggerPattern != "docker" {
		t.Errorf("TriggerPattern = %q, want %q (unchanged)", updated.TriggerPattern, "docker")
	}
	if updated.Description != "original desc" {
		t.Errorf("Description = %q, want %q (unchanged)", updated.Description, "original desc")
	}
	if updated.Prompt != "original prompt" {
		t.Errorf("Prompt = %q, want %q (unchanged)", updated.Prompt, "original prompt")
	}
}

func TestMicroagentService_Update_EnableDisable(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	created, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "toggle-agent",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "test",
		Prompt:         "test prompt",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !created.Enabled {
		t.Fatal("new microagent should be enabled by default")
	}

	// Disable via *bool.
	disabled := false
	updated, err := svc.Update(context.Background(), created.ID, microagent.UpdateRequest{
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("Update(disable) error = %v", err)
	}
	if updated.Enabled {
		t.Error("expected Enabled = false after disable update")
	}

	// Re-enable via *bool.
	enabled := true
	updated2, err := svc.Update(context.Background(), created.ID, microagent.UpdateRequest{
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Update(enable) error = %v", err)
	}
	if !updated2.Enabled {
		t.Error("expected Enabled = true after re-enable update")
	}
}

func TestMicroagentService_Delete_HappyPath(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	created, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "ephemeral",
		Type:           microagent.TypeTask,
		TriggerPattern: "cleanup",
		Prompt:         "clean up",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it is gone.
	_, err = svc.Get(context.Background(), created.ID)
	if err == nil {
		t.Error("Get() after Delete() should return error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Get() after Delete() error = %q, want containing %q", err.Error(), "not found")
	}
}

func TestMicroagentService_Match_SubstringAndRegex(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	// Substring trigger.
	if _, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "docker-agent",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "docker",
		Prompt:         "Docker specialist.",
	}); err != nil {
		t.Fatalf("Create(docker) error = %v", err)
	}

	// Regex trigger.
	if _, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "error-agent",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "(error|warning)",
		Prompt:         "Error handler.",
	}); err != nil {
		t.Fatalf("Create(error) error = %v", err)
	}

	// Non-matching agent (different pattern).
	if _, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "rust-agent",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "rust",
		Prompt:         "Rust specialist.",
	}); err != nil {
		t.Fatalf("Create(rust) error = %v", err)
	}

	tests := []struct {
		name      string
		text      string
		wantCount int
		wantNames []string
	}{
		{
			name:      "substring match docker",
			text:      "How do I build a Docker image?",
			wantCount: 1,
			wantNames: []string{"docker-agent"},
		},
		{
			name:      "regex match error",
			text:      "I got an error in my code",
			wantCount: 1,
			wantNames: []string{"error-agent"},
		},
		{
			name:      "no match",
			text:      "Tell me about Go generics",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			matched, err := svc.Match(context.Background(), "proj-1", tt.text)
			if err != nil {
				t.Fatalf("Match() error = %v", err)
			}
			if len(matched) != tt.wantCount {
				t.Errorf("Match() returned %d agents, want %d", len(matched), tt.wantCount)
			}
			for _, wantName := range tt.wantNames {
				found := false
				for _, m := range matched {
					if m.Name == wantName {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Match() missing expected agent %q", wantName)
				}
			}
		})
	}
}

func TestMicroagentService_Match_DisabledSkipped(t *testing.T) {
	t.Parallel()

	store := newFakeMicroagentStore()
	svc := NewMicroagentService(store)

	created, err := svc.Create(context.Background(), &microagent.CreateRequest{
		ProjectID:      "proj-1",
		Name:           "docker-agent",
		Type:           microagent.TypeKnowledge,
		TriggerPattern: "docker",
		Prompt:         "Docker specialist.",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify it matches when enabled.
	matched, err := svc.Match(context.Background(), "proj-1", "Help with Docker")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("Match() returned %d agents, want 1 (enabled)", len(matched))
	}

	// Disable it.
	disabled := false
	if _, err := svc.Update(context.Background(), created.ID, microagent.UpdateRequest{
		Enabled: &disabled,
	}); err != nil {
		t.Fatalf("Update(disable) error = %v", err)
	}

	// Verify it is skipped when disabled.
	matched2, err := svc.Match(context.Background(), "proj-1", "Help with Docker")
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if len(matched2) != 0 {
		t.Errorf("Match() returned %d agents, want 0 (disabled agent should be skipped)", len(matched2))
	}
}
