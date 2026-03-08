"""Memory store and recall handler mixins."""

from __future__ import annotations

import hashlib
import json
from datetime import UTC
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_MEMORY_RECALL_RESULT

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class MemoryHandlerMixin:
    """Handles memory.store and memory.recall messages."""

    async def _handle_memory_store(self, msg: nats.aio.msg.Msg) -> None:
        """Process a memory store request: compute embedding and persist."""
        from codeforge.memory.models import MemoryStoreRequest

        await self._handle_request(
            msg=msg,
            request_model=MemoryStoreRequest,
            dedup_key=lambda r: f"memstore-{r.project_id}-{r.run_id}-{r.kind.value}",
            handler=self._do_memory_store,
            result_subject=None,
            log_context=lambda r: {"project_id": r.project_id, "kind": r.kind},
        )

    async def _do_memory_store(self, req: object, log: structlog.BoundLogger) -> None:
        """Business logic for memory storage. Catches errors to ensure ack (not nak)."""
        try:
            log.info("received memory store request")

            embedding = None
            try:
                emb_resp = await self._llm.embedding(req.content)
                if emb_resp:
                    import numpy as np

                    embedding = np.array(emb_resp, dtype=np.float32).tobytes()
            except Exception as exc:
                log.warning(
                    "embedding computation failed for memory",
                    exc_info=True,
                    error=str(exc),
                )

            import psycopg

            async with (
                await psycopg.AsyncConnection.connect(self._db_url) as conn,
                conn.cursor() as cur,
            ):
                await cur.execute(
                    """INSERT INTO agent_memories
                           (tenant_id, project_id, agent_id, run_id, content,
                            kind, importance, embedding, metadata)
                           VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb)""",
                    (
                        req.tenant_id,
                        req.project_id,
                        req.agent_id,
                        req.run_id,
                        req.content,
                        req.kind.value,
                        req.importance,
                        embedding,
                        dict(req.metadata),
                    ),
                )
                await conn.commit()

            log.info("memory stored successfully")
        except Exception as exc:
            # Log but do not re-raise: memory store is best-effort, ack to prevent
            # infinite redelivery.
            logger.exception("failed to process memory store request", error=str(exc))

    async def _handle_memory_recall(self, msg: nats.aio.msg.Msg) -> None:
        """Process a memory recall request: score and return top-k memories."""
        from codeforge.memory.models import MemoryRecallRequest

        await self._handle_request(
            msg=msg,
            request_model=MemoryRecallRequest,
            dedup_key=lambda r: f"memrecall-{r.project_id}-{hashlib.sha256(r.query.encode()).hexdigest()[:16]}",
            handler=self._do_memory_recall,
            result_subject=None,
            log_context=lambda r: {"project_id": r.project_id, "top_k": r.top_k},
        )

    async def _do_memory_recall(self, req: object, log: structlog.BoundLogger) -> None:
        """Business logic for memory recall. Catches errors to ensure ack (not nak)."""
        try:
            from codeforge.memory.scorer import CompositeScorer

            log.info("received memory recall request")

            import numpy as np

            query_emb = None
            try:
                emb_resp = await self._llm.embedding(req.query)
                if emb_resp:
                    query_emb = np.array(emb_resp, dtype=np.float32)
            except Exception as exc:
                log.warning(
                    "embedding computation failed for recall query",
                    exc_info=True,
                    error=str(exc),
                )

            if query_emb is None:
                if self._js is not None:
                    error_payload = {
                        "request_id": req.request_id,
                        "project_id": req.project_id,
                        "query": req.query,
                        "results": [],
                        "error": "embedding computation failed for recall query",
                    }
                    await self._js.publish(
                        SUBJECT_MEMORY_RECALL_RESULT,
                        json.dumps(error_payload).encode(),
                    )
                return

            import psycopg

            scorer = CompositeScorer()

            async with (
                await psycopg.AsyncConnection.connect(self._db_url) as conn,
                conn.cursor() as cur,
            ):
                kind_filter = ""
                params: list[object] = [req.project_id]
                if req.kind:
                    kind_filter = " AND kind = %s"
                    params.append(req.kind)

                base_query = (
                    "SELECT id, content, kind, importance, embedding, created_at"
                    " FROM agent_memories"
                    " WHERE project_id = %s"
                )
                query = base_query + kind_filter + " ORDER BY created_at DESC LIMIT 500"
                await cur.execute(query, params)
                rows = await cur.fetchall()

            scored = []
            for row in rows:
                mem_emb_bytes = row[4]
                if mem_emb_bytes is None:
                    continue
                mem_emb = np.frombuffer(mem_emb_bytes, dtype=np.float32)
                created_at = row[5]
                if created_at.tzinfo is None:
                    created_at = created_at.replace(tzinfo=UTC)
                score = scorer.score(query_emb, mem_emb, created_at, float(row[3]))
                scored.append(
                    {
                        "id": str(row[0]),
                        "content": row[1],
                        "kind": row[2],
                        "score": score,
                    }
                )

            scored.sort(key=lambda x: x["score"], reverse=True)
            top = scored[: req.top_k]

            if self._js is not None:
                result_payload = {
                    "request_id": req.request_id,
                    "project_id": req.project_id,
                    "query": req.query,
                    "results": top,
                }
                await self._js.publish(
                    SUBJECT_MEMORY_RECALL_RESULT,
                    json.dumps(result_payload).encode(),
                )

            log.info("memory recall completed", result_count=len(top))
        except Exception as exc:
            # Log but do not re-raise: memory recall is best-effort, ack to prevent
            # infinite redelivery.
            logger.exception("failed to process memory recall request", error=str(exc))
