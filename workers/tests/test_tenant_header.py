"""The tenant travels as a NATS header and the worker echoes it (KI-64).

The Go Core stamps ``X-Tenant-ID`` on what it publishes and runs the handler
of a message in the tenant of its header. The worker handles each message in
the tenant of its header and adds it to everything it publishes meanwhile, so
a result or event reaches the right tenant even when its payload lacks
``tenant_id``.
"""

from __future__ import annotations

import asyncio
from unittest.mock import AsyncMock, MagicMock, patch

from codeforge.consumer import TaskConsumer
from codeforge.nats_subjects import HEADER_TENANT_ID
from codeforge.tenant_context import bind_tenant, current_tenant, reset_tenant
from codeforge.tracing.propagation import TracingJetStreamContext
from tests.jetstream_fakes import jetstream_msg

TENANT = "aaaaaaaa-0000-4000-8000-000000000001"


def test_header_name_matches_the_go_core() -> None:
    assert HEADER_TENANT_ID == "X-Tenant-ID"


async def _published_headers(tenant: str | None, headers: dict[str, str] | None = None) -> dict[str, str] | None:
    js = TracingJetStreamContext(MagicMock())
    with patch("nats.js.client.JetStreamContext.publish", new=AsyncMock()) as publish:
        token = bind_tenant(tenant) if tenant is not None else None
        try:
            await js.publish("runs.complete", b"{}", headers=headers)
        finally:
            if token is not None:
                reset_tenant(token)
    return publish.await_args.kwargs["headers"]


async def test_publish_adds_the_current_tenant() -> None:
    headers = await _published_headers(TENANT)
    assert headers is not None
    assert headers[HEADER_TENANT_ID] == TENANT


async def test_publish_without_a_tenant_adds_none() -> None:
    headers = await _published_headers(None)
    assert not headers or HEADER_TENANT_ID not in headers


async def test_publish_keeps_an_explicit_tenant_header() -> None:
    headers = await _published_headers(TENANT, {HEADER_TENANT_ID: "bbbbbbbb-0000-4000-8000-000000000002"})
    assert headers is not None
    assert headers[HEADER_TENANT_ID] == "bbbbbbbb-0000-4000-8000-000000000002"


async def test_messages_are_handled_in_the_tenant_of_their_header() -> None:
    consumer = TaskConsumer(nats_url="nats://test:4222", litellm_url="http://test:4000")
    consumer._running = True
    with_tenant, _ = jetstream_msg(b"{}", headers={HEADER_TENANT_ID: TENANT})
    without_tenant, _ = jetstream_msg(b"{}")
    batches: list[list[object]] = [[with_tenant], [without_tenant]]
    seen: list[str] = []
    in_task: list[str] = []

    async def fetch(**_kwargs: object) -> list[object]:
        if batches:
            return batches.pop(0)
        consumer._running = False
        raise TimeoutError

    async def handler(msg: object) -> None:
        seen.append(current_tenant())

        async def spawned() -> None:
            in_task.append(current_tenant())

        await asyncio.create_task(spawned())

    sub = MagicMock()
    sub.fetch = fetch
    await consumer._message_loop(sub, handler, "test.request")

    assert seen == [TENANT, ""]
    assert in_task == [TENANT, ""], "tasks started by the handler keep its tenant"
    assert current_tenant() == "", "the tenant does not leak past the message"
