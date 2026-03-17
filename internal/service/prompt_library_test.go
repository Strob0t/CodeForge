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

func TestPromptLibrary_LoadsEmbeddedPrompts(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load embedded prompts: %v", err)
	}
	if lib.Len() < 40 {
		t.Errorf("expected at least 40 embedded prompt entries, got %d", lib.Len())
	}

	// Verify key entries exist by category.
	categories := map[prompt.Category]int{
		prompt.CategoryIdentity:      1,
		prompt.CategorySystem:        2,
		prompt.CategoryContext:       6,
		prompt.CategoryBehavior:      18,
		prompt.CategoryActions:       2,
		prompt.CategoryTools:         6,
		prompt.CategoryOutput:        3,
		prompt.CategoryTone:          2,
		prompt.CategoryAutonomy:      5,
		prompt.CategoryModelAdaptive: 3,
		prompt.CategoryMemory:        1,
		prompt.CategoryReminder:      7,
	}
	for cat, wantMin := range categories {
		entries := lib.GetByCategory(cat)
		if len(entries) < wantMin {
			t.Errorf("category %q: got %d entries, want at least %d", cat, len(entries), wantMin)
		}
	}
}

func TestPromptLibrary_EmbeddedIDsAreUnique(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load embedded prompts: %v", err)
	}

	// Query all entries (no conditions filter).
	all := lib.Query(prompt.AssemblyContext{
		ModeID:          "",
		Autonomy:        0,
		ModelCapability: "",
		Env:             "",
		Agentic:         false,
	})

	// We need all entries, not just unconditional ones.
	// Use GetByCategory for each category to collect them all.
	seen := make(map[string]bool)
	allCategories := []prompt.Category{
		prompt.CategoryIdentity, prompt.CategorySystem, prompt.CategoryContext,
		prompt.CategoryBehavior, prompt.CategoryActions, prompt.CategoryTools,
		prompt.CategoryOutput, prompt.CategoryTone, prompt.CategoryAutonomy,
		prompt.CategoryModelAdaptive, prompt.CategoryMemory, prompt.CategoryReminder,
	}
	for _, cat := range allCategories {
		entries := lib.GetByCategory(cat)
		for _, e := range entries {
			if seen[e.ID] {
				t.Errorf("duplicate entry ID: %q", e.ID)
			}
			seen[e.ID] = true
		}
	}

	_ = all // suppress unused warning
}

func TestPromptLibrary_EmbeddedPrioritiesInRange(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load embedded prompts: %v", err)
	}

	allCategories := []prompt.Category{
		prompt.CategoryIdentity, prompt.CategorySystem, prompt.CategoryContext,
		prompt.CategoryBehavior, prompt.CategoryActions, prompt.CategoryTools,
		prompt.CategoryOutput, prompt.CategoryTone, prompt.CategoryAutonomy,
		prompt.CategoryModelAdaptive, prompt.CategoryMemory, prompt.CategoryReminder,
	}
	for _, cat := range allCategories {
		for _, e := range lib.GetByCategory(cat) {
			if e.Priority < 0 || e.Priority > 100 {
				t.Errorf("entry %q has priority %d, want 0-100", e.ID, e.Priority)
			}
			if e.Content == "" {
				t.Errorf("entry %q has empty content", e.ID)
			}
		}
	}
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
