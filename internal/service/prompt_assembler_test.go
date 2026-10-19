package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Strob0t/CodeForge/internal/domain/prompt"
	"github.com/Strob0t/CodeForge/internal/port/messagequeue"
)

// testAssemblerFS returns a minimal prompt library FS for assembler testing.
func testAssemblerFS() fstest.MapFS {
	return fstest.MapFS{
		"prompts/identity.yaml": &fstest.MapFile{
			Data: []byte(`
id: identity-core
category: identity
name: Core Identity
priority: 95
sort_order: 0
content: You are CodeForge.
`),
		},
		"prompts/behavior.yaml": &fstest.MapFile{
			Data: []byte(`
id: behavior-coding
category: behavior
name: Coding Standards
priority: 80
sort_order: 0
content: Write clean, tested code.
`),
		},
		"prompts/tools.yaml": &fstest.MapFile{
			Data: []byte(`
id: tools-agentic
category: tools
name: Agentic Tools
priority: 70
sort_order: 0
conditions:
  agentic_only: true
content: "Available tools: Read, Write, Edit, Bash."
`),
		},
		"prompts/reminder.yaml": &fstest.MapFile{
			Data: []byte(`
id: reminder-final
category: reminder
name: Final Reminder
priority: 30
sort_order: 0
content: Always commit your changes.
`),
		},
	}
}

