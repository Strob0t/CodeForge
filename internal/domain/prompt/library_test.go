package prompt

import "testing"

func TestValidCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category Category
		want     bool
	}{
		{"identity", CategoryIdentity, true},
		{"system", CategorySystem, true},
		{"context", CategoryContext, true},
		{"behavior", CategoryBehavior, true},
		{"actions", CategoryActions, true},
		{"tools", CategoryTools, true},
		{"output", CategoryOutput, true},
		{"tone", CategoryTone, true},
		{"autonomy", CategoryAutonomy, true},
		{"model_adaptive", CategoryModelAdaptive, true},
		{"memory", CategoryMemory, true},
		{"reminder", CategoryReminder, true},
		{"empty string", Category(""), false},
		{"unknown", Category("unknown"), false},
		{"typo", Category("identiy"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ValidCategory(tc.category)
			if got != tc.want {
				t.Errorf("ValidCategory(%q) = %v, want %v", tc.category, got, tc.want)
			}
		})
	}
}

func TestCategoryOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		category Category
		want     int
	}{
		{"identity is 0", CategoryIdentity, 0},
		{"reminder is 11", CategoryReminder, 11},
		{"unknown returns 999", Category("bogus"), 999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CategoryOrder(tc.category)
			if got != tc.want {
				t.Errorf("CategoryOrder(%q) = %d, want %d", tc.category, got, tc.want)
			}
		})
	}

	// Verify all known categories have unique, ascending order values.
	t.Run("all categories have unique order", func(t *testing.T) {
		t.Parallel()
		seen := make(map[int]Category)
		allCategories := []Category{
			CategoryIdentity, CategorySystem, CategoryContext, CategoryBehavior,
			CategoryActions, CategoryTools, CategoryOutput, CategoryTone,
			CategoryAutonomy, CategoryModelAdaptive, CategoryMemory, CategoryReminder,
		}
		for _, c := range allCategories {
			order := CategoryOrder(c)
			if prev, ok := seen[order]; ok {
				t.Errorf("CategoryOrder(%q) = %d, same as %q", c, order, prev)
			}
			seen[order] = c
		}
	})
}

