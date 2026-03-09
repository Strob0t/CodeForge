"""Multi-format skill file parser.

Supported formats:
- CodeForge YAML (.yaml/.yml with structured fields)
- Claude Code Skills (YAML frontmatter + --- + Markdown body)
- Cursor Rules (.mdc, .cursorrules)
- Plain Markdown (.md without frontmatter)
"""

from __future__ import annotations

import re

import yaml

from codeforge.skills.models import Skill

_FRONTMATTER_RE = re.compile(r"^---\s*\n(.*?\n)?---\s*\n(.*)$", re.DOTALL)
_HEADING_RE = re.compile(r"^#\s+(.+)$", re.MULTILINE)


def parse_skill_file(filename: str, raw_content: str) -> Skill:
    """Parse a skill file and return a normalized Skill model.

    Dispatches to the appropriate format-specific parser based on file
    extension and filename conventions.

    Raises:
        ValueError: If the file format is not supported.
    """
    ext = _extension(filename)
    base = _basename(filename)

    if ext in (".yaml", ".yml"):
        return _parse_codeforge_yaml(raw_content)
    if ext == ".mdc" or base == ".cursorrules":
        return _parse_cursor(raw_content, base)
    if ext == ".md":
        return _parse_markdown_or_claude(raw_content)

    msg = f"unsupported skill file format: {filename}"
    raise ValueError(msg)


def _parse_codeforge_yaml(raw: str) -> Skill:
    """Parse a native CodeForge YAML skill file."""
    data = yaml.safe_load(raw)
    if not isinstance(data, dict):
        data = {}
    return Skill(
        name=data.get("name", ""),
        type=data.get("type", "pattern"),
        description=data.get("description", ""),
        language=data.get("language", ""),
        content=data.get("content", ""),
        tags=data.get("tags", []),
        format_origin="codeforge",
        source="import",
    )


def _parse_markdown_or_claude(raw: str) -> Skill:
    """Detect frontmatter and dispatch to Claude or plain Markdown parser."""
    match = _FRONTMATTER_RE.match(raw.strip())
    if match:
        return _parse_claude(match.group(1) or "", match.group(2))
    return _parse_plain_markdown(raw)


def _parse_claude(frontmatter: str, body: str) -> Skill:
    """Parse Claude Code skill: YAML frontmatter + Markdown body."""
    meta = yaml.safe_load(frontmatter) or {}
    return Skill(
        name=meta.get("name", ""),
        type=meta.get("type", "workflow"),
        description=meta.get("description", ""),
        content=body.strip(),
        tags=meta.get("tags", []),
        format_origin="claude",
        source="import",
    )


def _parse_plain_markdown(raw: str) -> Skill:
    """Parse a plain Markdown file (no YAML frontmatter)."""
    heading = _HEADING_RE.search(raw)
    name = heading.group(1).strip() if heading else "Untitled Skill"
    return Skill(
        name=name,
        type="workflow",
        content=raw.strip(),
        format_origin="markdown",
        source="import",
    )


def _parse_cursor(raw: str, basename: str) -> Skill:
    """Parse Cursor .cursorrules or .mdc files."""
    heading = _HEADING_RE.search(raw)
    name = heading.group(1).strip() if heading else basename
    return Skill(
        name=name,
        type="workflow",
        content=raw.strip(),
        format_origin="cursor",
        source="import",
    )


def _extension(filename: str) -> str:
    """Extract lowercase file extension including the dot."""
    dot = filename.rfind(".")
    return filename[dot:].lower() if dot >= 0 else ""


def _basename(filename: str) -> str:
    """Extract the filename without directory path."""
    slash = filename.rfind("/")
    return filename[slash + 1 :] if slash >= 0 else filename
