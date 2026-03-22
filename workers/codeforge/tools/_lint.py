"""Post-write syntax checking for agent-generated files.

Runs a lightweight, language-specific syntax check after write_file/edit_file.
Returns None if OK, or an error string if syntax issues are detected.
Does NOT block the write -- appends warnings to tool result for LLM self-correction.

Pattern: SWE-agent edit_linting.sh + Aider tree-sitter lint.
"""

from __future__ import annotations

import ast
import logging
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Callable

logger = logging.getLogger(__name__)

# Map file extensions to checker functions.
# Each checker takes (content, file_path) and returns None (OK) or an error string.
_CHECKERS: dict[str, Callable[[str, str], str | None]] = {}


def post_write_check(file_path: str, content: str) -> str | None:
    """Run syntax check for the given file. Returns None or error string."""
    ext = Path(file_path).suffix.lower()
    checker = _CHECKERS.get(ext)
    if checker is None:
        return None
    try:
        return checker(content, file_path)
    except Exception as exc:
        logger.debug("post_write_check failed for %s: %s", file_path, exc)
        return None


def _check_python(content: str, file_path: str) -> str | None:
    """Python syntax check via ast.parse (stdlib, zero deps)."""
    try:
        ast.parse(content, filename=file_path)
        return None
    except SyntaxError as e:
        return f"SyntaxError at line {e.lineno}: {e.msg}"


# Register checkers.
_CHECKERS[".py"] = _check_python
_CHECKERS[".pyw"] = _check_python