func TestPromptAssembler_Assemble(t *testing.T) {
	t.Parallel()

	t.Run("basic assembly with all matching entries", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  true,
		}
		result := asm.Assemble(ctx, nil)

		// All 3 non-reminder entries should be included.
		if !strings.Contains(result, "You are CodeForge.") {
			t.Error("result should contain identity text")
		}
		if !strings.Contains(result, "Write clean, tested code.") {
			t.Error("result should contain behavior text")
		}
		if !strings.Contains(result, "Available tools") {
			t.Error("result should contain tools text")
		}
		// Reminders fire per turn, never in the system prompt (KI-190).
		if strings.Contains(result, "Always commit your changes.") {
			t.Error("result should not contain reminder text")
		}
	})

	t.Run("non-agentic excludes agentic-only entry", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  false,
		}
		result := asm.Assemble(ctx, nil)

		if strings.Contains(result, "Available tools") {
			t.Error("non-agentic result should NOT contain tools text (agentic_only)")
		}
		if !strings.Contains(result, "You are CodeForge.") {
			t.Error("result should still contain identity text")
		}
	})

	t.Run("empty result when no entries match", func(t *testing.T) {
		t.Parallel()
		// Create a library where all entries have restrictive conditions.
		fsys := fstest.MapFS{
			"prompts/restricted.yaml": &fstest.MapFile{
				Data: []byte(`
id: restricted
category: system
name: Restricted
priority: 50
conditions:
  modes:
    - nonexistent-mode
content: Should not appear.
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{ModeID: "coder"}
		result := asm.Assemble(ctx, nil)
		if result != "" {
			t.Errorf("expected empty result, got %q", result)
		}
	})

	t.Run("empty library returns empty string", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/readme.md": &fstest.MapFile{Data: []byte("nothing")},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		result := asm.Assemble(prompt.AssemblyContext{}, nil)
		if result != "" {
			t.Errorf("expected empty result, got %q", result)
		}
	})
}

func TestPromptAssembler_SortOrder(t *testing.T) {
	t.Parallel()

	t.Run("entries sorted by category order then priority", func(t *testing.T) {
		t.Parallel()
		// identity (cat=0, prio=95) should appear before behavior (cat=3, prio=80)
		// which should appear before tools (cat=5, prio=70). Reminders are not
		// part of the system prompt (KI-190).
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  true,
		}
		result := asm.Assemble(ctx, nil)

		idxIdentity := strings.Index(result, "You are CodeForge.")
		idxBehavior := strings.Index(result, "Write clean, tested code.")
		idxTools := strings.Index(result, "Available tools")

		if idxIdentity > idxBehavior {
			t.Error("identity should come before behavior")
		}
		if idxBehavior > idxTools {
			t.Error("behavior should come before tools")
		}
	})

	t.Run("same category sorted by priority desc then sort_order asc", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/a.yaml": &fstest.MapFile{
				Data: []byte(`
id: behavior-a
category: behavior
name: Behavior A
priority: 80
sort_order: 10
content: Behavior A text.
`),
			},
			"prompts/b.yaml": &fstest.MapFile{
				Data: []byte(`
id: behavior-b
category: behavior
name: Behavior B
priority: 90
sort_order: 5
content: Behavior B text.
`),
			},
			"prompts/c.yaml": &fstest.MapFile{
				Data: []byte(`
id: behavior-c
category: behavior
name: Behavior C
priority: 80
sort_order: 5
content: Behavior C text.
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		result := asm.Assemble(prompt.AssemblyContext{}, nil)

		// B (prio=90) before C (prio=80, sort=5) before A (prio=80, sort=10).
		idxB := strings.Index(result, "Behavior B text.")
		idxC := strings.Index(result, "Behavior C text.")
		idxA := strings.Index(result, "Behavior A text.")

		if idxB > idxC {
			t.Error("B (priority=90) should come before C (priority=80)")
		}
		if idxC > idxA {
			t.Error("C (sort_order=5) should come before A (sort_order=10) within same priority")
		}
	})
}

func TestPromptAssembler_TemplateRendering(t *testing.T) {
	t.Parallel()

	t.Run("renders Go templates with data", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/ctx.yaml": &fstest.MapFile{
				Data: []byte(`
id: context-project
category: context
name: Project Context
priority: 85
content: "Project: {{.ProjectName}}. Path: {{.WorkspacePath}}."
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		data := struct {
			ProjectName   string
			WorkspacePath string
		}{
			ProjectName:   "MyProject",
			WorkspacePath: "/workspace/myproject",
		}

		result := asm.Assemble(prompt.AssemblyContext{}, data)
		if !strings.Contains(result, "Project: MyProject.") {
			t.Errorf("template should render ProjectName, got %q", result)
		}
		if !strings.Contains(result, "Path: /workspace/myproject.") {
			t.Errorf("template should render WorkspacePath, got %q", result)
		}
	})

	t.Run("nil data leaves templated entries out", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/tmpl.yaml": &fstest.MapFile{
				Data: []byte(`
id: tmpl-test
category: system
name: Template Test
priority: 50
content: "Hello {{.Name}}, welcome."
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		result := asm.Assemble(prompt.AssemblyContext{}, nil)
		// A raw template never reaches a prompt (KI-190).
		if result != "" {
			t.Errorf("nil data should leave the templated entry out, got %q", result)
		}
	})

	t.Run("invalid template leaves the entry out", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/bad-tmpl.yaml": &fstest.MapFile{
				Data: []byte(`
id: bad-tmpl
category: system
name: Bad Template
priority: 50
content: "Hello {{.BadSyntax"
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		data := struct{ Name string }{Name: "test"}
		result := asm.Assemble(prompt.AssemblyContext{}, data)
		// A failed template is logged and left out, never sent raw (KI-190).
		if result != "" {
			t.Errorf("bad template should be left out, got %q", result)
		}
	})

	t.Run("template execution error leaves the entry out", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/exec-err.yaml": &fstest.MapFile{
				Data: []byte(`
id: exec-err
category: system
name: Exec Error
priority: 50
content: "Result: {{.MissingMethod | call}}"
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		data := struct{ Name string }{Name: "test"}
		result := asm.Assemble(prompt.AssemblyContext{}, data)
		if result != "" {
			t.Errorf("template execution error should leave the entry out, got %q", result)
		}
	})

	t.Run("content without template markers is not parsed", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/plain.yaml": &fstest.MapFile{
				Data: []byte(`
id: plain
category: system
name: Plain
priority: 50
content: No templates here, just plain text.
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		data := struct{ Name string }{Name: "test"}
		result := asm.Assemble(prompt.AssemblyContext{}, data)
		if result != "No templates here, just plain text." {
			t.Errorf("plain content should pass through unchanged, got %q", result)
		}
	})
}

func TestPromptAssembler_Pruning(t *testing.T) {
	t.Parallel()

	t.Run("budget prunes low-priority entries", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		// Set a very tight budget: identity (18 chars = ~4 tokens) + behavior (25 chars = ~6 tokens)
		// = 10 tokens. Set budget to 10 so tools (39 chars = ~9 tokens) and reminder (28 chars = ~7 tokens) get pruned.
		// Actually let's compute more carefully:
		// "You are CodeForge." = 18 chars = 4 tokens
		// "Write clean, tested code." = 25 chars = 6 tokens
		// So set budget to 10 to keep only these two highest-priority.
		asm := NewPromptAssembler(lib, 10)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  true,
		}
		result := asm.Assemble(ctx, nil)

		// Highest priority entries should survive: identity (95) and behavior (80).
		if !strings.Contains(result, "You are CodeForge.") {
			t.Error("identity (priority=95) should survive pruning")
		}
		if !strings.Contains(result, "Write clean, tested code.") {
			t.Error("behavior (priority=80) should survive pruning")
		}
		// Lower priority entries should be pruned first.
		if strings.Contains(result, "Available tools") {
			t.Error("tools (priority=70) should be pruned")
		}
	})

	t.Run("zero budget means no pruning", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  true,
		}
		result := asm.Assemble(ctx, nil)

		// All entries should be present.
		if !strings.Contains(result, "Available tools") {
			t.Error("with budget=0, no pruning should occur")
		}
	})

	t.Run("large budget keeps everything", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 100000)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  true,
		}
		result := asm.Assemble(ctx, nil)

		// Budget is large enough for all entries.
		if !strings.Contains(result, "You are CodeForge.") {
			t.Error("identity should be present with large budget")
		}
		if !strings.Contains(result, "Available tools") {
			t.Error("tools should be present with large budget")
		}
	})
}

func TestPromptAssembler_SectionsJoin(t *testing.T) {
	t.Parallel()

	t.Run("sections are joined with double newline", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{
			ModeID:   "coder",
			Autonomy: 3,
			Agentic:  true,
		}
		result := asm.Assemble(ctx, nil)

		// Sections should be separated by "\n\n".
		parts := strings.Split(result, "\n\n")
		if len(parts) < 2 {
			t.Errorf("expected multiple sections separated by double newline, got %d parts", len(parts))
		}
	})
}

func TestRenderEntry(t *testing.T) {
	t.Parallel()

	data := struct{ Name string }{Name: "World"}
	tests := []struct {
		name    string
		content string
		noData  bool
		want    string
		wantErr bool
	}{
		{name: "content without markers is returned unchanged", content: "No templates here", want: "No templates here"},
		{name: "content without markers needs no data", content: "No templates here", noData: true, want: "No templates here"},
		{name: "renders template with valid data", content: "Hello {{.Name}}!", want: "Hello World!"},
		// A raw template never reaches a prompt (KI-190).
		{name: "templated entry without data is an error", content: "Hello {{.Name}}", noData: true, wantErr: true},
		{name: "parse error", content: "Bad {{.Syntax", wantErr: true},
		{name: "missing field", content: "Value: {{.Missing.Deep.Field}}", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := &prompt.PromptEntry{ID: "test", Content: tc.content}
			var got string
			var err error
			if tc.noData {
				got, err = renderEntry(e, nil)
			} else {
				got, err = renderEntry(e, data)
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("renderEntry() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("renderEntry() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEvaluateReminders(t *testing.T) {
	t.Parallel()

	t.Run("returns nil when promptAssembler is nil", func(t *testing.T) {
		t.Parallel()
		svc := &ConversationService{} // no promptAssembler set
		result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("returns nil when library is nil", func(t *testing.T) {
		t.Parallel()
		svc := &ConversationService{
			promptAssembler: &PromptAssembler{library: nil},
		}
		result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, nil)
		if result != nil {
			t.Errorf("expected nil, got %v", result)
		}
	})

	t.Run("returns nil when library has no reminders", func(t *testing.T) {
		t.Parallel()
		// Library with no reminder entries.
		fsys := fstest.MapFS{
			"prompts/identity.yaml": &fstest.MapFile{
				Data: []byte(`
id: identity-core
category: identity
name: Core Identity
priority: 95
content: You are CodeForge.
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		svc := &ConversationService{
			promptAssembler: NewPromptAssembler(lib, 0),
		}
		result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, nil)
		if result != nil {
			t.Errorf("expected nil (no reminders in library), got %v", result)
		}
	})

	t.Run("returns rendered reminders from library", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/reminder.yaml": &fstest.MapFile{
				Data: []byte(`
id: reminder-budget
category: reminder
name: Budget Warning
priority: 50
conditions:
  modes: [coder]
content: "Budget at {{.BudgetPercent}}%."
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		svc := &ConversationService{
			promptAssembler: NewPromptAssembler(lib, 0),
		}
		history := []messagequeue.ConversationMessagePayload{
			{Role: "user", Content: "hello"},
		}
		result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, history)
		if len(result) != 1 {
			t.Fatalf("expected 1 reminder, got %d", len(result))
		}
		if result[0] != "Budget at 0%." {
			t.Errorf("reminder = %q, want %q", result[0], "Budget at 0%.")
		}
	})

	t.Run("renders StallIterations from history", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/reminder.yaml": &fstest.MapFile{
				Data: []byte(`
id: reminder-stall
category: reminder
name: Stall Warning
priority: 50
conditions:
  modes: [coder]
content: "Stall: {{.StallIterations}}"
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		svc := &ConversationService{
			promptAssembler: NewPromptAssembler(lib, 0),
		}
		history := []messagequeue.ConversationMessagePayload{
			{Role: "tool", Name: "Edit", Content: "ok"},    // progress → reset
			{Role: "tool", Name: "Read", Content: "data"},  // non-progress → 1
			{Role: "tool", Name: "Glob", Content: "files"}, // non-progress → 2
		}
		result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, history)
		if len(result) != 1 {
			t.Fatalf("expected 1 reminder, got %d", len(result))
		}
		if result[0] != "Stall: 2" {
			t.Errorf("reminder = %q, want %q", result[0], "Stall: 2")
		}
	})

	t.Run("renders TurnCount from history length", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/reminder.yaml": &fstest.MapFile{
				Data: []byte(`
id: reminder-turns
category: reminder
name: Turn Count
priority: 50
conditions:
  modes: [coder]
content: "Turns: {{.TurnCount}}"
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		svc := &ConversationService{
			promptAssembler: NewPromptAssembler(lib, 0),
		}
		history := []messagequeue.ConversationMessagePayload{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: "bye"},
		}
		result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, history)
		if len(result) != 1 {
			t.Fatalf("expected 1 reminder, got %d", len(result))
		}
		if result[0] != "Turns: 3" {
			t.Errorf("reminder = %q, want %q", result[0], "Turns: 3")
		}
	})

	t.Run("reminder without a condition never fires", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/reminder.yaml": &fstest.MapFile{
				Data: []byte(`
id: reminder-always
category: reminder
name: Always
priority: 50
content: "Always shown."
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		svc := &ConversationService{
			promptAssembler: NewPromptAssembler(lib, 0),
		}
		history := []messagequeue.ConversationMessagePayload{{Role: "user", Content: "hello"}}
		if result := svc.evaluateReminders(context.Background(), "conv-1", reminderCoderMode, 3, history); len(result) != 0 {
			t.Errorf("expected no reminders, got %q", result)
		}
	})
}

