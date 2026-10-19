package service_test

import (
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/pipeline"
	"github.com/Strob0t/CodeForge/internal/domain/plan"
	"github.com/Strob0t/CodeForge/internal/domain/run"
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

// KI-17: every built-in pipeline must produce steps whose runs can start: the
// step's mode exists and its deliver mode is one StartRun accepts ("append"
// and "diff" were never valid, so every pipeline step failed to start).
func TestPipeline_BuiltinStepsCanStart(t *testing.T) {
	svc := service.NewPipelineService(service.NewModeService())
	for _, tmpl := range svc.List() {
		req, err := svc.Instantiate(t.Context(), tmpl.ID, pipeline.InstantiateRequest{ProjectID: "p"})
		if err != nil {
			t.Errorf("%s: %v", tmpl.ID, err)
			continue
		}
		for i := range req.Steps {
			step := req.Steps[i]
			start := run.StartRequest{TaskID: "t", AgentID: "a", ProjectID: "p", ModeID: step.ModeID, DeliverMode: run.DeliverMode(step.DeliverMode)}
			if err := start.Validate(); err != nil {
				t.Errorf("%s step %d (%s): %v", tmpl.ID, i, step.ModeID, err)
			}
		}
	}
}

func TestPipeline_BoundaryAnalysisTemplate(t *testing.T) {
	tmpl, err := service.NewPipelineService(service.NewModeService()).Get("boundary-analysis")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(tmpl.Steps) != 1 || tmpl.Steps[0].ModeID != "boundary_analyzer" {
		t.Fatalf("steps = %+v, want one boundary_analyzer step", tmpl.Steps)
	}
}
