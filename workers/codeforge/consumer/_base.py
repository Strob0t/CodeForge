"""Base mixin with shared helpers used by all handler groups."""

from __future__ import annotations

import json
from collections import OrderedDict
from typing import TYPE_CHECKING, Any, ClassVar

import structlog

from codeforge.consumer._subjects import HEADER_REQUEST_ID, HEADER_RETRY_COUNT, SUBJECT_OUTPUT
from codeforge.trust.middleware import stamp_outgoing

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

    import nats.aio.msg
    from nats.js.client import JetStreamContext
    from pydantic import BaseModel

logger = structlog.get_logger()

_PROCESSED_IDS_MAX = 10_000


class ConsumerBaseMixin:
    """Shared helper methods inherited by the TaskConsumer via mixin pattern."""

    # These attributes are set on the concrete TaskConsumer class.
    _js: JetStreamContext | None
    _litellm_url: str
    _litellm_key: str

    # Shared idempotency guard: bounded FIFO cache that evicts oldest entries first.
    _processed_ids: ClassVar[OrderedDict[str, None]] = OrderedDict()

    @classmethod
    def _is_duplicate(cls, msg_id: str) -> bool:
        """Return True if *msg_id* was already processed (and mark it as processed)."""
        if msg_id in cls._processed_ids:
            # Move to end so it stays fresh.
            cls._processed_ids.move_to_end(msg_id)
            return True
        cls._processed_ids[msg_id] = None
        while len(cls._processed_ids) > _PROCESSED_IDS_MAX:
            cls._processed_ids.popitem(last=False)  # evict oldest
        return False

    @classmethod
    def _clear_processed(cls, msg_id: str) -> None:
        """Remove a message ID so it can be reprocessed (e.g. after a failure)."""
        cls._processed_ids.discard(msg_id)

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
        except Exception as exc:
            logger.exception("failed to publish to DLQ", dlq_subject=dlq_subject, error=str(exc))
        await msg.ack()

    @staticmethod
    def _stamp_trust(payload: dict, source_id: str = "python-worker") -> dict:
        """Add trust annotation to an outgoing NATS payload."""
        return stamp_outgoing(payload, source_id=source_id)

    async def _publish_output(self, task_id: str, line: str, stream: str = "stdout", request_id: str = "") -> None:
        """Publish a streaming output line for a task."""
        if self._js is None:
            return
        payload = json.dumps({"task_id": task_id, "line": line, "stream": stream})
        headers: dict[str, str] = {}
        if request_id:
            headers[HEADER_REQUEST_ID] = request_id
        await self._js.publish(SUBJECT_OUTPUT, payload.encode(), headers=headers or None)

    async def _publish_error(self, result: BaseModel, subject: str) -> None:
        """Publish a pre-built error result model to NATS."""
        try:
            if self._js is not None:
                await self._js.publish(subject, result.model_dump_json().encode())
        except Exception as exc:
            logger.exception("failed to publish error result", subject=subject, error=str(exc))

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
            await self._publish_error(error_result, subject)
        except Exception as exc:
            logger.exception("failed to publish error result", subject=subject, error=str(exc))
        await msg.nak()

    async def _handle_request(
        self,
        msg: nats.aio.msg.Msg,
        request_model: type,
        dedup_key: Callable[[Any], str],
        handler: Callable[[Any, Any], Awaitable[Any | None]],
        result_subject: str | None = None,
        log_context: Callable[[Any], dict[str, Any]] | None = None,
    ) -> None:
        """Generic NATS handler with dedup, processing, and error handling."""
        try:
            request = request_model.model_validate_json(msg.data)
            bind_kwargs = log_context(request) if log_context else {}
            log = logger.bind(**bind_kwargs)

            key = dedup_key(request)
            if self._is_duplicate(key):
                log.warning("duplicate request, skipping", dedup_key=key)
                await msg.ack()
                return

            result = await handler(request, log)

            if result is not None and result_subject and self._js is not None:
                await self._js.publish(result_subject, result.model_dump_json().encode())

            await msg.ack()
            log.info("request processed", dedup_key=key)

        except Exception as exc:
            logger.exception("failed to process request", error=str(exc))
            await msg.nak()
