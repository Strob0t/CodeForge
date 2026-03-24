"""Skill and tool wiring for conversation handler."""

from __future__ import annotations

import structlog

logger = structlog.get_logger()


def wire_skill_tools(
    registry: object,
    skills: list,
    project_id: str,
    log: structlog.stdlib.BoundLogger,
    db_url: str,
) -> None:
    """Populate search_skills and create_skill tools with loaded data."""
    from codeforge.tools.create_skill import CreateSkillTool
    from codeforge.tools.search_skills import SearchSkillsTool

    for _defn, executor in registry._tools.values():  # type: ignore[attr-defined]
        if isinstance(executor, SearchSkillsTool):
            executor.set_skills(skills)
            log.debug("search_skills tool populated", skill_count=len(skills))
        elif isinstance(executor, CreateSkillTool) and executor._save_fn is None:
            executor._save_fn = make_skill_save_fn(project_id, db_url)
            log.debug("create_skill tool save_fn wired")


def make_skill_save_fn(project_id: str, db_url: str) -> object:
    """Create an async callback that saves a skill draft to the database."""
    import psycopg

    async def save_fn(skill_data: dict) -> str:
        import uuid

        skill_id = str(uuid.uuid4())
        async with await psycopg.AsyncConnection.connect(db_url) as conn:
            async with conn.cursor() as cur:
                await cur.execute(
                    "INSERT INTO skills (id, tenant_id, project_id, name, type, description,"
                    " language, content, code, tags, source, source_url, format_origin, status)"
                    " VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                    (
                        skill_id,
                        "",  # tenant_id set by trigger or default
                        project_id,
                        skill_data["name"],
                        skill_data["type"],
                        skill_data["description"],
                        skill_data.get("language", ""),
                        skill_data["content"],
                        skill_data["content"],  # backwards compat: also populate code
                        skill_data.get("tags", []),
                        "agent",
                        "",
                        skill_data.get("format_origin", "codeforge"),
                        "draft",
                    ),
                )
            await conn.commit()
        return skill_id

    return save_fn


def register_handoff_tool(registry: object, run_id: str, js: object) -> None:
    """Register the handoff tool in the tool registry if NATS is available."""
    if js is None:
        return

    from codeforge.tools._base import ToolDefinition
    from codeforge.tools._base import ToolResult as _ToolResult
    from codeforge.tools.handoff import HANDOFF_TOOL_DEF

    func_def = HANDOFF_TOOL_DEF["function"]

    class _HandoffProxy:
        def __init__(self, js_client: object, rid: str) -> None:
            self._js = js_client
            self._run_id = rid

        async def execute(self, arguments: dict, workspace_path: str) -> _ToolResult:
            from codeforge.tools.handoff import execute_handoff

            result = await execute_handoff(self._run_id, arguments, self._js.publish)
            return _ToolResult(output=result)

    registry.register(
        ToolDefinition(name=func_def["name"], description=func_def["description"], parameters=func_def["parameters"]),
        _HandoffProxy(js, run_id),
    )


def register_propose_goal_tool(registry: object, runtime: object) -> None:
    """Register the propose_goal tool for agent-driven goal proposals."""
    from codeforge.tools.propose_goal import PROPOSE_GOAL_DEFINITION, ProposeGoalExecutor

    registry.register(PROPOSE_GOAL_DEFINITION, ProposeGoalExecutor(runtime))
