"""The experience pool is off by default, tenant-scoped, and never replaces agent work (KI-16).

Before, the worker always created one pool bound to the zero-UUID tenant and
ignored ``experience.enabled`` (documented default: off),
``confidence_threshold`` and ``max_entries``. Every tenant's experiences
landed in one tenant, and the agent loop answered a similar prompt from the
cache without running: an agentic turn reported "done" without changing the
workspace (D-S3: no fake success).

Now the pool exists only when enabled, every lookup and store names the
conversation's tenant, and only a first-turn simple chat (no tools, no
history) is answered from or stored in the cache.
"""

from __future__ import annotations

import json
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, MagicMock

import numpy as np
import psycopg
import pytest

from codeforge.config import get_settings
from codeforge.consumer import TaskConsumer
from codeforge.consumer._base import ConsumerBaseMixin
from codeforge.consumer._conversation import ConversationHandlerMixin
from codeforge.llm import ChatCompletionResponse, RoutingResult
from codeforge.memory.experience import ExperiencePool
from codeforge.models import ConversationMessagePayload, ConversationRunStartMessage, TerminationConfig
from codeforge.runtime import RuntimeClient
from codeforge.tools import build_default_registry
from tests.jetstream_fakes import RecordingJetStream
from tests.pg_schema import create_schema

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

TENANT_A = "aaaaaaaa-0000-4000-8000-000000000001"
TENANT_B = "bbbbbbbb-0000-4000-8000-000000000002"
MODEL = "openai/gpt-4o"


# ---------------------------------------------------------------------------
# Settings and wiring
# ---------------------------------------------------------------------------


def test_pool_is_off_by_default() -> None:
    settings = get_settings()
    assert settings.experience_enabled is False
    assert settings.experience_confidence_threshold == pytest.approx(0.85)
    assert settings.experience_max_entries == 1000

    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    assert worker._experience_pool is None


