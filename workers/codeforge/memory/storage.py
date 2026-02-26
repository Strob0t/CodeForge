"""MemoryStore: persistence and retrieval of agent memories via PostgreSQL.

Embedding computation is delegated to the LiteLLM client. Retrieval uses
the CompositeScorer for ranking.
"""

from __future__ import annotations

from datetime import UTC, datetime
from typing import TYPE_CHECKING

import numpy as np
import structlog

from codeforge.memory.models import (
    Memory,
    MemoryKind,
    MemoryRecallRequest,
    MemoryStoreRequest,
    ScoredMemory,
    ScoreWeights,
)
from codeforge.memory.scorer import CompositeScorer

if TYPE_CHECKING:
    import psycopg

    from codeforge.llm import LiteLLMClient

logger = structlog.get_logger()


class MemoryStore:
    """Manages persistent agent memories with embedding-based retrieval."""

    def __init__(
        self,
        db: psycopg.AsyncConnection[object],
        llm: LiteLLMClient,
        weights: ScoreWeights | None = None,
        half_life_hours: float = 168.0,
    ) -> None:
        self._db = db
        self._llm = llm
        self._scorer = CompositeScorer(weights=weights, half_life_hours=half_life_hours)

    async def store(self, req: MemoryStoreRequest) -> str:
        """Store a new memory, computing its embedding via LLM."""
        embedding = await self._compute_embedding(req.content)
        embedding_bytes = embedding.tobytes() if embedding is not None else None

        async with self._db.cursor() as cur:
            await cur.execute(
                """INSERT INTO agent_memories
                   (tenant_id, project_id, agent_id, run_id, content, kind, importance, embedding, metadata)
                   VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb)
                   RETURNING id""",
                (
                    "00000000-0000-0000-0000-000000000000",
                    req.project_id,
                    req.agent_id,
                    req.run_id,
                    req.content,
                    req.kind.value,
                    req.importance,
                    embedding_bytes,
                    dict(req.metadata),
                ),
            )
            row = await cur.fetchone()
            await self._db.commit()
            memory_id = str(row[0]) if row else ""
            logger.info("memory stored", memory_id=memory_id, kind=req.kind)
            return memory_id

    async def recall(
        self,
        req: MemoryRecallRequest,
    ) -> list[ScoredMemory]:
        """Recall top-k memories by composite scoring."""
        query_embedding = await self._compute_embedding(req.query)
        if query_embedding is None:
            logger.warning("could not compute query embedding for recall")
            return []

        # Fetch candidate memories from DB
        kind_filter = ""
        params: list[object] = [req.project_id]
        if req.kind is not None:
            kind_filter = " AND kind = %s"
            params.append(req.kind.value)

        base_query = (
            "SELECT id, tenant_id, project_id, agent_id, run_id,"
            "       content, kind, importance, embedding, metadata, created_at"
            " FROM agent_memories"
            " WHERE project_id = %s"
        )
        query = base_query + kind_filter + " ORDER BY created_at DESC LIMIT 500"
        async with self._db.cursor() as cur:
            await cur.execute(query, params)
            rows = await cur.fetchall()

        # Score each candidate
        scored: list[ScoredMemory] = []
        for row in rows:
            mem_embedding_bytes = row[8]
            if mem_embedding_bytes is None:
                continue

            mem_embedding = np.frombuffer(mem_embedding_bytes, dtype=np.float32)
            created_at: datetime = row[10]
            if created_at.tzinfo is None:
                created_at = created_at.replace(tzinfo=UTC)

            score = self._scorer.score(
                query_embedding=query_embedding,
                memory_embedding=mem_embedding,
                created_at=created_at,
                importance=float(row[7]),
            )

            mem = Memory(
                id=str(row[0]),
                tenant_id=str(row[1]),
                project_id=str(row[2]),
                agent_id=row[3] or "",
                run_id=row[4] or "",
                content=row[5],
                kind=MemoryKind(row[6]),
                importance=float(row[7]),
                metadata=row[9] if isinstance(row[9], dict) else {},
                created_at=created_at,
            )
            scored.append(ScoredMemory(memory=mem, score=score))

        # Sort by score descending and return top-k
        scored.sort(key=lambda s: s.score, reverse=True)
        return scored[: req.top_k]

    async def _compute_embedding(self, text: str) -> np.ndarray | None:
        """Compute an embedding vector for the given text."""
        try:
            resp = await self._llm.embedding(text)
            if resp and len(resp) > 0:
                return np.array(resp, dtype=np.float32)
        except Exception:
            logger.warning("embedding computation failed", exc_info=True)
        return None
