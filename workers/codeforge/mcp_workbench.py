"""MCP workbench: manages connections to MCP servers and tool discovery."""

from __future__ import annotations

import io
import logging
from contextlib import AsyncExitStack
from typing import TYPE_CHECKING, Any

from mcp import ClientSession, StdioServerParameters, stdio_client
from mcp.client.sse import sse_client

from codeforge.mcp_models import MCPServerDef, MCPTool, MCPToolCallResult

if TYPE_CHECKING:
    from collections.abc import Sequence

logger = logging.getLogger(__name__)


class McpServerConnection:
    """Manages a single MCP server connection."""

    def __init__(self, server_def: MCPServerDef) -> None:
        self._def = server_def
        self._session: ClientSession | None = None
        self._exit_stack: AsyncExitStack | None = None
        self._tools: list[MCPTool] = []

    @property
    def server_id(self) -> str:
        return self._def.id

    @property
    def connected(self) -> bool:
        return self._session is not None

    async def connect(self) -> None:
        """Establish a connection to the MCP server."""
        self._exit_stack = AsyncExitStack()

        if self._def.transport == "stdio":
            params = StdioServerParameters(
                command=self._def.command,
                args=self._def.args,
                env=self._def.env or None,
            )
            # stdio_client is an async context manager yielding (read, write) streams
            read_stream, write_stream = await self._exit_stack.enter_async_context(
                stdio_client(params, errlog=io.StringIO())
            )
        elif self._def.transport == "sse":
            read_stream, write_stream = await self._exit_stack.enter_async_context(
                sse_client(
                    url=self._def.url,
                    headers=self._def.headers or None,
                )
            )
        else:
            msg = f"unsupported transport: {self._def.transport}"
            raise ValueError(msg)

        self._session = await self._exit_stack.enter_async_context(ClientSession(read_stream, write_stream))
        await self._session.initialize()
        logger.info("connected to MCP server %s (%s)", self._def.id, self._def.transport)

    async def disconnect(self) -> None:
        """Close the connection to the MCP server."""
        if self._exit_stack is not None:
            await self._exit_stack.aclose()
            self._exit_stack = None
        self._session = None
        self._tools = []
        logger.info("disconnected from MCP server %s", self._def.id)

    async def list_tools(self) -> list[MCPTool]:
        """Discover tools exposed by the MCP server."""
        if self._session is None:
            return []

        result = await self._session.list_tools()
        self._tools = [
            MCPTool(
                server_id=self._def.id,
                name=tool.name,
                description=tool.description or "",
                input_schema=tool.inputSchema or {},
            )
            for tool in result.tools
        ]
        return self._tools

    async def call_tool(self, tool_name: str, arguments: dict[str, Any]) -> MCPToolCallResult:
        """Call a tool on the MCP server."""
        if self._session is None:
            return MCPToolCallResult(
                success=False,
                error="not connected",
                is_error=True,
            )

        try:
            result = await self._session.call_tool(tool_name, arguments)
            output_parts = [block.text for block in result.content if hasattr(block, "text")]
            output = "\n".join(output_parts)

            return MCPToolCallResult(
                success=not result.isError,
                output=output,
                is_error=result.isError or False,
            )
        except Exception as exc:
            logger.exception("MCP tool call failed: %s/%s", self._def.id, tool_name)
            return MCPToolCallResult(
                success=False,
                error=str(exc),
                is_error=True,
            )


class McpWorkbench:
    """Container for multiple MCP server connections scoped to a run."""

    def __init__(self) -> None:
        self._connections: dict[str, McpServerConnection] = {}
        self._tools: list[MCPTool] = []

    async def connect_servers(self, defs: list[MCPServerDef]) -> None:
        """Connect to all enabled MCP servers."""
        for server_def in defs:
            if not server_def.enabled:
                logger.info("skipping disabled MCP server %s", server_def.id)
                continue

            conn = McpServerConnection(server_def)
            try:
                await conn.connect()
                self._connections[server_def.id] = conn
            except Exception:
                logger.exception("failed to connect to MCP server %s", server_def.id)

    async def discover_tools(self) -> list[MCPTool]:
        """Discover tools from all connected servers."""
        self._tools = []
        for conn in self._connections.values():
            tools = await conn.list_tools()
            self._tools.extend(tools)
        return self._tools

    async def call_tool(self, server_id: str, tool_name: str, arguments: dict[str, Any]) -> MCPToolCallResult:
        """Call a tool on a specific MCP server."""
        conn = self._connections.get(server_id)
        if conn is None:
            return MCPToolCallResult(
                success=False,
                error=f"server not connected: {server_id}",
                is_error=True,
            )
        return await conn.call_tool(tool_name, arguments)

    async def disconnect_all(self) -> None:
        """Disconnect from all MCP servers."""
        for conn in self._connections.values():
            try:
                await conn.disconnect()
            except Exception:
                logger.exception("error disconnecting MCP server %s", conn.server_id)
        self._connections.clear()
        self._tools = []

    def get_tools_for_llm(self) -> list[dict[str, object]]:
        """Format discovered tools as OpenAI-compatible function definitions."""
        return [
            {
                "type": "function",
                "function": {
                    "name": f"mcp__{tool.server_id}__{tool.name}",
                    "description": tool.description,
                    "parameters": tool.input_schema,
                },
            }
            for tool in self._tools
        ]


class McpToolRecommender:
    """BM25-based tool recommendation for MCP tools."""

    def __init__(self, tools: Sequence[MCPTool]) -> None:
        self._tools = list(tools)
        self._retriever: object | None = None
        self._build_index()

    def _build_index(self) -> None:
        """Build a BM25 index from tool names and descriptions."""
        if not self._tools:
            return

        import bm25s

        corpus = [f"{t.name} {t.description}" for t in self._tools]
        corpus_tokens = bm25s.tokenize(corpus)
        self._retriever = bm25s.BM25()
        self._retriever.index(corpus_tokens)

    def recommend(self, query: str, top_k: int = 10) -> list[MCPTool]:
        """Recommend tools matching the query."""
        if not self._tools or self._retriever is None:
            return []

        import bm25s

        query_tokens = bm25s.tokenize([query])
        effective_k = min(top_k, len(self._tools))
        results, _scores = self._retriever.retrieve(query_tokens, k=effective_k)

        return [self._tools[int(idx)] for idx in results[0]]
