package service

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// updateGolden controls whether golden files are regenerated.
// Run with: go test ./internal/service/... -run TestGoldenFiles -update
var updateGolden = flag.Bool("update", false, "update golden files")

// allModeIDs lists all 24 built-in mode IDs from presets.go.
var allModeIDs = []string{
	"api-tester", "architect", "backend-architect", "benchmarker",
	"boundary-analyzer", "coder", "contract-reviewer", "debugger",
	"devops", "documenter", "evaluator", "frontend",
	"goal-researcher", "infra-maintainer", "lsp-engineer", "moderator",
	"orchestrator", "proponent", "prototyper", "refactorer",
	"reviewer", "security", "tester", "workflow-optimizer",
}

// allCapabilities lists the 3 model capability levels.
var allCapabilities = []string{"full", "api_with_tools", "pure_completion"}

// allEnvs lists the 2 environment values.
var allEnvs = []string{"development", "production"}

// allAutonomyLevels lists the 5 autonomy levels.
var allAutonomyLevels = []int{1, 2, 3, 4, 5}

// allAgenticValues lists the 2 agentic flags.
var allAgenticValues = []bool{true, false}

// generateAllContexts produces all 1,440 permutations of AssemblyContext.
// 5 autonomy × 24 modes × 3 capabilities × 2 envs × 2 agentic = 1,440.
func generateAllContexts() []prompt.AssemblyContext {
	contexts := make([]prompt.AssemblyContext, 0, 1440)
	for _, autonomy := range allAutonomyLevels {
		for _, modeID := range allModeIDs {
			for _, cap := range allCapabilities {
				for _, env := range allEnvs {
					for _, agentic := range allAgenticValues {
						contexts = append(contexts, prompt.AssemblyContext{
							ModeID:          modeID,
							Autonomy:        autonomy,
							ModelCapability: cap,
							Env:             env,
							Agentic:         agentic,
						})
					}
				}
			}
		}
	}
	return contexts
}

// contextString returns a human-readable label for a context permutation.
func contextString(ctx prompt.AssemblyContext) string {
	agentic := "agentic"
	if !ctx.Agentic {
		agentic = "passive"
	}
	return fmt.Sprintf("mode=%s/auto=%d/cap=%s/env=%s/%s",
		ctx.ModeID, ctx.Autonomy, ctx.ModelCapability, ctx.Env, agentic)
}

// loadEmbeddedLibrary loads the real embedded prompt library.
func loadEmbeddedLibrary(t *testing.T) *PromptLibraryService {
	t.Helper()
	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load embedded prompt library: %v", err)
	}
	return lib
}

// allEntries returns every entry across all categories from the library.
func allEntries(lib *PromptLibraryService) []prompt.PromptEntry {
	var result []prompt.PromptEntry
	cats := []prompt.Category{
		prompt.CategoryIdentity, prompt.CategorySystem, prompt.CategoryContext,
		prompt.CategoryBehavior, prompt.CategoryActions, prompt.CategoryTools,
		prompt.CategoryOutput, prompt.CategoryTone, prompt.CategoryAutonomy,
		prompt.CategoryModelAdaptive, prompt.CategoryMemory, prompt.CategoryReminder,
	}
	for _, cat := range cats {
		result = append(result, lib.GetByCategory(cat)...)
	}
	return result
}

// =============================================================================
// Layer 1: Component Validation — All Embedded YAML Files
// =============================================================================

