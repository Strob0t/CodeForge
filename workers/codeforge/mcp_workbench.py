"""MCP workbench: manages connections to MCP servers and tool discovery."""

from __future__ import annotations

import logging
import os
from contextlib import AsyncExitStack
from typing import TYPE_CHECKING, Any

import httpx
from mcp import ClientSession
from mcp.client.sse import sse_client
from mcp.client.streamable_http import streamable_http_client

from codeforge.mcp_models import MCPServerDef, MCPTool, MCPToolCallResult
from codeforge.mcp_outbound import OutboundPolicy, guarded_client_factory, scrub_url_secrets
from codeforge.tool_process import tool_stdio_client
from codeforge.tracing import tracing_manager

if TYPE_CHECKING:
    from collections.abc import Sequence

    from anyio.streams.memory import MemoryObjectReceiveStream, MemoryObjectSendStream
    from mcp.shared.message import SessionMessage

    _Streams = tuple[MemoryObjectReceiveStream[SessionMessage | Exception], MemoryObjectSendStream[SessionMessage]]

logger = logging.getLogger(__name__)

_tracer = tracing_manager.get_tracer()


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
        """Establish a connection to the MCP server.

        A connection that fails closes what it opened (the server process,
        its log handle) before the error is raised.
        """
        self._exit_stack = AsyncExitStack()
        try:
            await self._open(self._exit_stack)
        except BaseException:
            await self._exit_stack.aclose()
            self._exit_stack = None
            self._session = None
            raise
        logger.info("connected to MCP server %s (%s)", self._def.id, self._def.transport)

    async def _open(self, stack: AsyncExitStack) -> None:
        if self._def.transport == "stdio":
            # The server runs as the tool user, like every agent tool (KI-71). Its
            # stderr needs a real file (an io.StringIO has no file descriptor).
            errlog = stack.enter_context(open(os.devnull, "w"))  # noqa: SIM115 - closed by the exit stack
            read_stream, write_stream = await stack.enter_async_context(
                tool_stdio_client(self._def.command, self._def.args, declared_env=self._def.env, errlog=errlog)
            )
        elif self._def.transport in ("sse", "streamable_http"):
            read_stream, write_stream = await self._open_remote(stack)
        else:
            msg = f"unsupported transport: {self._def.transport}"
            raise ValueError(msg)

        self._session = await stack.enter_async_context(ClientSession(read_stream, write_stream))
        await self._session.initialize()

    async def _open_remote(self, stack: AsyncExitStack) -> _Streams:
        """Open an sse or streamable_http connection (KI-100).

        A url whose host is, or resolves to, a refused address fails here with
        the reason, before anything connects; the client's transport checks
        the address of every request again (DNS rebinding, redirects).
        """
        policy = OutboundPolicy(self._def.allowed_private_hosts, trusted=self._def.trusted)
        await policy.check_url(self._def.url)
        client_factory = guarded_client_factory(policy, use_proxy=self._def.use_proxy)
        if self._def.transport == "sse":
            return await stack.enter_async_context(
                sse_client(
                    url=self._def.url,
                    headers=self._def.headers or None,
                    httpx_client_factory=client_factory,
                )
            )
        client = await stack.enter_async_context(client_factory(headers=self._def.headers or None))
        read_stream, write_stream, _session_id = await stack.enter_async_context(
            streamable_http_client(self._def.url, http_client=client)
        )
        return read_stream, write_stream

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

    @_tracer.trace_tool("mcp_tool")
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

    @_tracer.trace_agent("mcp_workbench")
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
            except Exception as exc:
                # No traceback: httpx errors quote the url with its secrets (KI-97 security review).
                logger.error(
                    "failed to connect to MCP server %s (%s): %s",
                    server_def.id,
                    _host_of(server_def.url),
                    scrub_url_secrets(_describe(exc), server_def.url),
                )

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
            except Exception as exc:
                logger.exception("error disconnecting MCP server %s: %s", conn.server_id, exc)
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


def _describe(exc: BaseException) -> str:
    """The class and message of exc, or of the leaves of an exception group (anyio task groups)."""
    if isinstance(exc, BaseExceptionGroup):
        return "; ".join(_describe(inner) for inner in exc.exceptions)
    return f"{type(exc).__name__}: {exc}"


def _host_of(url: str) -> str:
    try:
        return httpx.URL(url).host or "-"
    except httpx.InvalidURL:
        return "-"


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
