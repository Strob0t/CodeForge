package service

import (
	"bytes"
	"sort"
	"strings"
	"text/template"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/prompt"
)

// PromptAssembler builds system prompts from the modular prompt library.
type PromptAssembler struct {
	library *PromptLibraryService
	budget  int // token budget for pruning (0 = no pruning)
}

// NewPromptAssembler creates a new assembler backed by the given library.
func NewPromptAssembler(lib *PromptLibraryService, budget int) *PromptAssembler {
	return &PromptAssembler{library: lib, budget: budget}
}

// Assemble builds the full system prompt for the given context.
// Pipeline: filter -> sort -> render templates -> convert to PromptSection -> prune -> assemble.
func (a *PromptAssembler) Assemble(ctx prompt.AssemblyContext, templateData any) string {
	// 1. Filter: query the library for matching entries.
	entries := a.library.Query(ctx)
	if len(entries) == 0 {
		return ""
	}

	// 2. Sort: by category order, then priority desc, then sort_order asc.
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

	// 3. Render templates and convert to PromptSection.
	var sections []PromptSection
	for i := range entries {
		text := renderEntry(&entries[i], templateData)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		sections = append(sections, PromptSection{
			Name:     entries[i].Name,
			Text:     text,
			Tokens:   cfcontext.EstimateTokens(text),
			Priority: entries[i].Priority,
			Source:   "library",
			Enabled:  true,
		})
	}

	// 4. Prune to fit budget (reuse existing function).
	if a.budget > 0 {
		sections = PruneToFitBudget(sections, a.budget)
	}

	// 5. Assemble (reuse existing function).
	return AssembleSections(sections)
}

// renderEntry renders a prompt entry's content, executing Go templates if present.
func renderEntry(e *prompt.PromptEntry, data any) string {
	if data == nil || !strings.Contains(e.Content, "{{") {
		return e.Content
	}
	tmpl, err := template.New(e.ID).Parse(e.Content)
	if err != nil {
		return e.Content // return raw content on parse failure
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return e.Content // return raw content on execution failure
	}
	return buf.String()
}
