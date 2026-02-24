"""Tests for MCP Pydantic models."""

from codeforge.mcp_models import MCPServerDef, MCPTool, MCPToolCallResult
from codeforge.models import RunStartMessage

# --- MCPServerDef Tests ---


def test_mcp_server_def_stdio() -> None:
    """MCPServerDef should accept valid stdio transport."""
    server = MCPServerDef(
        id="test-server",
        name="Test Server",
        transport="stdio",
        command="npx",
        args=["-y", "@modelcontextprotocol/server-filesystem"],
    )
    assert server.id == "test-server"
    assert server.transport == "stdio"
    assert server.command == "npx"
    assert server.args == ["-y", "@modelcontextprotocol/server-filesystem"]
    assert server.url == ""
    assert server.enabled is True


def test_mcp_server_def_sse() -> None:
    """MCPServerDef should accept valid SSE transport."""
    server = MCPServerDef(
        id="remote-server",
        name="Remote Server",
        transport="sse",
        url="http://localhost:3000/sse",
        headers={"Authorization": "Bearer token123"},
    )
    assert server.transport == "sse"
    assert server.url == "http://localhost:3000/sse"
    assert server.headers == {"Authorization": "Bearer token123"}


def test_mcp_server_def_defaults() -> None:
    """MCPServerDef should have sensible defaults."""
    server = MCPServerDef(id="s1", name="S1", transport="stdio")
    assert server.description == ""
    assert server.command == ""
    assert server.args == []
    assert server.url == ""
    assert server.env == {}
    assert server.headers == {}
    assert server.enabled is True


def test_mcp_server_def_disabled() -> None:
    """MCPServerDef should support disabled flag."""
    server = MCPServerDef(id="s1", name="S1", transport="stdio", enabled=False)
    assert server.enabled is False


def test_mcp_server_def_with_env() -> None:
    """MCPServerDef should support environment variables."""
    server = MCPServerDef(
        id="s1",
        name="S1",
        transport="stdio",
        command="python",
        env={"API_KEY": "secret"},
    )
    assert server.env == {"API_KEY": "secret"}


def test_mcp_server_def_from_json() -> None:
    """MCPServerDef should deserialize from JSON."""
    raw = '{"id": "s1", "name": "S1", "transport": "stdio", "command": "node", "args": ["server.js"]}'
    server = MCPServerDef.model_validate_json(raw)
    assert server.id == "s1"
    assert server.command == "node"
    assert server.args == ["server.js"]


# --- MCPTool Tests ---


def test_mcp_tool_construction() -> None:
    """MCPTool should construct with all required fields."""
    tool = MCPTool(
        server_id="s1",
        name="read_file",
        description="Read a file from disk",
        input_schema={
            "type": "object",
            "properties": {"path": {"type": "string"}},
            "required": ["path"],
        },
    )
    assert tool.server_id == "s1"
    assert tool.name == "read_file"
    assert tool.description == "Read a file from disk"
    assert "properties" in tool.input_schema


def test_mcp_tool_defaults() -> None:
    """MCPTool should have empty input_schema by default."""
    tool = MCPTool(server_id="s1", name="ping", description="Ping the server")
    assert tool.input_schema == {}


# --- MCPToolCallResult Tests ---


def test_mcp_tool_call_result_success() -> None:
    """MCPToolCallResult should represent a successful call."""
    result = MCPToolCallResult(success=True, output="file contents here")
    assert result.success is True
    assert result.output == "file contents here"
    assert result.error == ""
    assert result.is_error is False


def test_mcp_tool_call_result_error() -> None:
    """MCPToolCallResult should represent an error call."""
    result = MCPToolCallResult(success=False, error="file not found", is_error=True)
    assert result.success is False
    assert result.error == "file not found"
    assert result.is_error is True


def test_mcp_tool_call_result_defaults() -> None:
    """MCPToolCallResult should have sensible defaults."""
    result = MCPToolCallResult(success=True)
    assert result.output == ""
    assert result.error == ""
    assert result.is_error is False


# --- RunStartMessage MCP integration ---


def test_run_start_message_mcp_servers_default() -> None:
    """RunStartMessage should default mcp_servers to empty list."""
    raw = '{"run_id": "r1", "task_id": "t1", "project_id": "p1", "agent_id": "a1", "prompt": "Fix bug"}'
    msg = RunStartMessage.model_validate_json(raw)
    assert msg.mcp_servers == []


def test_run_start_message_mcp_servers_null_coerced() -> None:
    """RunStartMessage should coerce null mcp_servers to empty list."""
    raw = '{"run_id": "r1", "task_id": "t1", "project_id": "p1", "agent_id": "a1", "prompt": "Fix bug", "mcp_servers": null}'
    msg = RunStartMessage.model_validate_json(raw)
    assert msg.mcp_servers == []


def test_run_start_message_with_mcp_servers() -> None:
    """RunStartMessage should include MCP server definitions."""
    raw = (
        '{"run_id": "r1", "task_id": "t1", "project_id": "p1", "agent_id": "a1",'
        ' "prompt": "Fix bug", "mcp_servers": [{"id": "fs", "name": "Filesystem",'
        ' "transport": "stdio", "command": "npx", "args": ["-y", "server-fs"]}]}'
    )
    msg = RunStartMessage.model_validate_json(raw)
    assert len(msg.mcp_servers) == 1
    assert msg.mcp_servers[0].id == "fs"
    assert msg.mcp_servers[0].transport == "stdio"