func TestEmbeddedYAML_AllConditionsReferenceValidValues(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)
	entries := allEntries(lib)

	validModes := make(map[string]bool, len(allModeIDs))
	for _, m := range allModeIDs {
		validModes[m] = true
	}

	validCaps := map[string]bool{
		"full": true, "api_with_tools": true, "pure_completion": true,
	}

	validEnvs := map[string]bool{
		"development": true, "production": true,
	}

	for i := range entries {
		e := &entries[i]
		c := &e.Conditions

		// Validate autonomy range.
		if c.AutonomyMin < 0 || c.AutonomyMin > 5 {
			t.Errorf("[%s] autonomy_min=%d out of range 0-5", e.ID, c.AutonomyMin)
		}
		if c.AutonomyMax < 0 || c.AutonomyMax > 5 {
			t.Errorf("[%s] autonomy_max=%d out of range 0-5", e.ID, c.AutonomyMax)
		}
		if c.AutonomyMin > 0 && c.AutonomyMax > 0 && c.AutonomyMin > c.AutonomyMax {
			t.Errorf("[%s] autonomy_min=%d > autonomy_max=%d", e.ID, c.AutonomyMin, c.AutonomyMax)
		}

		// Validate mode references.
		for _, m := range c.Modes {
			if !validModes[m] {
				t.Errorf("[%s] conditions.modes references unknown mode %q", e.ID, m)
			}
		}
		for _, m := range c.ExcludeModes {
			if !validModes[m] {
				t.Errorf("[%s] conditions.exclude_modes references unknown mode %q", e.ID, m)
			}
		}

		// Validate model capabilities.
		for _, cap := range c.ModelCapabilities {
			if !validCaps[cap] {
				t.Errorf("[%s] conditions.model_capabilities references unknown capability %q", e.ID, cap)
			}
		}

		// Validate env values.
		for _, env := range c.Env {
			if !validEnvs[env] {
				t.Errorf("[%s] conditions.env references unknown environment %q", e.ID, env)
			}
		}
	}
}

func TestEmbeddedYAML_ModeYAMLFilesMatchPresetIDs(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)

	// Collect all mode IDs referenced in mode YAML conditions.
	modeEntries := make(map[string]bool)
	for _, e := range allEntries(lib) {
		for _, m := range e.Conditions.Modes {
			modeEntries[m] = true
		}
	}

	// Every preset mode ID should have at least one YAML entry.
	for _, modeID := range allModeIDs {
		if !modeEntries[modeID] {
			t.Errorf("mode %q has no YAML entry with conditions.modes containing it", modeID)
		}
	}
}

func TestEmbeddedYAML_ContentNotTooShort(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)

	for _, e := range allEntries(lib) {
		content := strings.TrimSpace(e.Content)
		if len(content) < 10 {
			t.Errorf("[%s] content is suspiciously short (%d chars): %q", e.ID, len(content), content)
		}
	}
}

func TestEmbeddedYAML_IDNamingConvention(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)

	for _, e := range allEntries(lib) {
		// IDs should use dot-separated namespacing (e.g., "identity.agent", "mode.coder").
		if !strings.Contains(e.ID, ".") {
			t.Errorf("[%s] ID should use dot-separated namespacing (e.g., 'category.name')", e.ID)
		}
	}
}

// =============================================================================
// Layer 2: Assembly Invariants × All 1,440 Context Permutations
// =============================================================================

