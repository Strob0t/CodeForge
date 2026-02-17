package policy

import "testing"

func TestPresetPlanReadonly(t *testing.T) {
	p := PresetPlanReadonly()
	if p.Name != "plan-readonly" {
		t.Errorf("expected name 'plan-readonly', got %q", p.Name)
	}
	if p.Mode != ModePlan {
		t.Errorf("expected mode %q, got %q", ModePlan, p.Mode)
	}
	if len(p.Rules) < 3 {
		t.Errorf("expected at least 3 rules, got %d", len(p.Rules))
	}
	if p.Termination.MaxSteps != 30 {
		t.Errorf("expected max_steps 30, got %d", p.Termination.MaxSteps)
	}
}

func TestPresetHeadlessSafeSandbox(t *testing.T) {
	p := PresetHeadlessSafeSandbox()
	if p.Name != "headless-safe-sandbox" {
		t.Errorf("expected name 'headless-safe-sandbox', got %q", p.Name)
	}
	if p.Mode != ModeDefault {
		t.Errorf("expected mode %q, got %q", ModeDefault, p.Mode)
	}
	if !p.QualityGate.RequireTestsPass {
		t.Error("expected RequireTestsPass=true")
	}
	if !p.QualityGate.RequireLintPass {
		t.Error("expected RequireLintPass=true")
	}
	if !p.QualityGate.RollbackOnGateFail {
		t.Error("expected RollbackOnGateFail=true")
	}
}

func TestPresetHeadlessPermissiveSandbox(t *testing.T) {
	p := PresetHeadlessPermissiveSandbox()
	if p.Name != "headless-permissive-sandbox" {
		t.Errorf("expected name 'headless-permissive-sandbox', got %q", p.Name)
	}
	if p.Mode != ModeAcceptEdits {
		t.Errorf("expected mode %q, got %q", ModeAcceptEdits, p.Mode)
	}
	if !p.QualityGate.RequireTestsPass {
		t.Error("expected RequireTestsPass=true")
	}
	if p.QualityGate.RequireLintPass {
		t.Error("expected RequireLintPass=false")
	}
	if p.Termination.MaxSteps != 100 {
		t.Errorf("expected max_steps 100, got %d", p.Termination.MaxSteps)
	}
}

func TestPresetTrustedMountAutonomous(t *testing.T) {
	p := PresetTrustedMountAutonomous()
	if p.Name != "trusted-mount-autonomous" {
		t.Errorf("expected name 'trusted-mount-autonomous', got %q", p.Name)
	}
	if p.Mode != ModeAcceptEdits {
		t.Errorf("expected mode %q, got %q", ModeAcceptEdits, p.Mode)
	}
	if p.Termination.MaxSteps != 200 {
		t.Errorf("expected max_steps 200, got %d", p.Termination.MaxSteps)
	}
	if p.Termination.MaxCost != 50.0 {
		t.Errorf("expected max_cost 50.0, got %f", p.Termination.MaxCost)
	}
}

func TestPresetByName(t *testing.T) {
	for _, name := range PresetNames() {
		p, ok := PresetByName(name)
		if !ok {
			t.Errorf("preset %q not found", name)
		}
		if p.Name != name {
			t.Errorf("expected name %q, got %q", name, p.Name)
		}
	}
}

func TestPresetByNameUnknown(t *testing.T) {
	_, ok := PresetByName("nonexistent")
	if ok {
		t.Error("expected false for unknown preset")
	}
}

func TestPresetNames(t *testing.T) {
	names := PresetNames()
	if len(names) != 4 {
		t.Fatalf("expected 4 preset names, got %d", len(names))
	}
}

func TestAllPresetsValidate(t *testing.T) {
	for _, name := range PresetNames() {
		p, _ := PresetByName(name)
		if err := p.Validate(); err != nil {
			t.Errorf("preset %q failed validation: %v", name, err)
		}
	}
}
