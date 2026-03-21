"""Tests for GraphHandlerMixin (FIX-033).

Verifies the graph build and search consumer mixin:
- Mixin class exists and has expected methods
- Uses _handle_request pattern (delegates msg.ack/nak)
- Error handling uses `except Exception` (not bare)
"""

from __future__ import annotations

import inspect

from codeforge.consumer._graph import GraphHandlerMixin


class TestGraphHandlerMixinStructure:
    """GraphHandlerMixin has expected interface."""

    def test_class_exists(self) -> None:
        assert inspect.isclass(GraphHandlerMixin)

    def test_has_handle_graph_build(self) -> None:
        assert hasattr(GraphHandlerMixin, "_handle_graph_build")
        assert inspect.iscoroutinefunction(GraphHandlerMixin._handle_graph_build)

    def test_has_handle_graph_search(self) -> None:
        assert hasattr(GraphHandlerMixin, "_handle_graph_search")
        assert inspect.iscoroutinefunction(GraphHandlerMixin._handle_graph_search)

    def test_has_do_graph_build(self) -> None:
        assert hasattr(GraphHandlerMixin, "_do_graph_build")
        assert inspect.iscoroutinefunction(GraphHandlerMixin._do_graph_build)

    def test_has_do_graph_search(self) -> None:
        assert hasattr(GraphHandlerMixin, "_do_graph_search")
        assert inspect.iscoroutinefunction(GraphHandlerMixin._do_graph_search)


class TestGraphHandlerMixinErrorHandling:
    """Verify error handling patterns in graph mixin source."""

    def test_graph_search_uses_except_exception(self) -> None:
        """_do_graph_search must use `except Exception` (not bare except)."""
        source = inspect.getsource(GraphHandlerMixin._do_graph_search)
        assert "except Exception" in source, "_do_graph_search must use `except Exception`, not bare except"

    def test_handle_methods_delegate_to_handle_request(self) -> None:
        """Handler methods should delegate to _handle_request for ack/nak."""
        build_source = inspect.getsource(GraphHandlerMixin._handle_graph_build)
        assert "_handle_request" in build_source, "_handle_graph_build must delegate to _handle_request"
        search_source = inspect.getsource(GraphHandlerMixin._handle_graph_search)
        assert "_handle_request" in search_source, "_handle_graph_search must delegate to _handle_request"

    def test_publishes_error_result_on_search_failure(self) -> None:
        """Graph search should publish error result so Go waiter is unblocked."""
        source = inspect.getsource(GraphHandlerMixin._do_graph_search)
        assert "_publish_error" in source, "_do_graph_search should publish error result on exception"
