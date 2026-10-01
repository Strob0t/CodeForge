"""The worker's NATS user may do what the worker does, and no more (KI-71).

Starts a nats-server with configs/nats/nats-server.conf (the configuration of
docker-compose.prod.yml) and runs the worker's real NATS operations as the
"worker" user: its durable consumers, notification consumers, results,
dead-letter copies and acks. Subjects only the Go Core sends are refused, and
so are the Go Core's consumers.
Needs a nats-server binary (NATS_SERVER_BIN or nats-server on PATH); skipped
otherwise.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import os
import shutil
import socket
import subprocess
import time
from pathlib import Path
from typing import TYPE_CHECKING

import nats
import pytest
from nats.js.api import AckPolicy, ConsumerConfig, DeliverPolicy, StreamConfig

from codeforge.consumer import TaskConsumer
from codeforge.consumer._cancel_registry import CancelRegistry, record_cancels, run_key
from codeforge.consumer._delivery import ensure_durable
from codeforge.nats_subjects import (
    INBOX_PREFIX,
    STREAM_NAME,
    STREAM_SUBJECTS,
    SUBJECT_RUN_CANCEL,
    SUBJECT_RUN_START,
    SUBJECT_TOOLCALL_RESPONSE,
    consumer_name,
)
from codeforge.notifications import NotificationHub, notification_consumer_name
from codeforge.runtime import notification_consumer

if TYPE_CHECKING:
    from collections.abc import Callable, Iterator

CONFIG = Path(__file__).resolve().parents[2] / "configs" / "nats" / "nats-server.conf"
# The Go Core's inbox prefix (internal/adapter/nats/nats.go).
CORE_INBOX_PREFIX = "_INBOX_core"
NATS_SERVER = os.environ.get("NATS_SERVER_BIN") or shutil.which("nats-server")

pytestmark = pytest.mark.skipif(NATS_SERVER is None, reason="needs a nats-server binary (NATS_SERVER_BIN)")

# What the worker publishes (worker -> Go Core) and dead-letter copies of what it consumes.
WORKER_PUBLISHES = [
    "tasks.result",
    "tasks.output",
    "tasks.heartbeat",
    "agents.output",
    "runs.complete",
    "runs.output",
    "runs.heartbeat",
    "runs.toolcall.request",
    "runs.toolcall.result",
    "runs.trajectory.event",
    "runs.qualitygate.result",
    "repomap.generate.result",
    "retrieval.index.result",
    "retrieval.search.result",
    "retrieval.subagent.result",
    "graph.build.result",
    "graph.search.result",
    "context.rerank.result",
    "conversation.run.complete",
    "conversation.compact.complete",
    "conversation.test.result",
    "benchmark.run.result",
    "benchmark.task.started",
    "benchmark.task.progress",
    "evaluation.gemmas.result",
    "memory.recall.result",
    "a2a.task.complete",
    "handoff.request",
    "backends.health.result",
    "prompt.evolution.reflect.complete",
    "prompt.evolution.mutate.complete",
    "tasks.agent.aider.dlq",
    "runs.start.dlq",
    "conversation.run.start.dlq",
    "context.shared.updated.dlq",
]
# What only the Go Core sends: a worker that could publish these could start
# runs, cancel them or answer its own tool-call requests.
CORE_ONLY = [
    "runs.start",
    "runs.cancel",
    "runs.toolcall.response",
    "runs.qualitygate.request",
    "conversation.run.start",
    "conversation.run.cancel",
    "conversation.test.request",
    "tasks.agent.aider",
    "tasks.cancel",
    "memory.store",
    "a2a.task.created",
    "prompt.evolution.promoted",
    "context.shared.updated",
]


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return int(s.getsockname()[1])


@pytest.fixture(scope="module")
def server(tmp_path_factory: pytest.TempPathFactory) -> Iterator[int]:
    work = tmp_path_factory.mktemp("nats")
    shutil.copy(CONFIG, work / "nats-server.conf")
    (work / "passwords.conf").write_text('CORE_PASSWORD: "core-pw"\nWORKER_PASSWORD: "worker-pw"\n')
    port = _free_port()
    assert NATS_SERVER is not None
    proc = subprocess.Popen(  # noqa: S603 - the test's own server
        [
            NATS_SERVER,
            "-c",
            str(work / "nats-server.conf"),
            "-a",
            "127.0.0.1",
            "-p",
            str(port),
            "-js",
            "-sd",
            str(work),
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        with contextlib.suppress(OSError), socket.create_connection(("127.0.0.1", port), timeout=0.2):
            break
        time.sleep(0.05)
    yield port
    proc.terminate()
    proc.wait(timeout=10)


async def _connect(port: int, user: str = "", password: str = "") -> nats.NATS:
    errors: list[Exception] = []

    async def on_error(exc: Exception) -> None:
        errors.append(exc)

    creds = f"{user}:{password}@" if user else ""
    # Each service uses inboxes of its own, as the Go Core and the worker do.
    inbox_prefix = CORE_INBOX_PREFIX if user == "core" else INBOX_PREFIX
    nc = await nats.connect(
        f"nats://{creds}127.0.0.1:{port}",
        error_cb=on_error,
        allow_reconnect=False,
        max_reconnect_attempts=0,
        inbox_prefix=inbox_prefix,
    )
    nc._test_errors = errors  # type: ignore[attr-defined]
    return nc


async def _call_ids(sub: object, until: str) -> set[str]:
    """The call IDs *sub* receives up to *until* (a replayed message may arrive twice)."""
    seen: set[str] = set()
    while until not in seen:
        msg = await sub.next_msg(timeout=5)  # type: ignore[attr-defined]
        seen.add(json.loads(msg.data)["call_id"])
    return seen


async def _eventually(check: Callable[[], bool], timeout: float = 5.0) -> None:
    deadline = time.monotonic() + timeout
    while not check():
        assert time.monotonic() < deadline, "condition not met in time"
        await asyncio.sleep(0.02)


async def _core_with_stream(port: int) -> nats.NATS:
    core = await _connect(port, "core", "core-pw")
    js = core.jetstream()
    with contextlib.suppress(Exception):
        await js.add_stream(StreamConfig(name=STREAM_NAME, subjects=STREAM_SUBJECTS))
    return core


@pytest.mark.parametrize(("user", "password"), [("", ""), ("worker", "wrong"), ("nobody", "worker-pw")])
async def test_unauthenticated_connections_are_refused(server: int, user: str, password: str) -> None:
    with pytest.raises(Exception, match=r"(?i)authorization"):
        await _connect(server, user, password)


async def test_the_worker_runs_its_nats_operations(server: int) -> None:
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        js = worker.jetstream()
        # Every durable the worker consumes, created the way the worker does.
        consumer = TaskConsumer(nats_url="", litellm_url="http://127.0.0.1:9", litellm_key="k")
        loops = await consumer._subscribe_all(js)
        assert len(loops) > 20

        # A message the Go Core dispatches reaches the worker's durable; it acks it.
        await core.jetstream().publish(SUBJECT_RUN_START, b"{}")
        sub = await ensure_durable(js, consumer_name(SUBJECT_RUN_START), SUBJECT_RUN_START)
        (msg,) = await sub.fetch(1, timeout=5)
        await msg.in_progress()
        await msg.ack_sync()

        # Notifications (tool-call decisions, cancels), from now and from a sequence.
        hub = NotificationHub(worker, js)
        await hub.start()
        info = await core.jetstream().publish(SUBJECT_TOOLCALL_RESPONSE, b'{"call_id": "old"}')
        responses = await hub.subscribe(SUBJECT_TOOLCALL_RESPONSE, config=notification_consumer(after=info.seq - 1))
        await core.jetstream().publish(SUBJECT_TOOLCALL_RESPONSE, b'{"call_id": "new"}')
        assert await _call_ids(responses, until="new") >= {"old", "new"}
        await responses.unsubscribe()
        # Deliveries carry their stream sequence (the cancel registry orders cancels by it).
        registry = CancelRegistry()
        cancels = await hub.subscribe(SUBJECT_RUN_CANCEL)
        recording = asyncio.create_task(record_cancels(cancels, registry, lambda run_id, _task_id: run_key(run_id)))
        cancel = await core.jetstream().publish(SUBJECT_RUN_CANCEL, b'{"run_id": "r1"}')
        await _eventually(lambda: registry.cancelled(run_key("r1"), cancel.seq - 1))
        recording.cancel()

        for subject in WORKER_PUBLISHES:
            ack = await js.publish(subject, b"{}")
            assert ack.stream == STREAM_NAME, subject
        assert worker._test_errors == []  # type: ignore[attr-defined]
    finally:
        await worker.close()
        await core.close()


@pytest.mark.parametrize("subject", CORE_ONLY)
async def test_the_worker_cannot_publish_core_subjects(server: int, subject: str) -> None:
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        received = await core.jetstream().subscribe(subject, config=notification_consumer())
        with pytest.raises(Exception):  # noqa: B017 - no PubAck: the server dropped the message
            await worker.jetstream().publish(subject, b"{}", timeout=1)
        await asyncio.sleep(0.2)
        assert any("permissions violation" in str(e).lower() for e in worker._test_errors)  # type: ignore[attr-defined]
        with pytest.raises(nats.errors.TimeoutError):
            await received.next_msg(timeout=0.5)
    finally:
        await worker.close()
        await core.close()


async def test_the_worker_cannot_change_the_stream_or_read_kv(server: int) -> None:
    core = await _core_with_stream(server)
    with contextlib.suppress(Exception):
        await core.jetstream().create_key_value(bucket="IDEMPOTENCY")
    worker = await _connect(server, "worker", "worker-pw")
    try:
        js = worker.jetstream(timeout=1.0)
        with pytest.raises(Exception):  # noqa: B017 - permission violation, no API response
            await js.update_stream(StreamConfig(name=STREAM_NAME, subjects=[*STREAM_SUBJECTS, "x.>"]), timeout=1)
        with pytest.raises(Exception):  # noqa: B017
            await js.purge_stream(STREAM_NAME)
        with pytest.raises(Exception):  # noqa: B017
            await js.key_value("IDEMPOTENCY")
    finally:
        await worker.close()
        await core.close()


# ---------------------------------------------------------------------------
# Deliveries the worker could point elsewhere
# ---------------------------------------------------------------------------


async def _no_message(sub: object, timeout: float = 0.5) -> bool:
    try:
        await sub.next_msg(timeout=timeout)  # type: ignore[attr-defined]
    except nats.errors.TimeoutError:
        return True
    return False


async def test_the_worker_cannot_deliver_onto_core_subjects(server: int) -> None:
    """A consumer's deliveries never reach a subject only the Go Core may publish.

    nats-server does not check a push consumer's deliver subject, nor a pull
    request's reply subject, against the creator's publish rights; a delivery
    onto a stream subject is refused as a cycle, and nothing is stored.
    """
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        js = worker.jetstream(timeout=2.0)
        with pytest.raises(nats.js.errors.BadRequestError, match="cycle"):
            await js.add_consumer(
                STREAM_NAME,
                ConsumerConfig(
                    name=notification_consumer_name("runs.cancel"),
                    deliver_subject="runs.cancel",
                    filter_subject="runs.complete",
                    ack_policy=AckPolicy.NONE,
                ),
            )

        watch = await core.jetstream().subscribe("runs.toolcall.response", config=notification_consumer())
        await js.publish("runs.complete", b'{"call_id": "c1", "decision": "allow"}')
        sub = await ensure_durable(js, consumer_name(SUBJECT_RUN_START), SUBJECT_RUN_START)
        await sub.unsubscribe()
        await core.jetstream().publish(SUBJECT_RUN_START, b'{"run_id": "r1"}')
        name = consumer_name(SUBJECT_RUN_START)
        await worker.publish(
            f"$JS.API.CONSUMER.MSG.NEXT.{STREAM_NAME}.{name}", b'{"batch": 1}', reply="runs.toolcall.response"
        )
        await worker.flush()
        assert await _no_message(watch, 1.0), "a pull reply reached a Go Core subject"
    finally:
        await worker.close()
        await core.close()


async def test_the_worker_cannot_see_core_inboxes(server: int) -> None:
    """Without the Go Core's inbox names the worker cannot point deliveries at them."""
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        for subject in ("_INBOX_core.>", "_INBOX.>", ">"):
            sub = await worker.subscribe(subject)
            await core.publish(subject.replace(">", "x"), b"secret reply")
            await core.flush()
            assert await _no_message(sub), subject
        await asyncio.sleep(0.2)
        assert any("permissions violation" in str(e).lower() for e in worker._test_errors)  # type: ignore[attr-defined]
    finally:
        await worker.close()
        await core.close()


