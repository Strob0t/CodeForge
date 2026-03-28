"""Adaptive tool-usage guide builder for different model capability levels.

Generates system prompt supplements that help weaker models use tools correctly:
- full: no extra guide (model is capable enough)
- api_with_tools: concise hints (when_to_use + common_mistakes)
- pure_completion: full guide with examples and output format
- compact mode: one-line descriptions, no examples/mistakes (for small-context models)
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from codeforge.tools.capability import CapabilityLevel

if TYPE_CHECKING:
    from codeforge.tools import ToolRegistry
    from codeforge.tools._base import ToolDefinition


_MAX_GUIDE_CHARS = 6000  # ~1500 tokens — prevent guide from eating the context budget
_MAX_COMPACT_CHARS = 2000  # ~500 tokens — tight budget for small-context models


def build_tool_usage_guide(
    registry: ToolRegistry,
    capability_level: CapabilityLevel,
    *,
    compact: bool = False,
) -> str:
    """Build a tool-usage guide string based on model capability level.

    Returns an empty string for full-capability models.

    When ``compact=True``, returns a minimal guide that skips examples and
    common-mistakes sections, suitable for models with context windows
    below 32K tokens.
    """
    if capability_level == CapabilityLevel.FULL:
        return ""

    if compact:
        guide = _build_compact_guide(registry)
        limit = _MAX_COMPACT_CHARS
    elif capability_level == CapabilityLevel.API_WITH_TOOLS:
        guide = _build_concise_guide(registry)
        limit = _MAX_GUIDE_CHARS
    else:
        guide = _build_full_guide(registry)
        limit = _MAX_GUIDE_CHARS

    if len(guide) > limit:
        guide = guide[:limit] + "\n\n[Tool guide truncated]"
    return guide


def _build_compact_guide(registry: ToolRegistry) -> str:
    """Build a minimal guide: one line per tool, no examples or mistakes."""
    lines: list[str] = ["## Tools"]

    for defn in _iter_tool_definitions(registry):
        # First sentence of description, or the full description if short.
        short_desc = defn.description.split(". ")[0].rstrip(".")
        lines.append(f"- **{defn.name}**: {short_desc}")

    if len(lines) == 1:
        return ""

    return "\n".join(lines)


def _build_concise_guide(registry: ToolRegistry) -> str:
    """Build concise hints for models with API tool support but weaker tool use."""
    sections: list[str] = ["## Tool Usage Tips"]

    for defn in _iter_tool_definitions(registry):
        if not defn.when_to_use and not defn.common_mistakes:
            continue

        parts: list[str] = [f"### {defn.name}"]
        if defn.when_to_use:
            parts.append(f"When to use: {defn.when_to_use}")
        if defn.common_mistakes:
            parts.append("Avoid: " + "; ".join(defn.common_mistakes))
        sections.append("\n".join(parts))

    if len(sections) == 1:
        return ""

    return "\n\n".join(sections)


def _build_full_guide(registry: ToolRegistry) -> str:
    """Build comprehensive guide with examples for pure-completion models."""
    sections: list[str] = [
        "## Tool Usage Guide",
        (
            "You have access to the following tools. To use a tool, respond with a "
            "function call in the format specified by the API. Each tool has specific "
            "parameters - follow the examples carefully."
        ),
    ]

    for defn in _iter_tool_definitions(registry):
        parts: list[str] = [f"### {defn.name}", defn.description]

        if defn.when_to_use:
            parts.append(f"**When to use:** {defn.when_to_use}")
        if defn.output_format:
            parts.append(f"**Output format:** {defn.output_format}")
        if defn.common_mistakes:
            parts.append("**Common mistakes:**")
            parts.extend(f"- {mistake}" for mistake in defn.common_mistakes)
        if defn.examples:
            parts.append("**Examples:**")
            for ex in defn.examples:
                parts.append(f"*{ex.description}:*")
                parts.append(f"```json\n{ex.tool_call_json}\n```")
                parts.append(f"Expected result: {ex.expected_result}")

        sections.append("\n".join(parts))

    if len(sections) == 2:
        return ""

    return "\n\n".join(sections)


def _iter_tool_definitions(registry: ToolRegistry) -> list[ToolDefinition]:
    """Return built-in tool definitions from the registry (skip MCP proxy tools)."""
    return [defn for defn in registry.get_definitions() if not defn.name.startswith("mcp__")]
