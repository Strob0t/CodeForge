package service

import (
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/mode"
)

func TestNewModeService_LoadsBuiltins(t *testing.T) {
	s := NewModeService()
	modes := s.List()
	if len(modes) != 8 {
		t.Fatalf("expected 8 built-in modes, got %d", len(modes))
	}
}

func TestModeService_List(t *testing.T) {
	s := NewModeService()
	modes := s.List()
	if len(modes) == 0 {
		t.Fatal("expected non-empty mode list")
	}
	for _, m := range modes {
		if m.ID == "" {
			t.Fatal("mode in list has empty ID")
		}
	}
}

func TestModeService_Get_Existing(t *testing.T) {
	s := NewModeService()
	m, err := s.Get("coder")
	if err != nil {
		t.Fatalf("expected to find coder mode, got error: %v", err)
	}
	if m.Name != "Coder" {
		t.Fatalf("expected name Coder, got %q", m.Name)
	}
}

func TestModeService_Get_NotFound(t *testing.T) {
	s := NewModeService()
	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent mode")
	}
}

func TestModeService_Register_Custom(t *testing.T) {
	s := NewModeService()
	custom := mode.Mode{
		ID:       "custom-mode",
		Name:     "Custom",
		Autonomy: 3,
		Tools:    []string{"Read"},
	}
	if err := s.Register(&custom); err != nil {
		t.Fatalf("expected register to succeed, got error: %v", err)
	}
	if len(s.List()) != 9 {
		t.Fatalf("expected 9 modes after registration, got %d", len(s.List()))
	}
}

func TestModeService_Register_ValidationError(t *testing.T) {
	s := NewModeService()
	invalid := mode.Mode{ID: "", Name: "Bad", Autonomy: 3}
	if err := s.Register(&invalid); err == nil {
		t.Fatal("expected validation error for empty ID")
	}
}

func TestModeService_Register_CannotOverwriteBuiltin(t *testing.T) {
	s := NewModeService()
	override := mode.Mode{ID: "coder", Name: "Evil Coder", Autonomy: 3}
	if err := s.Register(&override); err == nil {
		t.Fatal("expected error when overwriting built-in mode")
	}
}

func TestModeService_Register_ThenGet(t *testing.T) {
	s := NewModeService()
	custom := mode.Mode{
		ID:          "my-agent",
		Name:        "My Agent",
		Description: "A custom agent mode",
		Autonomy:    4,
		Tools:       []string{"Read", "Write"},
		LLMScenario: "default",
	}
	if err := s.Register(&custom); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	m, err := s.Get("my-agent")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if m.Name != "My Agent" {
		t.Fatalf("expected name 'My Agent', got %q", m.Name)
	}
	if m.Autonomy != 4 {
		t.Fatalf("expected autonomy 4, got %d", m.Autonomy)
	}
}