func TestConversationRunStartPayload_RemindersField(t *testing.T) {
	t.Parallel()

	t.Run("reminders field round-trips through JSON", func(t *testing.T) {
		t.Parallel()
		payload := messagequeue.ConversationRunStartPayload{
			RunID:          "run-1",
			ConversationID: "conv-1",
			ProjectID:      "proj-1",
			SystemPrompt:   "You are an assistant.",
			Model:          "gpt-4",
			Agentic:        true,
			Reminders:      []string{"Check your budget.", "Stay on topic."},
		}

		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var decoded messagequeue.ConversationRunStartPayload
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		if len(decoded.Reminders) != 2 {
			t.Fatalf("expected 2 reminders, got %d", len(decoded.Reminders))
		}
		if decoded.Reminders[0] != "Check your budget." {
			t.Errorf("reminder[0] = %q, want %q", decoded.Reminders[0], "Check your budget.")
		}
		if decoded.Reminders[1] != "Stay on topic." {
			t.Errorf("reminder[1] = %q, want %q", decoded.Reminders[1], "Stay on topic.")
		}
	})

	t.Run("empty reminders omitted from JSON", func(t *testing.T) {
		t.Parallel()
		payload := messagequeue.ConversationRunStartPayload{
			RunID:          "run-1",
			ConversationID: "conv-1",
			ProjectID:      "proj-1",
			Model:          "gpt-4",
			Agentic:        true,
		}

		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		// With omitempty, nil slice should not appear in JSON.
		if strings.Contains(string(data), `"reminders"`) {
			t.Error("nil reminders should be omitted from JSON (omitempty)")
		}
	})

	t.Run("nil reminders deserializes as nil", func(t *testing.T) {
		t.Parallel()
		raw := `{"run_id":"r","conversation_id":"c","project_id":"p","model":"m","agentic":true}`
		var payload messagequeue.ConversationRunStartPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if payload.Reminders != nil {
			t.Errorf("expected nil reminders, got %v", payload.Reminders)
		}
	})
}

