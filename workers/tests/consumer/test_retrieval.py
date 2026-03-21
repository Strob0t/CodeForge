"""Tests for RetrievalHandlerMixin (FIX-033).

Verifies:
- Mixin class exists and has expected methods
- Uses _handle_request pattern (delegates msg.ack/nak)
- Error handling uses `except Exception` (not bare)
"""

from __future__ import annotations

import inspect

from codeforge.consumer._retrieval import RetrievalHandlerMixin


class TestRetrievalHandlerMixinStructure:
    """RetrievalHandlerMixin has expected interface."""

    def test_class_exists(self) -> None:
        assert inspect.isclass(RetrievalHandlerMixin)

    def test_has_handle_retrieval_index(self) -> None:
        assert hasattr(RetrievalHandlerMixin, "_handle_retrieval_index")
        assert inspect.iscoroutinefunction(RetrievalHandlerMixin._handle_retrieval_index)

    def test_has_handle_retrieval_search(self) -> None:
        assert hasattr(RetrievalHandlerMixin, "_handle_retrieval_search")
        assert inspect.iscoroutinefunction(RetrievalHandlerMixin._handle_retrieval_search)

    def test_has_handle_subagent_search(self) -> None:
        assert hasattr(RetrievalHandlerMixin, "_handle_subagent_search")
        assert inspect.iscoroutinefunction(RetrievalHandlerMixin._handle_subagent_search)

    def test_has_do_retrieval_index(self) -> None:
        assert hasattr(RetrievalHandlerMixin, "_do_retrieval_index")
        assert inspect.iscoroutinefunction(RetrievalHandlerMixin._do_retrieval_index)

    def test_has_do_retrieval_search(self) -> None:
        assert hasattr(RetrievalHandlerMixin, "_do_retrieval_search")
        assert inspect.iscoroutinefunction(RetrievalHandlerMixin._do_retrieval_search)

    def test_has_do_subagent_search(self) -> None:
        assert hasattr(RetrievalHandlerMixin, "_do_subagent_search")
        assert inspect.iscoroutinefunction(RetrievalHandlerMixin._do_subagent_search)


class TestRetrievalHandlerMixinErrorHandling:
    """Verify error handling patterns in retrieval mixin source."""

    def test_do_retrieval_search_uses_except_exception(self) -> None:
        source = inspect.getsource(RetrievalHandlerMixin._do_retrieval_search)
        assert "except Exception" in source, "_do_retrieval_search must use `except Exception`, not bare except"

    def test_do_subagent_search_uses_except_exception(self) -> None:
        source = inspect.getsource(RetrievalHandlerMixin._do_subagent_search)
        assert "except Exception" in source, "_do_subagent_search must use `except Exception`, not bare except"

    def test_all_handlers_delegate_to_handle_request(self) -> None:
        """All _handle_* methods should delegate to _handle_request."""
        for method_name in ("_handle_retrieval_index", "_handle_retrieval_search", "_handle_subagent_search"):
            source = inspect.getsource(getattr(RetrievalHandlerMixin, method_name))
            assert "_handle_request" in source, f"{method_name} must delegate to _handle_request"

    def test_search_publishes_error_result_on_failure(self) -> None:
        """Retrieval search should publish error result so Go waiter is unblocked."""
        source = inspect.getsource(RetrievalHandlerMixin._do_retrieval_search)
        assert "_publish_error" in source

    def test_subagent_publishes_error_result_on_failure(self) -> None:
        """Sub-agent search should publish error result so Go waiter is unblocked."""
        source = inspect.getsource(RetrievalHandlerMixin._do_subagent_search)
        assert "_publish_error" in source
