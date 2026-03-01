"""Shared constants for tree-sitter based code analysis modules."""

from __future__ import annotations

from codeforge.constants import CHARS_PER_TOKEN

# Directories to skip during file collection
_SKIP_DIRS: frozenset[str] = frozenset(
    {
        ".git",
        "node_modules",
        "vendor",
        "__pycache__",
        "dist",
        "build",
        ".venv",
        ".tox",
        ".mypy_cache",
        ".ruff_cache",
        ".pytest_cache",
    }
)

# Maximum file size in bytes (100KB)
_MAX_FILE_SIZE = 100 * 1024

# Maximum number of files to collect
_MAX_FILES = 2000

# Re-export for backwards compatibility within tree-sitter modules.
_CHARS_PER_TOKEN = CHARS_PER_TOKEN

# File extension to tree-sitter language name mapping
_EXTENSION_MAP: dict[str, str] = {
    ".py": "python",
    ".go": "go",
    ".ts": "typescript",
    ".tsx": "tsx",
    ".js": "javascript",
    ".jsx": "javascript",
    ".java": "java",
    ".rs": "rust",
    ".rb": "ruby",
    ".c": "c",
    ".cpp": "cpp",
    ".cc": "cpp",
    ".cxx": "cpp",
    ".cs": "csharp",
    ".kt": "kotlin",
    ".swift": "swift",
    ".php": "php",
    ".h": "c",
    ".hpp": "cpp",
}

# Definition node types per language -- maps language name to a set of
# AST node types that represent symbol definitions.
_DEF_NODE_TYPES: dict[str, frozenset[str]] = {
    "go": frozenset(
        {
            "function_declaration",
            "method_declaration",
            "type_declaration",
            "const_declaration",
            "var_declaration",
        }
    ),
    "python": frozenset(
        {
            "function_definition",
            "class_definition",
            "assignment",
        }
    ),
    "typescript": frozenset(
        {
            "function_declaration",
            "class_declaration",
            "lexical_declaration",
            "method_definition",
            "interface_declaration",
            "type_alias_declaration",
        }
    ),
    "tsx": frozenset(
        {
            "function_declaration",
            "class_declaration",
            "lexical_declaration",
            "method_definition",
            "interface_declaration",
            "type_alias_declaration",
        }
    ),
    "javascript": frozenset(
        {
            "function_declaration",
            "class_declaration",
            "lexical_declaration",
            "method_definition",
        }
    ),
    "java": frozenset(
        {
            "class_declaration",
            "method_declaration",
            "interface_declaration",
        }
    ),
    "rust": frozenset(
        {
            "function_item",
            "struct_item",
            "enum_item",
            "impl_item",
            "trait_item",
        }
    ),
    "ruby": frozenset(
        {
            "method",
            "class",
            "module",
            "singleton_method",
        }
    ),
    "c": frozenset(
        {
            "function_definition",
            "struct_specifier",
            "enum_specifier",
            "type_definition",
            "declaration",
        }
    ),
    "cpp": frozenset(
        {
            "function_definition",
            "class_specifier",
            "struct_specifier",
            "enum_specifier",
            "type_definition",
            "declaration",
            "namespace_definition",
        }
    ),
    "csharp": frozenset(
        {
            "class_declaration",
            "method_declaration",
            "interface_declaration",
            "struct_declaration",
            "enum_declaration",
        }
    ),
    "kotlin": frozenset(
        {
            "function_declaration",
            "class_declaration",
            "object_declaration",
        }
    ),
    "swift": frozenset(
        {
            "function_declaration",
            "class_declaration",
            "struct_declaration",
            "enum_declaration",
            "protocol_declaration",
        }
    ),
    "php": frozenset(
        {
            "function_definition",
            "class_declaration",
            "method_declaration",
            "interface_declaration",
        }
    ),
}