// --- Fingerprint tests ---

func TestPromptAssembler_AssembleWithFingerprint(t *testing.T) {
	t.Parallel()

	t.Run("returns non-empty fingerprint", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)

		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}
		result := asm.AssembleWithFingerprint(ctx, nil, "openai")

		if result.Prompt == "" {
			t.Error("expected non-empty prompt")
		}
		if result.Fingerprint == "" {
			t.Error("expected non-empty fingerprint")
		}
		if len(result.Fingerprint) != 64 {
			t.Errorf("fingerprint length = %d, want 64 (SHA256 hex)", len(result.Fingerprint))
		}
	})

	t.Run("same context produces same fingerprint", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}

		r1 := asm.AssembleWithFingerprint(ctx, nil, "openai")
		r2 := asm.AssembleWithFingerprint(ctx, nil, "openai")

		if r1.Fingerprint != r2.Fingerprint {
			t.Error("same context should produce same fingerprint")
		}
	})

	t.Run("empty result has empty fingerprint", func(t *testing.T) {
		t.Parallel()
		fsys := fstest.MapFS{
			"prompts/restricted.yaml": &fstest.MapFile{
				Data: []byte(`
id: restricted
category: system
name: Restricted
priority: 50
conditions:
  modes:
    - nonexistent-mode
content: Should not appear.
`),
			},
		}
		lib, err := NewPromptLibraryService(fsys, "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		result := asm.AssembleWithFingerprint(prompt.AssemblyContext{ModeID: "coder"}, nil, "openai")
		if result.Prompt != "" {
			t.Error("expected empty prompt")
		}
		if result.Fingerprint != "" {
			t.Error("expected empty fingerprint for empty result")
		}
	})

	t.Run("Assemble returns same prompt as AssembleWithFingerprint", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}

		plain := asm.Assemble(ctx, nil)
		withFP := asm.AssembleWithFingerprint(ctx, nil, "openai")

		if plain != withFP.Prompt {
			t.Error("Assemble and AssembleWithFingerprint should produce same prompt text")
		}
	})
}

