package service

import (
	"testing"
	"testing/fstest"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// testLibraryFS returns an in-memory FS with a small prompt library for testing.
func testLibraryFS() fstest.MapFS {
	return fstest.MapFS{
		"prompts/identity.yaml": &fstest.MapFile{
			Data: []byte(`
id: identity-core
category: identity
name: Core Identity
priority: 95
content: You are CodeForge, an AI coding orchestrator.
`),
		},
		"prompts/behavior-safety.yaml": &fstest.MapFile{
			Data: []byte(`
id: behavior-safety
category: behavior
name: Safety Rules
priority: 90
conditions:
  agentic_only: true
content: Always validate user input before executing commands.
`),
		},
		"prompts/autonomy-full.yaml": &fstest.MapFile{
			Data: []byte(`
id: autonomy-full
category: autonomy
name: Full Auto
priority: 80
conditions:
  autonomy_min: 4
  autonomy_max: 5
content: Operate autonomously without confirmation.
`),
		},
		"prompts/tools-coder.yaml": &fstest.MapFile{
			Data: []byte(`
id: tools-coder
category: tools
name: Coder Tools
priority: 70
conditions:
  modes:
    - coder
content: Use Read, Write, Edit, Bash tools for code changes.
`),
		},
		"prompts/env-dev.yaml": &fstest.MapFile{
			Data: []byte(`
id: env-dev
category: system
name: Dev Environment
priority: 50
conditions:
  env:
    - development
content: Running in development mode with extra logging.
`),
		},
	}
}

func TestNewPromptLibraryService(t *testing.T) {
	t.Parallel()

	t.Run("loads valid filesystem", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lib.Len() != 5 {
			t.Errorf("Len() = %d, want 5", lib.Len())
		}
	})

	t.Run("returns error for invalid files", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/bad.yaml": &fstest.MapFile{
				Data: []byte(`id: bad`),
			},
		}
		_, err := NewPromptLibraryService(fsys, "prompts")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("returns error for nonexistent root", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{}
		_, err := NewPromptLibraryService(fsys, "nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty directory yields zero entries", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/readme.md": &fstest.MapFile{Data: []byte("# nothing")},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lib.Len() != 0 {
			t.Errorf("Len() = %d, want 0", lib.Len())
		}
	})
}

func TestPromptLibraryService_Query(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tests := []struct {
		name    string
		ctx     prompt.AssemblyContext
		wantIDs []string
	}{
		{
			name: "broad context matches identity, tools-coder, behavior-safety, env-dev",
			ctx: prompt.AssemblyContext{
				ModeID:   "coder",
				Autonomy: 3,
				Env:      "development",
				Agentic:  true,
			},
			wantIDs: []string{"identity-core", "behavior-safety", "tools-coder", "env-dev"},
		},
		{
			name: "non-agentic excludes behavior-safety",
			ctx: prompt.AssemblyContext{
				ModeID:   "coder",
				Autonomy: 3,
				Env:      "development",
				Agentic:  false,
			},
			wantIDs: []string{"identity-core", "tools-coder", "env-dev"},
		},
		{
			name: "high autonomy includes autonomy-full",
			ctx: prompt.AssemblyContext{
				ModeID:   "coder",
				Autonomy: 5,
				Env:      "development",
				Agentic:  true,
			},
			wantIDs: []string{"identity-core", "behavior-safety", "autonomy-full", "tools-coder", "env-dev"},
		},
		{
			name: "reviewer mode excludes tools-coder",
			ctx: prompt.AssemblyContext{
				ModeID:   "reviewer",
				Autonomy: 3,
				Env:      "development",
				Agentic:  true,
			},
			wantIDs: []string{"identity-core", "behavior-safety", "env-dev"},
		},
		{
			name: "production excludes env-dev",
			ctx: prompt.AssemblyContext{
				ModeID:   "coder",
				Autonomy: 3,
				Env:      "production",
				Agentic:  true,
			},
			wantIDs: []string{"identity-core", "behavior-safety", "tools-coder"},
		},
		{
			name: "zero-value context matches only unconditional entries",
			ctx:  prompt.AssemblyContext{},
			// identity-core has no conditions (matches everything)
			// behavior-safety: agentic_only=true -> fails for Agentic=false (zero)
			// autonomy-full: autonomy_min=4 -> fails for Autonomy=0
			// tools-coder: modes=[coder] -> fails for ModeID="" (empty)
			// env-dev: env=[development] -> fails for Env="" (empty)
			wantIDs: []string{"identity-core"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := lib.Query(tc.ctx)
			gotIDs := make(map[string]bool, len(result))
			for _, e := range result {
				gotIDs[e.ID] = true
			}
			if len(result) != len(tc.wantIDs) {
				t.Errorf("got %d entries, want %d; got IDs: %v", len(result), len(tc.wantIDs), gotIDs)
			}
			for _, id := range tc.wantIDs {
				if !gotIDs[id] {
					t.Errorf("expected entry %q in results, got %v", id, gotIDs)
				}
			}
		})
	}
}

