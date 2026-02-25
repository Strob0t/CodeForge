"""Tests for the built-in tool registry and individual tool executors."""

from __future__ import annotations

from typing import TYPE_CHECKING
from unittest.mock import MagicMock

import pytest

from codeforge.tools import ToolRegistry, build_default_registry
from codeforge.tools._base import ToolDefinition
from codeforge.tools.bash import BashTool

if TYPE_CHECKING:
    from pathlib import Path
from codeforge.tools.edit_file import EditFileTool
from codeforge.tools.glob_files import GlobFilesTool
from codeforge.tools.list_directory import ListDirectoryTool
from codeforge.tools.read_file import ReadFileTool
from codeforge.tools.search_files import SearchFilesTool
from codeforge.tools.write_file import WriteFileTool


@pytest.fixture
def workspace(tmp_path: Path) -> Path:
    """Create a workspace directory with some sample files."""
    (tmp_path / "hello.txt").write_text("line one\nline two\nline three\n")
    (tmp_path / "sub").mkdir()
    (tmp_path / "sub" / "deep.py").write_text("def foo():\n    return 42\n")
    return tmp_path


# --- ReadFileTool ---


@pytest.mark.asyncio
async def test_read_file_basic(workspace: Path) -> None:
    tool = ReadFileTool()
    result = await tool.execute({"file_path": "hello.txt"}, str(workspace))
    assert result.success is True
    assert "line one" in result.output
    assert "line two" in result.output
    assert "line three" in result.output


@pytest.mark.asyncio
async def test_read_file_with_offset_and_limit(workspace: Path) -> None:
    tool = ReadFileTool()
    result = await tool.execute({"file_path": "hello.txt", "offset": 2, "limit": 1}, str(workspace))
    assert result.success is True
    assert "line two" in result.output
    assert "line one" not in result.output
    assert "line three" not in result.output


@pytest.mark.asyncio
async def test_read_file_not_found(workspace: Path) -> None:
    tool = ReadFileTool()
    result = await tool.execute({"file_path": "nonexistent.txt"}, str(workspace))
    assert result.success is False
    assert "not found" in result.error


@pytest.mark.asyncio
async def test_read_file_path_traversal(workspace: Path) -> None:
    tool = ReadFileTool()
    result = await tool.execute({"file_path": "../../etc/passwd"}, str(workspace))
    assert result.success is False
    assert "traversal" in result.error


# --- WriteFileTool ---


@pytest.mark.asyncio
async def test_write_file_create(workspace: Path) -> None:
    tool = WriteFileTool()
    result = await tool.execute({"file_path": "new.txt", "content": "hello world"}, str(workspace))
    assert result.success is True
    assert (workspace / "new.txt").read_text() == "hello world"


@pytest.mark.asyncio
async def test_write_file_with_subdirs(workspace: Path) -> None:
    tool = WriteFileTool()
    result = await tool.execute({"file_path": "a/b/c.txt", "content": "nested"}, str(workspace))
    assert result.success is True
    assert (workspace / "a" / "b" / "c.txt").read_text() == "nested"


@pytest.mark.asyncio
async def test_write_file_overwrite(workspace: Path) -> None:
    tool = WriteFileTool()
    result = await tool.execute({"file_path": "hello.txt", "content": "overwritten"}, str(workspace))
    assert result.success is True
    assert (workspace / "hello.txt").read_text() == "overwritten"


@pytest.mark.asyncio
async def test_write_file_path_traversal(workspace: Path) -> None:
    tool = WriteFileTool()
    result = await tool.execute({"file_path": "../outside.txt", "content": "bad"}, str(workspace))
    assert result.success is False
    assert "traversal" in result.error


# --- EditFileTool ---


@pytest.mark.asyncio
async def test_edit_file_success(workspace: Path) -> None:
    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": "hello.txt", "old_text": "line two", "new_text": "LINE TWO"},
        str(workspace),
    )
    assert result.success is True
    content = (workspace / "hello.txt").read_text()
    assert "LINE TWO" in content
    assert "line two" not in content


@pytest.mark.asyncio
async def test_edit_file_not_found(workspace: Path) -> None:
    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": "hello.txt", "old_text": "does not exist", "new_text": "x"},
        str(workspace),
    )
    assert result.success is False
    assert "not found" in result.error


@pytest.mark.asyncio
async def test_edit_file_multiple_matches(workspace: Path) -> None:
    (workspace / "dup.txt").write_text("aaa\naaa\n")
    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": "dup.txt", "old_text": "aaa", "new_text": "bbb"},
        str(workspace),
    )
    assert result.success is False
    assert "2 times" in result.error


@pytest.mark.asyncio
async def test_edit_file_path_traversal(workspace: Path) -> None:
    tool = EditFileTool()
    result = await tool.execute(
        {"file_path": "../../etc/passwd", "old_text": "root", "new_text": "hacked"},
        str(workspace),
    )
    assert result.success is False
    assert "traversal" in result.error


# --- BashTool ---


@pytest.mark.asyncio
async def test_bash_simple_command(workspace: Path) -> None:
    tool = BashTool()
    result = await tool.execute({"command": "echo hello"}, str(workspace))
    assert result.success is True
    assert "hello" in result.output


@pytest.mark.asyncio
async def test_bash_timeout(workspace: Path) -> None:
    tool = BashTool()
    result = await tool.execute({"command": "sleep 60", "timeout": 1}, str(workspace))
    assert result.success is False
    assert "timed out" in result.error


