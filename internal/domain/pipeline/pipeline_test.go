package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/plan"
)

// --- Validate ---

func TestValidate_Valid(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps: []Step{
			{Name: "Step 1", ModeID: "architect"},
		},
	}
	if err := tmpl.Validate(); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidate_MissingID(t *testing.T) {
	tmpl := Template{
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps:    []Step{{Name: "S", ModeID: "m"}},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestValidate_MissingName(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Protocol: plan.ProtocolSequential,
		Steps:    []Step{{Name: "S", ModeID: "m"}},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestValidate_InvalidProtocol(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: "invalid",
		Steps:    []Step{{Name: "S", ModeID: "m"}},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for invalid protocol")
	}
}

func TestValidate_NoSteps(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for no steps")
	}
}

func TestValidate_StepMissingName(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps:    []Step{{ModeID: "m"}},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for step missing name")
	}
}

func TestValidate_StepMissingMode(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps:    []Step{{Name: "S"}},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for step missing mode")
	}
}

func TestValidate_DAGCycle(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps: []Step{
			{Name: "A", ModeID: "m", DependsOn: []int{1}},
			{Name: "B", ModeID: "m", DependsOn: []int{0}},
		},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for DAG cycle")
	}
}

func TestValidate_DAGSelfRef(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps: []Step{
			{Name: "A", ModeID: "m", DependsOn: []int{0}},
		},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for self-referencing step")
	}
}

func TestValidate_DAGInvalidRef(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps: []Step{
			{Name: "A", ModeID: "m", DependsOn: []int{5}},
		},
	}
	if err := tmpl.Validate(); err == nil {
		t.Fatal("expected error for invalid dependency reference")
	}
}

// --- Instantiate ---

func TestInstantiate_Success(t *testing.T) {
	tmpl := Template{
		ID:          "test",
		Name:        "Test Pipeline",
		Description: "desc",
		Protocol:    plan.ProtocolSequential,
		Steps: []Step{
			{Name: "Plan", ModeID: "architect", DeliverMode: "append"},
			{Name: "Code", ModeID: "coder", DeliverMode: "diff", DependsOn: []int{0}},
		},
	}

	req := InstantiateRequest{
		ProjectID: "proj-1",
		TeamID:    "team-1",
		Bindings: []StepBinding{
			{TaskID: "t1", AgentID: "a1"},
			{TaskID: "t2", AgentID: "a2"},
		},
	}

	result, err := tmpl.Instantiate(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Name != "Test Pipeline" {
		t.Errorf("name = %q, want %q", result.Name, "Test Pipeline")
	}
	if result.ProjectID != "proj-1" {
		t.Errorf("project_id = %q, want %q", result.ProjectID, "proj-1")
	}
	if result.TeamID != "team-1" {
		t.Errorf("team_id = %q, want %q", result.TeamID, "team-1")
	}
	if result.Protocol != plan.ProtocolSequential {
		t.Errorf("protocol = %q, want %q", result.Protocol, plan.ProtocolSequential)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(result.Steps))
	}

	s0 := result.Steps[0]
	if s0.TaskID != "t1" || s0.AgentID != "a1" {
		t.Errorf("step 0: task=%q agent=%q, want t1/a1", s0.TaskID, s0.AgentID)
	}
	if s0.ModeID != "architect" {
		t.Errorf("step 0: mode=%q, want architect", s0.ModeID)
	}
	if len(s0.DependsOn) != 0 {
		t.Errorf("step 0: depends_on = %v, want empty", s0.DependsOn)
	}

	s1 := result.Steps[1]
	if s1.TaskID != "t2" || s1.AgentID != "a2" {
		t.Errorf("step 1: task=%q agent=%q, want t2/a2", s1.TaskID, s1.AgentID)
	}
	if s1.ModeID != "coder" {
		t.Errorf("step 1: mode=%q, want coder", s1.ModeID)
	}
	if len(s1.DependsOn) != 1 || s1.DependsOn[0] != "0" {
		t.Errorf("step 1: depends_on = %v, want [0]", s1.DependsOn)
	}
}

