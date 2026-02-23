package service

import (
	"bytes"
	"log/slog"
	"strings"
	"text/template"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/mode"
)

// DefaultModePromptBudget is the soft token limit for assembled mode prompts.
// A warning is logged when the total exceeds this value.
const DefaultModePromptBudget = 1024

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
	Name   string // "role", "tools", "artifact", "actions", "guardrails"
	Text   string // rendered output
	Tokens int    // estimated via EstimateTokens
}

// modeTemplateDef maps section names to template file names.
var modeTemplateDefs = []struct {
	name     string
	tmplFile string
}{
	{"role", "mode_role.tmpl"},
	{"tools", "mode_tools.tmpl"},
	{"artifact", "mode_artifact.tmpl"},
	{"actions", "mode_actions.tmpl"},
	{"guardrails", "mode_guardrails.tmpl"},
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
	var assembled bytes.Buffer

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
		})
		if assembled.Len() > 0 {
			assembled.WriteString("\n\n")
		}
		assembled.WriteString(text)
	}

	result := assembled.String()
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
