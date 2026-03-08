"""Comprehensive tests for ToolRegistry and build_default_registry."""

from __future__ import annotations

from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import pytest

from codeforge.tools import ToolRegistry, build_default_registry
from codeforge.tools._base import ToolDefinition, ToolResult

if TYPE_CHECKING:
    from pathlib import Path


@pytest.fixture
def workspace(tmp_path: Path) -> Path:
    """Workspace with a single sample file for registry-level tests."""
    (tmp_path / "hello.txt").write_text("hello world\n")
    return tmp_path


# ---------------------------------------------------------------------------
# build_default_registry
# ---------------------------------------------------------------------------


class TestBuildDefaultRegistry:
    """Tests for the build_default_registry() factory."""

    def test_returns_registry_with_seven_tools(self) -> None:
        registry = build_default_registry()
        assert len(registry.tool_names) == 7

    def test_contains_expected_tool_names(self) -> None:
        registry = build_default_registry()
        expected = {
            "bash",
            "edit_file",
            "glob_files",
            "list_directory",
            "read_file",
            "search_files",
            "write_file",
        }
        assert set(registry.tool_names) == expected

    def test_tool_names_are_sorted(self) -> None:
        registry = build_default_registry()
        names = registry.tool_names
        assert names == sorted(names)

    def test_get_definitions_returns_all_seven(self) -> None:
        registry = build_default_registry()
        defs = registry.get_definitions()
        assert len(defs) == 7

    def test_get_definitions_sorted_by_name(self) -> None:
        registry = build_default_registry()
        defs = registry.get_definitions()
        names = [d.name for d in defs]
        assert names == sorted(names)

    def test_get_definitions_are_tool_definition_instances(self) -> None:
        registry = build_default_registry()
        for defn in registry.get_definitions():
            assert isinstance(defn, ToolDefinition)

    def test_each_definition_has_required_fields(self) -> None:
        registry = build_default_registry()
        for defn in registry.get_definitions():
            assert defn.name
            assert defn.description
            assert isinstance(defn.parameters, dict)


# ---------------------------------------------------------------------------
# get_openai_tools
# ---------------------------------------------------------------------------


class TestGetOpenAITools:
    """Tests for ToolRegistry.get_openai_tools() format compliance."""

    def test_returns_list_of_dicts(self) -> None:
        registry = build_default_registry()
        tools = registry.get_openai_tools()
        assert isinstance(tools, list)
        for tool in tools:
            assert isinstance(tool, dict)

    def test_each_tool_has_type_function(self) -> None:
        registry = build_default_registry()
        for tool in registry.get_openai_tools():
            assert tool["type"] == "function"

    def test_each_tool_has_function_dict_with_name_description_parameters(self) -> None:
        registry = build_default_registry()
        for tool in registry.get_openai_tools():
            func = tool["function"]
            assert "name" in func
            assert "description" in func
            assert "parameters" in func
            assert isinstance(func["name"], str)
            assert isinstance(func["description"], str)
            assert isinstance(func["parameters"], dict)

    def test_returns_correct_count(self) -> None:
        registry = build_default_registry()
        tools = registry.get_openai_tools()
        assert len(tools) == 7

    def test_empty_registry_returns_empty_list(self) -> None:
        registry = ToolRegistry()
        assert registry.get_openai_tools() == []

    def test_single_tool_format(self) -> None:
        registry = ToolRegistry()
        defn = ToolDefinition(
            name="my_tool",
            description="A tool",
            parameters={"type": "object", "properties": {"x": {"type": "string"}}},
        )
        registry.register(defn, MagicMock())

        tools = registry.get_openai_tools()
        assert len(tools) == 1
        assert tools[0] == {
            "type": "function",
            "function": {
                "name": "my_tool",
                "description": "A tool",
                "parameters": {"type": "object", "properties": {"x": {"type": "string"}}},
            },
        }


# ---------------------------------------------------------------------------
# execute
# ---------------------------------------------------------------------------


