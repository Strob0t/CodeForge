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
            artifacts: list[str] = payload.get("artifacts", [])
            plan_id = payload.get("plan_id", "")
            step_id = payload.get("step_id", "")
            metadata: dict[str, str] = payload.get("metadata", {})

            log = logger.bind(
                target_agent=target_agent,
                source_run=source_run,
                target_mode=target_mode,
            )

            if self._is_duplicate(f"handoff-{source_run}-{target_agent}"):
                log.warning("duplicate handoff request, skipping")
                await msg.ack()
                return

            log.info("received handoff request")

            handoff_context = f"[Handoff from run {source_run}]\n\n{context_msg}"
            if artifacts:
                handoff_context += f"\n\nArtifacts: {', '.join(artifacts)}"

            # Increment hop counter
            hop = int(metadata.get("handoff_hop", "0")) + 1
            metadata["handoff_hop"] = str(hop)

            # Build config with plan/step fields and prefixed metadata
            config: dict[str, str] = {
                "source_run_id": source_run,
                "handoff_type": "true",
                "plan_id": plan_id,
                "step_id": step_id,
            }
            for k, v in metadata.items():
                config[f"handoff_{k}"] = str(v)

            run_payload: dict[str, object] = {
                "run_id": f"handoff-{source_run}-{target_agent}",
                "task_id": "",
                "project_id": payload.get("project_id", ""),
                "agent_id": target_agent,
                "prompt": handoff_context,
                "policy_profile": "standard",
                "exec_mode": "sandbox",
                "config": config,
                "termination": {
                    "max_steps": 50,
                    "timeout_seconds": 600,
                    "max_cost": 1.0,
                },
            }

            # Stamp trust on outgoing payload
            stamped = self._stamp_trust(run_payload)

            if self._js is not None:
                from codeforge.consumer._subjects import SUBJECT_RUN_START

                await self._js.publish(
                    SUBJECT_RUN_START,
                    json.dumps(stamped).encode(),
                )

            await msg.ack()
            log.info("handoff dispatched to execution")

        except Exception as exc:
            logger.exception("failed to process handoff request", error=str(exc))
            await msg.ack()
