"""Agent skill drafts are saved in the conversation's tenant (KI-58).

The save inserted tenant_id '' into the UUID NOT NULL column skills.tenant_id,
which PostgreSQL rejects, so no agent draft was ever stored; and a tool
without storage reported the draft as validated (success). Now the draft is
saved with the conversation's tenant and project, and without a tenant (or
storage) the tool fails visibly.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import psycopg
import pytest
import structlog

from codeforge.consumer._conversation_skill_integration import make_skill_save_fn, wire_skill_tools
from codeforge.tools import build_default_registry
from codeforge.tools.create_skill import CreateSkillTool
from tests.pg_schema import create_schema

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

TENANT_A = "aaaaaaaa-0000-4000-8000-000000000001"
TENANT_B = "bbbbbbbb-0000-4000-8000-000000000002"
PROJECT = "11111111-0000-4000-8000-000000000001"

SKILL = {
    "name": "nats-error-handling",
    "type": "pattern",
    "description": "Settle every NATS message exactly once.",
    "content": "Ack on success, NAK with delay on failure.",
    "language": "python",
    "tags": ["nats", "errors"],
}


@pytest.fixture
async def skills_db() -> AsyncIterator[str]:
    async for dsn in create_schema("045_create_skills.sql", "067_extend_skills.sql"):
        yield dsn


async def _skills(dsn: str) -> list[tuple[str, str, str, str, str]]:
    async with await psycopg.AsyncConnection.connect(dsn) as conn:
        cur = await conn.execute(
            "SELECT tenant_id::text, project_id, name, source, status FROM skills ORDER BY tenant_id"
        )
        return [(str(r[0]), str(r[1]), str(r[2]), str(r[3]), str(r[4])) for r in await cur.fetchall()]


def _create_skill_tool(dsn: str, tenant_id: str) -> CreateSkillTool:
    registry = build_default_registry()
    wire_skill_tools(registry, [], PROJECT, structlog.get_logger(), dsn, tenant_id=tenant_id)
    tools = [executor for _defn, executor in registry.iter_executors() if isinstance(executor, CreateSkillTool)]
    assert len(tools) == 1
    return tools[0]


async def test_agent_draft_is_saved_in_the_conversation_tenant(skills_db: str) -> None:
    tool = _create_skill_tool(skills_db, TENANT_A)

    result = await tool.execute(dict(SKILL), "/tmp")

    assert result.success, result.error
    assert await _skills(skills_db) == [(TENANT_A, PROJECT, "nats-error-handling", "agent", "draft")]


async def test_same_skill_name_in_two_tenants(skills_db: str) -> None:
    for tenant in (TENANT_A, TENANT_B):
        save = make_skill_save_fn(PROJECT, skills_db, tenant_id=tenant)
        await save(dict(SKILL, source="agent", status="draft", format_origin="codeforge"))

    assert [row[0] for row in await _skills(skills_db)] == [TENANT_A, TENANT_B]


async def test_without_tenant_the_tool_fails_and_saves_nothing(skills_db: str) -> None:
    tool = _create_skill_tool(skills_db, "")

    result = await tool.execute(dict(SKILL), "/tmp")

    assert not result.success
    assert "not saved" in (result.error or "")
    assert await _skills(skills_db) == []


def test_save_fn_requires_a_tenant() -> None:
    with pytest.raises(ValueError, match="tenant"):
        make_skill_save_fn(PROJECT, "postgresql://x", tenant_id="")


async def test_tool_without_storage_does_not_report_success() -> None:
    result = await CreateSkillTool(save_fn=None).execute(dict(SKILL), "/tmp")

    assert not result.success
    assert "not saved" in (result.error or "")
