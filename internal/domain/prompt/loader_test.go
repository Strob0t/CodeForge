package prompt

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		yaml    string
		wantErr string
		check   func(t *testing.T, e PromptEntry)
	}{
		{
			name: "valid minimal entry",
			yaml: `
id: identity-core
category: identity
name: Core Identity
priority: 90
content: You are CodeForge.
`,
			check: func(t *testing.T, e PromptEntry) {
				if e.ID != "identity-core" {
					t.Errorf("ID = %q, want %q", e.ID, "identity-core")
				}
				if e.Category != CategoryIdentity {
					t.Errorf("Category = %q, want %q", e.Category, CategoryIdentity)
				}
				if e.Name != "Core Identity" {
					t.Errorf("Name = %q, want %q", e.Name, "Core Identity")
				}
				if e.Priority != 90 {
					t.Errorf("Priority = %d, want %d", e.Priority, 90)
				}
				if e.Content != "You are CodeForge." {
					t.Errorf("Content = %q, want %q", e.Content, "You are CodeForge.")
				}
				if e.SortOrder != 0 {
					t.Errorf("SortOrder = %d, want 0", e.SortOrder)
				}
			},
		},
		{
			name: "valid entry with all fields",
			yaml: `
id: autonomy-full-auto
category: autonomy
name: Full Auto Rules
priority: 80
sort_order: 5
conditions:
  autonomy_min: 4
  autonomy_max: 5
  modes:
    - coder
    - debugger
  exclude_modes:
    - reviewer
  model_capabilities:
    - full
    - api_with_tools
  agentic_only: true
  env:
    - production
content: |
  You operate in full-auto mode.
  Do not ask for confirmation.
`,
			check: func(t *testing.T, e PromptEntry) {
				if e.ID != "autonomy-full-auto" {
					t.Errorf("ID = %q", e.ID)
				}
				if e.SortOrder != 5 {
					t.Errorf("SortOrder = %d, want 5", e.SortOrder)
				}
				c := e.Conditions
				if c.AutonomyMin != 4 {
					t.Errorf("AutonomyMin = %d, want 4", c.AutonomyMin)
				}
				if c.AutonomyMax != 5 {
					t.Errorf("AutonomyMax = %d, want 5", c.AutonomyMax)
				}
				if len(c.Modes) != 2 || c.Modes[0] != "coder" || c.Modes[1] != "debugger" {
					t.Errorf("Modes = %v, want [coder, debugger]", c.Modes)
				}
				if len(c.ExcludeModes) != 1 || c.ExcludeModes[0] != "reviewer" {
					t.Errorf("ExcludeModes = %v", c.ExcludeModes)
				}
				if len(c.ModelCapabilities) != 2 {
					t.Errorf("ModelCapabilities = %v", c.ModelCapabilities)
				}
				if !c.AgenticOnly {
					t.Error("AgenticOnly should be true")
				}
				if len(c.Env) != 1 || c.Env[0] != "production" {
					t.Errorf("Env = %v", c.Env)
				}
				if !strings.Contains(e.Content, "full-auto mode") {
					t.Errorf("Content missing expected text: %q", e.Content)
				}
			},
		},
		{
			name: "priority zero is valid",
			yaml: `
id: test-zero
category: system
name: Zero Priority
priority: 0
content: Low priority content.
`,
			check: func(t *testing.T, e PromptEntry) {
				if e.Priority != 0 {
					t.Errorf("Priority = %d, want 0", e.Priority)
				}
			},
		},
		{
			name: "priority 100 is valid",
			yaml: `
id: test-hundred
category: system
name: Max Priority
priority: 100
content: High priority content.
`,
			check: func(t *testing.T, e PromptEntry) {
				if e.Priority != 100 {
					t.Errorf("Priority = %d, want 100", e.Priority)
				}
			},
		},

		// --- Validation errors ---
		{
			name: "missing id",
			yaml: `
category: system
name: No ID
priority: 50
content: Something.
`,
			wantErr: "missing required field 'id'",
		},
		{
			name: "missing category",
			yaml: `
id: test
name: No Category
priority: 50
content: Something.
`,
			wantErr: "missing required field 'category'",
		},
		{
			name: "missing name",
			yaml: `
id: test
category: system
priority: 50
content: Something.
`,
			wantErr: "missing required field 'name'",
		},
		{
			name: "missing content",
			yaml: `
id: test
category: system
name: No Content
priority: 50
`,
			wantErr: "missing required field 'content'",
		},
		{
			name: "invalid category",
			yaml: `
id: test
category: bogus
name: Bad Category
priority: 50
content: Something.
`,
			wantErr: `invalid category "bogus"`,
		},
		{
			name: "priority too high",
			yaml: `
id: test
category: system
name: Too High
priority: 101
content: Something.
`,
			wantErr: "priority must be 0-100, got 101",
		},
		{
			name: "priority negative",
			yaml: `
id: test
category: system
name: Negative
priority: -1
content: Something.
`,
			wantErr: "priority must be 0-100, got -1",
		},
		{
			name:    "invalid yaml syntax",
			yaml:    `{{{not valid yaml`,
			wantErr: "yaml parse error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entry, err := LoadFile([]byte(tc.yaml))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, entry)
			}
		})
	}
}