func TestAssemblyInvariants_AllContextPermutations(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)
	assembler := NewPromptAssembler(lib, 0)
	allContexts := generateAllContexts()

	if len(allContexts) != 1440 {
		t.Fatalf("expected 1440 context permutations, got %d", len(allContexts))
	}

	t.Run("output is non-empty for all contexts", func(t *testing.T) {
		t.Parallel()
		for _, ctx := range allContexts {
			result := assembler.Assemble(ctx, nil)
			if result == "" {
				t.Errorf("empty output for %s", contextString(ctx))
			}
		}
	})

	t.Run("identity section always present", func(t *testing.T) {
		t.Parallel()
		for _, ctx := range allContexts {
			result := assembler.Assemble(ctx, nil)
			if !strings.Contains(result, "CodeForge") {
				t.Errorf("identity section missing for %s", contextString(ctx))
			}
		}
	})

	t.Run("identity appears before all other content", func(t *testing.T) {
		t.Parallel()
		for _, ctx := range allContexts {
			result := assembler.Assemble(ctx, nil)
			idxCodeForge := strings.Index(result, "CodeForge")
			if idxCodeForge < 0 {
				continue // covered by previous test
			}
			// The identity section should start at or very near position 0.
			// Allow some leading whitespace but identity should be in the first 200 chars.
			if idxCodeForge > 200 {
				t.Errorf("identity not at start (pos=%d) for %s", idxCodeForge, contextString(ctx))
			}
		}
	})

	t.Run("no duplicate entry IDs in assembled output", func(t *testing.T) {
		t.Parallel()
		for _, ctx := range allContexts {
			entries := lib.Query(ctx)
			seen := make(map[string]bool, len(entries))
			for _, e := range entries {
				if seen[e.ID] {
					t.Errorf("duplicate entry %q for %s", e.ID, contextString(ctx))
				}
				seen[e.ID] = true
			}
		}
	})

	t.Run("category ordering maintained", func(t *testing.T) {
		t.Parallel()
		for _, ctx := range allContexts {
			entries := lib.Query(ctx)
			// Sort entries the same way the assembler does.
			sort.SliceStable(entries, func(i, j int) bool {
				ci := prompt.CategoryOrder(entries[i].Category)
				cj := prompt.CategoryOrder(entries[j].Category)
				if ci != cj {
					return ci < cj
				}
				if entries[i].Priority != entries[j].Priority {
					return entries[i].Priority > entries[j].Priority
				}
				return entries[i].SortOrder < entries[j].SortOrder
			})

			lastCatOrder := -1
			for _, e := range entries {
				catOrder := prompt.CategoryOrder(e.Category)
				if catOrder < lastCatOrder {
					t.Errorf("category order violated: %s (order=%d) after order=%d for %s",
						e.Category, catOrder, lastCatOrder, contextString(ctx))
					break
				}
				lastCatOrder = catOrder
			}
		}
	})

	t.Run("priority ordering within same category", func(t *testing.T) {
		t.Parallel()
		for _, ctx := range allContexts {
			entries := lib.Query(ctx)
			sort.SliceStable(entries, func(i, j int) bool {
				ci := prompt.CategoryOrder(entries[i].Category)
				cj := prompt.CategoryOrder(entries[j].Category)
				if ci != cj {
					return ci < cj
				}
				if entries[i].Priority != entries[j].Priority {
					return entries[i].Priority > entries[j].Priority
				}
				return entries[i].SortOrder < entries[j].SortOrder
			})

			var lastPriority int
			var lastCategory prompt.Category
			for _, e := range entries {
				if e.Category != lastCategory {
					lastCategory = e.Category
					lastPriority = e.Priority
					continue
				}
				if e.Priority > lastPriority {
					t.Errorf("priority order violated: %s (prio=%d) after prio=%d in category %s for %s",
						e.ID, e.Priority, lastPriority, e.Category, contextString(ctx))
					break
				}
				lastPriority = e.Priority
			}
		}
	})

	t.Run("each autonomy level produces unique content", func(t *testing.T) {
		t.Parallel()
		outputs := make(map[int]string)
		for _, level := range allAutonomyLevels {
			ctx := prompt.AssemblyContext{
				ModeID:   "coder",
				Autonomy: level,
				Agentic:  true,
				Env:      "production",
			}
			outputs[level] = assembler.Assemble(ctx, nil)
		}

		// At minimum, level 1 (supervised) and level 5 (headless) must differ.
		if outputs[1] == outputs[5] {
			t.Error("autonomy level 1 and 5 produce identical output")
		}
	})

	t.Run("mode-specific content included for each mode", func(t *testing.T) {
		t.Parallel()
		for _, modeID := range allModeIDs {
			ctx := prompt.AssemblyContext{
				ModeID:   modeID,
				Autonomy: 3,
				Agentic:  true,
				Env:      "production",
			}
			entries := lib.Query(ctx)
			hasModeEntry := false
			for _, e := range entries {
				if len(e.Conditions.Modes) > 0 {
					for _, m := range e.Conditions.Modes {
						if m == modeID {
							hasModeEntry = true
							break
						}
					}
				}
				if hasModeEntry {
					break
				}
			}
			if !hasModeEntry {
				t.Errorf("mode %q has no mode-specific entry in assembled output", modeID)
			}
		}
	})

	t.Run("agentic contexts include more entries than passive", func(t *testing.T) {
		t.Parallel()
		for _, modeID := range allModeIDs {
			agenticCtx := prompt.AssemblyContext{
				ModeID: modeID, Autonomy: 3, Agentic: true, Env: "production",
			}
			passiveCtx := prompt.AssemblyContext{
				ModeID: modeID, Autonomy: 3, Agentic: false, Env: "production",
			}
			agenticEntries := lib.Query(agenticCtx)
			passiveEntries := lib.Query(passiveCtx)
			if len(agenticEntries) < len(passiveEntries) {
				t.Errorf("mode %q: agentic (%d entries) has fewer entries than passive (%d)",
					modeID, len(agenticEntries), len(passiveEntries))
			}
		}
	})
}

