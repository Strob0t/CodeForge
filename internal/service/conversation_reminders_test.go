package service

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/port/eventstore"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// KI-190: reminders fire only under their own runtime condition, the system
// prompt carries none of them, and no raw template reaches a prompt.

// reminderCostStore reports the conversation's accumulated cost.
type reminderCostStore struct {
	trajectoryMockEventStore
	cost float64
}

func (m *reminderCostStore) TrajectoryStats(_ context.Context, _ string) (*eventstore.TrajectorySummary, error) {
	return &eventstore.TrajectorySummary{TotalCostUSD: m.cost}, nil
}

var (
	reminderCoderMode = &messagequeue.ModePayload{
		ID:    "coder",
		Tools: []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep", "ListDir"},
	}
	reminderArchitectMode = &messagequeue.ModePayload{
		ID:          "architect",
		Tools:       []string{"Read", "Glob", "Grep", "ListDir"},
		DeniedTools: []string{"Write", "Edit", "Bash"},
	}
)

const (
	planReminderText   = "You are in PLAN mode"
	stallReminderText  = "No meaningful progress detected"
	budgetReminderText = "of the cost budget"
)

// turnHistory is a conversation whose last turn made the given tool calls
// (worker tool names), followed by the new user message.
func turnHistory(tools ...string) []messagequeue.ConversationMessagePayload {
	h := []messagequeue.ConversationMessagePayload{{Role: "user", Content: "implement the feature"}}
	for _, name := range tools {
		h = append(h,
			messagequeue.ConversationMessagePayload{Role: "assistant", Content: ""},
			messagequeue.ConversationMessagePayload{Role: "tool", Name: name, Content: "ok"},
		)
	}
	return append(h, messagequeue.ConversationMessagePayload{Role: "user", Content: "go on"})
}

// reads returns n read_file calls.
func reads(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "read_file"
	}
	return out
}

func assertNoRawTemplate(t *testing.T, label, text string) {
	t.Helper()
	if strings.Contains(text, "{{") || strings.Contains(text, "}}") || strings.Contains(text, "<no value>") {
		t.Errorf("%s contains an unrendered template: %q", label, text)
	}
}

func TestEvaluateReminders_FireOnlyUnderTheirCondition(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)

	tests := []struct {
		name     string
		cost     float64
		mode     *messagequeue.ModePayload
		autonomy int
		history  []messagequeue.ConversationMessagePayload
		want     []string
		notWant  []string
	}{
		{
			name: "first turn of a coder conversation carries none",
			mode: reminderCoderMode, autonomy: 3,
			history: []messagequeue.ConversationMessagePayload{{Role: "user", Content: "add a login page"}},
		},
		{
			name:    "no mode (no mode service) on the first turn carries none",
			history: []messagequeue.ConversationMessagePayload{{Role: "user", Content: "add a login page"}},
		},
		{
			name: "edit/test loop is no stall",
			mode: reminderCoderMode, autonomy: 3,
			history: turnHistory("read_file", "edit_file", "bash", "edit_file", "bash", "read_file", "edit_file", "bash"),
		},
		{
			name: "nine reads are below the stall minimum",
			mode: reminderCoderMode, autonomy: 3,
			history: turnHistory(reads(9)...),
			notWant: []string{stallReminderText},
		},
		{
			name: "ten reads without progress are a stall",
			mode: reminderCoderMode, autonomy: 3,
			history: turnHistory(reads(10)...),
			want:    []string{stallReminderText + " in 10 iterations"},
		},
		{
			name: "a worker edit_file call is progress",
			mode: reminderCoderMode, autonomy: 3,
			history: turnHistory(append(reads(10), "edit_file")...),
			notWant: []string{stallReminderText},
		},
		{
			name: "a read-only mode never stalls on reads",
			mode: reminderArchitectMode, autonomy: 2,
			history: turnHistory(reads(20)...),
			want:    []string{planReminderText},
			notWant: []string{stallReminderText},
		},
		{
			name: "budget below 80 percent",
			cost: 3.99, mode: reminderCoderMode, autonomy: 3,
			history: turnHistory("edit_file"),
			notWant: []string{budgetReminderText},
		},
		{
			name: "budget at 80 percent",
			cost: 4.0, mode: reminderCoderMode, autonomy: 3,
			history: turnHistory("edit_file"),
			want:    []string{"You have used 80% of the cost budget ($4.0000 / $5.00)"},
		},
		{
			name: "plan reminder only in a planning mode",
			mode: reminderArchitectMode, autonomy: 2,
			history: []messagequeue.ConversationMessagePayload{{Role: "user", Content: "design the cache"}},
			want:    []string{planReminderText},
			notWant: []string{stallReminderText, budgetReminderText},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &reminderCostStore{cost: tc.cost}
			// Both entry points: the prompt service and the conversation
			// service's fallback without it.
			promptSvc := NewPromptAssemblyService(nil, nil, nil, nil, NewPromptAssembler(lib, 0), store, nil)
			convSvc := &ConversationService{promptAssembler: NewPromptAssembler(lib, 0), events: store}
			results := map[string][]string{
				"prompt service":       promptSvc.EvaluateReminders(context.Background(), "conv-1", tc.mode, tc.autonomy, tc.history),
				"conversation service": convSvc.evaluateReminders(context.Background(), "conv-1", tc.mode, tc.autonomy, tc.history),
			}
			for label, got := range results {
				joined := strings.Join(got, "\n")
				if len(tc.want) == 0 && len(got) != 0 {
					t.Errorf("%s: want no reminders, got %q", label, got)
				}
				for _, w := range tc.want {
					if !strings.Contains(joined, w) {
						t.Errorf("%s: reminders %q lack %q", label, got, w)
					}
				}
				for _, nw := range tc.notWant {
					if strings.Contains(joined, nw) {
						t.Errorf("%s: reminders %q contain %q", label, got, nw)
					}
				}
				for _, r := range got {
					assertNoRawTemplate(t, label, r)
				}
			}
		})
	}
}

