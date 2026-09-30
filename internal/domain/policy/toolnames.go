package policy

import "strings"

// Canonical tool names (ADR-007). Presets, custom profiles and mode tool
// lists are written with these names; every tool name a worker sends is
// mapped to them by CanonicalTool before evaluation.
const (
	ToolRead    = "Read"
	ToolWrite   = "Write"
	ToolEdit    = "Edit"
	ToolBash    = "Bash"
	ToolGrep    = "Grep"
	ToolGlob    = "Glob"
	ToolListDir = "ListDir"
	// ToolLLM is the permission a worker requests before each LLM completion.
	ToolLLM = "LLM"
)

// toolAliases maps lower-cased tool names to their canonical name: the
// canonical names themselves, the Python agent loop tools
// (workers/codeforge/tools), Claude Code tools and the legacy categories of
// the Claude Code executor.
var toolAliases = map[string]string{
	"read": ToolRead, "write": ToolWrite, "edit": ToolEdit, "bash": ToolBash,
	"grep": ToolGrep, "glob": ToolGlob, "listdir": ToolListDir, "llm": ToolLLM,

	"read_file":      ToolRead,
	"write_file":     ToolWrite,
	"edit_file":      ToolEdit,
	"search_files":   ToolGrep,
	"glob_files":     ToolGlob,
	"list_directory": ToolListDir,

	"multiedit":    ToolEdit,
	"notebookedit": ToolEdit,
	"search":       ToolGrep,
	"ls":           ToolListDir,

	"command:execute": ToolBash,
	"file:read":       ToolRead,
	"file:write":      ToolWrite,
	"file:edit":       ToolEdit,
}

// builtinTools are the canonical names of the file and shell tools that a
// mode's Tools list selects from.
var builtinTools = []string{ToolRead, ToolWrite, ToolEdit, ToolBash, ToolGrep, ToolGlob, ToolListDir}

// CanonicalTool maps a worker or Claude Code tool name to its ADR-007 name,
// ignoring case. Names without a mapping (MCP tools, propose_goal, ...) are
// returned unchanged.
func CanonicalTool(name string) string {
	if canonical, ok := toolAliases[strings.ToLower(name)]; ok {
		return canonical
	}
	return name
}

// BuiltinTools returns the canonical names of the built-in file and shell tools.
func BuiltinTools() []string {
	return append([]string(nil), builtinTools...)
}

// IsBuiltinTool reports whether name is the canonical name of a built-in tool.
func IsBuiltinTool(name string) bool {
	for _, t := range builtinTools {
		if t == name {
			return true
		}
	}
	return false
}
