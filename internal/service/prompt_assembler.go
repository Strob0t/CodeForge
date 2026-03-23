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
	library  *PromptLibraryService
	budget   int // token budget for pruning (0 = no pruning)
	selector PromptVariantSelector
}

// PromptVariantSelector allows the assembler to override entry content
// with evolved prompt variants. If nil, base YAML content is used.
type PromptVariantSelector interface {
	SelectVariant(entryID, modelFamily string) (content string, ok bool)
}

// NewPromptAssembler creates a new assembler backed by the given library.
func NewPromptAssembler(lib *PromptLibraryService, budget int) *PromptAssembler {
	return &PromptAssembler{library: lib, budget: budget}
}

// SetSelector sets the variant selector for prompt evolution integration.
func (a *PromptAssembler) SetSelector(sel PromptVariantSelector) {
	a.selector = sel
}

// AssemblyResult holds the output of prompt assembly including metadata.
type AssemblyResult struct {
	Prompt      string
	Fingerprint string
}

// Assemble builds the full system prompt for the given context.
// Pipeline: filter -> sort -> variant override -> render templates -> prune -> assemble.
func (a *PromptAssembler) Assemble(ctx prompt.AssemblyContext, templateData any) string {
	return a.assembleInternal(ctx, templateData, "").Prompt
}

// AssembleWithFingerprint builds the system prompt and returns both the prompt
// and a SHA256 fingerprint identifying the exact prompt entries used.
func (a *PromptAssembler) AssembleWithFingerprint(ctx prompt.AssemblyContext, templateData any, modelFamily string) AssemblyResult {
	return a.assembleInternal(ctx, templateData, modelFamily)
}

func (a *PromptAssembler) assembleInternal(ctx prompt.AssemblyContext, templateData any, modelFamily string) AssemblyResult {
	// 1. Filter: query the library for matching entries.
	entries := a.library.Query(ctx)
	if len(entries) == 0 {
		return AssemblyResult{}
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

	// 3. Apply variant overrides from evolution selector.
	if a.selector != nil && modelFamily != "" {
		for i := range entries {
			if content, ok := a.selector.SelectVariant(entries[i].ID, modelFamily); ok {
				entries[i].Content = content
			}
		}
	}

	// 4. Compute fingerprint before template rendering (based on raw content).
	fingerprint := prompt.Fingerprint(entries)

	// 5. Render templates and convert to PromptSection.
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

	// 6. Prune to fit budget (reuse existing function).
	if a.budget > 0 {
		sections = PruneToFitBudget(sections, a.budget)
	}

	// 7. Assemble (reuse existing function).
	return AssemblyResult{
		Prompt:      AssembleSections(sections),
		Fingerprint: fingerprint,
	}
}

// FingerprintForMode returns the current prompt fingerprint for a given mode ID.
// Returns empty string if the library has no entries for that mode (caller skips scoring).
func (a *PromptAssembler) FingerprintForMode(modeID string) string {
	if a.library == nil {
		return ""
	}
	ctx := prompt.AssemblyContext{ModeID: modeID}
	entries := a.library.Query(ctx)
	if len(entries) == 0 {
		return ""
	}
	return prompt.Fingerprint(entries)
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
