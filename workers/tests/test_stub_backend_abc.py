"""Tests for StubBackendExecutor ABC enforcement (STUB-002)."""

from __future__ import annotations

import pytest

from codeforge.backends._base import BackendInfo, StubBackendExecutor


class TestStubBackendExecutorABC:
    """StubBackendExecutor should be abstract — cannot instantiate without overriding info."""

    def test_cannot_instantiate_directly(self) -> None:
        """Instantiating StubBackendExecutor without overriding info raises TypeError."""
        with pytest.raises(TypeError, match="info"):
            StubBackendExecutor()

    def test_subclass_without_info_cannot_instantiate(self) -> None:
        """A subclass that forgets to override info raises TypeError at instantiation."""

        class BadBackend(StubBackendExecutor):
            pass

        with pytest.raises(TypeError, match="info"):
            BadBackend()

    def test_subclass_with_info_can_instantiate(self) -> None:
        """A subclass that overrides info can be instantiated."""

        class GoodBackend(StubBackendExecutor):
            @property
            def info(self) -> BackendInfo:
                return BackendInfo(
                    name="good",
                    display_name="Good",
                    cli_command="good",
                )

        backend = GoodBackend()
        assert backend.info.name == "good"

    @pytest.mark.asyncio
    async def test_subclass_execute_returns_not_implemented(self) -> None:
        """Default execute() returns failed status with not-yet-implemented message."""

        class TestBackend(StubBackendExecutor):
            @property
            def info(self) -> BackendInfo:
                return BackendInfo(
                    name="test",
                    display_name="Test",
                    cli_command="test",
                )

        backend = TestBackend()
        result = await backend.execute("t1", "do stuff", "/workspace")
        assert result.status == "failed"
        assert "not yet implemented" in result.error

    @pytest.mark.asyncio
    async def test_subclass_cancel_is_noop(self) -> None:
        """Default cancel() does not raise."""

        class TestBackend(StubBackendExecutor):
            @property
            def info(self) -> BackendInfo:
                return BackendInfo(
                    name="test",
                    display_name="Test",
                    cli_command="test",
                )

        backend = TestBackend()
        await backend.cancel("t1")  # Should not raise
