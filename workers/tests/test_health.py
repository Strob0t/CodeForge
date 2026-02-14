"""Tests for the health check module."""

import json
from http import HTTPStatus
from unittest.mock import MagicMock

from codeforge.health import HealthHandler


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