func TestPromptLibraryService_GetByCategory(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	tests := []struct {
		name     string
		category prompt.Category
		wantLen  int
	}{
		{"identity has 1", prompt.CategoryIdentity, 1},
		{"behavior has 1", prompt.CategoryBehavior, 1},
		{"autonomy has 1", prompt.CategoryAutonomy, 1},
		{"tools has 1", prompt.CategoryTools, 1},
		{"system has 1", prompt.CategorySystem, 1},
		{"context has 0", prompt.CategoryContext, 0},
		{"output has 0", prompt.CategoryOutput, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := lib.GetByCategory(tc.category)
			if len(result) != tc.wantLen {
				t.Errorf("GetByCategory(%q) returned %d entries, want %d", tc.category, len(result), tc.wantLen)
			}
		})
	}
}

func TestPromptLibraryService_LoadOverlay(t *testing.T) {
	t.Parallel()

	t.Run("replace existing entry", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		initialLen := lib.Len()

		overlayFS := fstest.MapFS{
			"overlay/identity.yaml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Custom Identity
priority: 99
content: You are a custom CodeForge instance.
`),
			},
		}

		if err := lib.LoadOverlay(overlayFS, "overlay"); err != nil {
			t.Fatalf("LoadOverlay error: %v", err)
		}

		// Length should not change (replaced, not added).
		if lib.Len() != initialLen {
			t.Errorf("Len() = %d after overlay, want %d (replace should not add)", lib.Len(), initialLen)
		}

		// Verify the content was replaced.
		entries := lib.GetByCategory(prompt.CategoryIdentity)
		if len(entries) != 1 {
			t.Fatalf("expected 1 identity entry, got %d", len(entries))
		}
		if entries[0].Priority != 99 {
			t.Errorf("Priority = %d, want 99 (should be replaced)", entries[0].Priority)
		}
		if entries[0].Content != "You are a custom CodeForge instance." {
			t.Errorf("Content = %q, should be replaced", entries[0].Content)
		}
	})

	t.Run("add new entry", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		initialLen := lib.Len()

		overlayFS := fstest.MapFS{
			"overlay/tone.yaml": &fstest.MapFile{
				Data: []byte(`
id: tone-friendly
category: tone
name: Friendly Tone
priority: 60
content: Be friendly and helpful.
`),
			},
		}

		if err := lib.LoadOverlay(overlayFS, "overlay"); err != nil {
			t.Fatalf("LoadOverlay error: %v", err)
		}

		if lib.Len() != initialLen+1 {
			t.Errorf("Len() = %d, want %d (should add 1)", lib.Len(), initialLen+1)
		}

		entries := lib.GetByCategory(prompt.CategoryTone)
		if len(entries) != 1 {
			t.Fatalf("expected 1 tone entry, got %d", len(entries))
		}
		if entries[0].ID != "tone-friendly" {
			t.Errorf("ID = %q, want %q", entries[0].ID, "tone-friendly")
		}
	})

	t.Run("replace and add combined", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		initialLen := lib.Len()

		overlayFS := fstest.MapFS{
			"overlay/identity.yaml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Replaced
priority: 99
content: Replaced content.
`),
			},
			"overlay/reminder.yaml": &fstest.MapFile{
				Data: []byte(`
id: reminder-new
category: reminder
name: New Reminder
priority: 50
content: Remember to commit.
`),
			},
		}

		if err := lib.LoadOverlay(overlayFS, "overlay"); err != nil {
			t.Fatalf("LoadOverlay error: %v", err)
		}

		// +1 new entry, identity-core replaced (no extra).
		if lib.Len() != initialLen+1 {
			t.Errorf("Len() = %d, want %d", lib.Len(), initialLen+1)
		}
	})

	t.Run("overlay with invalid file returns error", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		overlayFS := fstest.MapFS{
			"overlay/bad.yaml": &fstest.MapFile{
				Data: []byte(`id: bad`),
			},
		}

		if err := lib.LoadOverlay(overlayFS, "overlay"); err == nil {
			t.Fatal("expected error for invalid overlay file, got nil")
		}
	})
}

func TestPromptLibraryService_Len(t *testing.T) {
	t.Parallel()

	t.Run("returns correct count", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testLibraryFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if lib.Len() != 5 {
			t.Errorf("Len() = %d, want 5", lib.Len())
		}
	})

	t.Run("returns zero for empty library", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/readme.md": &fstest.MapFile{Data: []byte("nothing")},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if lib.Len() != 0 {
			t.Errorf("Len() = %d, want 0", lib.Len())
		}
	})
}
