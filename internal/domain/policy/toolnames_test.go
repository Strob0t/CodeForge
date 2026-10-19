package policy

import "testing"

func TestCanonicalTool(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		// Python agent loop tools (workers/codeforge/tools).
		{"read_file", ToolRead},
		{"write_file", ToolWrite},
		{"edit_file", ToolEdit},
		{"bash", ToolBash},
		{"search_files", ToolGrep},
		{"glob_files", ToolGlob},
		{"list_directory", ToolListDir},
		// Claude Code tools.
		{"Read", ToolRead},
		{"Write", ToolWrite},
		{"Edit", ToolEdit},
		{"MultiEdit", ToolEdit},
		{"NotebookEdit", ToolEdit},
		{"Bash", ToolBash},
		{"Glob", ToolGlob},
		{"Grep", ToolGrep},
		{"Search", ToolGrep},
		{"LS", ToolListDir},
		{"ListDir", ToolListDir},
		// Monitor runs a shell command in the background.
		{"Monitor", ToolBash},
		// Legacy Claude Code executor categories.
		{"command:execute", ToolBash},
		{"file:read", ToolRead},
		{"file:write", ToolWrite},
		{"file:edit", ToolEdit},
		// Case does not matter for known names.
		{"BASH", ToolBash},
		{"Read_File", ToolRead},
		{"llm", ToolLLM},
		{"LLM", ToolLLM},
		// Unknown names stay unchanged.
		{"propose_goal", "propose_goal"},
		{"mcp__github__create_issue", "mcp__github__create_issue"},
		{"handoff_to", "handoff_to"},
		{"WebFetch", "WebFetch"},
		{"", ""},
		{" bash", " bash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalTool(tt.name); got != tt.want {
				t.Errorf("CanonicalTool(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// Every tool the Python agent loop registers must be known to the policy
// layer: built-in tools map to a canonical name, the rest keep their name.
func TestCanonicalTool_WorkerToolsCovered(t *testing.T) {
	builtin := map[string]string{
		"read_file":      ToolRead,
		"write_file":     ToolWrite,
		"edit_file":      ToolEdit,
		"bash":           ToolBash,
		"search_files":   ToolGrep,
		"glob_files":     ToolGlob,
		"list_directory": ToolListDir,
	}
	for worker, want := range builtin {
		if got := CanonicalTool(worker); got != want {
			t.Errorf("CanonicalTool(%q) = %q, want %q", worker, got, want)
		}
		if !IsBuiltinTool(CanonicalTool(worker)) {
			t.Errorf("IsBuiltinTool(%q) = false, want true", CanonicalTool(worker))
		}
	}
	for _, other := range []string{"search_conversations", "search_skills", "create_skill", "handoff_to", "propose_goal", "propose_roadmap", "spawn_subagent", "mcp__fs__read"} {
		if got := CanonicalTool(other); got != other {
			t.Errorf("CanonicalTool(%q) = %q, want unchanged", other, got)
		}
		if IsBuiltinTool(CanonicalTool(other)) {
			t.Errorf("IsBuiltinTool(%q) = true, want false", other)
		}
	}
}

func TestIsBuiltinTool(t *testing.T) {
	for _, name := range BuiltinTools() {
		if !IsBuiltinTool(name) {
			t.Errorf("IsBuiltinTool(%q) = false", name)
		}
	}
	for _, name := range []string{"LLM", "propose_goal", "read_file", "Search", ""} {
		if IsBuiltinTool(name) {
			t.Errorf("IsBuiltinTool(%q) = true, want false", name)
		}
	}
}