// --- Variant selector integration tests ---

// mockVariantSelector is a test double for PromptVariantSelector.
type mockVariantSelector struct {
	variants map[string]string // entryID -> overridden content
}

func (m *mockVariantSelector) SelectVariant(entryID, modelFamily string) (string, bool) {
	content, ok := m.variants[entryID]
	return content, ok
}

func TestPromptAssembler_VariantSelector(t *testing.T) {
	t.Parallel()

	t.Run("nil selector uses base content", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		// No selector set.

		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}
		result := asm.AssembleWithFingerprint(ctx, nil, "openai")
		if !strings.Contains(result.Prompt, "You are CodeForge.") {
			t.Error("should use base content when no selector set")
		}
	})

	t.Run("selector overrides matching entry", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		asm.SetSelector(&mockVariantSelector{
			variants: map[string]string{
				"identity-core": "You are CodeForge EVOLVED.",
			},
		})

		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}
		result := asm.AssembleWithFingerprint(ctx, nil, "openai")

		if !strings.Contains(result.Prompt, "You are CodeForge EVOLVED.") {
			t.Error("selector should override identity content")
		}
		if strings.Contains(result.Prompt, "You are CodeForge.") {
			t.Error("original identity content should be replaced")
		}
	})

	t.Run("selector does not affect non-matching entries", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		asm.SetSelector(&mockVariantSelector{
			variants: map[string]string{
				"identity-core": "Evolved identity.",
			},
		})

		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}
		result := asm.AssembleWithFingerprint(ctx, nil, "openai")

		// Non-overridden entries should keep original content.
		if !strings.Contains(result.Prompt, "Write clean, tested code.") {
			t.Error("non-overridden behavior entry should keep original content")
		}
	})

	t.Run("selector not applied when modelFamily is empty", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		asm := NewPromptAssembler(lib, 0)
		asm.SetSelector(&mockVariantSelector{
			variants: map[string]string{
				"identity-core": "Should not appear.",
			},
		})

		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}
		// Empty modelFamily — selector should not be applied.
		result := asm.AssembleWithFingerprint(ctx, nil, "")

		if strings.Contains(result.Prompt, "Should not appear.") {
			t.Error("selector should not be applied when modelFamily is empty")
		}
		if !strings.Contains(result.Prompt, "You are CodeForge.") {
			t.Error("original content should be used")
		}
	})

	t.Run("variant changes fingerprint", func(t *testing.T) {
		t.Parallel()
		lib, err := NewPromptLibraryService(testAssemblerFS(), "prompts")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		asmBase := NewPromptAssembler(lib, 0)
		asmEvolved := NewPromptAssembler(lib, 0)
		asmEvolved.SetSelector(&mockVariantSelector{
			variants: map[string]string{
				"identity-core": "Evolved identity text.",
			},
		})

		ctx := prompt.AssemblyContext{ModeID: "coder", Autonomy: 3, Agentic: true}
		base := asmBase.AssembleWithFingerprint(ctx, nil, "openai")
		evolved := asmEvolved.AssembleWithFingerprint(ctx, nil, "openai")

		if base.Fingerprint == evolved.Fingerprint {
			t.Error("variant override should produce different fingerprint")
		}
	})
}

