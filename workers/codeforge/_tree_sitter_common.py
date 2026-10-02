"""Shared constants and file collection for tree-sitter based code analysis modules."""

from __future__ import annotations

import os
import stat
from typing import TYPE_CHECKING

import structlog

from codeforge.constants import CHARS_PER_TOKEN

if TYPE_CHECKING:
    from collections.abc import Container

    from codeforge.workspace_fs import WorkspaceRoot

logger = structlog.get_logger()

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


def collect_source_files(root: WorkspaceRoot, extensions: Container[str]) -> tuple[list[str], int]:
    """Workspace-relative paths of the indexable source files, and how many entries were skipped.

    Walks the workspace without entering _SKIP_DIRS or any symlinked
    directory, up to _MAX_FILES files with one of *extensions* and at most
    _MAX_FILE_SIZE bytes. A file must be a regular file, or a symlink that
    resolves to one inside the workspace (KI-95); symlinks that leave the
    workspace, dangle or loop, and FIFOs, sockets and devices are skipped and
    counted. Read the files through *root* (they may change meanwhile).
    """
    collected: list[str] = []
    skipped = 0
    for dirpath, dirnames, filenames, dir_fd in root.walk():
        dirnames[:] = [d for d in dirnames if d not in _SKIP_DIRS]
        for name in filenames:
            if len(collected) >= _MAX_FILES:
                return collected, skipped
            if os.path.splitext(name)[1] not in extensions:
                continue
            rel = name if dirpath == "." else f"{dirpath}/{name}"
            try:
                info = os.stat(name, dir_fd=dir_fd, follow_symlinks=False)
                if stat.S_ISLNK(info.st_mode):
                    info = root.stat(rel)
            except OSError:
                skipped += 1
                continue
            if not stat.S_ISREG(info.st_mode):
                skipped += 1
                continue
            if info.st_size <= _MAX_FILE_SIZE:
                collected.append(rel)
    return collected, skipped


def log_skipped(indexer: str, skipped: int) -> None:
    """Log once per run how many workspace entries an indexer skipped as unsafe (KI-95)."""
    if skipped:
        logger.info(
            "skipped workspace entries",
            indexer=indexer,
            skipped=skipped,
            reason="symlink leaving the workspace, dangling or looping, or not a regular file",
        )


def read_source(root: WorkspaceRoot, rel_path: str) -> bytes | None:
    """The bytes of a collected source file, None when it cannot be read any more (swapped, grown, removed)."""
    try:
        return root.read_bytes(rel_path, max_bytes=_MAX_FILE_SIZE)
    except OSError as exc:
        logger.warning("cannot read file", path=rel_path, error=str(exc))
        return None
