"""Tests for the worker health endpoints (KI-34).

GET /health is liveness (the process serves requests); GET /health/ready is
readiness: connected to NATS, every consumer loop running, not given up.
"""

from __future__ import annotations

import asyncio
import json
import os
import socket
import subprocess
import sys
import urllib.error
import urllib.request
from http import HTTPStatus
from pathlib import Path
from typing import TYPE_CHECKING, ClassVar
from unittest.mock import MagicMock

import pytest

from codeforge.health import HealthHandler, start_health_server

if TYPE_CHECKING:
    from collections.abc import Callable, Iterator

    from codeforge.health import HealthServer

HEALTHCHECK_SCRIPT = Path(__file__).resolve().parents[2] / "scripts" / "worker-healthcheck.py"


def _make_handler(path: str) -> HealthHandler:
    """Create a HealthHandler with mocked internals for testing."""
    handler = HealthHandler.__new__(HealthHandler)
    handler.path = path
    handler.wfile = MagicMock()
    handler.send_response = MagicMock()
    handler.send_header = MagicMock()
    handler.end_headers = MagicMock()
    return handler


def test_health_endpoint_returns_ok() -> None:
    handler = _make_handler("/health")
    handler.do_GET()

    handler.send_response.assert_called_once_with(HTTPStatus.OK)
    handler.wfile.write.assert_called_once_with(json.dumps({"status": "ok"}).encode())


def test_unknown_path_returns_404() -> None:
    handler = _make_handler("/unknown")
    handler.do_GET()

    handler.send_response.assert_called_once_with(HTTPStatus.NOT_FOUND)


def test_consumer_initializes() -> None:
    from codeforge.consumer import TaskConsumer

    consumer = TaskConsumer(nats_url="nats://test:4222")
    assert consumer.nats_url == "nats://test:4222"
    assert consumer._running is False


# ---------------------------------------------------------------------------
# HTTP server
# ---------------------------------------------------------------------------


def _free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _get(port: int, path: str) -> tuple[int, dict[str, str] | None]:
    try:
        with urllib.request.urlopen(f"http://127.0.0.1:{port}{path}", timeout=2) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as err:
        body = err.read()
        return err.code, json.loads(body) if body else None


@pytest.fixture
def serve() -> Iterator[Callable[[Callable[[], bool]], int]]:
    """Start a health server on an ephemeral port with the given readiness check; return its port."""
    servers: list[HealthServer] = []

    def start(is_ready: Callable[[], bool]) -> int:
        server = start_health_server(0, is_ready)
        servers.append(server)
        return int(server.server_address[1])

    yield start
    for server in servers:
        server.shutdown()
        server.server_close()


def test_ready_endpoint_reflects_readiness(serve: Callable[[Callable[[], bool]], int]) -> None:
    ready = {"value": False}
    port = serve(lambda: ready["value"])

    assert _get(port, "/health/ready") == (HTTPStatus.SERVICE_UNAVAILABLE, {"status": "not ready"})
    ready["value"] = True
    assert _get(port, "/health/ready") == (HTTPStatus.OK, {"status": "ready"})
    ready["value"] = False
    assert _get(port, "/health/ready")[0] == HTTPStatus.SERVICE_UNAVAILABLE


def test_liveness_does_not_depend_on_readiness(serve: Callable[[Callable[[], bool]], int]) -> None:
    port = serve(lambda: False)
    assert _get(port, "/health") == (HTTPStatus.OK, {"status": "ok"})


@pytest.mark.parametrize(
    ("path", "status"),
    [
        ("/health?probe=docker", HTTPStatus.OK),
        ("/health/ready?probe=docker", HTTPStatus.OK),
        ("/health/", HTTPStatus.NOT_FOUND),
        ("/healthz", HTTPStatus.NOT_FOUND),
        ("/", HTTPStatus.NOT_FOUND),
    ],
)
def test_paths(serve: Callable[[Callable[[], bool]], int], path: str, status: HTTPStatus) -> None:
    port = serve(lambda: True)
    assert _get(port, path)[0] == status


def test_a_failing_readiness_check_is_not_ready(serve: Callable[[Callable[[], bool]], int]) -> None:
    def broken() -> bool:
        raise RuntimeError("state unavailable")

    port = serve(broken)
    assert _get(port, "/health/ready") == (HTTPStatus.SERVICE_UNAVAILABLE, {"status": "not ready"})
    assert _get(port, "/health")[0] == HTTPStatus.OK, "the server keeps serving"


def test_port_in_use_raises() -> None:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as taken:
        taken.bind(("", 0))
        taken.listen()
        with pytest.raises(OSError):
            start_health_server(int(taken.getsockname()[1]), lambda: True)


# ---------------------------------------------------------------------------
# Consumer readiness
# ---------------------------------------------------------------------------


def _running_consumer(*, connected: bool = True, loops_done: tuple[bool, ...] = (False, False)) -> object:
    from codeforge.consumer import TaskConsumer

    consumer = TaskConsumer(nats_url="nats://test:4222")
    consumer._running = True
    consumer._nc = MagicMock(is_connected=connected)
    consumer._loop_tasks = [MagicMock(done=MagicMock(return_value=done)) for done in loops_done]
    return consumer


def test_consumer_readiness() -> None:
    from codeforge.consumer import TaskConsumer

    assert TaskConsumer(nats_url="nats://test:4222").ready is False, "not started"
    assert _running_consumer().ready is True

    assert _running_consumer(connected=False).ready is False, "NATS disconnected (reconnecting)"
    assert _running_consumer(loops_done=()).ready is False, "no consumer loop started yet"
    assert _running_consumer(loops_done=(False, True)).ready is False, "a consumer loop ended"

    stopping = _running_consumer()
    stopping._running = False
    assert stopping.ready is False, "stopping"

    given_up = _running_consumer()
    given_up.failed = True
    assert given_up.ready is False, "a consumer loop gave up"