@pytest.mark.parametrize(
    "api",
    [
        f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.codeforge-go-runs-complete.runs.complete",
        f"$JS.API.CONSUMER.DURABLE.CREATE.{STREAM_NAME}.codeforge-go-runs-complete",
        f"$JS.API.CONSUMER.DELETE.{STREAM_NAME}.codeforge-go-runs-complete",
        f"$JS.API.CONSUMER.INFO.{STREAM_NAME}.codeforge-go-runs-complete",
        f"$JS.API.CONSUMER.MSG.NEXT.{STREAM_NAME}.codeforge-go-runs-complete",
        f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.codeforge-py-unknown.runs.cancel",
        f"$JS.ACK.{STREAM_NAME}.codeforge-go-runs-complete.1.1.1.1.0",
        # Without a name in the subject the request could name any consumer.
        f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}",
        f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.codeforge-py-notify-runs-cancel",
    ],
)
async def test_the_worker_cannot_use_other_consumers(server: int, api: str) -> None:
    """The worker may create, read, fetch from and ack only its own durables."""
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        with contextlib.suppress(nats.errors.TimeoutError, nats.errors.NoRespondersError):
            await worker.request(api, b"{}", timeout=0.5)
        await asyncio.sleep(0.2)
        assert any(
            "permissions violation" in str(e).lower() and api.lower() in str(e).lower()
            for e in worker._test_errors  # type: ignore[attr-defined]
        ), worker._test_errors  # type: ignore[attr-defined]
    finally:
        await worker.close()
        await core.close()


