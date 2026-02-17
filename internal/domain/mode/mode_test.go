package mode

import "testing"

func TestValidate_ValidMode(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 3}
	if err := m.Validate(); err != nil {
		t.Fatalf("expected valid mode, got error: %v", err)
	}
}

func TestValidate_MissingID(t *testing.T) {
	m := Mode{Name: "Test", Autonomy: 3}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestValidate_MissingName(t *testing.T) {
	m := Mode{ID: "test", Autonomy: 3}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidate_AutonomyZero(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 0}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for autonomy 0")
	}
}

func TestValidate_AutonomySix(t *testing.T) {
	m := Mode{ID: "test", Name: "Test", Autonomy: 6}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for autonomy 6")
	}
}

func TestValidate_AutonomyBoundaries(t *testing.T) {
	for _, level := range []int{1, 5} {
		m := Mode{ID: "test", Name: "Test", Autonomy: level}
		if err := m.Validate(); err != nil {
			t.Fatalf("autonomy %d should be valid, got error: %v", level, err)
		}
	}
}

func TestBuiltinModes_Count(t *testing.T) {
	modes := BuiltinModes()
	if len(modes) != 8 {
		t.Fatalf("expected 8 built-in modes, got %d", len(modes))
	}
}

func TestBuiltinModes_AllValid(t *testing.T) {
	for _, m := range BuiltinModes() {
		if err := m.Validate(); err != nil {
			t.Fatalf("built-in mode %q failed validation: %v", m.ID, err)
		}
		if !m.Builtin {
			t.Fatalf("built-in mode %q has Builtin=false", m.ID)
		}
	}
}
