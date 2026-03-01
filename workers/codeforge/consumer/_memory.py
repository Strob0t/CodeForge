"""Memory store and recall handler mixins."""

from __future__ import annotations

from datetime import UTC
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class MemoryHandlerMixin:
    """Handles memory.store and memory.recall messages."""

    async def _handle_memory_store(self, msg: nats.aio.msg.Msg) -> None:
        """Process a memory store request: compute embedding and persist."""
        try:
            from codeforge.memory.models import MemoryStoreRequest

            req = MemoryStoreRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=req.project_id, kind=req.kind)
            log.info("received memory store request")

            embedding = None
            try:
                emb_resp = await self._llm.embedding(req.content)
                if emb_resp:
                    import numpy as np

                    embedding = np.array(emb_resp, dtype=np.float32).tobytes()
            except Exception:
                log.warning("embedding computation failed for memory", exc_info=True)

            import psycopg

            async with await psycopg.AsyncConnection.connect(self._db_url) as conn, conn.cursor() as cur:
                await cur.execute(
                    """INSERT INTO agent_memories
                           (tenant_id, project_id, agent_id, run_id, content, kind, importance, embedding, metadata)
                           VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s::jsonb)""",
                    (
                        "00000000-0000-0000-0000-000000000000",
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

            await msg.ack()
            log.info("memory stored successfully")

        except Exception:
            logger.exception("failed to process memory store request")
            await msg.ack()

    async def _handle_memory_recall(self, msg: nats.aio.msg.Msg) -> None:
        """Process a memory recall request: score and return top-k memories."""
        try:
            from codeforge.memory.models import MemoryRecallRequest
            from codeforge.memory.scorer import CompositeScorer

            req = MemoryRecallRequest.model_validate_json(msg.data)
            log = logger.bind(project_id=req.project_id, top_k=req.top_k)
            log.info("received memory recall request")

            import numpy as np

            query_emb = None
            try:
                emb_resp = await self._llm.embedding(req.query)
                if emb_resp:
                    query_emb = np.array(emb_resp, dtype=np.float32)
            except Exception:
                log.warning("embedding computation failed for recall query", exc_info=True)

            if query_emb is None:
                await msg.ack()
                return

            import psycopg

            scorer = CompositeScorer()

            async with await psycopg.AsyncConnection.connect(self._db_url) as conn, conn.cursor() as cur:
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
                scored.append({"id": str(row[0]), "content": row[1], "kind": row[2], "score": score})

            scored.sort(key=lambda x: x["score"], reverse=True)
            top = scored[: req.top_k]

            if self._js is not None:
                import json

                result_payload = {
                    "project_id": req.project_id,
                    "query": req.query,
                    "results": top,
                }
                await self._js.publish(
                    "memory.recall.result",
                    json.dumps(result_payload).encode(),
                )

            await msg.ack()
            log.info("memory recall completed", result_count=len(top))

        except Exception:
            logger.exception("failed to process memory recall request")
            await msg.ack()
