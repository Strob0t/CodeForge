"""Health endpoints of the Python worker (the Go core serves the same paths).

- GET /health        liveness: 200 while the process serves requests
- GET /health/ready  readiness: 200 when the worker is ready (connected to
                     NATS, every consumer loop running, not given up), else 503

The server runs in a daemon thread next to the asyncio event loop; the
container healthcheck (scripts/worker-healthcheck.py) asks /health/ready.
"""

from __future__ import annotations

import json
import threading
from http import HTTPStatus
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import TYPE_CHECKING
from urllib.parse import urlsplit

import structlog

if TYPE_CHECKING:
    from collections.abc import Callable

logger = structlog.get_logger()


class HealthServer(ThreadingHTTPServer):
    """HTTP server that asks the worker whether it is ready."""

    daemon_threads = True

    def __init__(
        self, address: tuple[str, int], is_ready: Callable[[], bool], describe: Callable[[], str] | None = None
    ) -> None:
        super().__init__(address, HealthHandler)
        self.is_ready = is_ready
        # The status a not-ready worker reports (e.g. "starting").
        self.describe = describe or (lambda: "not ready")


class HealthHandler(BaseHTTPRequestHandler):
    """Serves /health and /health/ready."""

    server: HealthServer

    def do_GET(self) -> None:
        path = urlsplit(self.path).path
        if path == "/health":
            self._send_json(HTTPStatus.OK, {"status": "ok"})
        elif path == "/health/ready":
            if self._ready():
                self._send_json(HTTPStatus.OK, {"status": "ready"})
            else:
                self._send_json(HTTPStatus.SERVICE_UNAVAILABLE, {"status": self._not_ready_status()})
        else:
            self.send_response(HTTPStatus.NOT_FOUND)
            self.end_headers()

    def _ready(self) -> bool:
        """Ask the worker; a check that fails means not ready."""
        try:
            return bool(self.server.is_ready())
        except Exception as exc:
            logger.warning("readiness check failed", error=str(exc))
            return False

    def _not_ready_status(self) -> str:
        try:
            return str(self.server.describe())
        except Exception as exc:
            logger.warning("readiness status failed", error=str(exc))
            return "not ready"

    def _send_json(self, status: HTTPStatus, body: dict[str, str]) -> None:
        payload = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, fmt: str, *args: object) -> None:
        pass  # healthchecks every few seconds would flood the log


def start_health_server(
    port: int, is_ready: Callable[[], bool], describe: Callable[[], str] | None = None
) -> HealthServer:
    """Serve the health endpoints on *port* (all interfaces; 0 picks a free port) in a daemon thread.

    *describe* gives the status a not-ready worker reports ("not ready" by default).

    Raises OSError if the port cannot be bound and OverflowError if it is out
    of range. Stop the server with shutdown() and server_close().
    """
    server = HealthServer(("", port), is_ready, describe)
    threading.Thread(target=server.serve_forever, name="health-server", daemon=True).start()
    return server