func TestLoadFS(t *testing.T) {
	t.Parallel()

	t.Run("loads files from flat directory", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/identity.yaml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Core Identity
priority: 90
content: You are CodeForge.
`),
			},
			"prompts/system.yaml": &fstest.MapFile{
				Data: []byte(`
id: system-rules
category: system
name: System Rules
priority: 80
content: Follow the rules.
`),
			},
		}

		entries, err := LoadFS(fsys, "prompts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}

		ids := make(map[string]bool)
		for _, e := range entries {
			ids[e.ID] = true
		}
		if !ids["identity-core"] || !ids["system-rules"] {
			t.Errorf("expected identity-core and system-rules, got %v", ids)
		}
	})

	t.Run("loads files from nested directories", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"lib/identity/core.yaml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Core
priority: 90
content: Core identity.
`),
			},
			"lib/behavior/safety.yaml": &fstest.MapFile{
				Data: []byte(`
id: behavior-safety
category: behavior
name: Safety
priority: 85
content: Be safe.
`),
			},
			"lib/behavior/coding.yaml": &fstest.MapFile{
				Data: []byte(`
id: behavior-coding
category: behavior
name: Coding
priority: 80
content: Write clean code.
`),
			},
		}

		entries, err := LoadFS(fsys, "lib")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
	})

	t.Run("skips non-yaml files", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/identity.yaml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Core
priority: 90
content: Content.
`),
			},
			"prompts/README.md": &fstest.MapFile{
				Data: []byte("# Prompts"),
			},
			"prompts/notes.txt": &fstest.MapFile{
				Data: []byte("some notes"),
			},
		}

		entries, err := LoadFS(fsys, "prompts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("loads .yml extension", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/identity.yml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Core
priority: 90
content: Content.
`),
			},
		}

		entries, err := LoadFS(fsys, "prompts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(entries))
		}
	})

	t.Run("returns error for invalid file", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/good.yaml": &fstest.MapFile{
				Data: []byte(`
id: good
category: system
name: Good
priority: 50
content: OK.
`),
			},
			"prompts/bad.yaml": &fstest.MapFile{
				Data: []byte(`
id: bad
category: bogus
name: Bad
priority: 50
content: Not OK.
`),
			},
		}

		_, err := LoadFS(fsys, "prompts")
		if err == nil {
			t.Fatal("expected error for invalid file, got nil")
		}
		if !strings.Contains(err.Error(), "bad.yaml") {
			t.Errorf("error should reference file path, got: %v", err)
		}
		if !strings.Contains(err.Error(), "invalid category") {
			t.Errorf("error should mention invalid category, got: %v", err)
		}
	})

	t.Run("returns error for missing required fields in nested file", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"lib/deep/nested/missing.yaml": &fstest.MapFile{
				Data: []byte(`
category: system
name: Missing ID
priority: 50
content: Something.
`),
			},
		}

		_, err := LoadFS(fsys, "lib")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "missing.yaml") {
			t.Errorf("error should reference file path, got: %v", err)
		}
	})

	t.Run("directory with only non-yaml files returns empty slice", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/readme.md": &fstest.MapFile{
				Data: []byte("# Nothing here"),
			},
		}

		entries, err := LoadFS(fsys, "prompts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(entries))
		}
	})

	t.Run("nonexistent root returns error", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{}

		_, err := LoadFS(fsys, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent root, got nil")
		}
	})
}
