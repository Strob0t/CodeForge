"""Tests for ContextHandlerMixin (FIX-033).

Verifies the context re-ranking consumer mixin:
- Mixin class exists and has expected methods
- Uses _handle_request pattern (delegates msg.ack/nak)
- Error handling uses `except Exception` (not bare)
"""

from __future__ import annotations

import inspect

from codeforge.consumer._context import ContextHandlerMixin


class TestContextHandlerMixinStructure:
    """ContextHandlerMixin has expected interface."""

    def test_class_exists(self) -> None:
        assert inspect.isclass(ContextHandlerMixin)

    def test_has_handle_context_rerank(self) -> None:
        assert hasattr(ContextHandlerMixin, "_handle_context_rerank")
        assert inspect.iscoroutinefunction(ContextHandlerMixin._handle_context_rerank)

    def test_has_do_context_rerank(self) -> None:
        assert hasattr(ContextHandlerMixin, "_do_context_rerank")
        assert inspect.iscoroutinefunction(ContextHandlerMixin._do_context_rerank)


class TestContextHandlerMixinErrorHandling:
    """Verify error handling patterns in context mixin source."""

    def test_do_context_rerank_uses_except_exception(self) -> None:
        """_do_context_rerank must use `except Exception` (not bare except)."""
        source = inspect.getsource(ContextHandlerMixin._do_context_rerank)
        assert "except Exception" in source, "_do_context_rerank must use `except Exception`, not bare except"

    def test_handle_method_delegates_to_handle_request(self) -> None:
        """Handler method should delegate to _handle_request for ack/nak."""
        source = inspect.getsource(ContextHandlerMixin._handle_context_rerank)
        assert "_handle_request" in source, "_handle_context_rerank must delegate to _handle_request"

    def test_publishes_error_result_on_failure(self) -> None:
        """Context rerank returns an error result on failure, so the Go waiter is unblocked.

        _handle_request publishes it and acks the request; a re-raise would retry
        the LLM rerank after Go already got its answer (behaviour:
        tests/consumer/test_delivery_handlers.py::test_error_result_settles_the_request).
        """
        source = inspect.getsource(ContextHandlerMixin._do_context_rerank)
        assert 'error="internal worker error"' in source, "_do_context_rerank should return an error result"
        assert "raise" not in source, "a failed rerank must not be retried after its error result"