func TestPromptEntry_Matches(t *testing.T) {
	t.Parallel()

	// baseCtx is a fully-specified context that matches an unconditioned entry.
	baseCtx := AssemblyContext{
		ModeID:          "coder",
		Autonomy:        3,
		ModelCapability: "full",
		Env:             "development",
		Agentic:         true,
	}

	tests := []struct {
		name       string
		conditions Conditions
		ctx        AssemblyContext
		want       bool
	}{
		// --- No conditions (always matches) ---
		{
			name:       "empty conditions match anything",
			conditions: Conditions{},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "empty conditions match zero-value context",
			conditions: Conditions{},
			ctx:        AssemblyContext{},
			want:       true,
		},

		// --- AutonomyMin ---
		{
			name:       "autonomy_min satisfied exactly",
			conditions: Conditions{AutonomyMin: 3},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "autonomy_min satisfied above",
			conditions: Conditions{AutonomyMin: 2},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "autonomy_min not satisfied",
			conditions: Conditions{AutonomyMin: 4},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "autonomy_min zero is ignored",
			conditions: Conditions{AutonomyMin: 0},
			ctx:        AssemblyContext{Autonomy: 0},
			want:       true,
		},

		// --- AutonomyMax ---
		{
			name:       "autonomy_max satisfied exactly",
			conditions: Conditions{AutonomyMax: 3},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "autonomy_max satisfied below",
			conditions: Conditions{AutonomyMax: 5},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "autonomy_max not satisfied",
			conditions: Conditions{AutonomyMax: 2},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "autonomy_max zero is ignored",
			conditions: Conditions{AutonomyMax: 0},
			ctx:        AssemblyContext{Autonomy: 99},
			want:       true,
		},

		// --- AutonomyMin + AutonomyMax combined ---
		{
			name:       "autonomy range satisfied",
			conditions: Conditions{AutonomyMin: 2, AutonomyMax: 4},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "autonomy below range",
			conditions: Conditions{AutonomyMin: 4, AutonomyMax: 5},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "autonomy above range",
			conditions: Conditions{AutonomyMin: 1, AutonomyMax: 2},
			ctx:        baseCtx,
			want:       false,
		},

		// --- Modes ---
		{
			name:       "modes includes current mode",
			conditions: Conditions{Modes: []string{"coder", "reviewer"}},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "modes does not include current mode",
			conditions: Conditions{Modes: []string{"reviewer", "architect"}},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "modes empty is ignored",
			conditions: Conditions{Modes: []string{}},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "modes with empty mode ID",
			conditions: Conditions{Modes: []string{"coder"}},
			ctx:        AssemblyContext{ModeID: ""},
			want:       false,
		},

		// --- ExcludeModes ---
		{
			name:       "exclude_modes does not contain current",
			conditions: Conditions{ExcludeModes: []string{"reviewer"}},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "exclude_modes contains current",
			conditions: Conditions{ExcludeModes: []string{"coder", "reviewer"}},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "exclude_modes empty is ignored",
			conditions: Conditions{ExcludeModes: []string{}},
			ctx:        baseCtx,
			want:       true,
		},

		// --- Modes + ExcludeModes combined ---
		{
			name: "modes allows but exclude_modes blocks",
			conditions: Conditions{
				Modes:        []string{"coder", "reviewer"},
				ExcludeModes: []string{"coder"},
			},
			ctx:  baseCtx,
			want: false,
		},

		// --- ModelCapabilities ---
		{
			name:       "model_capabilities includes current",
			conditions: Conditions{ModelCapabilities: []string{"full", "api_with_tools"}},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "model_capabilities does not include current",
			conditions: Conditions{ModelCapabilities: []string{"pure_completion"}},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "model_capabilities empty is ignored",
			conditions: Conditions{ModelCapabilities: []string{}},
			ctx:        baseCtx,
			want:       true,
		},

		// --- AgenticOnly ---
		{
			name:       "agentic_only true and context is agentic",
			conditions: Conditions{AgenticOnly: true},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "agentic_only true and context is not agentic",
			conditions: Conditions{AgenticOnly: true},
			ctx:        AssemblyContext{Agentic: false},
			want:       false,
		},
		{
			name:       "agentic_only false is ignored",
			conditions: Conditions{AgenticOnly: false},
			ctx:        AssemblyContext{Agentic: false},
			want:       true,
		},

		// --- Env ---
		{
			name:       "env includes current",
			conditions: Conditions{Env: []string{"development", "staging"}},
			ctx:        baseCtx,
			want:       true,
		},
		{
			name:       "env does not include current",
			conditions: Conditions{Env: []string{"production"}},
			ctx:        baseCtx,
			want:       false,
		},
		{
			name:       "env empty is ignored",
			conditions: Conditions{Env: []string{}},
			ctx:        baseCtx,
			want:       true,
		},

		// --- Multiple conditions combined ---
		{
			name: "all conditions satisfied",
			conditions: Conditions{
				AutonomyMin:       2,
				AutonomyMax:       4,
				Modes:             []string{"coder"},
				ModelCapabilities: []string{"full"},
				AgenticOnly:       true,
				Env:               []string{"development"},
			},
			ctx:  baseCtx,
			want: true,
		},
		{
			name: "one condition fails among many",
			conditions: Conditions{
				AutonomyMin:       2,
				AutonomyMax:       4,
				Modes:             []string{"coder"},
				ModelCapabilities: []string{"pure_completion"}, // fails
				AgenticOnly:       true,
				Env:               []string{"development"},
			},
			ctx:  baseCtx,
			want: false,
		},
		{
			name: "exclude_modes overrides modes match",
			conditions: Conditions{
				Modes:        []string{"coder"},
				ExcludeModes: []string{"coder"},
			},
			ctx:  baseCtx,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			entry := PromptEntry{
				ID:         "test",
				Category:   CategorySystem,
				Name:       "test",
				Content:    "test content",
				Conditions: tc.conditions,
			}
			got := entry.Matches(tc.ctx)
			if got != tc.want {
				t.Errorf("Matches() = %v, want %v (conditions=%+v, ctx=%+v)",
					got, tc.want, tc.conditions, tc.ctx)
			}
		})
	}
}

func TestContainsStr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		slice  []string
		target string
		want   bool
	}{
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"nil slice", nil, "a", false},
		{"empty target in slice", []string{""}, "", true},
		{"empty target not in slice", []string{"a"}, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := containsStr(tc.slice, tc.target)
			if got != tc.want {
				t.Errorf("containsStr(%v, %q) = %v, want %v", tc.slice, tc.target, got, tc.want)
			}
		})
	}
}