// --- Integration tests using the real embedded prompt library ---

func TestBuildSystemPrompt_ModularAssembler(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load prompt library: %v", err)
	}

	assembler := NewPromptAssembler(lib, 0)

	ctx := prompt.AssemblyContext{
		ModeID:   "coder",
		Autonomy: 3,
		Env:      "development",
		Agentic:  true,
	}
	result := assembler.Assemble(ctx, nil)
	if result == "" {
		t.Fatal("expected non-empty assembled prompt for coder mode")
	}

	// Verify identity section is present.
	if !strings.Contains(result, "CodeForge") {
		t.Error("expected identity section with 'CodeForge'")
	}
	// Verify behavior rules about reading before modifying.
	if !strings.Contains(result, "read") && !strings.Contains(result, "Read") {
		t.Error("expected behavior rules about reading files")
	}
	// Verify coder mode-specific content is included.
	if !strings.Contains(result, "Coder") && !strings.Contains(result, "coder") {
		t.Error("expected coder mode-specific content")
	}
	// Verify autonomy level 3 (auto-edit) is included.
	if !strings.Contains(result, "AUTO-EDIT") {
		t.Error("expected AUTO-EDIT autonomy content for level 3")
	}
}

func TestBuildSystemPrompt_IncludesChatFirstOrchestration(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load prompt library: %v", err)
	}

	// Verify the entry exists in the library by ID.
	entry := lib.GetEntry("behavior.chat_first_orchestration")
	if entry == nil {
		t.Fatal("behavior.chat_first_orchestration entry not found in embedded library")
	}
	if entry.Category != prompt.CategoryBehavior {
		t.Errorf("expected category %q, got %q", prompt.CategoryBehavior, entry.Category)
	}

	// Use the same AssemblyContext as BuildSystemPrompt (coder, autonomy 3, agentic).
	assembler := NewPromptAssembler(lib, 0)
	asmCtx := prompt.AssemblyContext{
		ModeID:   "coder",
		Autonomy: 3,
		Env:      "development",
		Agentic:  true,
	}
	result := assembler.Assemble(asmCtx, nil)
	if result == "" {
		t.Fatal("expected non-empty assembled prompt")
	}

	// Verify key strings from the orchestrator prompt are present.
	requiredStrings := []struct {
		needle string
		desc   string
	}{
		{"Goal Extraction", "section 1 heading"},
		{"propose_goal", "propose_goal tool reference"},
		{"Roadmap Generation", "section 2 heading"},
		{"propose_roadmap", "propose_roadmap tool reference"},
		{"Atomic Step Design", "section 5 heading"},
		{"Proactive Suggestions", "section 3 heading"},
		{"Confidence-Based Escalation", "section 6 heading"},
	}
	for _, tc := range requiredStrings {
		if !strings.Contains(result, tc.needle) {
			t.Errorf("assembled prompt missing %q (%s)", tc.needle, tc.desc)
		}
	}

	// spawn_subagent starts nothing yet and is not offered to the model (KI-25).
	if strings.Contains(result, "spawn_subagent") {
		t.Error("assembled prompt tells the model to use spawn_subagent, which is not offered")
	}

	// Verify it also appears for a zero-condition context (empty conditions = always match).
	minimalCtx := prompt.AssemblyContext{}
	minimalResult := assembler.Assemble(minimalCtx, nil)
	if !strings.Contains(minimalResult, "Goal Extraction") {
		t.Error("chat-first orchestration should match unconditionally (conditions: {}), but missing for empty context")
	}
}

