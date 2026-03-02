package service_test

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/service"
)

func TestPipeline_ListAndRegister(t *testing.T) {
	modes := service.NewModeService()
	svc := service.NewPipelineService(modes)

	// 1. Built-in templates are loaded
	builtins := svc.List()
	if len(builtins) == 0 {
		t.Fatal("expected built-in templates, got 0")
	}
	initialCount := len(builtins)

	// 2. Get unknown template returns error
	_, err := svc.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown template")
	}

	// 3. Get known built-in succeeds
	first := builtins[0]
	got, err := svc.Get(first.ID)
	if err != nil {
		t.Fatalf("Get(%q): %v", first.ID, err)
	}
	if got.Name != first.Name {
		t.Errorf("expected name %q, got %q", first.Name, got.Name)
	}

	// 4. Register valid custom template
	custom := &pipeline.Template{
		ID:       "custom-pipeline",
		Name:     "Custom Pipeline",
		Protocol: plan.ProtocolSequential,
		Steps: []pipeline.Step{
			{Name: "Code", ModeID: "coder"},
		},
	}
	if err := svc.Register(custom); err != nil {
		t.Fatalf("Register custom: %v", err)
	}
	if len(svc.List()) != initialCount+1 {
		t.Errorf("expected %d templates after register, got %d", initialCount+1, len(svc.List()))
	}

	// 5. Cannot overwrite built-in
	overwrite := &pipeline.Template{
		ID:       first.ID,
		Name:     "Override Attempt",
		Protocol: plan.ProtocolSequential,
		Steps: []pipeline.Step{
			{Name: "X", ModeID: "coder"},
		},
	}
	err = svc.Register(overwrite)
	if err == nil {
		t.Fatal("expected error when overwriting built-in template")
	}
	if !strings.Contains(err.Error(), "cannot overwrite built-in") {
		t.Errorf("expected 'cannot overwrite built-in' error, got: %s", err.Error())
	}

	// 6. Validation error for missing name
	invalid := &pipeline.Template{
		ID:       "bad",
		Name:     "",
		Protocol: plan.ProtocolSequential,
		Steps: []pipeline.Step{
			{Name: "X", ModeID: "coder"},
		},
	}
	err = svc.Register(invalid)
	if err == nil {
		t.Fatal("expected validation error for missing name")
	}
}
