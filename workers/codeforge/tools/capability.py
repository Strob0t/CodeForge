"""Model capability classification for adaptive tool guidance.

Three capability levels determine how much tool-use guidance a model needs:
- full: Advanced models with native tool calling (Claude, GPT-4, Gemini Pro)
- api_with_tools: Models with function-calling API but weaker tool use
- pure_completion: Models without function-calling support (most local models)
"""

from __future__ import annotations

import re
from enum import StrEnum
from fnmatch import fnmatchcase

from codeforge.config import get_settings

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
# classify_model() applies these patterns only without an override or
# metadata for the model.
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
    re.compile(r"qwen3", re.IGNORECASE),
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
            "propose_roadmap",
            "handoff_to",
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
            "propose_roadmap",
            "transition_to_act",
        }
    ),
}


# Tools offered whenever they are registered, whatever the capability level or
# the ToolRouter's selection: a run can only hand off (handoff_to is
# registered only where a handoff is possible) if the model sees the tool, and
# the router's keywords cannot predict that from the prompt.
ALWAYS_OFFERED_TOOLS: frozenset[str] = frozenset({"handoff_to"})


def configured_capability(model: str) -> CapabilityLevel | None:
    """Return the operator's capability for *model* (first matching pattern), or None.

    Set by ``litellm.model_capabilities`` / ``CODEFORGE_MODEL_CAPABILITIES``;
    the patterns are case-sensitive shell-style globs on the model name.
    """
    for pattern, level in get_settings().model_capabilities:
        if fnmatchcase(model, pattern):
            return CapabilityLevel(level)
    return None


def classify_model(model: str, *, supports_function_calling: bool | None = None) -> CapabilityLevel:
    """Classify a model's tool-use capability level (KI-125).

    In order: the operator's override (``configured_capability``), the
    model's metadata (*supports_function_calling*, from LiteLLM's
    ``/model/info``; None when it reports nothing), then the name patterns.
    The override comes first so that a model the operator marks as
    tool-capable gets tools even where the metadata says otherwise.
    """
    if not model:
        return CapabilityLevel.PURE_COMPLETION

    configured = configured_capability(model)
    if configured is not None:
        return configured

    if supports_function_calling is False:
        return CapabilityLevel.PURE_COMPLETION
    if supports_function_calling is True:
        if any(pat.search(model) for pat in _FULL_CAPABILITY_PATTERNS):
            return CapabilityLevel.FULL
        return CapabilityLevel.API_WITH_TOOLS

    # No metadata: local model prefixes (ollama/, lm_studio/) and families
    # that rarely support tools count as pure completion, unless the name
    # matches a known FC-capable local model.
    for pat in _PURE_COMPLETION_PATTERNS:
        if pat.search(model):
            if any(fc_pat.search(model) for fc_pat in _LOCAL_FC_CAPABLE_PATTERNS):
                return CapabilityLevel.API_WITH_TOOLS
            return CapabilityLevel.PURE_COMPLETION

    for pat in _FULL_CAPABILITY_PATTERNS:
        if pat.search(model):
            return CapabilityLevel.FULL

    for pat in _API_WITH_TOOLS_PATTERNS:
        if pat.search(model):
            return CapabilityLevel.API_WITH_TOOLS

    # OpenAI-compatible proxies (e.g. openai/container for LM Studio)
    # typically support function calling via the /v1/chat/completions
    # tool_choice parameter. Default to api_with_tools rather than
    # pure_completion so the agent loop sends structured tool calls.
    if model.startswith("openai/"):
        return CapabilityLevel.API_WITH_TOOLS

    # Unknown model without metadata: be conservative.
    return CapabilityLevel.PURE_COMPLETION