func TestAssemble_AutonomyLevels_ProduceDifferentOutput(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load prompt library: %v", err)
	}
	assembler := NewPromptAssembler(lib, 0)

	supervised := assembler.Assemble(prompt.AssemblyContext{
		ModeID: "coder", Autonomy: 1, Agentic: true,
	}, nil)
	headless := assembler.Assemble(prompt.AssemblyContext{
		ModeID: "coder", Autonomy: 5, Agentic: true,
	}, nil)

	if supervised == headless {
		t.Error("supervised (level 1) and headless (level 5) should produce different prompts")
	}

	if !strings.Contains(supervised, "SUPERVISED") {
		t.Error("supervised output should contain 'SUPERVISED'")
	}
	if !strings.Contains(headless, "HEADLESS") {
		t.Error("headless output should contain 'HEADLESS'")
	}
}

func TestAssemble_DifferentModes_ProduceDifferentOutput(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load prompt library: %v", err)
	}
	assembler := NewPromptAssembler(lib, 0)

	coder := assembler.Assemble(prompt.AssemblyContext{
		ModeID: "coder", Autonomy: 3, Agentic: true,
	}, nil)
	reviewer := assembler.Assemble(prompt.AssemblyContext{
		ModeID: "reviewer", Autonomy: 3, Agentic: true,
	}, nil)

	if coder == reviewer {
		t.Error("coder and reviewer modes should produce different prompts")
	}
}

