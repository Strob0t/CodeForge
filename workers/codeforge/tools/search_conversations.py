"""Built-in tool: search past conversation messages via Core API."""

from __future__ import annotations

import logging
import os
from typing import Any

import httpx

from codeforge.tools._base import ToolDefinition, ToolExample, ToolResult

logger = logging.getLogger(__name__)

DEFINITION = ToolDefinition(
    name="search_conversations",
    description="Search past conversation messages by keyword. Returns matching messages with their role, timestamp, and content snippet.",
    parameters={
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "Search query to find in conversation messages.",
            },
            "limit": {
                "type": "integer",
                "description": "Maximum number of results to return (default 5, max 20).",
            },
        },
        "required": ["query"],
    },
    when_to_use="Use to find previous conversation messages about a topic, recall past decisions, or find context from earlier discussions.",
    output_format="Each result shows: [role] (timestamp) content_snippet. Returns 'no matches found' if nothing matches.",
    examples=[
        ToolExample(
            description="Find past discussions about authentication",
            tool_call_json='{"query": "authentication login"}',
            expected_result="[assistant] (2026-03-09T14:30:00Z) The authentication system uses JWT tokens with...",
        ),
    ],
)

_MAX_CONTENT_LEN = 300
_DEFAULT_LIMIT = 5
_MAX_LIMIT = 20


class SearchConversationsTool:
    """Search conversation history via Core API."""

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        query = arguments.get("query", "")
        if not query:
            return ToolResult(output="", error="query is required", success=False)

        limit = min(arguments.get("limit", _DEFAULT_LIMIT), _MAX_LIMIT)

        core_url = os.environ.get("CODEFORGE_CORE_URL", "http://localhost:8080")
        url = f"{core_url}/api/v1/search/conversations"

        try:
            async with httpx.AsyncClient() as client:
                resp = await client.post(
                    url,
                    json={"query": query, "limit": limit},
                    timeout=10.0,
                )
                if resp.status_code != 200:
                    return ToolResult(
                        output="",
                        error=f"search API returned {resp.status_code}",
                        success=False,
                    )
                data = resp.json()
        except Exception as exc:
            return ToolResult(output="", error=f"search request failed: {exc}", success=False)

        results: list[dict[str, str]] = data.get("results", [])
        if not results:
            return ToolResult(output="no matches found")

        lines: list[str] = []
        for r in results:
            role = r.get("role", "unknown")
            ts = r.get("created_at", "")
            content = r.get("content", "")
            if len(content) > _MAX_CONTENT_LEN:
                content = content[:_MAX_CONTENT_LEN] + "..."
            lines.append(f"[{role}] ({ts}) {content}")

        return ToolResult(output="\n\n".join(lines))