class TestRegistryExecute:
    """Tests for ToolRegistry.execute()."""

    async def test_unknown_tool_returns_error(self, workspace: Path) -> None:
        registry = ToolRegistry()
        result = await registry.execute("no_such_tool", {}, str(workspace))
        assert result.success is False
        assert "unknown tool" in result.error
        assert "no_such_tool" in result.error

    async def test_execute_delegates_to_executor(self, workspace: Path) -> None:
        registry = ToolRegistry()
        defn = ToolDefinition(name="test_tool", description="test", parameters={})
        mock_executor = AsyncMock()
        mock_executor.execute.return_value = ToolResult(output="done")
        registry.register(defn, mock_executor)

        result = await registry.execute("test_tool", {"key": "val"}, str(workspace))
        assert result.output == "done"
        assert result.success is True
        mock_executor.execute.assert_awaited_once_with({"key": "val"}, str(workspace))

    async def test_execute_via_default_registry(self, workspace: Path) -> None:
        registry = build_default_registry()
        result = await registry.execute("read_file", {"file_path": "hello.txt"}, str(workspace))
        assert result.success is True
        assert "hello world" in result.output

    async def test_execute_unknown_tool_output_is_empty(self, workspace: Path) -> None:
        registry = ToolRegistry()
        result = await registry.execute("nope", {}, str(workspace))
        assert result.output == ""


# ---------------------------------------------------------------------------
# register / tool_names
# ---------------------------------------------------------------------------


class TestRegisterAndToolNames:
    """Tests for register() and tool_names property."""

    def test_register_single_tool(self) -> None:
        registry = ToolRegistry()
        defn = ToolDefinition(name="alpha", description="a", parameters={})
        registry.register(defn, MagicMock())
        assert registry.tool_names == ["alpha"]

    def test_register_multiple_tools_sorted(self) -> None:
        registry = ToolRegistry()
        for name in ["charlie", "alpha", "bravo"]:
            defn = ToolDefinition(name=name, description=name, parameters={})
            registry.register(defn, MagicMock())
        assert registry.tool_names == ["alpha", "bravo", "charlie"]

    def test_overwrite_registration(self) -> None:
        registry = ToolRegistry()
        defn1 = ToolDefinition(name="tool", description="v1", parameters={})
        defn2 = ToolDefinition(name="tool", description="v2", parameters={})
        registry.register(defn1, MagicMock())
        registry.register(defn2, MagicMock())
        # Should still have one tool, not duplicated
        assert registry.tool_names == ["tool"]
        defs = registry.get_definitions()
        assert len(defs) == 1
        assert defs[0].description == "v2"

    def test_empty_registry_has_no_tools(self) -> None:
        registry = ToolRegistry()
        assert registry.tool_names == []
        assert registry.get_definitions() == []
        assert registry.get_openai_tools() == []


# ---------------------------------------------------------------------------
# merge_mcp_tools
# ---------------------------------------------------------------------------


class TestMergeMcpTools:
    """Tests for MCP tool merging."""

    def test_merges_valid_mcp_tools(self) -> None:
        registry = ToolRegistry()
        mock_wb = MagicMock()
        mock_wb.get_tools_for_llm.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "mcp__server1__tool_a",
                    "description": "Tool A",
                    "parameters": {"type": "object"},
                },
            },
        ]
        registry.merge_mcp_tools(mock_wb)
        assert "mcp__server1__tool_a" in registry.tool_names

    def test_skips_tools_without_double_underscore_format(self) -> None:
        registry = ToolRegistry()
        mock_wb = MagicMock()
        mock_wb.get_tools_for_llm.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "invalid_name",
                    "description": "Bad format",
                    "parameters": {},
                },
            },
        ]
        registry.merge_mcp_tools(mock_wb)
        assert registry.tool_names == []

    def test_mcp_tools_coexist_with_builtin(self) -> None:
        registry = build_default_registry()
        mock_wb = MagicMock()
        mock_wb.get_tools_for_llm.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "mcp__ext__fetch",
                    "description": "Fetch URL",
                    "parameters": {"type": "object"},
                },
            },
        ]
        registry.merge_mcp_tools(mock_wb)
        assert "mcp__ext__fetch" in registry.tool_names
        assert "read_file" in registry.tool_names
        assert len(registry.tool_names) == 8

    async def test_mcp_proxy_delegates_to_workbench(self) -> None:
        registry = ToolRegistry()
        mock_wb = MagicMock()
        mock_wb.get_tools_for_llm.return_value = [
            {
                "type": "function",
                "function": {
                    "name": "mcp__srv__do_thing",
                    "description": "Do thing",
                    "parameters": {},
                },
            },
        ]
        mock_wb.call_tool = AsyncMock(
            return_value=ToolResult(output="mcp result", error="", success=True),
        )
        registry.merge_mcp_tools(mock_wb)

        result = await registry.execute("mcp__srv__do_thing", {"arg": 1}, "/tmp")
        assert result.success is True
        assert result.output == "mcp result"
        mock_wb.call_tool.assert_awaited_once_with("srv", "do_thing", {"arg": 1})
