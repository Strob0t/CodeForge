package service

import (
	"bytes"
	"log/slog"
	"sort"
	"strings"
	"text/template"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// DefaultModePromptBudget is the soft token limit for assembled mode prompts.
const DefaultModePromptBudget = 2048

// Priority constants for prompt section ordering during pruning.
const (
	PrioritySystem     = 100
	PrioritySafety     = 95
	PriorityRole       = 90
	PriorityTools      = 75
	PriorityGuardrails = 70
	PriorityUser       = 40
)

// modeTemplates is parsed from the shared templateFS (declared in meta_agent.go).
var modeTemplates = template.Must(template.ParseFS(templateFS, "templates/mode_*.tmpl"))

// modePromptData carries mode metadata into the prompt templates.
type modePromptData struct {
	Name             string
	Description      string
	Tools            []string
	DeniedTools      []string
	DeniedActions    []string
	RequiredArtifact string
	Autonomy         int
	LLMScenario      string
}

// PromptSection holds a rendered template section with its token estimate.
type PromptSection struct {
	ID       string // DB UUID (empty for embedded defaults)
	Name     string // "role", "tools", "artifact", "actions", "guardrails", custom names
	Text     string // rendered output
	Tokens   int    // estimated via EstimateTokens
	Priority int    // 0-100, higher = more important (kept during pruning)
	Source   string // "embedded", "db_override", "db_custom"
	Enabled  bool   // if false, section is skipped
}

// modeTemplateDef maps section names to template file names with default priority.
var modeTemplateDefs = []struct {
	name     string
	tmplFile string
	priority int
}{
	{"role", "mode_role.tmpl", PriorityRole},
	{"tools", "mode_tools.tmpl", PriorityTools},
	{"artifact", "mode_artifact.tmpl", PriorityGuardrails},
	{"actions", "mode_actions.tmpl", PrioritySafety},
	{"guardrails", "mode_guardrails.tmpl", PriorityGuardrails},
}

// BuildModePrompt assembles a system prompt from modular template sections.
// Custom modes with an explicit PromptPrefix bypass template assembly.
// Returns the assembled prompt string and the individual sections with token counts.
func BuildModePrompt(m *mode.Mode) (string, []PromptSection) {
	// Custom modes with explicit PromptPrefix skip template assembly.
	if !m.Builtin && m.PromptPrefix != "" {
		tokens := cfcontext.EstimateTokens(m.PromptPrefix)
		return m.PromptPrefix, []PromptSection{{
			Name: "custom", Text: m.PromptPrefix, Tokens: tokens,
			Priority: PriorityRole, Source: "embedded", Enabled: true,
		}}
	}

	data := modePromptData{
		Name:             m.Name,
		Description:      m.Description,
		Tools:            m.Tools,
		DeniedTools:      m.DeniedTools,
		DeniedActions:    m.DeniedActions,
		RequiredArtifact: m.RequiredArtifact,
		Autonomy:         m.Autonomy,
		LLMScenario:      m.LLMScenario,
	}

	var sections []PromptSection

	for _, td := range modeTemplateDefs {
		var buf bytes.Buffer
		if err := modeTemplates.ExecuteTemplate(&buf, td.tmplFile, data); err != nil {
			slog.Error("mode template render failed", "template", td.tmplFile, "mode", m.ID, "error", err)
			continue
		}
		text := strings.TrimSpace(buf.String())
		if text == "" {
			continue
		}
		tokens := cfcontext.EstimateTokens(text)
		sections = append(sections, PromptSection{
			Name: td.name, Text: text, Tokens: tokens,
			Priority: td.priority, Source: "embedded", Enabled: true,
		})
	}

	result := AssembleSections(sections)
	if result == "" {
		// Fallback to PromptPrefix if all templates produce empty output.
		slog.Warn("all mode templates empty, falling back to PromptPrefix", "mode", m.ID)
		return m.PromptPrefix, nil
	}

	return result, sections
}

// WarnIfOverBudget logs a warning if the total prompt tokens exceed the budget.
func WarnIfOverBudget(modeID string, sections []PromptSection, budget int) {
	total := 0
	for i := range sections {
		total += sections[i].Tokens
	}
	if total > budget {
		slog.Warn("assembled mode prompt exceeds token budget",
			"mode_id", modeID,
			"total_tokens", total,
			"budget", budget,
			"sections", len(sections),
		)
	}
}

// PruneToFitBudget removes the lowest-priority sections until the total tokens
// fit within the budget. Sections are removed in ascending priority order.
// Returns the surviving sections in their original order.
func PruneToFitBudget(sections []PromptSection, budget int) []PromptSection {
	if budget <= 0 {
		return sections
	}

	total := 0
	for i := range sections {
		total += sections[i].Tokens
	}
	if total <= budget {
		return sections
	}

	// Build sorted index by priority (ascending) for removal order.
	type indexed struct {
		idx      int
		priority int
	}
	order := make([]indexed, len(sections))
	for i := range sections {
		order[i] = indexed{idx: i, priority: sections[i].Priority}
	}
	sort.Slice(order, func(i, j int) bool {
		return order[i].priority < order[j].priority
	})

	removed := make(map[int]bool)
	for _, o := range order {
		if total <= budget {
			break
		}
		total -= sections[o.idx].Tokens
		removed[o.idx] = true
	}

	result := make([]PromptSection, 0, len(sections)-len(removed))
	for i := range sections {
		if !removed[i] {
			result = append(result, sections[i])
		}
	}
	return result
}

// AssembleSections joins prompt sections into a single string.
func AssembleSections(sections []PromptSection) string {
	var buf bytes.Buffer
	for _, s := range sections {
		if !s.Enabled || s.Text == "" {
			continue
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(s.Text)
	}
	return buf.String()
}

// ApplyDBOverrides merges database prompt section rows into the embedded sections.
// Merge strategies: "replace" overwrites the section text, "append" appends to it,
// "prepend" prepends before it. Unmatched DB rows are added as new sections.
// Rows with Enabled=false mark the matching section as disabled.
func ApplyDBOverrides(sections []PromptSection, dbRows []prompt.SectionRow) []PromptSection {
	result := make([]PromptSection, len(sections))
	copy(result, sections)

	matched := make(map[string]bool)

	for _, row := range dbRows {
		found := false
		for i := range result {
			if result[i].Name != row.Name {
				continue
			}
			found = true
			matched[row.Name] = true

			if !row.Enabled {
				result[i].Enabled = false
				result[i].Source = "db_override"
				continue
			}

			switch row.Merge {
			case "replace":
				result[i].Text = row.Content
				result[i].Source = "db_override"
			case "append":
				result[i].Text = result[i].Text + "\n\n" + row.Content
				result[i].Source = "db_override"
			case "prepend":
				result[i].Text = row.Content + "\n\n" + result[i].Text
				result[i].Source = "db_override"
			}
			if row.Priority > 0 {
				result[i].Priority = row.Priority
			}
			result[i].Tokens = cfcontext.EstimateTokens(result[i].Text)
		}

		if !found && row.Enabled {
			result = append(result, PromptSection{
				ID:       row.ID,
				Name:     row.Name,
				Text:     row.Content,
				Tokens:   cfcontext.EstimateTokens(row.Content),
				Priority: row.Priority,
				Source:   "db_custom",
				Enabled:  true,
			})
		}
	}

	return result
}
