"""Agent tool for managing project goals via the Go Core HTTP API."""

from __future__ import annotations

import json
import logging
import os
from typing import Any

import httpx

from ._base import ToolDefinition, ToolResult

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Tool definition
# ---------------------------------------------------------------------------

MANAGE_GOALS_DEFINITION = ToolDefinition(
    name="manage_goals",
    description=(
        "Create, list, update, or delete project goals. "
        "Goals represent the project's vision, requirements, constraints, "
        "current state, and context."
    ),
    parameters={
        "type": "object",
        "properties": {
            "command": {
                "type": "string",
                "enum": ["create", "list", "update", "delete"],
                "description": "The operation to perform.",
            },
            "kind": {
                "type": "string",
                "enum": ["vision", "requirement", "constraint", "state", "context"],
                "description": "Goal kind (required for create, optional for update).",
            },
            "title": {
                "type": "string",
                "description": "Goal title (required for create, optional for update).",
            },
            "content": {
                "type": "string",
                "description": "Goal content in markdown (required for create, optional for update).",
            },
            "priority": {
                "type": "integer",
                "description": "Priority 0-100, higher = more important (optional, default 90).",
            },
            "goal_id": {
                "type": "string",
                "description": "Goal ID (required for update and delete).",
            },
        },
        "required": ["command"],
    },
    when_to_use=(
        "Use this tool to create, read, update, or delete project goals. "
        "Always list existing goals first before creating new ones to avoid duplicates."
    ),
    output_format="JSON summary of the operation result.",
    common_mistakes=[
        "Forgetting to list goals first, leading to duplicates.",
        "Missing required fields: kind/title/content for create, goal_id for update/delete.",
    ],
)


# ---------------------------------------------------------------------------
# Executor
# ---------------------------------------------------------------------------


class ManageGoalsExecutor:
    """Tool executor that proxies goal CRUD to the Go Core HTTP API."""

    def __init__(self, project_id: str) -> None:
        self._project_id = project_id
        self._core_url = os.environ.get("CODEFORGE_CORE_URL", "http://localhost:8080")

    async def execute(self, arguments: dict[str, Any], workspace_path: str) -> ToolResult:
        command = arguments.get("command", "")
        try:
            if command == "create":
                return await self._create(arguments)
            if command == "list":
                return await self._list()
            if command == "update":
                return await self._update(arguments)
            if command == "delete":
                return await self._delete(arguments)
            return ToolResult(
                output="",
                error=f"Unknown command: {command}. Use create, list, update, or delete.",
                success=False,
            )
        except httpx.HTTPError as exc:
            logger.warning("manage_goals HTTP error: %s", exc)
            return ToolResult(output="", error=f"HTTP error: {exc}", success=False)

    async def _create(self, args: dict[str, Any]) -> ToolResult:
        kind = args.get("kind")
        title = args.get("title")
        content = args.get("content")
        if not kind or not title or not content:
            return ToolResult(
                output="",
                error="create requires kind, title, and content.",
                success=False,
            )
        payload = {
            "kind": kind,
            "title": title,
            "content": content,
            "source": "agent",
            "priority": args.get("priority", 90),
        }
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.post(
                f"{self._core_url}/api/v1/projects/{self._project_id}/goals",
                json=payload,
            )
            resp.raise_for_status()
            data = resp.json()
        return ToolResult(
            output=json.dumps(
                {"status": "created", "id": data.get("id", ""), "title": title},
                indent=2,
            )
        )

    async def _list(self) -> ToolResult:
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.get(
                f"{self._core_url}/api/v1/projects/{self._project_id}/goals",
            )
            resp.raise_for_status()
            goals = resp.json()

        if not goals:
            return ToolResult(output="No goals found for this project.")

        lines = ["| Kind | Title | Source | Enabled |", "| --- | --- | --- | --- |"]
        lines.extend(
            f"| {g.get('kind', '')} | {g.get('title', '')} | {g.get('source', '')} | {g.get('enabled', True)} |"
            for g in goals
        )
        return ToolResult(output="\n".join(lines))

    async def _update(self, args: dict[str, Any]) -> ToolResult:
        goal_id = args.get("goal_id")
        if not goal_id:
            return ToolResult(output="", error="update requires goal_id.", success=False)
        payload: dict[str, Any] = {}
        for field in ("kind", "title", "content", "priority"):
            if field in args and args[field] is not None:
                payload[field] = args[field]
        if not payload:
            return ToolResult(output="", error="update requires at least one field to change.", success=False)
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.put(
                f"{self._core_url}/api/v1/goals/{goal_id}",
                json=payload,
            )
            resp.raise_for_status()
        return ToolResult(output=json.dumps({"status": "updated", "id": goal_id}, indent=2))

    async def _delete(self, args: dict[str, Any]) -> ToolResult:
        goal_id = args.get("goal_id")
        if not goal_id:
            return ToolResult(output="", error="delete requires goal_id.", success=False)
        async with httpx.AsyncClient(timeout=10.0) as client:
            resp = await client.delete(
                f"{self._core_url}/api/v1/goals/{goal_id}",
            )
            resp.raise_for_status()
        return ToolResult(output=json.dumps({"status": "deleted", "id": goal_id}, indent=2))
