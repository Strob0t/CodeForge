"""Base mixin with shared helpers used by all handler groups."""

from __future__ import annotations

import json
from typing import TYPE_CHECKING

import structlog

from codeforge.consumer._subjects import HEADER_REQUEST_ID, HEADER_RETRY_COUNT, SUBJECT_OUTPUT

if TYPE_CHECKING:
    import nats.aio.msg
    from nats.js.client import JetStreamContext

logger = structlog.get_logger()


class ConsumerBaseMixin:
    """Shared helper methods inherited by the TaskConsumer via mixin pattern."""

    # These attributes are set on the concrete TaskConsumer class.
    _js: JetStreamContext | None
    _litellm_url: str
    _litellm_key: str

    @staticmethod
    def _retry_count(msg: nats.aio.msg.Msg) -> int:
        """Extract the Retry-Count header value, defaulting to 0."""
        if msg.headers and HEADER_RETRY_COUNT in msg.headers:
            try:
                return int(msg.headers[HEADER_RETRY_COUNT])
            except (ValueError, TypeError):
                return 0
        return 0

    async def _move_to_dlq(self, msg: nats.aio.msg.Msg) -> None:
        """Publish message to DLQ subject and ack the original."""
        if self._js is None:
            return
        dlq_subject = msg.subject + ".dlq"
        headers = dict(msg.headers) if msg.headers else {}
        try:
            await self._js.publish(dlq_subject, msg.data, headers=headers or None)
            logger.warning("message moved to DLQ", dlq_subject=dlq_subject)
        except Exception:
            logger.exception("failed to publish to DLQ", dlq_subject=dlq_subject)
        await msg.ack()

    async def _publish_output(self, task_id: str, line: str, stream: str = "stdout", request_id: str = "") -> None:
        """Publish a streaming output line for a task."""
        if self._js is None:
            return
        payload = json.dumps({"task_id": task_id, "line": line, "stream": stream})
        headers: dict[str, str] = {}
        if request_id:
            headers[HEADER_REQUEST_ID] = request_id
        await self._js.publish(SUBJECT_OUTPUT, payload.encode(), headers=headers or None)

    async def _publish_error_result(
        self,
        msg: nats.aio.msg.Msg,
        request_model: type,
        result_model: type,
        subject: str,
    ) -> None:
        """Publish an error result so the Go waiter gets an immediate response, then nak."""
        try:
            req = request_model.model_validate_json(msg.data)
            error_result = result_model(
                project_id=req.project_id,
                query=req.query,
                request_id=req.request_id,
                error="internal worker error",
            )
            if self._js is not None:
                await self._js.publish(subject, error_result.model_dump_json().encode())
        except Exception:
            logger.exception("failed to publish error result", subject=subject)
        await msg.nak()