// Every embedded reminder names the condition it fires on; a reminder
// without one would be sent on every turn.
func TestEmbeddedReminders_HaveACondition(t *testing.T) {
	t.Parallel()
	reminders := loadEmbeddedLibrary(t).GetByCategory(prompt.CategoryReminder)
	if len(reminders) == 0 {
		t.Fatal("no embedded reminders")
	}
	for i := range reminders {
		if !reminders[i].HasReminderCondition() {
			t.Errorf("reminder %q has no condition", reminders[i].ID)
		}
	}
}

func TestAssemble_ExcludesReminders(t *testing.T) {
	t.Parallel()

	t.Run("test library", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		got := NewPromptAssembler(lib, 0).Assemble(prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}, nil)
		if strings.Contains(got, "Always commit your changes.") {
			t.Errorf("assembled prompt contains a reminder: %q", got)
		}
		if !strings.Contains(got, "You are CodeForge.") {
			t.Errorf("assembled prompt lost the identity entry: %q", got)
		}
	})

	t.Run("embedded library, every context", func(t *testing.T) {
		t.Parallel()
		lib := loadEmbeddedLibrary(t)
		asm := NewPromptAssembler(lib, 0)
		reminders := lib.GetByCategory(prompt.CategoryReminder)
		for _, ctx := range generateAllContexts() {
			got := asm.Assemble(ctx, conversationPromptData{ProjectName: "p"})
			for i := range reminders {
				first := strings.SplitN(strings.TrimSpace(reminders[i].Content), "\n", 2)[0]
				if len(first) > 40 {
					first = first[:40]
				}
				if strings.Contains(got, first) {
					t.Fatalf("%s: assembled prompt contains reminder %q", contextString(ctx), reminders[i].ID)
				}
			}
		}
	})
}

// No assembled system prompt carries a raw template, with or without data.
func TestAssemble_NoRawTemplates(t *testing.T) {
	t.Parallel()
	lib := loadEmbeddedLibrary(t)
	asm := NewPromptAssembler(lib, 0)
	full := conversationPromptData{
		ProjectName: "demo", ProjectDescription: "d", WorkspacePath: "/ws", Provider: "github",
		RepoURL: "https://example.invalid/r", Stack: "Go", Agents: []string{"a"}, Modes: []string{"coder"},
		RecentTasks:    []conversationTaskSummary{{ID: "t1", Name: "task", Status: "done"}},
		RoadmapSummary: "rm", GoalContext: "goal",
		BuiltinTools: []builtinToolSummary{{Name: "Read", Description: "read"}},
	}
	cases := []struct {
		label    string
		assemble func(prompt.AssemblyContext) string
	}{
		{"nil data", func(c prompt.AssemblyContext) string { return asm.Assemble(c, nil) }},
		{"empty data", func(c prompt.AssemblyContext) string { return asm.Assemble(c, conversationPromptData{}) }},
		{"full data", func(c prompt.AssemblyContext) string { return asm.Assemble(c, full) }},
	}
	for _, tc := range cases {
		for _, ctx := range generateAllContexts() {
			got := tc.assemble(ctx)
			if strings.Contains(got, "{{") || strings.Contains(got, "<no value>") {
				t.Fatalf("%s, %s: raw template in the assembled prompt", tc.label, contextString(ctx))
			}
		}
	}
}

// A template that fails to render is skipped, never sent raw.
func TestAssemble_TemplateErrorSkipsEntry(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"prompts/ok.yaml": &fstest.MapFile{Data: []byte(`
id: ok
category: identity
name: OK
priority: 90
content: You are CodeForge.
`)},
		"prompts/parse.yaml": &fstest.MapFile{Data: []byte(`
id: parse-err
category: system
name: Parse Error
priority: 50
content: "Hello {{.BadSyntax"
`)},
		"prompts/exec.yaml": &fstest.MapFile{Data: []byte(`
id: exec-err
category: system
name: Exec Error
priority: 40
content: "Budget {{.BudgetPercent}}"
`)},
	}
	lib, err := NewPromptLibraryService(fsys, "prompts")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := NewPromptAssembler(lib, 0).Assemble(prompt.AssemblyContext{}, conversationPromptData{ProjectName: "p"})
	if got != "You are CodeForge." {
		t.Errorf("assembled prompt = %q, want only the entry that rendered", got)
	}
}
