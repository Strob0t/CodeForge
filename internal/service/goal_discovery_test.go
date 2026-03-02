package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfcontext "github.com/Strob0t/CodeForge/internal/domain/context"
	"github.com/Strob0t/CodeForge/internal/domain/goal"
)

// --- Detection tests (unit, no DB) ---

func TestDetectGSD(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, ".planning")
	if err := os.MkdirAll(planDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(planDir, "PROJECT.md"), "# My Project\nWe build cool stuff.")
	writeFile(t, filepath.Join(planDir, "REQUIREMENTS.md"), "# Requirements\n- Feature A\n- Feature B")
	writeFile(t, filepath.Join(planDir, "STATE.md"), "# Current State\nPhase 2 in progress.")

	goals := detectGoalFiles(dir)

	if len(goals) != 3 {
		t.Fatalf("expected 3 goals, got %d", len(goals))
	}

	byKind := groupByKind(goals)
	if _, ok := byKind[goal.KindVision]; !ok {
		t.Error("expected vision goal from PROJECT.md")
	}
	if _, ok := byKind[goal.KindRequirement]; !ok {
		t.Error("expected requirement goal from REQUIREMENTS.md")
	}
	if _, ok := byKind[goal.KindState]; !ok {
		t.Error("expected state goal from STATE.md")
	}
}

func TestDetectReadme(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "README.md"), "# My Project\n\nThis is the vision.\n\n## Setup\nInstall stuff.")

	goals := detectGoalFiles(dir)

	if len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goals))
	}
	if goals[0].Kind != goal.KindVision {
		t.Errorf("expected vision kind, got %q", goals[0].Kind)
	}
	if goals[0].Source != "readme" {
		t.Errorf("expected source readme, got %q", goals[0].Source)
	}
	// Should only contain first section (before ## Setup)
	if strings.Contains(goals[0].Content, "Install stuff") {
		t.Error("README content should not include sections after first ##")
	}
}

func TestDetectClaudeMd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Claude Instructions\nAlways use Go.")

	goals := detectGoalFiles(dir)

	if len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goals))
	}
	if goals[0].Kind != goal.KindConstraint {
		t.Errorf("expected constraint kind, got %q", goals[0].Kind)
	}
	if goals[0].Source != "claude_md" {
		t.Errorf("expected source claude_md, got %q", goals[0].Source)
	}
}

func TestDetectEmpty(t *testing.T) {
	dir := t.TempDir()

	goals := detectGoalFiles(dir)

	if len(goals) != 0 {
		t.Fatalf("expected 0 goals from empty workspace, got %d", len(goals))
	}
}

func TestDetectMixed(t *testing.T) {
	dir := t.TempDir()

	// GSD
	planDir := filepath.Join(dir, ".planning")
	if err := os.MkdirAll(planDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(planDir, "PROJECT.md"), "# Vision")

	// README
	writeFile(t, filepath.Join(dir, "README.md"), "# Overview\nHello world.")

	// CLAUDE.md
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Rules\nDo this.")

	// docs/architecture.md
	docsDir := filepath.Join(dir, "docs")
	if err := os.MkdirAll(docsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(docsDir, "architecture.md"), "# Architecture\nHexagonal pattern.")

	goals := detectGoalFiles(dir)

	if len(goals) != 4 {
		t.Fatalf("expected 4 goals, got %d: %v", len(goals), goalSources(goals))
	}

	sources := goalSourceSet(goals)
	for _, expected := range []string{"gsd", "readme", "claude_md", "architecture"} {
		if !sources[expected] {
			t.Errorf("expected source %q in detected goals", expected)
		}
	}
}

func TestLargeFileSkip(t *testing.T) {
	dir := t.TempDir()
	// Create a file > 50KB
	large := strings.Repeat("x", 52*1024)
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), large)

	goals := detectGoalFiles(dir)

	if len(goals) != 0 {
		t.Fatalf("expected 0 goals for large file, got %d", len(goals))
	}
}

func TestAsContextEntries(t *testing.T) {
	goals := []goal.ProjectGoal{
		{Kind: goal.KindVision, Title: "Vision", Content: "Build X", Priority: 95, Enabled: true},
		{Kind: goal.KindConstraint, Title: "Rules", Content: "Use Go", Priority: 85, Enabled: true},
	}

	entries := asContextEntries(goals)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Kind != cfcontext.EntryGoal {
			t.Errorf("expected EntryGoal kind, got %q", e.Kind)
		}
	}
	if entries[0].Priority != 95 {
		t.Errorf("expected priority 95, got %d", entries[0].Priority)
	}
}

func TestAsContextEntriesEmpty(t *testing.T) {
	entries := asContextEntries(nil)

	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

func TestRenderGoalContext(t *testing.T) {
	goals := []goal.ProjectGoal{
		{Kind: goal.KindVision, Title: "Vision", Content: "Build X"},
		{Kind: goal.KindRequirement, Title: "Reqs", Content: "- Feature A"},
		{Kind: goal.KindConstraint, Title: "Rules", Content: "Use Go"},
	}

	out := renderGoalContext(goals)

	if !strings.Contains(out, "## Project Goals") {
		t.Error("expected header")
	}
	if !strings.Contains(out, "### Vision") {
		t.Error("expected Vision section")
	}
	if !strings.Contains(out, "### Requirements") {
		t.Error("expected Requirements section")
	}
	if !strings.Contains(out, "### Constraints & Decisions") {
		t.Error("expected Constraints section")
	}
	if !strings.Contains(out, "Build X") {
		t.Error("expected vision content")
	}
}

func TestRenderGoalContextEmpty(t *testing.T) {
	out := renderGoalContext(nil)
	if out != "" {
		t.Errorf("expected empty string, got %q", out)
	}
}

func TestDetectGSDContextFiles(t *testing.T) {
	dir := t.TempDir()
	planDir := filepath.Join(dir, ".planning")
	if err := os.MkdirAll(planDir, 0o750); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(planDir, "01-CONTEXT.md"), "# Phase 1 Context")
	writeFile(t, filepath.Join(planDir, "02-CONTEXT.md"), "# Phase 2 Context")

	goals := detectGoalFiles(dir)

	if len(goals) != 2 {
		t.Fatalf("expected 2 goals, got %d", len(goals))
	}
	for _, g := range goals {
		if g.Kind != goal.KindContext {
			t.Errorf("expected context kind, got %q", g.Kind)
		}
		if g.Source != "gsd" {
			t.Errorf("expected source gsd, got %q", g.Source)
		}
	}
}

