"""Experience cache of simple chats (KI-16).

Only the first turn of a chat without tools is answered from or stored in the
experience pool, in the conversation's tenant and project: its answer depends
on the question alone. An agentic turn never uses the cache: its result is
the work it does in the workspace, which a cached answer would report without
doing (no fake success). A follow-up turn depends on the history, which the
cached prompt does not carry. The cache is best effort: when it fails, the
chat runs as without it.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

import structlog

from codeforge.models import AgentLoopResult

if TYPE_CHECKING:
    from codeforge.memory.experience import ExperiencePool
    from codeforge.models import ConversationRunStartMessage
    from codeforge.runtime import RuntimeClient

logger = structlog.get_logger()


def cacheable_prompt(run_msg: ConversationRunStartMessage) -> str:
    """Return the prompt of a first-turn, text-only simple chat, or "" if the turn must not use the cache."""
    if run_msg.agentic or not run_msg.tenant_id or len(run_msg.messages) != 1:
        return ""
    message = run_msg.messages[0]
    if message.role != "user" or message.images or message.tool_calls:
        return ""
    return message.content.strip()


async def answer_from_experience(
    pool: ExperiencePool | None,
    run_msg: ConversationRunStartMessage,
    runtime: RuntimeClient,
    model: str,
) -> AgentLoopResult | None:
    """Answer the chat from the tenant's experience pool, streaming the answer, or return None."""
    prompt = cacheable_prompt(run_msg)
    if pool is None or not prompt:
        return None
    try:
        cached = await pool.lookup(prompt, run_msg.project_id, tenant_id=run_msg.tenant_id)
    except Exception as exc:
        logger.warning("experience lookup failed", run_id=run_msg.run_id, error=str(exc))
        return None
    if not cached:
        return None
    answer = str(cached["result_output"])
    await runtime.send_output(answer)
    logger.info(
        "chat answered from experience", run_id=run_msg.run_id, entry_id=cached["id"], similarity=cached["similarity"]
    )
    return AgentLoopResult(final_content=answer, model=model)


async def remember_answer(
    pool: ExperiencePool | None,
    run_msg: ConversationRunStartMessage,
    result: AgentLoopResult,
) -> None:
    """Store a successful first-turn chat answer in the tenant's experience pool."""
    prompt = cacheable_prompt(run_msg)
    if pool is None or not prompt or result.error or not result.final_content:
        return
    try:
        await pool.store(
            task_desc=prompt,
            project_id=run_msg.project_id,
            tenant_id=run_msg.tenant_id,
            result_output=result.final_content,
            result_cost=result.total_cost,
            result_status="completed",
            run_id=run_msg.run_id,
        )
    except Exception as exc:
        logger.warning("experience store failed", run_id=run_msg.run_id, error=str(exc))
