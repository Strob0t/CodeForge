"""Tests for model capability classification and tool filtering."""

from __future__ import annotations

import pytest

from codeforge.tools.capability import (
    TOOLS_BY_CAPABILITY,
    CapabilityLevel,
    classify_model,
)

# --- TOOLS_BY_CAPABILITY tests ---


class TestToolsByCapability:
    def test_full_allows_all(self) -> None:
        """FULL capability has an empty frozenset = no filtering."""
        allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.FULL]
        assert len(allowed) == 0

    def test_api_with_tools_allows_core_tools(self) -> None:
        allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.API_WITH_TOOLS]
        for tool in ("read_file", "write_file", "edit_file", "bash", "search_files"):
            assert tool in allowed

    def test_api_with_tools_includes_handoff(self) -> None:
        allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.API_WITH_TOOLS]
        assert "handoff" in allowed
        assert "transition_to_act" in allowed

    def test_pure_completion_filters_tools(self) -> None:
        allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.PURE_COMPLETION]
        assert "read_file" in allowed
        assert "write_file" in allowed
        assert "bash" in allowed
        assert "search_files" in allowed
        assert "propose_goal" in allowed
        assert "transition_to_act" in allowed

    def test_pure_completion_excludes_complex_tools(self) -> None:
        allowed = TOOLS_BY_CAPABILITY[CapabilityLevel.PURE_COMPLETION]
        assert "create_skill" not in allowed
        assert "handoff" not in allowed
        assert "edit_file" not in allowed
        assert "glob_files" not in allowed
        assert "list_directory" not in allowed

    def test_all_levels_have_entries(self) -> None:
        for level in CapabilityLevel:
            assert level in TOOLS_BY_CAPABILITY


# --- classify_model tests ---


class TestClassifyModel:
    def test_empty_model(self) -> None:
        assert classify_model("") == CapabilityLevel.PURE_COMPLETION

    def test_claude_is_full(self) -> None:
        assert classify_model("claude-3-opus") == CapabilityLevel.FULL
        assert classify_model("claude-sonnet-4") == CapabilityLevel.FULL

    def test_gpt4_is_full(self) -> None:
        assert classify_model("gpt-4o") == CapabilityLevel.FULL
        assert classify_model("gpt-4-turbo") == CapabilityLevel.FULL

    def test_gpt35_is_api_with_tools(self) -> None:
        assert classify_model("gpt-3.5-turbo") == CapabilityLevel.API_WITH_TOOLS

    def test_ollama_is_pure_completion(self) -> None:
        assert classify_model("ollama/llama3") == CapabilityLevel.PURE_COMPLETION

    def test_lm_studio_is_pure_completion(self) -> None:
        assert classify_model("lm_studio/qwen/qwen3-30b") == CapabilityLevel.PURE_COMPLETION

    def test_deepseek_is_api_with_tools(self) -> None:
        assert classify_model("deepseek/deepseek-chat") == CapabilityLevel.API_WITH_TOOLS


# --- _filter_tools_for_capability tests ---


class TestFilterToolsForCapability:
    """Test the static method via AgentLoopExecutor."""

    @pytest.fixture
    def sample_tools(self) -> list[dict[str, object]]:
        """Build a sample tools array in OpenAI format."""
        names = [
            "read_file",
            "write_file",
            "edit_file",
            "bash",
            "search_files",
            "glob_files",
            "list_directory",
            "create_skill",
            "search_skills",
            "handoff",
            "propose_goal",
            "transition_to_act",
        ]
        return [
            {"type": "function", "function": {"name": n, "description": f"tool {n}", "parameters": {}}} for n in names
        ]

    def test_full_returns_all(self, sample_tools: list[dict]) -> None:
        from codeforge.agent_loop import AgentLoopExecutor

        result = AgentLoopExecutor._filter_tools_for_capability(sample_tools, CapabilityLevel.FULL)
        assert len(result) == len(sample_tools)

    def test_pure_completion_filters(self, sample_tools: list[dict]) -> None:
        from codeforge.agent_loop import AgentLoopExecutor

        result = AgentLoopExecutor._filter_tools_for_capability(sample_tools, CapabilityLevel.PURE_COMPLETION)
        names = {t["function"]["name"] for t in result}
        assert "read_file" in names
        assert "write_file" in names
        assert "bash" in names
        assert "edit_file" not in names
        assert "create_skill" not in names
        assert "handoff" not in names

    def test_api_with_tools_filters(self, sample_tools: list[dict]) -> None:
        from codeforge.agent_loop import AgentLoopExecutor

        result = AgentLoopExecutor._filter_tools_for_capability(sample_tools, CapabilityLevel.API_WITH_TOOLS)
        names = {t["function"]["name"] for t in result}
        assert "read_file" in names
        assert "edit_file" in names
        assert "handoff" in names
        assert "create_skill" not in names
        assert "search_skills" not in names

    def test_mode_tools_extend_allowlist(self, sample_tools: list[dict]) -> None:
        from codeforge.agent_loop import AgentLoopExecutor

        result = AgentLoopExecutor._filter_tools_for_capability(
            sample_tools,
            CapabilityLevel.PURE_COMPLETION,
            mode_tools=frozenset({"create_skill"}),
        )
        names = {t["function"]["name"] for t in result}
        assert "create_skill" in names
        assert "read_file" in names

    def test_empty_tools_array(self) -> None:
        from codeforge.agent_loop import AgentLoopExecutor

        result = AgentLoopExecutor._filter_tools_for_capability([], CapabilityLevel.PURE_COMPLETION)
        assert result == []
