"""Tests for CLI-based backend executors (Goose, OpenCode, Plandex) and OpenHands HTTP executor."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from codeforge.backends.goose import GooseExecutor
from codeforge.backends.opencode import OpenCodeExecutor
from codeforge.backends.openhands import OpenHandsExecutor
from codeforge.backends.plandex import PlandexExecutor

# ---------- BackendInfo metadata tests ----------

EXECUTOR_INFO = [
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


class TestBackendInfo:
    """Each backend returns correct BackendInfo metadata."""

    @pytest.mark.parametrize(("cls", "expected"), EXECUTOR_INFO, ids=lambda x: x if isinstance(x, dict) else x.__name__)
    def test_info_fields(self, cls: type, expected: dict) -> None:
        executor = cls()
        info = executor.info

        assert info.name == expected["name"]
        assert info.display_name == expected["display_name"]
        assert info.cli_command == expected["cli_command"]
        assert info.requires_docker == expected.get("requires_docker", False)
        assert info.capabilities == expected["capabilities"]


# ---------- CLI-based backends (Goose, OpenCode, Plandex) ----------

CLI_BACKENDS = [
    (GooseExecutor, "/usr/bin/goose", "Goose", "goose"),
    (OpenCodeExecutor, "/usr/bin/opencode", "OpenCode", "opencode"),
    (PlandexExecutor, "/usr/bin/plandex", "Plandex", "plandex"),
]


class TestCLIBackendExecute:
    """CLI-based backends follow the Aider subprocess pattern."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_successful_execution(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b"Working...\n", b"Done.\n", b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute("t1", "fix bug", "/workspace")

        assert result.status == "completed"
        assert "Working..." in result.output
        assert "Done." in result.output

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_failed_execution(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b"Error occurred\n", b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 1
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute("t1", "fix bug", "/workspace")

        assert result.status == "failed"
        assert "exited with code 1" in result.error

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_timeout(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=TimeoutError)

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = None
        mock_proc.terminate = MagicMock()
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            result = await executor.execute("t1", "fix bug", "/workspace", config={"timeout": 5})

        assert result.status == "failed"
        assert "timed out" in result.error
        mock_proc.terminate.assert_called_once()

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_os_error(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        with patch("asyncio.create_subprocess_exec", side_effect=OSError("Permission denied")):
            result = await executor.execute("t1", "fix bug", "/workspace")

        assert result.status == "failed"
        assert f"Failed to start {name}" in result.error

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_output_callback(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b"line1\n", b"line2\n", b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        callback = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc):
            await executor.execute("t1", "fix bug", "/workspace", on_output=callback)

        assert callback.call_count == 2
        callback.assert_any_call("line1")
        callback.assert_any_call("line2")


class TestCLIBackendCancel:
    """CLI-based backends support process termination via cancel()."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_cancel_terminates(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_proc = AsyncMock()
        mock_proc.returncode = None
        mock_proc.terminate = MagicMock()
        mock_proc.wait = AsyncMock(return_value=0)

        executor._processes["t1"] = mock_proc
        await executor.cancel("t1")
        mock_proc.terminate.assert_called_once()

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_cancel_noop_unknown(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)
        await executor.cancel("no-such-task")


# ---------- Config Passthrough (extra_args) for CLI backends ----------


class TestCLIBackendExtraArgs:
    """All CLI-based backends support extra_args config passthrough."""

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_extra_args_list_appended(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                "t1",
                "fix bug",
                "/workspace",
                config={"extra_args": ["--verbose", "--debug"]},
            )

        cmd_args = mock_exec.call_args[0]
        assert "--verbose" in cmd_args
        assert "--debug" in cmd_args

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_extra_args_json_string_parsed(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute(
                "t1",
                "fix bug",
                "/workspace",
                config={"extra_args": '["--verbose"]'},
            )

        cmd_args = mock_exec.call_args[0]
        assert "--verbose" in cmd_args

    @pytest.mark.asyncio
    @pytest.mark.parametrize(
        ("cls", "cli_path", "display", "name"),
        CLI_BACKENDS,
        ids=lambda x: x if isinstance(x, str) and "/" not in x else "",
    )
    async def test_no_extra_args_no_change(self, cls: type, cli_path: str, display: str, name: str) -> None:
        executor = cls(cli_path=cli_path)

        mock_stdout = AsyncMock()
        mock_stdout.readline = AsyncMock(side_effect=[b""])

        mock_proc = AsyncMock()
        mock_proc.stdout = mock_stdout
        mock_proc.returncode = 0
        mock_proc.wait = AsyncMock()

        with patch("asyncio.create_subprocess_exec", return_value=mock_proc) as mock_exec:
            await executor.execute("t1", "fix bug", "/workspace")

        cmd_args = mock_exec.call_args[0]
        # No extra_args should be appended
        assert "--verbose" not in cmd_args
        assert "--debug" not in cmd_args


# ---------- OpenHands HTTP executor ----------


class TestOpenHandsExecute:
    """OpenHands uses HTTP API instead of subprocess."""

    @pytest.mark.asyncio
    async def test_successful_execution(self) -> None:
        executor = OpenHandsExecutor(url="http://test:3000")

        post_response = MagicMock()
        post_response.status_code = 200
        post_response.raise_for_status = MagicMock()
        post_response.json.return_value = {"conversation_id": "conv-123"}

        poll_response = MagicMock()
        poll_response.status_code = 200
        poll_response.raise_for_status = MagicMock()
        poll_response.json.return_value = {
            "status": "completed",
            "messages": [{"content": "Task done."}],
        }

        mock_client = AsyncMock()
        mock_client.post = AsyncMock(return_value=post_response)
        mock_client.get = AsyncMock(return_value=poll_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        mock_httpx = MagicMock()
        mock_httpx.AsyncClient.return_value = mock_client

        with patch.dict("sys.modules", {"httpx": mock_httpx}):
            result = await executor.execute("t1", "fix bug", "/workspace")

        assert result.status == "completed"
        assert "Task done." in result.output

    @pytest.mark.asyncio
    async def test_api_error(self) -> None:
        executor = OpenHandsExecutor(url="http://test:3000")

        mock_client = AsyncMock()
        mock_client.post = AsyncMock(side_effect=Exception("Connection refused"))
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        mock_httpx = MagicMock()
        mock_httpx.AsyncClient.return_value = mock_client

        with patch.dict("sys.modules", {"httpx": mock_httpx}):
            result = await executor.execute("t1", "fix bug", "/workspace")

        assert result.status == "failed"
        assert "API error" in result.error

    @pytest.mark.asyncio
    async def test_missing_httpx(self) -> None:
        executor = OpenHandsExecutor(url="http://test:3000")

        import builtins

        real_import = builtins.__import__

        def fail_httpx(name: str, *args: object, **kwargs: object) -> object:
            if name == "httpx":
                raise ImportError("No module named 'httpx'")
            return real_import(name, *args, **kwargs)

        with patch("builtins.__import__", side_effect=fail_httpx):
            result = await executor.execute("t1", "fix bug", "/workspace")

        assert result.status == "failed"
        assert "httpx is required" in result.error

    @pytest.mark.asyncio
    async def test_cancel(self) -> None:
        executor = OpenHandsExecutor(url="http://test:3000")
        executor._active_tasks["t1"] = "conv-123"

        mock_client = AsyncMock()
        mock_client.delete = AsyncMock()
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        mock_httpx = MagicMock()
        mock_httpx.AsyncClient.return_value = mock_client

        with patch.dict("sys.modules", {"httpx": mock_httpx}):
            await executor.cancel("t1")

        assert "t1" not in executor._active_tasks
