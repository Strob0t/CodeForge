"""Tests for the backend router dispatch logic."""

from __future__ import annotations

import logging
from typing import Any

import pytest

from codeforge.backends._base import BackendInfo, ConfigField, OutputCallback, TaskResult
from codeforge.backends.router import BackendRouter, validate_config


class FakeExecutor:
    """Fake backend executor for testing."""

    def __init__(
        self,
        name: str = "fake",
        available: bool = True,
        result: TaskResult | None = None,
    ) -> None:
        self._name = name
        self._available = available
        self._result = result or TaskResult(status="completed", output="done")
        self.execute_calls: list[dict[str, Any]] = []
        self.cancel_calls: list[str] = []

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name=self._name,
            display_name=self._name.title(),
            cli_command=self._name,
            capabilities=["test"],
        )

    async def check_available(self) -> bool:
        return self._available

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: dict[str, Any] | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        self.execute_calls.append(
            {
                "task_id": task_id,
                "prompt": prompt,
                "workspace_path": workspace_path,
                "config": config,
            }
        )
        return self._result

    async def cancel(self, task_id: str) -> None:
        self.cancel_calls.append(task_id)


@pytest.mark.asyncio
async def test_route_to_correct_executor() -> None:
    """Correct executor is called when routing by name."""
    router = BackendRouter()
    alpha = FakeExecutor(name="alpha", result=TaskResult(status="completed", output="alpha-out"))
    beta = FakeExecutor(name="beta", result=TaskResult(status="completed", output="beta-out"))
    router.register(alpha)
    router.register(beta)

    result = await router.execute("alpha", "t1", "do stuff", "/tmp")
    assert result.status == "completed"
    assert result.output == "alpha-out"
    assert len(alpha.execute_calls) == 1
    assert len(beta.execute_calls) == 0

    result = await router.execute("beta", "t2", "other stuff", "/tmp")
    assert result.output == "beta-out"
    assert len(beta.execute_calls) == 1


@pytest.mark.asyncio
async def test_unknown_backend_returns_failed() -> None:
    """Unknown backend name returns a FAILED result with available list."""
    router = BackendRouter()
    router.register(FakeExecutor(name="aider"))
    router.register(FakeExecutor(name="goose"))

    result = await router.execute("nonexistent", "t1", "prompt", "/tmp")
    assert result.status == "failed"
    assert "Unknown backend 'nonexistent'" in result.error
    assert "aider" in result.error
    assert "goose" in result.error


@pytest.mark.asyncio
async def test_unavailable_backend_returns_explicit_error() -> None:
    """Backend that is not available returns descriptive error."""
    router = BackendRouter()
    unavailable = FakeExecutor(name="aider", available=False)
    router.register(unavailable)

    result = await router.execute("aider", "t1", "prompt", "/tmp")
    assert result.status == "failed"
    assert "not available" in result.error
    assert len(unavailable.execute_calls) == 0


@pytest.mark.asyncio
async def test_cancel_routes_to_correct_backend() -> None:
    """Cancel dispatches to the executor that is running the task."""
    router = BackendRouter()
    executor = FakeExecutor(name="slow")
    router.register(executor)

    # Simulate an active task by inserting into _active directly
    router._active["task-42"] = "slow"

    await router.cancel("task-42")
    assert executor.cancel_calls == ["task-42"]


@pytest.mark.asyncio
async def test_cancel_unknown_task_is_noop() -> None:
    """Cancel for an unknown task_id does not raise."""
    router = BackendRouter()
    await router.cancel("no-such-task")


@pytest.mark.asyncio
async def test_available_backends_returns_all_registered() -> None:
    """available_backends returns names of all registered executors."""
    router = BackendRouter()
    router.register(FakeExecutor(name="a"))
    router.register(FakeExecutor(name="b"))
    router.register(FakeExecutor(name="c"))

    names = router.available_backends()
    assert set(names) == {"a", "b", "c"}


@pytest.mark.asyncio
async def test_get_returns_executor_or_none() -> None:
    """get() returns the executor for a known name, None otherwise."""
    router = BackendRouter()
    ex = FakeExecutor(name="alpha")
    router.register(ex)

    assert router.get("alpha") is ex
    assert router.get("missing") is None


@pytest.mark.asyncio
async def test_active_task_cleaned_up_after_execute() -> None:
    """Active task tracking is cleaned up after execution completes."""
    router = BackendRouter()
    router.register(FakeExecutor(name="fast"))

    await router.execute("fast", "t1", "prompt", "/tmp")
    assert "t1" not in router._active