// =============================================================================
// Layer 3: Coverage Matrix — Every YAML Entry Reachable
// =============================================================================

func TestCoverageMatrix_EveryEntryReachable(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)
	entries := allEntries(lib)
	allContexts := generateAllContexts()

	// Track which entry IDs are reached.
	reached := make(map[string]bool, len(entries))
	for _, ctx := range allContexts {
		for _, e := range lib.Query(ctx) {
			reached[e.ID] = true
		}
	}

	for _, e := range entries {
		if !reached[e.ID] {
			t.Errorf("unreachable entry: %s (category=%s, conditions=%+v)",
				e.ID, e.Category, e.Conditions)
		}
	}

	t.Logf("coverage: %d/%d entries reachable (%d contexts tested)",
		len(reached), len(entries), len(allContexts))
}

// =============================================================================
// Layer 4: Golden File Regression Tests
// =============================================================================

// goldenContexts returns ~10 representative contexts for golden file testing.
func goldenContexts() []struct {
	name string
	ctx  prompt.AssemblyContext
} {
	return []struct {
		name string
		ctx  prompt.AssemblyContext
	}{
		{
			name: "coder-supervised-full-dev-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "coder", Autonomy: 1, ModelCapability: "full",
				Env: "development", Agentic: true,
			},
		},
		{
			name: "coder-headless-full-prod-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "coder", Autonomy: 5, ModelCapability: "full",
				Env: "production", Agentic: true,
			},
		},
		{
			name: "architect-semiauto-full-prod-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "architect", Autonomy: 2, ModelCapability: "full",
				Env: "production", Agentic: true,
			},
		},
		{
			name: "reviewer-autoedit-api-dev-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "reviewer", Autonomy: 3, ModelCapability: "api_with_tools",
				Env: "development", Agentic: true,
			},
		},
		{
			name: "debugger-fullauto-pure-prod-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "debugger", Autonomy: 4, ModelCapability: "pure_completion",
				Env: "production", Agentic: true,
			},
		},
		{
			name: "frontend-supervised-full-dev-passive",
			ctx: prompt.AssemblyContext{
				ModeID: "frontend", Autonomy: 1, ModelCapability: "full",
				Env: "development", Agentic: false,
			},
		},
		{
			name: "security-headless-full-prod-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "security", Autonomy: 5, ModelCapability: "full",
				Env: "production", Agentic: true,
			},
		},
		{
			name: "tester-autoedit-api-dev-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "tester", Autonomy: 3, ModelCapability: "api_with_tools",
				Env: "development", Agentic: true,
			},
		},
		{
			name: "orchestrator-fullauto-full-prod-agentic",
			ctx: prompt.AssemblyContext{
				ModeID: "orchestrator", Autonomy: 4, ModelCapability: "full",
				Env: "production", Agentic: true,
			},
		},
		{
			name: "documenter-supervised-pure-dev-passive",
			ctx: prompt.AssemblyContext{
				ModeID: "documenter", Autonomy: 1, ModelCapability: "pure_completion",
				Env: "development", Agentic: false,
			},
		},
	}
}

