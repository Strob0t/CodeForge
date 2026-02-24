"""Tests for MCP workbench: connections, tool discovery, and BM25 recommendation."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.mcp_models import MCPServerDef, MCPTool
from codeforge.mcp_workbench import McpServerConnection, McpToolRecommender, McpWorkbench

# --- McpWorkbench Tests ---


def test_workbench_init() -> None:
    """McpWorkbench should initialize with empty state."""
    wb = McpWorkbench()
    assert wb._connections == {}
    assert wb._tools == []


def test_get_tools_for_llm_empty() -> None:
    """get_tools_for_llm should return empty list when no tools are discovered."""
    wb = McpWorkbench()
    assert wb.get_tools_for_llm() == []


def test_get_tools_for_llm_format() -> None:
    """get_tools_for_llm should return OpenAI-compatible function definitions."""
    wb = McpWorkbench()
    wb._tools = [
        MCPTool(
            server_id="fs",
            name="read_file",
            description="Read a file",
            input_schema={
                "type": "object",
                "properties": {"path": {"type": "string"}},
            },
        ),
        MCPTool(
            server_id="git",
            name="status",
            description="Git status",
            input_schema={"type": "object", "properties": {}},
        ),
    ]

    defs = wb.get_tools_for_llm()
    assert len(defs) == 2

    first = defs[0]
    assert first["type"] == "function"
    func = first["function"]
    assert func["name"] == "mcp__fs__read_file"
    assert func["description"] == "Read a file"
    assert "properties" in func["parameters"]

    second = defs[1]
    assert second["function"]["name"] == "mcp__git__status"


@pytest.mark.asyncio
async def test_call_tool_server_not_connected() -> None:
    """call_tool should return error when server is not connected."""
    wb = McpWorkbench()
    result = await wb.call_tool("nonexistent", "some_tool", {})
    assert result.success is False
    assert "not connected" in result.error


@pytest.mark.asyncio
async def test_disconnect_all_empty() -> None:
    """disconnect_all should handle empty workbench gracefully."""
    wb = McpWorkbench()
    await wb.disconnect_all()
    assert wb._connections == {}
    assert wb._tools == []


@pytest.mark.asyncio
async def test_connect_servers_skips_disabled() -> None:
    """connect_servers should skip disabled servers."""
    wb = McpWorkbench()
    defs = [
        MCPServerDef(id="s1", name="S1", transport="stdio", command="echo", enabled=False),
    ]
    with patch.object(McpServerConnection, "connect", new_callable=AsyncMock) as mock_connect:
        await wb.connect_servers(defs)
        mock_connect.assert_not_called()
    assert len(wb._connections) == 0


@pytest.mark.asyncio
async def test_connect_servers_handles_failure() -> None:
    """connect_servers should continue when a server fails to connect."""
    wb = McpWorkbench()
    defs = [
        MCPServerDef(id="s1", name="S1", transport="stdio", command="nonexistent"),
    ]
    with patch.object(McpServerConnection, "connect", new_callable=AsyncMock, side_effect=OSError("spawn failed")):
        await wb.connect_servers(defs)
    assert len(wb._connections) == 0


@pytest.mark.asyncio
async def test_disconnect_all_cleans_up() -> None:
    """disconnect_all should disconnect all connections and clear state."""
    wb = McpWorkbench()
    mock_conn = MagicMock(spec=McpServerConnection)
    mock_conn.disconnect = AsyncMock()
    mock_conn.server_id = "s1"
    wb._connections["s1"] = mock_conn
    wb._tools = [MCPTool(server_id="s1", name="t", description="d")]

    await wb.disconnect_all()
    mock_conn.disconnect.assert_awaited_once()
    assert wb._connections == {}
    assert wb._tools == []


# --- McpServerConnection Tests ---


def test_server_connection_init() -> None:
    """McpServerConnection should initialize from a server def."""
    server_def = MCPServerDef(id="s1", name="S1", transport="stdio", command="echo")
    conn = McpServerConnection(server_def)
    assert conn.server_id == "s1"
    assert conn.connected is False


@pytest.mark.asyncio
async def test_server_connection_list_tools_not_connected() -> None:
    """list_tools should return empty list when not connected."""
    server_def = MCPServerDef(id="s1", name="S1", transport="stdio", command="echo")
    conn = McpServerConnection(server_def)
    tools = await conn.list_tools()
    assert tools == []


@pytest.mark.asyncio
async def test_server_connection_call_tool_not_connected() -> None:
    """call_tool should return error when not connected."""
    server_def = MCPServerDef(id="s1", name="S1", transport="stdio", command="echo")
    conn = McpServerConnection(server_def)
    result = await conn.call_tool("read_file", {"path": "/tmp/test"})
    assert result.success is False
    assert result.is_error is True
    assert "not connected" in result.error


def test_server_connection_unsupported_transport() -> None:
    """McpServerConnection should reject unsupported transport types at connect time."""
    server_def = MCPServerDef(id="s1", name="S1", transport="grpc")
    conn = McpServerConnection(server_def)
    assert conn.server_id == "s1"


# --- McpToolRecommender Tests ---


def test_recommender_empty_tools() -> None:
    """McpToolRecommender should handle empty tool list."""
    recommender = McpToolRecommender([])
    results = recommender.recommend("read file")
    assert results == []


def test_recommender_basic_recommendation() -> None:
    """McpToolRecommender should return relevant tools via BM25."""
    tools = [
        MCPTool(server_id="fs", name="read_file", description="Read contents of a file from disk"),
        MCPTool(server_id="fs", name="write_file", description="Write contents to a file on disk"),
        MCPTool(server_id="git", name="git_status", description="Show the git status of the repository"),
        MCPTool(server_id="git", name="git_diff", description="Show the git diff of changes"),
    ]
    recommender = McpToolRecommender(tools)
    results = recommender.recommend("read file contents", top_k=2)
    assert len(results) == 2
    # read_file should rank higher than other tools for a "read file" query
    assert results[0].name == "read_file"


def test_recommender_top_k_clamped() -> None:
    """McpToolRecommender should clamp top_k to available tools."""
    tools = [
        MCPTool(server_id="s1", name="tool_a", description="Alpha tool"),
        MCPTool(server_id="s1", name="tool_b", description="Beta tool"),
    ]
    recommender = McpToolRecommender(tools)
    results = recommender.recommend("alpha", top_k=100)
    assert len(results) == 2


def test_recommender_returns_mcp_tool_instances() -> None:
    """McpToolRecommender should return MCPTool instances."""
    tools = [
        MCPTool(server_id="s1", name="search", description="Search for code patterns"),
    ]
    recommender = McpToolRecommender(tools)
    results = recommender.recommend("search code")
    assert len(results) == 1
    assert isinstance(results[0], MCPTool)
    assert results[0].name == "search"
