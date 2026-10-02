"""Pydantic models for MCP (Model Context Protocol) integration."""

from __future__ import annotations

from pydantic import BaseModel, Field


class MCPServerDef(BaseModel):
    """Definition of an MCP server that can be connected to during a run."""

    id: str
    name: str
    description: str = ""
    transport: str  # "stdio", "sse", or "streamable_http"
    command: str = ""
    args: list[str] = Field(default_factory=list)
    url: str = ""
    env: dict[str, str] = Field(default_factory=dict)
    headers: dict[str, str] = Field(default_factory=dict)
    enabled: bool = True
    # Host names, IPs and CIDR prefixes whose private addresses an sse or
    # streamable_http server may use (mcp.allowed_private_hosts, KI-100).
    allowed_private_hosts: list[str] = Field(default_factory=list)
    # An operator server (servers_dir): it may use private and loopback addresses.
    # Only the Go Core sets it, never for a server a tenant stored.
    trusted: bool = False
    # mcp.use_proxy: connect through the proxy of the environment, unpinned.
    use_proxy: bool = False


class MCPTool(BaseModel):
    """A tool discovered from an MCP server."""

    server_id: str
    name: str
    description: str
    input_schema: dict[str, object] = Field(default_factory=dict)


class MCPToolCallResult(BaseModel):
    """Result of calling a tool on an MCP server."""

    success: bool
    output: str = ""
    error: str = ""
    is_error: bool = False
