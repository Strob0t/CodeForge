"""Fixtures for the mdlinkcheck acceptance suite: a local HTTP server for --check-external."""

from __future__ import annotations

import socket
import threading
import time
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from typing import TYPE_CHECKING

import pytest

if TYPE_CHECKING:
    from collections.abc import Iterator


class _RecordingServer(ThreadingHTTPServer):
    daemon_threads = True
    block_on_close = False

    def __init__(self) -> None:
        super().__init__(("127.0.0.1", 0), _Handler)
        self.requests: list[tuple[str, str]] = []
        self.lock = threading.Lock()

    def handle_error(self, request: object, client_address: object) -> None:
        """A client that gave up (timeout test) breaks the pipe; that is expected here."""


class _Handler(BaseHTTPRequestHandler):
    """Routes: /ok, /status/<code>, /head-not-allowed, /redirect, /redirect-to-missing, /slow?seconds=<s>."""

    server: _RecordingServer

    def do_HEAD(self) -> None:
        self._respond()

    def do_GET(self) -> None:
        self._respond()

    def log_message(self, *args: object) -> None:
        """Keeps the test output quiet."""

    def _respond(self) -> None:
        with self.server.lock:
            self.server.requests.append((self.command, self.path))
        route, _, query = self.path.partition("?")
        if route == "/slow":
            time.sleep(float(query.removeprefix("seconds=") or "3"))
        status, location = self._route(route)
        body = b"" if self.command == "HEAD" else b"mdlinkcheck acceptance fixture\n"
        self.send_response(status)
        if location:
            self.send_header("Location", location)
        if status == 405:
            self.send_header("Allow", "GET")
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _route(self, route: str) -> tuple[int, str]:
        if route in ("/ok", "/slow"):
            return 200, ""
        if route.startswith("/status/"):
            return int(route.removeprefix("/status/")), ""
        if route == "/head-not-allowed":
            return (405 if self.command == "HEAD" else 200), ""
        if route == "/redirect":
            return 301, "/ok"
        if route == "/redirect-to-missing":
            return 302, "/status/404"
        return 404, ""


@dataclass(frozen=True)
class LocalServer:
    server: _RecordingServer

    def url(self, path: str) -> str:
        return f"http://127.0.0.1:{self.server.server_address[1]}{path}"

    @property
    def requests(self) -> list[tuple[str, str]]:
        with self.server.lock:
            return list(self.server.requests)


@pytest.fixture
def http_server() -> Iterator[LocalServer]:
    """A local HTTP server on 127.0.0.1 that records every request; no test needs the internet."""
    server = _RecordingServer()
    thread = threading.Thread(target=server.serve_forever, kwargs={"poll_interval": 0.05}, daemon=True)
    thread.start()
    try:
        yield LocalServer(server)
    finally:
        server.shutdown()
        server.server_close()


@pytest.fixture
def closed_port_url() -> str:
    """A URL on 127.0.0.1 where nothing listens, so the connection is refused."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        probe.bind(("127.0.0.1", 0))
        port = probe.getsockname()[1]
    return f"http://127.0.0.1:{port}/refused"
