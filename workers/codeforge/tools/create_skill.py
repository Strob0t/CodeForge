"""Built-in tool: propose a new skill as a draft for user approval."""

from __future__ import annotations

import logging
import re
from typing import Any, Callable, Coroutine

from codeforge.tools._base import ToolDefinition, ToolResult

logger = logging.getLogger(__name__)

_MAX_CONTENT_LENGTH = 10_000

# Prompt injection patterns (Python-side check, mirrors Go quarantine scorer)
_PROMPT_OVERRIDE_RE = re.compile(
    r"(?i)(ignore\s+(all\s+)?previous|disregard\s+(all\s+)?instructions|"
    r"you\s+are\s+now|forget\s+(everything|all)|new\s+instructions|"
    r"override\s+system|act\s+as\s+if|pretend\s+(you|that)|"
    r"do\s+not\s+follow|system\s+prompt\s+is)"
)
_ROLE_HIJACK_RE = re.compile(
    r"(?i)(from\s+now\s+on\s+you|switch\s+to\s+|change\s+your\s+behavior|"
    r"your\s+role\s+is\s+now)"
)
_EXFIL_RE = re.compile(
    r"(?i)(send\s+to\s+https?://|exfiltrate|leak\s+(the|all)\s+)"
)

_VALID_TYPES = {"workflow", "pattern"}

DEFINITION = ToolDefinition(
    name="create_skill",
    description=(
        "Propose a new reusable skill based on a pattern or workflow "
        "you discovered during this task. The skill is saved as a draft "
        "and requires user approval before activation."
    ),
    parameters={
        "type": "object",
        "properties": {
            "name": {
                "type": "string",
                "description": "Short, descriptive, kebab-case name (e.g. 'nats-error-handling').",
            },
            "type": {
                "type": "string",
                "enum": ["workflow", "pattern"],
                "description": "'workflow' for step-by-step instructions, 'pattern' for code templates.",
            },
            "description": {
                "type": "string",
                "description": "One sentence explaining what this skill does.",
            },
            "content": {
                "type": "string",
                "description": "The full skill body in Markdown.",
            },
            "language": {
                "type": "string",
                "description": "Programming language (only for type=pattern).",
            },
            "tags": {
                "type": "array",
                "items": {"type": "string"},
                "description": "2-5 relevant keywords.",
            },
        },
        "required": ["name", "type", "description", "content"],
    },
)

# Type alias for the async save callback
SaveFn = Callable[[dict[str, Any]], Coroutine[Any, Any, str]]


class CreateSkillTool:
    """Executor for the create_skill tool."""

    def __init__(self, save_fn: SaveFn | None = None) -> None:
        self._save_fn = save_fn

    async def execute(self, arguments: dict[str, object], workspace_path: str) -> ToolResult:
        name = str(arguments.get("name", "")).strip()
        skill_type = str(arguments.get("type", "")).strip()
        description = str(arguments.get("description", "")).strip()
        content = str(arguments.get("content", "")).strip()
        language = str(arguments.get("language", "")).strip()
        tags = arguments.get("tags", [])
        if not isinstance(tags, list):
            tags = []

        # Validation
        if not name:
            return ToolResult(output="", error="Validation error: name is required.", success=False)
        if not content:
            return ToolResult(output="", error="Validation error: content is required.", success=False)
        if not description:
            return ToolResult(output="", error="Validation error: description is required.", success=False)
        if skill_type not in _VALID_TYPES:
            return ToolResult(
                output="",
                error=f"Validation error: type must be 'workflow' or 'pattern', got '{skill_type}'.",
                success=False,
            )
        if len(content) > _MAX_CONTENT_LENGTH:
            return ToolResult(
                output="",
                error=f"Validation error: content too long ({len(content)} chars, max 10000).",
                success=False,
            )

        # Prompt injection check
        injection_issue = _check_injection(content)
        if injection_issue:
            return ToolResult(
                output="",
                error=f"Skill rejected: prompt injection detected — {injection_issue}.",
                success=False,
            )

        skill_data: dict[str, Any] = {
            "name": name,
            "type": skill_type,
            "description": description,
            "content": content,
            "language": language,
            "tags": [str(t) for t in tags],
            "source": "agent",
            "status": "draft",
            "format_origin": "codeforge",
        }

        if self._save_fn is None:
            return ToolResult(
                output=f"Skill '{name}' validated but no save function configured.",
                success=True,
            )

        try:
            skill_id = await self._save_fn(skill_data)
        except Exception as exc:
            return ToolResult(output="", error=f"Failed to save skill: {exc}", success=False)

        return ToolResult(
            output=(
                f"Skill draft created successfully!\n"
                f"- ID: {skill_id}\n"
                f"- Name: {name}\n"
                f"- Type: {skill_type}\n"
                f"- Status: draft (awaiting user approval)\n\n"
                f"The user will be notified to review and activate this skill."
            ),
            success=True,
        )


def _check_injection(content: str) -> str:
    """Check content for prompt injection patterns. Returns issue description or empty string."""
    if _PROMPT_OVERRIDE_RE.search(content):
        return "prompt override attempt"
    if _ROLE_HIJACK_RE.search(content):
        return "role hijack attempt"
    if _EXFIL_RE.search(content):
        return "data exfiltration attempt"
    return ""
