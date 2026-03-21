"""Tests for BackendHealthHandlerMixin (FIX-033).

Verifies:
- Mixin class exists and has expected methods
- Message handling calls msg.ack() on success
- Error handling uses `except Exception as exc:` (not bare)
- Error path calls msg.nak()
"""

from __future__ import annotations

import inspect
import json
from unittest.mock import AsyncMock

import pytest

from codeforge.consumer._backend_health import BackendHealthHandlerMixin


class _FakeBackendRouter:
    """Minimal fake for backend router dependency."""

    async def check_all_health(self) -> list[dict[str, str]]:
        return [{"name": "aider", "status": "healthy"}]

    def get_config_schemas(self) -> list[dict[str, object]]:
        return [{"name": "aider", "config_fields": []}]


class _FakeHandler(BackendHealthHandlerMixin):
    """Minimal concrete class for testing the mixin."""

    def __init__(self, js: AsyncMock) -> None:
        self._js = js
        self._backend_router = _FakeBackendRouter()


@pytest.fixture
def mock_js() -> AsyncMock:
    js = AsyncMock()
    js.publish = AsyncMock()
    return js


@pytest.fixture
def handler(mock_js: AsyncMock) -> _FakeHandler:
    return _FakeHandler(js=mock_js)


class TestBackendHealthStructure:
    """BackendHealthHandlerMixin has expected interface."""

    def test_class_exists(self) -> None:
        assert inspect.isclass(BackendHealthHandlerMixin)

    def test_has_handle_backend_health(self) -> None:
        assert hasattr(BackendHealthHandlerMixin, "_handle_backend_health")
        assert inspect.iscoroutinefunction(BackendHealthHandlerMixin._handle_backend_health)


class TestBackendHealthErrorHandling:
    """Verify error handling patterns in source."""

    def test_uses_except_exception_as_exc(self) -> None:
        source = inspect.getsource(BackendHealthHandlerMixin._handle_backend_health)
        assert "except Exception as exc" in source, (
            "_handle_backend_health must use `except Exception as exc:`, not bare except"
        )

    def test_acks_on_success(self) -> None:
        """Source must contain msg.ack() call on the success path."""
        source = inspect.getsource(BackendHealthHandlerMixin._handle_backend_health)
        assert "msg.ack()" in source

    def test_naks_on_error(self) -> None:
        """Source must contain msg.nak() call on the error path."""
        source = inspect.getsource(BackendHealthHandlerMixin._handle_backend_health)
        assert "msg.nak()" in source


class TestBackendHealthHappyPath:
    """Integration test for happy path."""

    async def test_ack_called_on_success(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
    ) -> None:
        msg = AsyncMock()
        msg.data = json.dumps({"request_id": "req-123"}).encode()
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        await handler._handle_backend_health(msg)

        msg.ack.assert_awaited_once()
        msg.nak.assert_not_awaited()
        mock_js.publish.assert_awaited_once()

    async def test_result_contains_backends(
        self,
        handler: _FakeHandler,
        mock_js: AsyncMock,
    ) -> None:
        msg = AsyncMock()
        msg.data = json.dumps({"request_id": "req-456"}).encode()
        msg.ack = AsyncMock()
        msg.nak = AsyncMock()

        await handler._handle_backend_health(msg)

        published_data = mock_js.publish.call_args[0][1]
        result = json.loads(published_data)
        assert result["request_id"] == "req-456"
        assert len(result["backends"]) > 0
