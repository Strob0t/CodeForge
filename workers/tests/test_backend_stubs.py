"""Tests for stub backend executors (Goose, OpenHands, OpenCode, Plandex)."""

from __future__ import annotations

import pytest

from codeforge.backends.goose import GooseExecutor
from codeforge.backends.opencode import OpenCodeExecutor
from codeforge.backends.openhands import OpenHandsExecutor
from codeforge.backends.plandex import PlandexExecutor

STUB_EXECUTORS = [
    (
        GooseExecutor,
        {
            "name": "goose",
            "display_name": "Goose",
            "cli_command": "goose",
            "requires_docker": False,
            "capabilities": ["code-edit", "mcp-native"],
        },
    ),
    (
        OpenHandsExecutor,
        {
            "name": "openhands",
            "display_name": "OpenHands",
            "cli_command": "http://localhost:3000",
            "requires_docker": True,
            "capabilities": ["code-edit", "browser", "sandbox"],
        },
    ),
    (
        OpenCodeExecutor,
        {
            "name": "opencode",
            "display_name": "OpenCode",
            "cli_command": "opencode",
            "requires_docker": False,
            "capabilities": ["code-edit", "lsp"],
        },
    ),
    (
        PlandexExecutor,
        {
            "name": "plandex",
            "display_name": "Plandex",
            "cli_command": "plandex",
            "requires_docker": False,
            "capabilities": ["code-edit", "planning", "multi-file"],
        },
    ),
]


class TestStubBackendInfo:
    """Each stub returns correct BackendInfo metadata."""

    @pytest.mark.parametrize(
        ("cls", "expected"), STUB_EXECUTORS, ids=lambda x: x if isinstance(x, dict) else x.__name__
    )
    def test_info_fields(self, cls: type, expected: dict) -> None:
        executor = cls()
        info = executor.info

        assert info.name == expected["name"]
        assert info.display_name == expected["display_name"]
        assert info.cli_command == expected["cli_command"]
        assert info.requires_docker == expected.get("requires_docker", False)
        assert info.capabilities == expected["capabilities"]


class TestStubExecuteReturnsNotImplemented:
    """Each stub's execute() returns FAILED with a descriptive message."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "expected"), STUB_EXECUTORS, ids=lambda x: x if isinstance(x, dict) else x.__name__
    )
    async def test_execute_returns_failed(self, cls: type, expected: dict) -> None:
        executor = cls()
        result = await executor.execute(
            task_id="test-1",
            prompt="do something",
            workspace_path="/tmp",
        )

        assert result.status == "failed"
        assert "not yet implemented" in result.error
        assert expected["display_name"] in result.error


class TestStubCancelIsNoop:
    """Each stub's cancel() is a no-op and does not raise."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "expected"), STUB_EXECUTORS, ids=lambda x: x if isinstance(x, dict) else x.__name__
    )
    async def test_cancel_noop(self, cls: type, expected: dict) -> None:
        executor = cls()
        # Should not raise
        await executor.cancel("test-1")