@pytest.mark.asyncio
async def test_active_task_cleaned_up_on_exception() -> None:
    """Active task tracking is cleaned up even if executor raises."""

    class FailingExecutor(FakeExecutor):
        async def execute(self, **kwargs: Any) -> TaskResult:
            raise RuntimeError("boom")

    router = BackendRouter()
    router.register(FailingExecutor(name="boom"))

    with pytest.raises(RuntimeError, match="boom"):
        await router.execute("boom", "t1", "prompt", "/tmp")

    assert "t1" not in router._active


@pytest.mark.asyncio
async def test_empty_router_unknown_backend() -> None:
    """Router with no executors returns 'none' as available list."""
    router = BackendRouter()
    result = await router.execute("anything", "t1", "prompt", "/tmp")
    assert result.status == "failed"
    assert "Available: none" in result.error


# ---------- Config Validation (A2) ----------


class TestValidateConfig:
    """Tests for validate_config()."""

    def test_valid_config_passes(self) -> None:
        """Config that matches schema produces no errors."""
        schema = (
            ConfigField(key="model", type=str, default="gpt-4"),
            ConfigField(key="timeout", type=int, default=600),
        )
        errors = validate_config({"model": "claude", "timeout": 300}, schema, "test")
        assert errors == []

    def test_unknown_key_logged_as_warning(self, caplog: pytest.LogCaptureFixture) -> None:
        """Unknown keys are reported as warnings."""
        schema = (ConfigField(key="model", type=str),)
        with caplog.at_level(logging.WARNING):
            errors = validate_config({"model": "gpt-4", "unknown_key": 42}, schema, "test")
        assert errors == []
        assert "unknown_key" in caplog.text

    def test_wrong_type_reported(self) -> None:
        """Type mismatch is reported as a validation error."""
        schema = (ConfigField(key="timeout", type=int, default=600),)
        errors = validate_config({"timeout": "not-a-number"}, schema, "test")
        assert len(errors) == 1
        assert "timeout" in errors[0]
        assert "int" in errors[0]

    def test_required_field_missing(self) -> None:
        """Missing required field is reported as a validation error."""
        schema = (ConfigField(key="model", type=str, required=True),)
        errors = validate_config({}, schema, "test")
        assert len(errors) == 1
        assert "model" in errors[0]
        assert "required" in errors[0].lower()

    def test_empty_schema_accepts_any_config(self) -> None:
        """Empty schema produces no errors for any config."""
        errors = validate_config({"anything": "goes", "x": 42}, (), "test")
        assert errors == []


# ---------- Capability Enforcement (A4) ----------


class TestCapabilityEnforcement:
    """Tests for required_capabilities in execute()."""

    @pytest.mark.asyncio
    async def test_no_required_capabilities_normal_execution(self) -> None:
        """Without required_capabilities, execution proceeds normally."""
        router = BackendRouter()
        executor = FakeExecutor(name="test")
        router.register(executor)

        result = await router.execute("test", "t1", "prompt", "/tmp")
        assert result.status == "completed"
        assert len(executor.execute_calls) == 1

    @pytest.mark.asyncio
    async def test_all_capabilities_present_normal_execution(self) -> None:
        """When all required capabilities are present, execution proceeds."""
        router = BackendRouter()
        executor = FakeExecutor(name="test")  # has ["test"] capability
        router.register(executor)

        result = await router.execute(
            "test",
            "t1",
            "prompt",
            "/tmp",
            required_capabilities=["test"],
        )
        assert result.status == "completed"
        assert len(executor.execute_calls) == 1

    @pytest.mark.asyncio
    async def test_missing_capability_returns_failed(self) -> None:
        """Missing capability results in a failed TaskResult without execution."""
        router = BackendRouter()
        executor = FakeExecutor(name="test")  # has ["test"] capability
        router.register(executor)

        result = await router.execute(
            "test",
            "t1",
            "prompt",
            "/tmp",
            required_capabilities=["code-edit"],
        )
        assert result.status == "failed"
        assert "code-edit" in result.error
        assert len(executor.execute_calls) == 0

    @pytest.mark.asyncio
    async def test_multiple_missing_capabilities_listed(self) -> None:
        """All missing capabilities are listed in the error message."""
        router = BackendRouter()
        executor = FakeExecutor(name="test")  # has ["test"] capability
        router.register(executor)

        result = await router.execute(
            "test",
            "t1",
            "prompt",
            "/tmp",
            required_capabilities=["code-edit", "sandbox", "test"],
        )
        assert result.status == "failed"
        assert "code-edit" in result.error
        assert "sandbox" in result.error
        # "test" is satisfied, so it should not appear as missing
        assert len(executor.execute_calls) == 0