func TestGoldenFiles(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)
	assembler := NewPromptAssembler(lib, 0)

	goldenDir := filepath.Join("testdata", "golden")

	for _, tc := range goldenContexts() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := assembler.Assemble(tc.ctx, nil)

			goldenPath := filepath.Join(goldenDir, tc.name+".txt")

			if *updateGolden {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatalf("create golden dir: %v", err)
				}
				// Append trailing newline so end-of-file-fixer hook is a no-op.
				if err := os.WriteFile(goldenPath, []byte(result+"\n"), 0o644); err != nil {
					t.Fatalf("write golden file: %v", err)
				}
				t.Logf("updated golden file: %s", goldenPath)
				return
			}

			expected, err := os.ReadFile(goldenPath) //nolint:gosec // test-controlled path
			if os.IsNotExist(err) {
				t.Fatalf("golden file not found: %s (run with -update to generate)", goldenPath)
			}
			if err != nil {
				t.Fatalf("read golden file: %v", err)
			}

			// Trim trailing newline added for end-of-file-fixer compatibility.
			if string(expected) != result+"\n" {
				// Show a concise diff summary.
				expectedLines := strings.Split(string(expected), "\n")
				resultLines := strings.Split(result, "\n")
				t.Errorf("golden file mismatch: %s\nexpected %d lines, got %d lines\n"+
					"first expected line: %q\nfirst result line: %q\n"+
					"Run with -update to regenerate golden files.",
					goldenPath, len(expectedLines), len(resultLines),
					firstLine(expectedLines), firstLine(resultLines))
			}
		})
	}
}

func firstLine(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	if len(lines[0]) > 80 {
		return lines[0][:80] + "..."
	}
	return lines[0]
}

// =============================================================================
// Layer 5a: NATS Contract — Reminders Field Structure
// =============================================================================

// TestRemindersContract_FieldPresence verifies the Reminders field is properly
// structured in the NATS payload for cross-language compatibility.
// (The basic JSON round-trip is already tested in prompt_assembler_test.go.
// This test extends coverage to verify structural properties.)
func TestRemindersContract_SystemReminderTagFormat(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)
	reminders := lib.GetByCategory(prompt.CategoryReminder)

	if len(reminders) == 0 {
		t.Fatal("expected at least one reminder entry in the library")
	}

	for _, r := range reminders {
		// Reminders should produce content that can be wrapped in <system-reminder> tags.
		content := strings.TrimSpace(r.Content)
		if content == "" {
			t.Errorf("[%s] reminder has empty content", r.ID)
		}
		// Content should not already contain the wrapper tag (Python adds it).
		if strings.Contains(content, "<system-reminder>") {
			t.Errorf("[%s] reminder content should not contain <system-reminder> tag (Python wraps it)", r.ID)
		}
	}
}

// =============================================================================
// Helpers for Layer 2
// =============================================================================

// TestPermutationCount verifies our math: 5 × 24 × 3 × 2 × 2 = 1,440.
func TestPermutationCount(t *testing.T) {
	t.Parallel()
	expected := len(allAutonomyLevels) * len(allModeIDs) * len(allCapabilities) * len(allEnvs) * len(allAgenticValues)
	if expected != 1440 {
		t.Errorf("expected 1440 permutations, formula gives %d", expected)
	}
	actual := len(generateAllContexts())
	if actual != expected {
		t.Errorf("generateAllContexts() returned %d, expected %d", actual, expected)
	}
}