func TestDetectCursorrules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursorrules"), "Always format code.")

	goals := detectGoalFiles(dir)

	if len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goals))
	}
	if goals[0].Kind != goal.KindConstraint {
		t.Errorf("expected constraint kind, got %q", goals[0].Kind)
	}
	if goals[0].Source != "cursorrules" {
		t.Errorf("expected source cursorrules, got %q", goals[0].Source)
	}
}

// --- Bug fix tests (30H) ---

func TestBinaryFileSkip(t *testing.T) {
	dir := t.TempDir()
	// Write a file with null bytes (binary content).
	binaryData := []byte("CLAUDE\x00\x01\x02binary content")
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), binaryData, 0o644); err != nil {
		t.Fatal(err)
	}

	goals := detectGoalFiles(dir)

	if len(goals) != 0 {
		t.Fatalf("expected 0 goals for binary file, got %d", len(goals))
	}
}

func TestExtractFirstSection_NoLevelOneHeading(t *testing.T) {
	// README that starts with ## (no # title) — should return content up to first heading only.
	content := "## Setup\nInstall stuff.\n\n## Usage\nRun it."

	out := extractFirstSection(content)

	if strings.Contains(out, "Run it") {
		t.Error("should not include content after second heading")
	}
	if !strings.Contains(out, "Install stuff") {
		t.Error("should include first section content")
	}
}

func TestExtractFirstSection_UTF8Truncation(t *testing.T) {
	// Create a string with multi-byte UTF-8 characters that would break at 2000 bytes.
	// Each CJK character is 3 bytes. 700 chars = 2100 bytes, exceeds 2000.
	content := "# Title\n" + strings.Repeat("\u4e16", 700) // 世 = 3 bytes each

	out := extractFirstSection(content)

	if len(out) > 2000 {
		t.Fatalf("expected <= 2000 bytes, got %d", len(out))
	}
	// Verify the result is valid UTF-8.
	for i := 0; i < len(out); {
		r, size := rune(out[i]), 1
		if out[i] >= 0x80 {
			var ok bool
			r, size = decodeRune(out[i:])
			ok = r != 0xFFFD || size != 1
			if !ok {
				t.Fatalf("invalid UTF-8 at byte %d", i)
			}
		}
		_ = r
		i += size
	}
}

// decodeRune is a minimal UTF-8 decoder for test verification.
func decodeRune(s string) (ch rune, size int) {
	r := rune(0xFFFD)
	if s == "" {
		return r, 0
	}
	b := s[0]
	switch {
	case b < 0x80:
		return rune(b), 1
	case b < 0xC0:
		return r, 1
	case b < 0xE0:
		if len(s) < 2 {
			return r, 1
		}
		return rune(b&0x1F)<<6 | rune(s[1]&0x3F), 2
	case b < 0xF0:
		if len(s) < 3 {
			return r, 1
		}
		return rune(b&0x0F)<<12 | rune(s[1]&0x3F)<<6 | rune(s[2]&0x3F), 3
	default:
		if len(s) < 4 {
			return r, 1
		}
		return rune(b&0x07)<<18 | rune(s[1]&0x3F)<<12 | rune(s[2]&0x3F)<<6 | rune(s[3]&0x3F), 4
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		wantLen  int // expected byte length
	}{
		{"ascii fits", "hello", 10, 5},
		{"ascii truncated", "hello world", 5, 5},
		{"multibyte no break", "abc\u4e16def", 6, 6}, // 世 starts at byte 3, ends at byte 6
		{"multibyte boundary", "abc\u4e16def", 5, 3}, // can't fit 世 (3 bytes), truncate before it
		{"empty", "", 10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateUTF8(tt.input, tt.maxBytes)
			if len(got) != tt.wantLen {
				t.Errorf("truncateUTF8(%q, %d) = %d bytes, want %d", tt.input, tt.maxBytes, len(got), tt.wantLen)
			}
		})
	}
}

// --- helpers ---

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func groupByKind(goals []goal.ProjectGoal) map[goal.GoalKind]goal.ProjectGoal {
	m := make(map[goal.GoalKind]goal.ProjectGoal, len(goals))
	for i := range goals {
		m[goals[i].Kind] = goals[i]
	}
	return m
}

func goalSources(goals []goal.ProjectGoal) []string {
	var s []string
	for i := range goals {
		s = append(s, goals[i].Source)
	}
	return s
}

func goalSourceSet(goals []goal.ProjectGoal) map[string]bool {
	m := make(map[string]bool, len(goals))
	for i := range goals {
		m[goals[i].Source] = true
	}
	return m
}
