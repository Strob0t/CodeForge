package microagent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMicroagentValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		agent   Microagent
		wantErr string
	}{
		{
			name: "valid microagent",
			agent: Microagent{
				Name: "python-testing",
				Type: TypeKnowledge,
			},
			wantErr: "",
		},
		{
			name: "valid repo type",
			agent: Microagent{
				Name: "repo-helper",
				Type: TypeRepo,
			},
			wantErr: "",
		},
		{
			name: "valid task type",
			agent: Microagent{
				Name: "task-runner",
				Type: TypeTask,
			},
			wantErr: "",
		},
		{
			name: "missing name",
			agent: Microagent{
				Type: TypeKnowledge,
			},
			wantErr: "name is required",
		},
		{
			name: "invalid type",
			agent: Microagent{
				Name: "bad-type",
				Type: Type("invalid"),
			},
			wantErr: "invalid type",
		},
		{
			name: "empty type",
			agent: Microagent{
				Name: "no-type",
				Type: Type(""),
			},
			wantErr: "invalid type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.agent.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() = nil, want error containing %q", tt.wantErr)
				} else if err.Error() != tt.wantErr {
					t.Errorf("Validate() = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

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
				Name:           "test-agent",
				Type:           TypeKnowledge,
				TriggerPattern: "*.py",
				Prompt:         "When working with Python files...",
			},
			wantErr: "",
		},
		{
			name: "valid with project_id",
			req: CreateRequest{
				ProjectID:      "proj-1",
				Name:           "test-agent",
				Type:           TypeRepo,
				TriggerPattern: "Dockerfile",
				Prompt:         "When working with Docker...",
			},
			wantErr: "",
		},
		{
			name: "missing name",
			req: CreateRequest{
				Type:           TypeKnowledge,
				TriggerPattern: "*.py",
				Prompt:         "prompt",
			},
			wantErr: "name is required",
		},
		{
			name: "invalid type",
			req: CreateRequest{
				Name:           "test",
				Type:           Type("bad"),
				TriggerPattern: "*.py",
				Prompt:         "prompt",
			},
			wantErr: "invalid type: must be knowledge, repo, or task",
		},
		{
			name: "missing trigger_pattern",
			req: CreateRequest{
				Name:   "test",
				Type:   TypeKnowledge,
				Prompt: "prompt",
			},
			wantErr: "trigger_pattern is required",
		},
		{
			name: "missing prompt",
			req: CreateRequest{
				Name:           "test",
				Type:           TypeKnowledge,
				TriggerPattern: "*.py",
			},
			wantErr: "prompt is required",
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

func TestValidTypes(t *testing.T) {
	t.Parallel()

	if len(ValidTypes) != 3 {
		t.Errorf("len(ValidTypes) = %d, want 3", len(ValidTypes))
	}

	expected := []Type{TypeKnowledge, TypeRepo, TypeTask}
	for i, vt := range ValidTypes {
		if vt != expected[i] {
			t.Errorf("ValidTypes[%d] = %q, want %q", i, vt, expected[i])
		}
	}
}

func TestTypeConstants(t *testing.T) {
	t.Parallel()

	if TypeKnowledge != "knowledge" {
		t.Errorf("TypeKnowledge = %q, want %q", TypeKnowledge, "knowledge")
	}
	if TypeRepo != "repo" {
		t.Errorf("TypeRepo = %q, want %q", TypeRepo, "repo")
	}
	if TypeTask != "task" {
		t.Errorf("TypeTask = %q, want %q", TypeTask, "task")
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := `name: python-testing
type: knowledge
trigger: "test_*.py"
description: Python testing best practices
---
When working with Python test files, always use pytest fixtures.
Use parametrize for similar test cases.`

	path := filepath.Join(dir, "python-testing.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	m, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	if m.Name != "python-testing" {
		t.Errorf("Name = %q, want %q", m.Name, "python-testing")
	}
	if m.Type != TypeKnowledge {
		t.Errorf("Type = %q, want %q", m.Type, TypeKnowledge)
	}
	if m.TriggerPattern != "test_*.py" {
		t.Errorf("TriggerPattern = %q, want %q", m.TriggerPattern, "test_*.py")
	}
	if m.Description != "Python testing best practices" {
		t.Errorf("Description = %q, want %q", m.Description, "Python testing best practices")
	}
	if m.Prompt == "" {
		t.Error("Prompt should not be empty")
	}
	if !m.Enabled {
		t.Error("Enabled should be true by default")
	}
}

func TestLoadFromFileInvalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{
			name: "missing name",
			content: `type: knowledge
trigger: "*.py"
---
Some prompt`,
		},
		{
			name: "invalid type",
			content: `name: test
type: invalid
trigger: "*.py"
---
Some prompt`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "test.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			_, err := LoadFromFile(path)
			if err == nil {
				t.Error("LoadFromFile() = nil, want error for invalid file")
			}
		})
	}
}

func TestLoadFromFileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadFromFile("/nonexistent/path/file.md")
	if err == nil {
		t.Error("LoadFromFile() = nil, want error for missing file")
	}
}

func TestLoadFromDirectory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create two valid microagent files.
	file1 := `name: agent-one
type: knowledge
trigger: "*.go"
---
Go best practices.`

	file2 := `name: agent-two
type: repo
trigger: "Makefile"
---
Build system guidelines.`

	if err := os.WriteFile(filepath.Join(dir, "agent-one.md"), []byte(file1), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-two.md"), []byte(file2), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Create a non-md file that should be skipped.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a microagent"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	agents, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}

	if len(agents) != 2 {
		t.Fatalf("len(agents) = %d, want 2", len(agents))
	}
}

func TestLoadFromDirectoryNotFound(t *testing.T) {
	t.Parallel()

	agents, err := LoadFromDirectory("/nonexistent/directory")
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v, want nil for non-existent dir", err)
	}
	if agents != nil {
		t.Errorf("agents = %v, want nil", agents)
	}
}

func TestLoadFromDirectoryEmpty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	agents, err := LoadFromDirectory(dir)
	if err != nil {
		t.Fatalf("LoadFromDirectory: %v", err)
	}
	if agents != nil {
		t.Errorf("agents = %v, want nil for empty dir", agents)
	}
}

func TestParseYAMLLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line      string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{"name: python-testing", "name", "python-testing", true},
		{`trigger: "*.py"`, "trigger", "*.py", true},
		{`trigger: '*.py'`, "trigger", "*.py", true},
		{"type: knowledge", "type", "knowledge", true},
		{"no-colon-here", "", "", false},
		{"key:", "key", "", true},
		{"  key  :  value  ", "key", "value", true},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()
			key, value, ok := parseYAMLLine(tt.line)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
			if value != tt.wantValue {
				t.Errorf("value = %q, want %q", value, tt.wantValue)
			}
		})
	}
}
