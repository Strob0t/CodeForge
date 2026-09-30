"""The values the Go policy layer evaluates for a tool call: (command, path).

One mapping for the agent loop (``tool_executor``) and Claude Code runs
(``claude_code_executor``). The tool name itself is sent unchanged; Go maps it
to a canonical name (internal/domain/policy/toolnames.go).

- Only command tools send a command, in full (the policy splits it).
- File tools send their file; directory tools send the directory they work
  in, the workspace root (".") by default. A glob pattern that is absolute
  or climbs with ".." is checked where it can reach.
- Paths are sent relative to the real workspace: symlinks are resolved on
  both sides (Go compares with the project's workspace path, which may be a
  symlink) and "~" is expanded as the tools do. A path outside the workspace
  is sent as its real absolute path, which Go denies.
"""

from __future__ import annotations

import itertools
import os
import re
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Iterable

# Canonical tool names (ADR-007) and the lower-cased names that map to them:
# a copy of toolAliases in internal/domain/policy/toolnames.go, kept equal by
# tests/test_mode_tool_lists.py.
_TOOL_ALIASES: dict[str, str] = {
    "read": "Read",
    "write": "Write",
    "edit": "Edit",
    "bash": "Bash",
    "grep": "Grep",
    "glob": "Glob",
    "listdir": "ListDir",
    "llm": "LLM",
    "read_file": "Read",
    "write_file": "Write",
    "edit_file": "Edit",
    "search_files": "Grep",
    "glob_files": "Glob",
    "list_directory": "ListDir",
    "multiedit": "Edit",
    "notebookedit": "Edit",
    "search": "Grep",
    "ls": "ListDir",
    "monitor": "Bash",
    "command:execute": "Bash",
    "file:read": "Read",
    "file:write": "Write",
    "file:edit": "Edit",
}

# The file and shell tools that a mode's tools list selects from.
_BUILTIN_TOOLS = frozenset({"Read", "Write", "Edit", "Bash", "Grep", "Glob", "ListDir"})


def canonical_tool(name: str) -> str:
    """Return the canonical policy name of a tool, ignoring case; other names unchanged."""
    return _TOOL_ALIASES.get(name.lower(), name)


def is_builtin_tool(name: str) -> bool:
    """Whether a tool (any of its names) is one of the file and shell tools a mode's tools list selects from."""
    return canonical_tool(name) in _BUILTIN_TOOLS


def mode_allows_tool(name: str, tools: Iterable[str], denied: Iterable[str]) -> bool:
    """Whether an agent mode's tool lists let it use a tool, as the Go policy decides.

    A tool in *denied* is not allowed; a built-in tool missing from a non-empty
    *tools* list is not allowed; other tools (MCP, propose_goal, ...) are only
    restricted by *denied*. Names are compared canonically
    (internal/domain/policy: WithModeTools).
    """
    tool = canonical_tool(name)
    if any(canonical_tool(d) == tool for d in denied):
        return False
    listed = {canonical_tool(t) for t in tools}
    return not listed or tool not in _BUILTIN_TOOLS or tool in listed


_COMMAND_TOOLS = frozenset({"bash", "Bash", "Monitor"})

# The argument naming the one file a file tool works on.
_FILE_TOOLS: dict[str, str] = {
    "read_file": "file_path",
    "write_file": "file_path",
    "edit_file": "file_path",
    "Read": "file_path",
    "Write": "file_path",
    "Edit": "file_path",
    "MultiEdit": "file_path",
    "NotebookEdit": "notebook_path",
}

# Tools that work in a directory (their "path" argument), with the argument
# that holds a glob pattern matched below it ("" for none).
_DIRECTORY_TOOLS: dict[str, str] = {
    "search_files": "include",
    "list_directory": "",
    "glob_files": "pattern",
    "Grep": "glob",
    "Glob": "pattern",
    "LS": "",
}

_GLOB_MAGIC = frozenset("*?[{")
# A brace group holding a "/": an alternative may be absolute or climb.
_BRACE_WITH_SLASH = re.compile(r"\{[^}]*/")


def policy_request_args(tool_name: str, arguments: dict[str, object], workspace: str) -> tuple[str, str]:
    """Return the (command, path) the Go policy layer evaluates for a tool call in ``workspace``."""
    command = _str_arg(arguments, "command") if tool_name in _COMMAND_TOOLS else ""
    return command, _workspace_relative(workspace, _target(tool_name, arguments))


def _str_arg(arguments: dict[str, object], key: str) -> str:
    value = arguments.get(key)
    return value if isinstance(value, str) else ""


def _target(tool_name: str, arguments: dict[str, object]) -> str:
    if tool_name in _FILE_TOOLS:
        return _str_arg(arguments, _FILE_TOOLS[tool_name])
    if tool_name not in _DIRECTORY_TOOLS:
        return ""
    directory = _str_arg(arguments, "path") or "."
    glob_key = _DIRECTORY_TOOLS[tool_name]
    reach = _glob_reach(_str_arg(arguments, glob_key)) if glob_key else ""
    return os.path.join(directory, reach) if reach else directory


def _glob_reach(pattern: str) -> str:
    """Return where a glob pattern can match, relative to its directory (or absolute).

    The literal segments before the first wildcard, then one ".." for every
    ".." after it: a wildcard may stand for any number of directories, so each
    ".." may climb one level. A brace group with a "/" may reach anywhere.
    """
    try:
        pattern = os.path.expanduser(pattern)
    except ValueError:  # "~<NUL>...": no user of that name can exist
        return os.sep
    if _BRACE_WITH_SLASH.search(pattern):
        return "/"
    parts = pattern.split("/")
    literal = list(itertools.takewhile(lambda part: not _GLOB_MAGIC.intersection(part), parts))
    climbs = "/".join(parts[len(literal) :]).count("..")
    reach = "/".join(literal + [".."] * climbs)
    if pattern.startswith("/") and not reach:
        return "/"
    return reach


def _workspace_relative(workspace: str, path: str) -> str:
    if not path or not workspace:
        return path
    root = os.path.realpath(workspace)
    try:
        real = os.path.realpath(os.path.join(root, os.path.expanduser(path)))
    except (ValueError, OSError):
        # Unresolvable (e.g. a NUL byte from the model): report it as outside
        # the workspace, which Go denies, instead of failing the whole loop.
        return os.sep
    if real == root:
        return "."
    if real.startswith(root.rstrip(os.sep) + os.sep):
        return os.path.relpath(real, root)
    return real