@pytest.mark.asyncio
async def test_bash_nonzero_exit(workspace: Path) -> None:
    tool = BashTool()
    result = await tool.execute({"command": "exit 1"}, str(workspace))
    assert result.success is False
    assert "exit code" in result.error


# --- SearchFilesTool ---


@pytest.mark.asyncio
async def test_search_files_match(workspace: Path) -> None:
    tool = SearchFilesTool()
    result = await tool.execute({"pattern": "def foo"}, str(workspace))
    assert result.success is True
    assert "def foo" in result.output


@pytest.mark.asyncio
async def test_search_files_no_match(workspace: Path) -> None:
    tool = SearchFilesTool()
    result = await tool.execute({"pattern": "zzz_nonexistent_zzz"}, str(workspace))
    assert result.success is True
    assert "no matches" in result.output


@pytest.mark.asyncio
async def test_search_files_with_include(workspace: Path) -> None:
    tool = SearchFilesTool()
    result = await tool.execute({"pattern": "line", "include": "*.txt"}, str(workspace))
    assert result.success is True
    assert "line" in result.output


# --- GlobFilesTool ---


@pytest.mark.asyncio
async def test_glob_files_match(workspace: Path) -> None:
    tool = GlobFilesTool()
    result = await tool.execute({"pattern": "**/*.py"}, str(workspace))
    assert result.success is True
    assert "deep.py" in result.output


@pytest.mark.asyncio
async def test_glob_files_no_match(workspace: Path) -> None:
    tool = GlobFilesTool()
    result = await tool.execute({"pattern": "**/*.rs"}, str(workspace))
    assert result.success is True
    assert "no matches" in result.output


# --- ListDirectoryTool ---


@pytest.mark.asyncio
async def test_list_directory_flat(workspace: Path) -> None:
    tool = ListDirectoryTool()
    result = await tool.execute({"path": "."}, str(workspace))
    assert result.success is True
    assert "[DIR]" in result.output
    assert "[FILE]" in result.output


@pytest.mark.asyncio
async def test_list_directory_recursive(workspace: Path) -> None:
    tool = ListDirectoryTool()
    result = await tool.execute({"path": ".", "recursive": True}, str(workspace))
    assert result.success is True
    assert "deep.py" in result.output


@pytest.mark.asyncio
async def test_list_directory_not_a_dir(workspace: Path) -> None:
    tool = ListDirectoryTool()
    result = await tool.execute({"path": "hello.txt"}, str(workspace))
    assert result.success is False
    assert "not a directory" in result.error


@pytest.mark.asyncio
async def test_list_directory_path_traversal(workspace: Path) -> None:
    tool = ListDirectoryTool()
    result = await tool.execute({"path": "../../"}, str(workspace))
    assert result.success is False
    assert "traversal" in result.error


# --- ToolRegistry ---


def test_registry_register_and_names() -> None:
    registry = ToolRegistry()
    defn = ToolDefinition(name="test_tool", description="A test tool", parameters={"type": "object"})
    executor = MagicMock()
    registry.register(defn, executor)
    assert "test_tool" in registry.tool_names


def test_registry_get_openai_tools_format() -> None:
    registry = ToolRegistry()
    defn = ToolDefinition(
        name="my_tool",
        description="Does something",
        parameters={"type": "object", "properties": {"x": {"type": "string"}}},
    )
    executor = MagicMock()
    registry.register(defn, executor)

    tools = registry.get_openai_tools()
    assert len(tools) == 1
    assert tools[0]["type"] == "function"
    func = tools[0]["function"]
    assert func["name"] == "my_tool"
    assert func["description"] == "Does something"
    assert "properties" in func["parameters"]


@pytest.mark.asyncio
async def test_registry_execute(workspace: Path) -> None:
    registry = build_default_registry()
    result = await registry.execute("read_file", {"file_path": "hello.txt"}, str(workspace))
    assert result.success is True
    assert "line one" in result.output


@pytest.mark.asyncio
async def test_registry_execute_unknown_tool(workspace: Path) -> None:
    registry = ToolRegistry()
    result = await registry.execute("nonexistent", {}, str(workspace))
    assert result.success is False
    assert "unknown tool" in result.error


def test_build_default_registry_has_all_tools() -> None:
    registry = build_default_registry()
    expected = {"bash", "edit_file", "glob_files", "list_directory", "read_file", "search_files", "write_file"}
    assert set(registry.tool_names) == expected


def test_build_default_registry_openai_format() -> None:
    registry = build_default_registry()
    tools = registry.get_openai_tools()
    assert len(tools) == 7
    for tool in tools:
        assert tool["type"] == "function"
        assert "name" in tool["function"]
        assert "description" in tool["function"]
        assert "parameters" in tool["function"]


# --- MCP Tool Merge ---


def test_merge_mcp_tools() -> None:
    """merge_mcp_tools should register MCP tools under mcp__server__tool names."""
    registry = ToolRegistry()

    # Create a mock workbench
    mock_wb = MagicMock()
    mock_wb.get_tools_for_llm.return_value = [
        {
            "type": "function",
            "function": {
                "name": "mcp__fs__read_file",
                "description": "Read a file via MCP",
                "parameters": {"type": "object", "properties": {"path": {"type": "string"}}},
            },
        },
        {
            "type": "function",
            "function": {
                "name": "mcp__git__status",
                "description": "Git status via MCP",
                "parameters": {"type": "object", "properties": {}},
            },
        },
    ]

    registry.merge_mcp_tools(mock_wb)
    assert "mcp__fs__read_file" in registry.tool_names
    assert "mcp__git__status" in registry.tool_names