func TestInstantiate_CustomName(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Default Name",
		Protocol: plan.ProtocolSequential,
		Steps:    []Step{{Name: "S", ModeID: "m"}},
	}

	result, err := tmpl.Instantiate(InstantiateRequest{
		ProjectID: "p",
		PlanName:  "Custom Name",
		Bindings:  []StepBinding{{TaskID: "t", AgentID: "a"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Custom Name" {
		t.Errorf("name = %q, want %q", result.Name, "Custom Name")
	}
}

func TestInstantiate_WrongBindingCount(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps: []Step{
			{Name: "A", ModeID: "m"},
			{Name: "B", ModeID: "m"},
		},
	}

	_, err := tmpl.Instantiate(InstantiateRequest{
		ProjectID: "p",
		Bindings:  []StepBinding{{TaskID: "t", AgentID: "a"}},
	})
	if err == nil {
		t.Fatal("expected error for wrong binding count")
	}
}

func TestInstantiate_EmptyBinding(t *testing.T) {
	tmpl := Template{
		ID:       "test",
		Name:     "Test",
		Protocol: plan.ProtocolSequential,
		Steps:    []Step{{Name: "S", ModeID: "m"}},
	}

	_, err := tmpl.Instantiate(InstantiateRequest{
		ProjectID: "p",
		Bindings:  []StepBinding{{TaskID: "", AgentID: "a"}},
	})
	if err == nil {
		t.Fatal("expected error for empty task_id in binding")
	}
}

// --- Presets ---

func TestBuiltinTemplates_AllValid(t *testing.T) {
	templates := BuiltinTemplates()
	if len(templates) != 3 {
		t.Fatalf("expected 3 builtin templates, got %d", len(templates))
	}

	for _, tmpl := range templates {
		if err := tmpl.Validate(); err != nil {
			t.Errorf("builtin template %q failed validation: %v", tmpl.ID, err)
		}
		if !tmpl.Builtin {
			t.Errorf("builtin template %q: Builtin = false", tmpl.ID)
		}
	}
}

func TestBuiltinTemplates_Instantiate(t *testing.T) {
	for _, tmpl := range BuiltinTemplates() {
		bindings := make([]StepBinding, len(tmpl.Steps))
		for i := range bindings {
			bindings[i] = StepBinding{TaskID: "task-" + tmpl.Steps[i].Name, AgentID: "agent-1"}
		}

		result, err := tmpl.Instantiate(InstantiateRequest{
			ProjectID: "proj-1",
			Bindings:  bindings,
		})
		if err != nil {
			t.Fatalf("template %q: instantiate error: %v", tmpl.ID, err)
		}
		if len(result.Steps) != len(tmpl.Steps) {
			t.Errorf("template %q: step count %d, want %d", tmpl.ID, len(result.Steps), len(tmpl.Steps))
		}
	}
}

// --- Loader ---

func TestLoadFromFile_Valid(t *testing.T) {
	content := `
id: custom-pipeline
name: Custom Pipeline
description: A custom pipeline
protocol: sequential
steps:
  - name: Step 1
    mode_id: architect
  - name: Step 2
    mode_id: coder
    depends_on: [0]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tmpl, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl.ID != "custom-pipeline" {
		t.Errorf("id = %q, want %q", tmpl.ID, "custom-pipeline")
	}
	if len(tmpl.Steps) != 2 {
		t.Errorf("steps = %d, want 2", len(tmpl.Steps))
	}
}

func TestLoadFromFile_Invalid(t *testing.T) {
	content := `
name: Missing ID
protocol: sequential
steps:
  - name: Step 1
    mode_id: architect
`
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadFromFile(path)
	if err == nil {
		t.Fatal("expected validation error for missing id")
	}
}

func TestLoadFromFile_NotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadFromDirectory_Valid(t *testing.T) {
	dir := t.TempDir()

	content1 := `
id: pipeline-a
name: Pipeline A
protocol: sequential
steps:
  - name: Step 1
    mode_id: architect
`
	content2 := `
id: pipeline-b
name: Pipeline B
protocol: parallel
max_parallel: 2
steps:
  - name: Review
    mode_id: reviewer
  - name: Audit
    mode_id: security
`
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(content1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yml"), []byte(content2), 0o600); err != nil {
		t.Fatal(err)
	}
	// non-yaml file should be ignored
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore"), 0o600); err != nil {
		t.Fatal(err)
	}

	templates, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(templates) != 2 {
		t.Errorf("loaded %d templates, want 2", len(templates))
	}
}

func TestLoadFromDirectory_MissingDir(t *testing.T) {
	templates, err := LoadFromDirectory("/nonexistent/dir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if templates != nil {
		t.Errorf("expected nil for missing dir, got %v", templates)
	}
}

func TestLoadFromDirectory_SubdirsIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	templates, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("loaded %d templates, want 0", len(templates))
	}
}
