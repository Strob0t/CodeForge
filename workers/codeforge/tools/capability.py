"""Model capability classification for adaptive tool guidance.

Three capability levels determine how much tool-use guidance a model needs:
- full: Advanced models with native tool calling (Claude, GPT-4, Gemini Pro)
- api_with_tools: Models with function-calling API but weaker tool use
- pure_completion: Models without function-calling support (most local models)
"""

from __future__ import annotations

import logging
import re
from enum import StrEnum

logger = logging.getLogger(__name__)

# Models known to have strong, reliable tool-calling behaviour.
_FULL_CAPABILITY_PATTERNS: list[re.Pattern[str]] = [
    re.compile(r"claude-(?:3|4|opus|sonnet)", re.IGNORECASE),
    re.compile(r"gpt-4(?:o|\.5|-turbo)", re.IGNORECASE),
    re.compile(r"gemini-(?:1\.5|2|pro|ultra)", re.IGNORECASE),
    re.compile(r"o[134]-", re.IGNORECASE),
]

# Models that support function-calling API but benefit from extra guidance.
_API_WITH_TOOLS_PATTERNS: list[re.Pattern[str]] = [
    re.compile(r"gpt-3\.5", re.IGNORECASE),
    re.compile(r"mistral", re.IGNORECASE),
    re.compile(r"mixtral", re.IGNORECASE),
    re.compile(r"command-r", re.IGNORECASE),
    re.compile(r"groq/", re.IGNORECASE),
    re.compile(r"deepseek", re.IGNORECASE),
    re.compile(r"qwen", re.IGNORECASE),
]

# Models that typically lack function-calling support entirely.
# NOTE: ollama/ and lm_studio/ are local provider prefixes.
# Some local models (e.g. Qwen2.5-Coder) DO support function calling.
# The classify_model() logic only applies these patterns when litellm
# has NOT confirmed FC support (supports_fc is not True).
_PURE_COMPLETION_PATTERNS: list[re.Pattern[str]] = [
    re.compile(r"ollama/", re.IGNORECASE),
    re.compile(r"lm[-_]studio/", re.IGNORECASE),
    re.compile(r"llama[-.]?[23]", re.IGNORECASE),
    re.compile(r"codellama", re.IGNORECASE),
    re.compile(r"phi[-.]?[23]", re.IGNORECASE),
    re.compile(r"starcoder", re.IGNORECASE),
]

# Local models known to support function calling reliably.
# These override the pure-completion classification for local prefixes.
_LOCAL_FC_CAPABLE_PATTERNS: list[re.Pattern[str]] = [
    re.compile(r"qwen2\.5.*(?:coder|instruct)", re.IGNORECASE),
    re.compile(r"mistral.*instruct", re.IGNORECASE),
    re.compile(r"functionary", re.IGNORECASE),
    re.compile(r"hermes.*pro", re.IGNORECASE),
]


class CapabilityLevel(StrEnum):
    """Model capability level for tool use."""

    FULL = "full"
    API_WITH_TOOLS = "api_with_tools"
    PURE_COMPLETION = "pure_completion"


# Tools allowed per capability level.
# Mode-declared tools (mode.tools) are always added on top.
# An empty frozenset means ALL tools are allowed (no filtering).
TOOLS_BY_CAPABILITY: dict[CapabilityLevel, frozenset[str]] = {
    CapabilityLevel.FULL: frozenset(),  # empty = all tools allowed
    CapabilityLevel.API_WITH_TOOLS: frozenset(
        {
            "read_file",
            "write_file",
            "edit_file",
            "bash",
            "search_files",
            "glob_files",
            "list_directory",
            "propose_goal",
            "handoff",
            "transition_to_act",
        }
    ),
    CapabilityLevel.PURE_COMPLETION: frozenset(
        {
            "read_file",
            "write_file",
            "bash",
            "search_files",
            "propose_goal",
            "transition_to_act",
        }
    ),
}


def classify_model(model: str) -> CapabilityLevel:
    """Classify a model's tool-use capability level.

    Uses litellm.supports_function_calling() as the primary check when
    available, then falls back to model name pattern matching.
    """
    if not model:
        return CapabilityLevel.PURE_COMPLETION

    # Try litellm check if available (it's a sidecar, not always importable).
    supports_fc = _check_litellm_function_calling(model)

    if supports_fc is False:
        return CapabilityLevel.PURE_COMPLETION

    # Check explicit pure-completion patterns first (most restrictive).
    # Even if litellm says it supports FC, local model prefixes (ollama/)
    # override because they rarely support tools reliably — UNLESS the
    # model name matches a known FC-capable local model.
    for pat in _PURE_COMPLETION_PATTERNS:
        if pat.search(model) and supports_fc is not True:
            if any(fc_pat.search(model) for fc_pat in _LOCAL_FC_CAPABLE_PATTERNS):
                return CapabilityLevel.API_WITH_TOOLS
            return CapabilityLevel.PURE_COMPLETION

    # Check full capability patterns.
    for pat in _FULL_CAPABILITY_PATTERNS:
        if pat.search(model):
            return CapabilityLevel.FULL

    # Check API-with-tools patterns.
    for pat in _API_WITH_TOOLS_PATTERNS:
        if pat.search(model):
            return CapabilityLevel.API_WITH_TOOLS

    # If litellm confirmed function calling, assume api_with_tools.
    if supports_fc is True:
        return CapabilityLevel.API_WITH_TOOLS

    # Unknown model without litellm confirmation — be conservative.
    return CapabilityLevel.PURE_COMPLETION


def _check_litellm_function_calling(model: str) -> bool | None:
    """Check litellm.supports_function_calling if available. Returns None if not importable."""
    try:
        import litellm

        return bool(litellm.supports_function_calling(model=model))
    except (ImportError, Exception):
        logger.debug("litellm not available for function-calling check, using pattern matching")
        return None