def test_enabled_pool_uses_the_configured_limits(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CODEFORGE_EXPERIENCE_ENABLED", "true")
    monkeypatch.setenv("CODEFORGE_EXPERIENCE_CONFIDENCE_THRESHOLD", "0.93")
    monkeypatch.setenv("CODEFORGE_EXPERIENCE_MAX_ENTRIES", "7")

    worker = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")

    pool = worker._experience_pool
    assert isinstance(pool, ExperiencePool)
    assert pool._threshold == pytest.approx(0.93)
    assert pool._max_entries == 7


@pytest.mark.parametrize(("threshold", "max_entries"), [(0.0, 10), (1.01, 10), (-0.5, 10), (0.9, -1)])
def test_invalid_limits_are_rejected(threshold: float, max_entries: int) -> None:
    with pytest.raises(ValueError, match="experience"):
        ExperiencePool(
            db_url="postgresql://x", llm=MagicMock(), confidence_threshold=threshold, max_entries=max_entries
        )


async def test_lookup_and_store_require_a_tenant() -> None:
    pool = ExperiencePool(db_url="postgresql://x", llm=MagicMock())
    with pytest.raises(ValueError, match="tenant"):
        await pool.lookup("prompt", "proj", tenant_id="")
    with pytest.raises(ValueError, match="tenant"):
        await pool.store(
            "prompt", "proj", tenant_id="", result_output="x", result_cost=0.0, result_status="completed", run_id="r"
        )
    with pytest.raises(ValueError, match="tenant"):
        await pool.invalidate("entry", tenant_id="")


# ---------------------------------------------------------------------------
# Two tenants against PostgreSQL
# ---------------------------------------------------------------------------


@pytest.fixture
async def experience_db() -> AsyncIterator[str]:
    async for dsn in create_schema("043_create_experience_pool.sql"):
        yield dsn


def _embed_by_text(monkeypatch: pytest.MonkeyPatch) -> None:
    """A deterministic embedding: equal texts are identical, different texts orthogonal."""
    vectors: dict[str, np.ndarray] = {}

    async def embed(_llm: object, text: str) -> np.ndarray:
        if text not in vectors:
            vec = np.zeros(64, dtype=np.float32)
            vec[len(vectors)] = 1.0
            vectors[text] = vec
        return vectors[text]

    monkeypatch.setattr("codeforge.memory.experience.compute_embedding", embed)


async def _new_project(dsn: str) -> str:
    async with await psycopg.AsyncConnection.connect(dsn, autocommit=True) as conn:
        row = await (await conn.execute("INSERT INTO projects DEFAULT VALUES RETURNING id")).fetchone()
    assert row is not None
    return str(row[0])


async def _rows(dsn: str) -> list[tuple[str, str, str]]:
    async with await psycopg.AsyncConnection.connect(dsn) as conn:
        cur = await conn.execute(
            "SELECT tenant_id::text, project_id::text, task_description FROM experience_entries ORDER BY created_at"
        )
        return [(str(r[0]), str(r[1]), str(r[2])) for r in await cur.fetchall()]


async def _store(pool: ExperiencePool, prompt: str, project: str, tenant: str, answer: str = "answer") -> str:
    return await pool.store(
        prompt, project, tenant_id=tenant, result_output=answer, result_cost=0.01, result_status="completed", run_id="r"
    )


async def test_tenants_do_not_share_experiences(experience_db: str, monkeypatch: pytest.MonkeyPatch) -> None:
    _embed_by_text(monkeypatch)
    pool = ExperiencePool(db_url=experience_db, llm=MagicMock())
    project = await _new_project(experience_db)

    await _store(pool, "explain the build", project, TENANT_A, answer="A's answer")

    assert await pool.lookup("explain the build", project, tenant_id=TENANT_B) is None
    hit = await pool.lookup("explain the build", project, tenant_id=TENANT_A)
    assert hit is not None
    assert hit["result_output"] == "A's answer"
    assert await _rows(experience_db) == [(TENANT_A, project, "explain the build")], "stored in the caller's tenant"

    await _store(pool, "explain the build", project, TENANT_B, answer="B's answer")
    hit_b = await pool.lookup("explain the build", project, tenant_id=TENANT_B)
    assert hit_b is not None
    assert hit_b["result_output"] == "B's answer", "the same prompt is independent per tenant"


async def test_eviction_and_invalidation_stay_in_the_tenant(
    experience_db: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    _embed_by_text(monkeypatch)
    pool = ExperiencePool(db_url=experience_db, llm=MagicMock(), max_entries=2)
    project = await _new_project(experience_db)

    b_entry = await _store(pool, "b prompt", project, TENANT_B)
    for i in range(3):
        await _store(pool, f"a prompt {i}", project, TENANT_A)

    rows = await _rows(experience_db)
    assert [r[0] for r in rows].count(TENANT_A) == 2, "max_entries is enforced per tenant and project"
    assert (TENANT_B, project, "b prompt") in rows, "another tenant's entries are not evicted"

    await pool.invalidate(b_entry, tenant_id=TENANT_A)
    assert (TENANT_B, project, "b prompt") in await _rows(experience_db), "invalidate is tenant-scoped"
    await pool.invalidate(b_entry, tenant_id=TENANT_B)
    assert (TENANT_B, project, "b prompt") not in await _rows(experience_db)


# ---------------------------------------------------------------------------
# Conversations: only a first-turn simple chat uses the cache
# ---------------------------------------------------------------------------


class _Handler(ConversationHandlerMixin, ConsumerBaseMixin):
    pass


class _AllowAll(RecordingJetStream):
    async def publish(self, subject: str, payload: bytes = b"", **kwargs: object) -> object:
        ack = await super().publish(subject, payload, **kwargs)  # type: ignore[arg-type]
        if subject == "runs.toolcall.request":
            request = json.loads(payload)
            response = {"run_id": request["run_id"], "call_id": request["call_id"], "decision": "allow"}
            listening = [s for s in self.subscriptions if s.subject == "runs.toolcall.response" and not s.unsubscribed]
            listening[-1].deliver(json.dumps(response).encode())
        return ack


class _LLM:
    def __init__(self, content: str = "fresh answer") -> None:
        self.calls = 0
        self._content = content

    async def chat_completion_stream(self, **_kwargs: object) -> ChatCompletionResponse:
        self.calls += 1
        return ChatCompletionResponse(
            content=self._content,
            tool_calls=[],
            finish_reason="stop",
            tokens_in=5,
            tokens_out=5,
            model=MODEL,
            cost_usd=0.02,
        )


def _pool(cached: str | None = None) -> MagicMock:
    pool = MagicMock()
    pool.lookup = AsyncMock(return_value={"id": "e1", "similarity": 0.99, "result_output": cached} if cached else None)
    pool.store = AsyncMock(return_value="e2")
    return pool


def _run_msg(
    *, agentic: bool, history: list[ConversationMessagePayload] | None = None, tenant: str = TENANT_A
) -> ConversationRunStartMessage:
    return ConversationRunStartMessage(
        run_id="run-1",
        conversation_id="conv-1",
        project_id="proj-1",
        tenant_id=tenant,
        messages=history
        if history is not None
        else [ConversationMessagePayload(role="user", content="explain the build")],
        system_prompt="s",
        model=MODEL,
        agentic=agentic,
        workspace_path="/tmp",
    )


async def _execute(run_msg: ConversationRunStartMessage, pool: MagicMock | None, llm: _LLM) -> tuple[object, _AllowAll]:
    handler = _Handler()
    handler._llm = llm  # type: ignore[assignment]
    handler._experience_pool = pool  # type: ignore[attr-defined]
    handler._js = _AllowAll()  # type: ignore[assignment]
    handler._db_url = ""
    handler._litellm_key = ""
    runtime = RuntimeClient(
        js=handler._js,  # type: ignore[arg-type]
        run_id=run_msg.run_id,
        task_id=run_msg.run_id,
        project_id=run_msg.project_id,
        termination=TerminationConfig(max_steps=5, max_cost=5.0),
        tenant_id=run_msg.tenant_id,
        approval_timeout_seconds=1,
    )
    messages = [{"role": "user", "content": m.content} for m in run_msg.messages]
    result = await handler._execute_conversation_run(
        run_msg=run_msg,
        messages=messages,
        primary_model=MODEL,
        routing=RoutingResult(model=MODEL),
        runtime=runtime,
        registry=build_default_registry(skill_tools=False),
        fallback_models=[],
    )
    return result, handler._js  # type: ignore[return-value]


def _outputs(js: _AllowAll) -> str:
    return "".join(
        json.loads(data).get("line", "") for subject, data in js.published if subject.startswith("runs.output")
    )


async def test_agentic_turn_never_answers_from_the_cache() -> None:
    pool = _pool(cached="cached: I changed the files")
    llm = _LLM("did the work")

    result, _ = await _execute(_run_msg(agentic=True), pool, llm)

    assert result.final_content == "did the work"  # type: ignore[attr-defined]
    assert llm.calls == 1, "the loop runs"
    pool.lookup.assert_not_awaited()
    pool.store.assert_not_awaited()


async def test_first_turn_chat_is_answered_from_the_tenant_cache() -> None:
    pool = _pool(cached="cached answer")
    llm = _LLM()

    result, js = await _execute(_run_msg(agentic=False), pool, llm)

    assert result.final_content == "cached answer"  # type: ignore[attr-defined]
    assert result.total_cost == 0.0  # type: ignore[attr-defined]
    assert llm.calls == 0
    pool.lookup.assert_awaited_once_with("explain the build", "proj-1", tenant_id=TENANT_A)
    assert "cached answer" in _outputs(js), "the cached answer is streamed like a live one"
    pool.store.assert_not_awaited()


async def test_first_turn_chat_miss_is_stored_in_the_tenant() -> None:
    pool = _pool()
    llm = _LLM("fresh answer")

    result, _ = await _execute(_run_msg(agentic=False), pool, llm)

    assert result.final_content == "fresh answer"  # type: ignore[attr-defined]
    assert llm.calls == 1
    pool.store.assert_awaited_once()
    kwargs = pool.store.await_args.kwargs
    assert kwargs["tenant_id"] == TENANT_A
    assert kwargs["project_id"] == "proj-1"
    assert kwargs["task_desc"] == "explain the build"
    assert kwargs["result_output"] == "fresh answer"


@pytest.mark.parametrize(
    "history",
    [
        [
            ConversationMessagePayload(role="user", content="explain the build"),
            ConversationMessagePayload(role="assistant", content="it uses make"),
            ConversationMessagePayload(role="user", content="and the tests?"),
        ],
        [ConversationMessagePayload(role="user", content="")],
    ],
    ids=["follow-up turn", "no text"],
)
async def test_other_chats_bypass_the_cache(history: list[ConversationMessagePayload]) -> None:
    pool = _pool(cached="cached answer")
    llm = _LLM()

    await _execute(_run_msg(agentic=False, history=history), pool, llm)

    assert llm.calls == 1
    pool.lookup.assert_not_awaited()
    pool.store.assert_not_awaited()


async def test_chat_without_tenant_bypasses_the_cache() -> None:
    pool = _pool(cached="cached answer")
    llm = _LLM()

    await _execute(_run_msg(agentic=False, tenant=""), pool, llm)

    assert llm.calls == 1
    pool.lookup.assert_not_awaited()
    pool.store.assert_not_awaited()


async def test_failing_cache_does_not_fail_the_chat() -> None:
    pool = _pool()
    pool.lookup = AsyncMock(side_effect=OSError("db down"))
    pool.store = AsyncMock(side_effect=OSError("db down"))
    llm = _LLM("fresh answer")

    result, _ = await _execute(_run_msg(agentic=False), pool, llm)

    assert result.final_content == "fresh answer"  # type: ignore[attr-defined]
    assert not result.error  # type: ignore[attr-defined]