# ---------------------------------------------------------------------------
# main(): the entry point serves the endpoints while the consumer runs
# ---------------------------------------------------------------------------


class _FakeConsumer:
    """Stands in for TaskConsumer in main(): start() runs until released."""

    instances: ClassVar[list[_FakeConsumer]] = []

    def __init__(self, **_kwargs: object) -> None:
        self.release = asyncio.Event()
        self.started = asyncio.Event()
        self.ready = False
        self.failed = False
        self.stopped = False
        _FakeConsumer.instances.append(self)

    async def start(self) -> None:
        self.started.set()
        await self.release.wait()

    async def stop(self) -> None:
        self.stopped = True
        self.release.set()


@pytest.fixture
def fake_main(monkeypatch: pytest.MonkeyPatch) -> Iterator[int]:
    """Patch main()'s consumer and logging; return the health port main() will use."""
    import codeforge.consumer as consumer_module

    _FakeConsumer.instances = []
    monkeypatch.setattr(consumer_module, "TaskConsumer", _FakeConsumer)
    monkeypatch.setattr(consumer_module, "setup_logging", lambda **_kwargs: None)
    port = _free_port()
    monkeypatch.setenv("CODEFORGE_WORKER_HEALTH_PORT", str(port))
    return port


async def test_main_serves_health_while_the_consumer_runs(fake_main: int) -> None:
    import codeforge.consumer as consumer_module

    main_task = asyncio.create_task(consumer_module.main())
    while not _FakeConsumer.instances:
        await asyncio.sleep(0.01)
    consumer = _FakeConsumer.instances[0]
    await asyncio.wait_for(consumer.started.wait(), timeout=5)

    assert await asyncio.to_thread(_get, fake_main, "/health") == (HTTPStatus.OK, {"status": "ok"})
    assert (await asyncio.to_thread(_get, fake_main, "/health/ready"))[0] == HTTPStatus.SERVICE_UNAVAILABLE
    consumer.ready = True
    assert (await asyncio.to_thread(_get, fake_main, "/health/ready"))[0] == HTTPStatus.OK

    consumer.release.set()
    await asyncio.wait_for(main_task, timeout=5)

    with pytest.raises(urllib.error.URLError):
        await asyncio.to_thread(_get, fake_main, "/health")


async def test_main_exits_non_zero_when_the_consumer_crashes(fake_main: int, monkeypatch: pytest.MonkeyPatch) -> None:
    """E.g. NATS unreachable at startup: the worker shuts down cleanly, closes the endpoint and exits 1."""
    import codeforge.consumer as consumer_module

    async def unreachable(_self: _FakeConsumer) -> None:
        raise ConnectionError("nats: no servers available for connection")

    monkeypatch.setattr(_FakeConsumer, "start", unreachable)
    with pytest.raises(SystemExit) as exc_info:
        await consumer_module.main()

    assert exc_info.value.code == 1
    assert _FakeConsumer.instances[0].stopped is True, "stop() flushes the logs"
    with pytest.raises(urllib.error.URLError):
        await asyncio.to_thread(_get, fake_main, "/health")


async def test_main_exits_non_zero_when_the_health_port_is_taken(fake_main: int) -> None:
    """A worker that cannot serve its health endpoint would be restarted as unhealthy: fail at once."""
    import codeforge.consumer as consumer_module

    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as taken:
        taken.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 0)
        taken.bind(("", fake_main))
        taken.listen()
        with pytest.raises(SystemExit) as exc_info:
            await consumer_module.main()

    assert exc_info.value.code == 1
    assert not any(c.started.is_set() for c in _FakeConsumer.instances), "no NATS connection without health"


@pytest.mark.parametrize("port", ["-1", "65536"])
async def test_main_exits_non_zero_for_an_invalid_health_port(
    fake_main: int, monkeypatch: pytest.MonkeyPatch, port: str
) -> None:
    import codeforge.consumer as consumer_module

    monkeypatch.setenv("CODEFORGE_WORKER_HEALTH_PORT", port)
    with pytest.raises(SystemExit) as exc_info:
        await consumer_module.main()
    assert exc_info.value.code == 1


# ---------------------------------------------------------------------------
# scripts/worker-healthcheck.py (the container HEALTHCHECK)
# ---------------------------------------------------------------------------


def _run_healthcheck(port: int | None) -> int:
    env = {k: v for k, v in os.environ.items() if k != "CODEFORGE_WORKER_HEALTH_PORT"}
    if port is not None:
        env["CODEFORGE_WORKER_HEALTH_PORT"] = str(port)
    result = subprocess.run(  # noqa: S603 - fixed interpreter and script
        [sys.executable, str(HEALTHCHECK_SCRIPT)], env=env, capture_output=True, timeout=30, check=False
    )
    return result.returncode


def test_healthcheck_script_passes_when_ready(serve: Callable[[Callable[[], bool]], int]) -> None:
    assert _run_healthcheck(serve(lambda: True)) == 0


def test_healthcheck_script_fails_when_not_ready(serve: Callable[[Callable[[], bool]], int]) -> None:
    """Liveness alone is not enough: the check asks /health/ready."""
    assert _run_healthcheck(serve(lambda: False)) == 1


def test_healthcheck_script_fails_without_a_server() -> None:
    assert _run_healthcheck(_free_port()) == 1


def test_healthcheck_script_default_port_is_the_worker_default() -> None:
    from codeforge.config import WorkerSettings

    source = HEALTHCHECK_SCRIPT.read_text()
    assert f'"{WorkerSettings().health_port}"' in source