func TestAssemble_AgenticVsNonAgentic_ProduceDifferentOutput(t *testing.T) {
	t.Parallel()

	lib, err := NewPromptLibraryService(promptsFS, "prompts")
	if err != nil {
		t.Fatalf("failed to load prompt library: %v", err)
	}
	assembler := NewPromptAssembler(lib, 0)

	agentic := assembler.Assemble(prompt.AssemblyContext{
		ModeID: "coder", Autonomy: 3, Agentic: true,
	}, nil)
	nonAgentic := assembler.Assemble(prompt.AssemblyContext{
		ModeID: "coder", Autonomy: 3, Agentic: false,
	}, nil)

	if agentic == nonAgentic {
		t.Error("agentic and non-agentic should produce different prompts")
	}
	// Agentic output should be longer (more tool/action sections included).
	if len(agentic) <= len(nonAgentic) {
		t.Errorf("agentic prompt (%d bytes) should be longer than non-agentic (%d bytes)",
			len(agentic), len(nonAgentic))
	}
}

func TestCountStallIterations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		history []messagequeue.ConversationMessagePayload
		want    int
	}{
		{
			name:    "empty history",
			history: nil,
			want:    0,
		},
		{
			name: "user messages only",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
			},
			want: 0,
		},
		{
			name: "progress tool resets count",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "tool", Name: "Read", Content: "data"},
				{Role: "tool", Name: "Edit", Content: "ok"},
			},
			want: 0,
		},
		{
			name: "consecutive non-progress tools",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "tool", Name: "Read", Content: "data"},
				{Role: "tool", Name: "Glob", Content: "files"},
				{Role: "tool", Name: "Search", Content: "results"},
			},
			want: 3,
		},
		{
			name: "progress in middle resets",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "tool", Name: "Read", Content: "data"},
				{Role: "tool", Name: "Read", Content: "data"},
				{Role: "tool", Name: "Write", Content: "ok"},
				{Role: "tool", Name: "Glob", Content: "files"},
			},
			want: 1,
		},
		{
			name: "assistant messages ignored",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "tool", Name: "Read", Content: "data"},
				{Role: "assistant", Content: "thinking"},
				{Role: "tool", Name: "Search", Content: "results"},
			},
			want: 2,
		},
		{
			// The worker stores its own tool names (KI-190).
			name: "worker progress tool names reset",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "tool", Name: "read_file", Content: "data"},
				{Role: "tool", Name: "edit_file", Content: "ok"},
				{Role: "tool", Name: "read_file", Content: "data"},
				{Role: "tool", Name: "bash", Content: "ok"},
				{Role: "tool", Name: "search_files", Content: "x"},
				{Role: "tool", Name: "write_file", Content: "ok"},
			},
			want: 0,
		},
		{
			name: "worker read tools count",
			history: []messagequeue.ConversationMessagePayload{
				{Role: "tool", Name: "write_file", Content: "ok"},
				{Role: "tool", Name: "read_file", Content: "data"},
				{Role: "tool", Name: "list_directory", Content: "data"},
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := countStallIterations(tt.history)
			if got != tt.want {
				t.Errorf("countStallIterations() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPromptsFS_Accessor(t *testing.T) {
	t.Parallel()

	fs := PromptsFS()
	// Verify the accessor returns a usable FS by reading a known directory.
	entries, err := fs.ReadDir("prompts")
	if err != nil {
		t.Fatalf("PromptsFS().ReadDir('prompts') failed: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one entry in prompts/ directory")
	}
}
