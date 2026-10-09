"""Shared constants and file collection for tree-sitter based code analysis modules."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import structlog

from codeforge.constants import CHARS_PER_TOKEN
from codeforge.workspace_fs import FileTooLargeError, WalkStats

if TYPE_CHECKING:
    from collections.abc import Container, Iterator

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


@dataclass
class SourceScan:
    """What one indexer run read and left out; logged once per run (KI-95)."""

    indexer: str
    files: int = 0
    skipped: int = 0  # symlinks leaving the workspace, dangling or looping; FIFOs, sockets, devices
    walk: WalkStats = field(default_factory=WalkStats)
    walk_error: str = ""

    def log(self) -> None:
        """Log the left-out entries once, nothing when nothing was left out."""
        if not (self.skipped or self.walk.too_deep or self.walk.errors or self.walk_error):
            return
        logger.info(
            "skipped workspace entries",
            indexer=self.indexer,
            skipped=self.skipped,
            too_deep=self.walk.too_deep,
            unreadable_dirs=self.walk.errors,
            walk_error=self.walk_error,
            reason="symlink leaving the workspace, dangling or looping, not a regular file, "
            "or a directory too deep or unreadable",
        )


def iter_source_files(root: WorkspaceRoot, extensions: Container[str], scan: SourceScan) -> Iterator[tuple[str, bytes]]:
    """(workspace-relative path, bytes) of the indexable source files.

    Walks the workspace without entering _SKIP_DIRS or any symlinked
    directory, up to _MAX_FILES files with one of *extensions* and at most
    _MAX_FILE_SIZE bytes (larger files are left out silently). Each file is
    opened relative to the walk's directory descriptor without following a
    symlink; only a symlink is resolved through the root and read when it
    leads to a regular file inside the workspace (KI-95). Everything else is
    counted in *scan*; a walk that fails ends the iteration (an indexer never
    fails on the workspace's shape).
    """
    try:
        for dirpath, dirnames, filenames, dir_fd in root.walk(stats=scan.walk):
            dirnames[:] = [d for d in dirnames if d not in _SKIP_DIRS]
            for name in filenames:
                if scan.files >= _MAX_FILES:
                    return
                if os.path.splitext(name)[1] not in extensions:
                    continue
                rel = name if dirpath == "." else f"{dirpath}/{name}"
                try:
                    source = root.read_entry(dir_fd, name, rel, max_bytes=_MAX_FILE_SIZE)
                except FileTooLargeError:
                    continue
                except OSError:
                    scan.skipped += 1
                    continue
                scan.files += 1
                yield rel, source
    except OSError as exc:
        scan.walk_error = str(exc)
