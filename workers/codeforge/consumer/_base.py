"""Base mixin with shared helpers used by all handler groups."""

from __future__ import annotations

import functools
import json
from collections import OrderedDict
from typing import TYPE_CHECKING, Any, ClassVar, TypeVar

import structlog
from pydantic import ValidationError

from codeforge.consumer._delivery import delivery_attempt, dlq_headers, is_last_attempt
from codeforge.consumer._in_flight import InFlightWork
from codeforge.consumer._subjects import (
    ACCEPT_ATTEMPTS,
    ACK_SYNC_TIMEOUT_SECONDS,
    DLQ_SUFFIX,
    HEADER_REQUEST_ID,
    NAK_DELAY_SECONDS,
    SUBJECT_OUTPUT,
)
from codeforge.nats_publish import publish_with_retry
from codeforge.trust.middleware import stamp_outgoing

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable

    import nats.aio.msg
    from nats.js.client import JetStreamContext
    from pydantic import BaseModel

logger = structlog.get_logger()

RequestT = TypeVar("RequestT", bound="BaseModel")
ResultT = TypeVar("ResultT", bound="BaseModel")

_PROCESSED_IDS_MAX = 10_000


def _echo_tenant[ModelT: BaseModel](request: BaseModel, result: ModelT) -> ModelT:
    """Copy the request's tenant_id into a result that has the field but no value.

    The Go core scopes store writes and WebSocket events to the tenant carried in
    worker results and drops events without one.
    """
    tenant_id = getattr(request, "tenant_id", "")
    if not tenant_id or "tenant_id" not in type(result).model_fields or getattr(result, "tenant_id", ""):
        return result
    return result.model_copy(update={"tenant_id": tenant_id})