# ---------------------------------------------------------------------------
# The Go Core's consumers
# ---------------------------------------------------------------------------

HANDOFF_APPROVED = "handoff.approved"
CORE_DURABLE = "codeforge-go-handoff-approved"
NOTIFY_RUNS_CANCEL = "codeforge-py-notify-runs-cancel"


async def _core_durable(core: nats.NATS) -> None:
    await core.jetstream().add_consumer(
        STREAM_NAME,
        ConsumerConfig(
            name=CORE_DURABLE,
            durable_name=CORE_DURABLE,
            filter_subject=HANDOFF_APPROVED,
            ack_policy=AckPolicy.EXPLICIT,
            deliver_policy=DeliverPolicy.NEW,
        ),
    )


def _create_request(**config: str) -> bytes:
    body = {"filter_subject": "tasks.output", "ack_policy": "explicit", "deliver_policy": "new", **config}
    return json.dumps({"stream_name": STREAM_NAME, "config": body}).encode()


@pytest.mark.parametrize(
    ("api", "config"),
    [
        # The review's proof of concept: the API subject without a consumer name.
        (f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}", {"name": CORE_DURABLE}),
        (f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}", {"durable_name": CORE_DURABLE}),
        # The worker's own consumer name in the subject, the Go Core's in the request.
        (f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.{NOTIFY_RUNS_CANCEL}.tasks.output", {"name": CORE_DURABLE}),
        (
            f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.{NOTIFY_RUNS_CANCEL}.tasks.output",
            {"name": NOTIFY_RUNS_CANCEL, "durable_name": CORE_DURABLE},
        ),
        (f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.codeforge-py-runs-start.tasks.output", {"durable_name": CORE_DURABLE}),
    ],
)
async def test_the_worker_cannot_reconfigure_core_consumers(server: int, api: str, config: dict[str, str]) -> None:
    """A Go Core durable keeps its filter, so a worker message never reaches its handler (KI-71 review)."""
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        await _core_durable(core)
        with contextlib.suppress(nats.errors.TimeoutError, nats.errors.NoRespondersError):
            await worker.request(api, _create_request(**config), timeout=1)
        info = await core.jetstream().consumer_info(STREAM_NAME, CORE_DURABLE)
        assert info.config.filter_subject == HANDOFF_APPROVED

        await worker.jetstream().publish("tasks.output", b'{"forged": "handoff"}')
        durable = await core.jetstream().pull_subscribe(HANDOFF_APPROVED, durable=CORE_DURABLE, stream=STREAM_NAME)
        with pytest.raises(nats.errors.TimeoutError):
            await durable.fetch(1, timeout=0.5)
    finally:
        # A notification consumer the request reconfigured is the worker's own
        # (the next hub start recreates it); remove it for the other tests.
        with contextlib.suppress(Exception):
            await core.jetstream().delete_consumer(STREAM_NAME, NOTIFY_RUNS_CANCEL)
        await worker.close()
        await core.close()


async def test_a_changed_notification_consumer_is_recreated(server: int) -> None:
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        await worker.request(
            f"$JS.API.CONSUMER.CREATE.{STREAM_NAME}.{NOTIFY_RUNS_CANCEL}.tasks.output",
            _create_request(name=NOTIFY_RUNS_CANCEL),
            timeout=2,
        )
        hub = NotificationHub(worker, worker.jetstream())
        await hub.start()
        info = await core.jetstream().consumer_info(STREAM_NAME, notification_consumer_name(SUBJECT_RUN_CANCEL))
        assert info.config.filter_subject == SUBJECT_RUN_CANCEL
        sub = await hub.subscribe(SUBJECT_RUN_CANCEL)
        await core.jetstream().publish(SUBJECT_RUN_CANCEL, b'{"run_id": "after-recreate"}')
        assert json.loads((await sub.next_msg(timeout=5)).data)["run_id"] == "after-recreate"
    finally:
        await worker.close()
        await core.close()


async def test_a_lost_notification_consumer_is_restored(server: int) -> None:
    """The worker recreates its notification consumers on a reconnect (codeforge.consumer)."""
    core = await _core_with_stream(server)
    worker = await _connect(server, "worker", "worker-pw")
    try:
        hub = NotificationHub(worker, worker.jetstream())
        await hub.start()
        sub = await hub.subscribe(SUBJECT_RUN_CANCEL)
        await core.jetstream().delete_consumer(STREAM_NAME, NOTIFY_RUNS_CANCEL)

        await hub.restore()
        await core.jetstream().publish(SUBJECT_RUN_CANCEL, b'{"run_id": "restored"}')
        assert json.loads((await sub.next_msg(timeout=5)).data)["run_id"] == "restored"
        assert worker._test_errors == []  # type: ignore[attr-defined]
    finally:
        await worker.close()
        await core.close()


async def test_every_worker_instance_sees_every_notification(server: int) -> None:
    """Two workers share the notification consumers; each sees every cancel (any may run the work)."""
    core = await _core_with_stream(server)
    first = await _connect(server, "worker", "worker-pw")
    second = await _connect(server, "worker", "worker-pw")
    try:
        hubs = [NotificationHub(first, first.jetstream()), NotificationHub(second, second.jetstream())]
        for hub in hubs:
            await hub.start()
        subs = [await hub.subscribe(SUBJECT_RUN_CANCEL) for hub in hubs]
        await core.jetstream().publish(SUBJECT_RUN_CANCEL, b'{"run_id": "both"}')
        for sub in subs:
            assert json.loads((await sub.next_msg(timeout=5)).data)["run_id"] == "both"
        assert first._test_errors == []  # type: ignore[attr-defined]
        assert second._test_errors == []  # type: ignore[attr-defined]
    finally:
        await first.close()
        await second.close()
        await core.close()
