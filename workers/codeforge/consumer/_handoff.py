"""Handoff request handler mixin."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class HandoffHandlerMixin:
    """Handles handoff.request messages."""

    async def _handle_handoff_request(self, msg: nats.aio.msg.Msg) -> None:
        """Process a handoff request: create a new agent run with injected context."""
        try:
            payload = json.loads(msg.data)
            target_agent = payload.get("target_agent_id", "")
            context_msg = payload.get("context", "")
            target_mode = payload.get("target_mode_id", "")
            source_run = payload.get("source_run_id", "")
            artifacts = payload.get("artifacts", [])

            log = logger.bind(
                target_agent=target_agent,
                source_run=source_run,
                target_mode=target_mode,
            )
            log.info("received handoff request")

            handoff_context = f"[Handoff from run {source_run}]\n\n{context_msg}"
            if artifacts:
                handoff_context += f"\n\nArtifacts: {', '.join(artifacts)}"

            run_payload = {
                "type": "handoff",
                "source_run_id": source_run,
                "target_agent_id": target_agent,
                "target_mode_id": target_mode,
                "context": handoff_context,
                "artifacts": artifacts,
            }

            if self._js is not None:
                await self._js.publish(
                    "handoff.execute",
                    json.dumps(run_payload).encode(),
                )

            await msg.ack()
            log.info("handoff dispatched to execution")

        except Exception:
            logger.exception("failed to process handoff request")
            await msg.ack()
