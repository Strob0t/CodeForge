package prompt

// Category classifies a prompt entry by its role in the assembled system prompt.
type Category string

const (
	CategoryIdentity      Category = "identity"
	CategorySystem        Category = "system"
	CategoryContext       Category = "context"
	CategoryBehavior      Category = "behavior"
	CategoryActions       Category = "actions"
	CategoryTools         Category = "tools"
	CategoryOutput        Category = "output"
	CategoryTone          Category = "tone"
	CategoryAutonomy      Category = "autonomy"
	CategoryModelAdaptive Category = "model_adaptive"
	CategoryMemory        Category = "memory"
	CategoryReminder      Category = "reminder"
)

// categoryOrder defines the canonical sort order for categories.
// Lower values appear first in the assembled prompt.
var categoryOrder = map[Category]int{
	CategoryIdentity:      0,
	CategorySystem:        1,
	CategoryContext:       2,
	CategoryBehavior:      3,
	CategoryActions:       4,
	CategoryTools:         5,
	CategoryOutput:        6,
	CategoryTone:          7,
	CategoryAutonomy:      8,
	CategoryModelAdaptive: 9,
	CategoryMemory:        10,
	CategoryReminder:      11,
}

// ValidCategory returns true if the given category is a known value.
func ValidCategory(c Category) bool {
	_, ok := categoryOrder[c]
	return ok
}

// CategoryOrder returns the sort position for a category.
// Unknown categories return a high value (999) so they sort last.
func CategoryOrder(c Category) int {
	if order, ok := categoryOrder[c]; ok {
		return order
	}
	return 999
}

// Conditions controls when a prompt entry is included in the assembled prompt.
type Conditions struct {
	AutonomyMin       int      `yaml:"autonomy_min"`
	AutonomyMax       int      `yaml:"autonomy_max"`
	Modes             []string `yaml:"modes"`
	ExcludeModes      []string `yaml:"exclude_modes"`
	ModelCapabilities []string `yaml:"model_capabilities"`
	AgenticOnly       bool     `yaml:"agentic_only"`
	Env               []string `yaml:"env"`
}

// PromptEntry represents a single modular prompt fragment loaded from YAML.
type PromptEntry struct {
	ID         string     `yaml:"id"`
	Category   Category   `yaml:"category"`
	Name       string     `yaml:"name"`
	Priority   int        `yaml:"priority"`
	SortOrder  int        `yaml:"sort_order"`
	Conditions Conditions `yaml:"conditions"`
	Content    string     `yaml:"content"`
}

// AssemblyContext holds the runtime parameters used to decide which prompt
// entries should be included in the assembled system prompt.
type AssemblyContext struct {
	ModeID          string
	Autonomy        int
	ModelCapability string // "full" | "api_with_tools" | "pure_completion"
	Env             string // "development" | "production"
	Agentic         bool
}

// Matches returns true if the entry's conditions are satisfied by the given context.
func (e *PromptEntry) Matches(ctx AssemblyContext) bool {
	c := &e.Conditions

	if c.AutonomyMin > 0 && ctx.Autonomy < c.AutonomyMin {
		return false
	}
	if c.AutonomyMax > 0 && ctx.Autonomy > c.AutonomyMax {
		return false
	}
	if len(c.Modes) > 0 && !containsStr(c.Modes, ctx.ModeID) {
		return false
	}
	if len(c.ExcludeModes) > 0 && containsStr(c.ExcludeModes, ctx.ModeID) {
		return false
	}
	if len(c.ModelCapabilities) > 0 && !containsStr(c.ModelCapabilities, ctx.ModelCapability) {
		return false
	}
	if c.AgenticOnly && !ctx.Agentic {
		return false
	}
	if len(c.Env) > 0 && !containsStr(c.Env, ctx.Env) {
		return false
	}
	return true
}

// containsStr reports whether slice contains the target string.
func containsStr(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
