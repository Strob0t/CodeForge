"""Backend health handler mixin."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import SUBJECT_BACKEND_HEALTH_RESULT

if TYPE_CHECKING:
    import nats.aio.msg

logger = structlog.get_logger()


class BackendHealthHandlerMixin:
    """Handles backends.health.request messages — checks all backend availability."""

    async def _handle_backend_health(self, msg: nats.aio.msg.Msg) -> None:
        """Check health of all registered backends and publish result."""
        try:
            payload = json.loads(msg.data) if msg.data else {}
            request_id = payload.get("request_id", "")

            log = logger.bind(request_id=request_id)
            log.info("backend health check requested")

            backends = await self._backend_router.check_all_health()
            config_schemas = self._backend_router.get_config_schemas()

            # Merge config schemas into health results
            schema_map = {s["name"]: s.get("config_fields", []) for s in config_schemas}
            for b in backends:
                b["config_fields"] = schema_map.get(b["name"], [])

            result = {
                "request_id": request_id,
                "backends": backends,
            }

            if self._js is not None:
                await self._js.publish(
                    SUBJECT_BACKEND_HEALTH_RESULT,
                    json.dumps(result).encode(),
                )

            await msg.ack()
            log.info("backend health check completed", count=len(backends))

        except Exception as exc:
            logger.exception("failed to process backend health request", error=str(exc))
            await msg.nak()