class ConsumerBaseMixin:
    """Shared helper methods inherited by the TaskConsumer via mixin pattern."""

    # These attributes are set on the concrete TaskConsumer class.
    _js: JetStreamContext | None
    _litellm_url: str
    _litellm_key: str

    # Shared idempotency guard: bounded FIFO cache that evicts oldest entries first.
    # FIX-048/FIX-054: Concurrency safety note — Python asyncio uses a single-threaded
    # event loop, so coroutines never execute simultaneously. The OrderedDict and its
    # mutations (_is_duplicate, _clear_processed) are safe without locks because
    # control only yields at `await` points, never mid-method. If this code is ever
    # used with threading or multiprocessing, a lock must be added.
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
        cls._processed_ids.pop(msg_id, None)

    async def _move_to_dlq(self, msg: nats.aio.msg.Msg, *, terminate: bool = False) -> None:
        """Copy *msg* to ``{subject}.dlq``, then settle it: term if *terminate*, else ack.

        If no copy was stored (publish error, or a duplicate PubAck) the message
        is NAK'd instead, so it is never acknowledged without a dead-letter copy
        (after the last attempt JetStream keeps it unacknowledged instead of
        redelivering it).
        """
        if self._js is None:
            return
        dlq_subject = msg.subject + DLQ_SUFFIX
        try:
            ack = await self._js.publish(dlq_subject, msg.data, headers=dlq_headers(msg.headers))
            # PubAck.duplicate is None unless the stream discarded the message.
            if ack.duplicate is True:
                reason = "dead-letter copy discarded as a duplicate"
                raise RuntimeError(reason)
        except Exception as exc:
            logger.exception("failed to publish to DLQ, keeping the message", dlq_subject=dlq_subject, error=str(exc))
            await msg.nak(delay=NAK_DELAY_SECONDS)
            return
        logger.warning("message moved to DLQ", dlq_subject=dlq_subject, attempt=delivery_attempt(msg))
        if terminate:
            await msg.term()
        else:
            await msg.ack()

    @functools.cached_property
    def _in_flight(self) -> InFlightWork:
        """Accepted at-most-once work and background tasks, failed and cancelled if the worker must stop."""
        return InFlightWork()

    @staticmethod
    async def _accept(msg: nats.aio.msg.Msg) -> bool:
        """Ack an at-most-once message before its work starts; False if the ack was not confirmed.

        A plain ack is fire-and-forget: if it were lost, JetStream would hand the
        running work to a second worker after the ack wait. A confirmed (double)
        ack rules that out. An unanswered double ack is repeated: the server
        confirms an ack it already applied. If none is confirmed, the work is
        not started and the message is NAK'd, so a message whose ack never
        arrived goes to the next worker at once (the server ignores the NAK of
        a message whose ack did arrive; that run is ended by the Go Core,
        ADR-016 section 6).
        """
        for attempt in range(1, ACCEPT_ATTEMPTS + 1):
            try:
                await msg.ack_sync(timeout=ACK_SYNC_TIMEOUT_SECONDS)
            except Exception as exc:
                logger.warning("ack on accept not confirmed", subject=msg.subject, attempt=attempt, error=str(exc))
            else:
                return True
        logger.error("ack on accept not confirmed, releasing the message", subject=msg.subject)
        try:
            await msg.nak()
        except Exception as exc:
            logger.warning("releasing the unaccepted message failed", subject=msg.subject, error=str(exc))
        return False

    async def _reject_invalid(self, msg: nats.aio.msg.Msg, error: str) -> None:
        """Dead-letter a payload that can never be processed and stop its redelivery."""
        logger.error("invalid message payload", subject=msg.subject, error=error)
        await self._move_to_dlq(msg, terminate=True)

    async def _retry_or_dead_letter(self, msg: nats.aio.msg.Msg) -> None:
        """Settle a failed message: retry it later, or dead-letter it on the last attempt."""
        if is_last_attempt(msg):
            await self._move_to_dlq(msg)
        else:
            await msg.nak(delay=NAK_DELAY_SECONDS)

    async def _parse_json_object(self, msg: nats.aio.msg.Msg) -> dict[str, object] | None:
        """Decode a JSON object payload; dead-letter the message and return None if it is not one."""
        try:
            payload = json.loads(msg.data)
        except (json.JSONDecodeError, UnicodeDecodeError) as exc:
            await self._reject_invalid(msg, str(exc))
            return None
        if not isinstance(payload, dict):
            await self._reject_invalid(msg, f"expected a JSON object, got {type(payload).__name__}")
            return None
        return payload

    @staticmethod
    def _stamp_trust(payload: dict, source_id: str = "python-worker") -> dict:
        """Add trust annotation to an outgoing NATS payload."""
        return stamp_outgoing(payload, source_id=source_id)

    async def _publish_output(
        self,
        task_id: str,
        line: str,
        stream: str = "stdout",
        request_id: str = "",
        tenant_id: str = "",
    ) -> None:
        """Publish a streaming output line for a task, tagged with the task's tenant."""
        if self._js is None:
            return
        payload = json.dumps({"task_id": task_id, "tenant_id": tenant_id, "line": line, "stream": stream})
        headers: dict[str, str] = {}
        if request_id:
            headers[HEADER_REQUEST_ID] = request_id
        await self._js.publish(SUBJECT_OUTPUT, payload.encode(), headers=headers or None)

    async def _publish_result(self, result: BaseModel, subject: str) -> None:
        """Publish the result or completion of accepted work, retrying transient failures.

        Accepted work is never redelivered, so this is the only way the Go Core
        learns its outcome; a result that could not be published is logged.
        """
        if self._js is None:
            logger.error("JetStream not available, result not published", subject=subject)
            return
        try:
            await publish_with_retry(self._js, subject, result.model_dump_json().encode())
        except Exception as exc:
            logger.exception("failed to publish result", subject=subject, error=str(exc))

    async def _parse_request(self, msg: nats.aio.msg.Msg, request_model: type[RequestT]) -> RequestT | None:
        """Validate the payload; dead-letter the message and return None if it is invalid."""
        try:
            return request_model.model_validate_json(msg.data)
        except ValidationError as exc:
            await self._reject_invalid(msg, str(exc))
            return None

    async def _handle_request(
        self,
        msg: nats.aio.msg.Msg,
        request_model: type[RequestT],
        dedup_key: Callable[[RequestT], str],
        handler: Callable[[RequestT, structlog.BoundLogger], Awaitable[ResultT | None]],
        result_subject: str | None = None,
        log_context: Callable[[RequestT], dict[str, Any]] | None = None,
        *,
        ack_on_accept: bool = False,
    ) -> None:
        """Generic NATS handler with validation, dedup, processing, and delivery settlement.

        Default (at-least-once): the message is acked after the handler
        succeeded; a failure is retried until the last JetStream delivery and
        then dead-lettered. With *ack_on_accept* (at-most-once, for runs that
        are not safe to execute twice) the message is acked before the handler
        runs and a failure is not retried; the Go Core owns the run's outcome.
        """
        request = await self._parse_request(msg, request_model)
        if request is None:
            return
        log = logger.bind(**(log_context(request) if log_context else {}))

        key = dedup_key(request)
        if self._is_duplicate(key):
            log.warning("duplicate request, skipping", dedup_key=key)
            await msg.ack()
            return

        if ack_on_accept and not await self._accept(msg):
            self._clear_processed(key)
            return

        try:
            result = await handler(request, log)
            if result is not None and result_subject and self._js is not None:
                result = _echo_tenant(request, result)
                await self._js.publish(result_subject, result.model_dump_json().encode())
        except Exception as exc:
            log.exception("failed to process request", error=str(exc), attempt=delivery_attempt(msg))
            if not ack_on_accept:
                # Not processed: the redelivery must not be skipped as a duplicate.
                self._clear_processed(key)
                await self._retry_or_dead_letter(msg)
            return

        if not ack_on_accept:
            await msg.ack()
        log.info("request processed", dedup_key=key)
