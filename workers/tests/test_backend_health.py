"""Tests for backend health check and config schema discovery."""

from __future__ import annotations

import pytest

from codeforge.backends._base import BackendInfo, ConfigField, TaskResult
from codeforge.backends.router import BackendRouter


class FakeExecutor:
    """A backend executor that reports itself as available."""

    def __init__(self, name: str = "fake", available: bool = True) -> None:
        self._name = name
        self._available = available

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name=self._name,
            display_name=f"Fake {self._name}",
            cli_command=f"/usr/bin/{self._name}",
            capabilities=["code-edit"],
            config_schema=(
                ConfigField(key="model", type=str, description="Model name"),
                ConfigField(key="timeout", type=int, default=300, description="Timeout"),
            ),
        )

    async def check_available(self) -> bool:
        return self._available

    async def execute(self, **_kwargs: object) -> TaskResult:
        return TaskResult(status="completed")

    async def cancel(self, _task_id: str) -> None:
        pass


class ErrorExecutor(FakeExecutor):
    """Backend that raises on check_available."""

    async def check_available(self) -> bool:
        msg = "connection refused"
        raise OSError(msg)


@pytest.mark.asyncio
async def test_check_all_health_available() -> None:
    """Available backends report available=True."""
    router = BackendRouter()
    router.register(FakeExecutor("alpha", available=True))

    results = await router.check_all_health()

    assert len(results) == 1
    assert results[0]["name"] == "alpha"
    assert results[0]["available"] is True
    assert results[0]["error"] == ""


@pytest.mark.asyncio
async def test_check_all_health_unavailable() -> None:
    """Unavailable backends report available=False with error."""
    router = BackendRouter()
    router.register(FakeExecutor("beta", available=False))

    results = await router.check_all_health()

    assert len(results) == 1
    assert results[0]["available"] is False
    assert "not found" in results[0]["error"]


@pytest.mark.asyncio
async def test_check_all_health_exception() -> None:
    """Backends that throw report available=False with exception message."""
    router = BackendRouter()
    router.register(ErrorExecutor("broken"))

    results = await router.check_all_health()

    assert len(results) == 1
    assert results[0]["available"] is False
    assert "connection refused" in results[0]["error"]


@pytest.mark.asyncio
async def test_check_all_health_multiple() -> None:
    """Multiple backends are checked concurrently."""
    router = BackendRouter()
    router.register(FakeExecutor("a", available=True))
    router.register(FakeExecutor("b", available=False))
    router.register(ErrorExecutor("c"))

    results = await router.check_all_health()

    assert len(results) == 3
    names = {r["name"] for r in results}
    assert names == {"a", "b", "c"}

    by_name = {r["name"]: r for r in results}
    assert by_name["a"]["available"] is True
    assert by_name["b"]["available"] is False
    assert by_name["c"]["available"] is False


def test_get_config_schemas() -> None:
    """Config schemas are returned with field metadata."""
    router = BackendRouter()
    router.register(FakeExecutor("test-backend"))

    schemas = router.get_config_schemas()

    assert len(schemas) == 1
    schema = schemas[0]
    assert schema["name"] == "test-backend"
    assert schema["display_name"] == "Fake test-backend"

    fields = schema["config_fields"]
    assert len(fields) == 2
    assert fields[0]["key"] == "model"
    assert fields[0]["type"] == "str"
    assert fields[1]["key"] == "timeout"
    assert fields[1]["default"] == 300


def test_get_config_schemas_empty() -> None:
    """Empty router returns empty schema list."""
    router = BackendRouter()
    assert router.get_config_schemas() == []
