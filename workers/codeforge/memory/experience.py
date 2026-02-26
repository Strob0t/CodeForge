"""Experience Pool: caching successful agent runs for similarity-based reuse.

The @exp_cache decorator wraps async functions to check for cached results
before executing, storing new results on success.
"""

from __future__ import annotations

import functools
from collections.abc import Callable
from typing import TYPE_CHECKING, Any, TypeVar

import numpy as np
import structlog

from codeforge.memory.scorer import CompositeScorer

if TYPE_CHECKING:
    import psycopg

    from codeforge.llm import LiteLLMClient

logger = structlog.get_logger()

F = TypeVar("F", bound=Callable[..., Any])


class ExperiencePool:
    """Caches successful agent runs for reuse via similarity-based matching."""

    def __init__(
        self,
        db: psycopg.AsyncConnection[object],
        llm: LiteLLMClient,
        scorer: CompositeScorer | None = None,
        confidence_threshold: float = 0.85,
    ) -> None:
        self._db = db
        self._llm = llm
        self._scorer = scorer or CompositeScorer()
        self._threshold = confidence_threshold

    async def lookup(
        self,
        task_desc: str,
        project_id: str,
        threshold: float | None = None,
    ) -> dict[str, Any] | None:
        """Look up a cached experience entry by task similarity."""
        threshold = threshold or self._threshold
        query_emb = await self._compute_embedding(task_desc)
        if query_emb is None:
            return None

        async with self._db.cursor() as cur:
            await cur.execute(
                """SELECT id, task_description, task_embedding, result_output,
                          result_cost, result_status, confidence, created_at
                   FROM experience_entries
                   WHERE project_id = %s
                   ORDER BY last_used_at DESC
                   LIMIT 200""",
                (project_id,),
            )
            rows = await cur.fetchall()

        best_score = 0.0
        best_entry: dict[str, Any] | None = None

        for row in rows:
            entry_emb_bytes = row[2]
            if entry_emb_bytes is None:
                continue
            entry_emb = np.frombuffer(entry_emb_bytes, dtype=np.float32)
            similarity = float(
                np.dot(query_emb, entry_emb) / (np.linalg.norm(query_emb) * np.linalg.norm(entry_emb) + 1e-8)
            )
            if similarity > best_score:
                best_score = similarity
                best_entry = {
                    "id": str(row[0]),
                    "task_description": row[1],
                    "result_output": row[3],
                    "result_cost": row[4],
                    "result_status": row[5],
                    "confidence": row[6],
                    "similarity": similarity,
                }

        if best_entry and best_score >= threshold:
            # Increment hit count
            async with self._db.cursor() as cur:
                await cur.execute(
                    "UPDATE experience_entries SET hit_count = hit_count + 1, last_used_at = NOW() WHERE id = %s",
                    (best_entry["id"],),
                )
                await self._db.commit()
            logger.info(
                "experience cache hit",
                entry_id=best_entry["id"],
                similarity=best_score,
            )
            return best_entry

        return None

    async def store(
        self,
        task_desc: str,
        project_id: str,
        result_output: str,
        result_cost: float,
        result_status: str,
        run_id: str,
    ) -> str:
        """Store a new experience entry."""
        embedding = await self._compute_embedding(task_desc)
        embedding_bytes = embedding.tobytes() if embedding is not None else None

        async with self._db.cursor() as cur:
            await cur.execute(
                """INSERT INTO experience_entries
                   (tenant_id, project_id, task_description, task_embedding,
                    result_output, result_cost, result_status, run_id, confidence)
                   VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
                   RETURNING id""",
                (
                    "00000000-0000-0000-0000-000000000000",
                    project_id,
                    task_desc,
                    embedding_bytes,
                    result_output,
                    result_cost,
                    result_status,
                    run_id,
                    1.0,  # initial confidence
                ),
            )
            row = await cur.fetchone()
            await self._db.commit()
            entry_id = str(row[0]) if row else ""
            logger.info("experience stored", entry_id=entry_id)
            return entry_id

    async def invalidate(self, entry_id: str) -> None:
        """Remove an experience entry."""
        async with self._db.cursor() as cur:
            await cur.execute("DELETE FROM experience_entries WHERE id = %s", (entry_id,))
            await self._db.commit()
        logger.info("experience invalidated", entry_id=entry_id)

    async def _compute_embedding(self, text: str) -> np.ndarray | None:
        """Compute an embedding vector for the given text."""
        try:
            resp = await self._llm.embedding(text)
            if resp and len(resp) > 0:
                return np.array(resp, dtype=np.float32)
        except Exception:
            logger.warning("embedding computation failed", exc_info=True)
        return None


def exp_cache(
    pool: ExperiencePool,
    project_id_arg: str = "project_id",
    task_desc_arg: str = "task_desc",
) -> Callable[[F], F]:
    """Decorator that checks the experience pool before executing a function.

    Usage:
        @exp_cache(pool, project_id_arg="project_id", task_desc_arg="prompt")
        async def run_agent(project_id: str, prompt: str) -> str:
            ...
    """

    def decorator(func: F) -> F:
        @functools.wraps(func)
        async def wrapper(*args: Any, **kwargs: Any) -> Any:
            project_id = kwargs.get(project_id_arg, "")
            task_desc = kwargs.get(task_desc_arg, "")

            if project_id and task_desc:
                cached = await pool.lookup(task_desc, project_id)
                if cached:
                    logger.info(
                        "using cached experience",
                        entry_id=cached["id"],
                        similarity=cached["similarity"],
                    )
                    return cached["result_output"]

            result = await func(*args, **kwargs)

            # Store successful result
            if project_id and task_desc and result:
                await pool.store(
                    task_desc=task_desc,
                    project_id=project_id,
                    result_output=str(result),
                    result_cost=0.0,
                    result_status="success",
                    run_id="",
                )

            return result

        return wrapper  # type: ignore[return-value]

    return decorator
