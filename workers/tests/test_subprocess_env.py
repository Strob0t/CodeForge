"""Agent-controlled subprocesses must not inherit the worker's credentials.

The worker holds CODEFORGE_INTERNAL_KEY (admin on the core API) and the
database, NATS and LiteLLM credentials. Commands an LLM chooses (bash tool,
grep, quality gates, git in the workspace, benchmark test commands, external
agent CLIs) get an allowlisted environment instead of os.environ.
"""

from __future__ import annotations

import asyncio
import os
from typing import TYPE_CHECKING, Any

import pytest

from codeforge.subprocess_env import tool_env
from codeforge.tools.bash import BashTool

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable
    from pathlib import Path

SECRET_ENV = {
    "CODEFORGE_INTERNAL_KEY": "internal-admin-key-123",
    "CODEFORGE_CORE_URL": "http://core:8080",
    "DATABASE_URL": "postgresql://codeforge:db-pass-123@postgres:5432/codeforge",
    "NATS_URL": "nats://user:nats-pass-123@nats:4222",
    "LITELLM_MASTER_KEY": "sk-master-123",
    "LITELLM_BASE_URL": "http://litellm:4000",
    "OPENAI_API_KEY": "sk-openai-123",
    "GITHUB_TOKEN": "ghp_token123",
    "AWS_SECRET_ACCESS_KEY": "aws-secret-123",
    "PGPASSWORD": "pg-pass-123",
    "SMTP_PASSWORD": "smtp-pass-123",
    "PYTHONPATH": "/app/workers",
    "UNRELATED_SETTING": "value",
}
SECRET_VALUES = ("internal-admin-key-123", "db-pass-123", "nats-pass-123", "sk-master-123", "sk-openai-123")

SAFE_ENV = {
    "PATH": "/usr/local/bin:/usr/bin:/bin",
    "HOME": "/home/codeforge",
    "LANG": "C.UTF-8",
    "LC_ALL": "C.UTF-8",
    "TERM": "xterm",
    "TZ": "UTC",
    "USER": "codeforge",
    "SHELL": "/bin/bash",
    "TMPDIR": "/tmp",
    "HTTPS_PROXY": "http://proxy:3128",
    "SSL_CERT_FILE": "/etc/ssl/certs/ca-certificates.crt",
}


@pytest.fixture
def worker_env(monkeypatch: pytest.MonkeyPatch) -> None:
    """Replace the process environment with a worker-like one."""
    for key in list(os.environ):
        monkeypatch.delenv(key, raising=False)
    for key, value in {**SAFE_ENV, **SECRET_ENV}.items():
        monkeypatch.setenv(key, value)


# ---------------------------------------------------------------------------
# tool_env
# ---------------------------------------------------------------------------


@pytest.mark.usefixtures("worker_env")
class TestToolEnv:
    @pytest.mark.parametrize("name", sorted(SAFE_ENV))
    def test_keeps_allowlisted_variable(self, name: str) -> None:
        assert tool_env()[name] == SAFE_ENV[name]

    @pytest.mark.parametrize("name", sorted(SECRET_ENV))
    def test_drops_credentials_and_unlisted_variables(self, name: str) -> None:
        assert name not in tool_env()

    def test_lc_prefix_is_kept(self, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("LC_MESSAGES", "C")
        assert tool_env()["LC_MESSAGES"] == "C"

    def test_empty_environment(self, monkeypatch: pytest.MonkeyPatch) -> None:
        for key in {**SAFE_ENV, **SECRET_ENV}:
            monkeypatch.delenv(key, raising=False)
        assert tool_env() == {}

    def test_passthrough_copies_named_variables_only_if_present(self) -> None:
        env = tool_env(passthrough=("OPENAI_API_KEY", "MISSING_VAR"))
        assert env["OPENAI_API_KEY"] == "sk-openai-123"
        assert "MISSING_VAR" not in env
        assert "CODEFORGE_INTERNAL_KEY" not in env

    def test_extra_values_are_added_and_override(self) -> None:
        env = tool_env(extra={"AIDER_MODEL": "x", "PATH": "/opt/bin"})
        assert env["AIDER_MODEL"] == "x"
        assert env["PATH"] == "/opt/bin"

    def test_returns_a_new_dict(self) -> None:
        env = tool_env()
        env["PATH"] = "changed"
        assert tool_env()["PATH"] == SAFE_ENV["PATH"]


# ---------------------------------------------------------------------------
# Bash tool, end to end
# ---------------------------------------------------------------------------


@pytest.mark.usefixtures("worker_env")
class TestBashToolEnvironment:
    async def test_env_output_contains_no_credentials(self, tmp_path: Path) -> None:
        result = await BashTool().execute({"command": "env"}, str(tmp_path))
        assert result.success, result.error
        for name in ("CODEFORGE_INTERNAL_KEY", "DATABASE_URL", "NATS_URL", "LITELLM_MASTER_KEY"):
            assert name not in result.output
        for value in SECRET_VALUES:
            assert value not in result.output
        assert "PATH=" in result.output

    async def test_expanding_the_internal_key_yields_nothing(self, tmp_path: Path) -> None:
        cmd = 'printf "[%s][%s]" "$CODEFORGE_INTERNAL_KEY" "$LITELLM_MASTER_KEY"'
        result = await BashTool().execute({"command": cmd}, str(tmp_path))
        assert result.output == "[][]"


# ---------------------------------------------------------------------------
# Every other agent-controlled spawn site
# ---------------------------------------------------------------------------


class _FakeStream:
    async def readline(self) -> bytes:
        return b""


class _FakeProc:
    returncode = 0
    stdout = _FakeStream()

    async def communicate(self) -> tuple[bytes, bytes]:
        return b"", b""

    async def wait(self) -> int:
        return 0


@pytest.fixture
def spawn_spy(monkeypatch: pytest.MonkeyPatch, worker_env: None) -> list[dict[str, Any]]:
    """Record the keyword arguments of every subprocess spawn."""
    calls: list[dict[str, Any]] = []

    async def fake_spawn(*_args: object, **kwargs: Any) -> _FakeProc:
        calls.append(kwargs)
        return _FakeProc()

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_spawn)
    monkeypatch.setattr(asyncio, "create_subprocess_shell", fake_spawn)
    return calls


async def _search_files(ws: Path) -> None:
    from codeforge.tools.search_files import SearchFilesTool

    await SearchFilesTool().execute({"pattern": "x"}, str(ws))


async def _quality_gate(ws: Path) -> None:
    import structlog

    from codeforge.qualitygate import QualityGateExecutor

    await QualityGateExecutor()._run_command("pytest -q", str(ws), structlog.get_logger())


async def _git_in_workspace(ws: Path) -> None:
    from codeforge.agent_loop import _run_git

    await _run_git(str(ws), "status")


async def _cli_backend(ws: Path) -> None:
    from codeforge.backends.aider import AiderExecutor

    await AiderExecutor(cli_path="aider").execute("t1", "do it", str(ws), {"extra_env": {"AIDER_X": "1"}})


async def _cli_backend_without_extra_env(ws: Path) -> None:
    from codeforge.backends.aider import AiderExecutor

    await AiderExecutor(cli_path="aider").execute("t1", "do it", str(ws), {})


async def _claude_code_cli(ws: Path) -> None:
    from unittest.mock import MagicMock

    from codeforge.claude_code_executor import ClaudeCodeExecutor

    await ClaudeCodeExecutor(str(ws), MagicMock())._run_via_cli([{"role": "user", "content": "hi"}], "", 1, "")


async def _benchmark_test_command(ws: Path) -> None:
    from codeforge.evaluation.runners.agent import _run_test_command

    await _run_test_command("pytest -q", ws)


async def _functional_test(ws: Path) -> None:
    from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator

    await FunctionalTestEvaluator(working_dir=str(ws))._run_command("pytest -q")


SPAWN_SITES: list[tuple[str, Callable[[Path], Awaitable[None]]]] = [
    ("search_files", _search_files),
    ("quality_gate", _quality_gate),
    ("git_in_workspace", _git_in_workspace),
    ("cli_backend", _cli_backend),
    ("cli_backend_without_extra_env", _cli_backend_without_extra_env),
    ("claude_code_cli", _claude_code_cli),
    ("benchmark_test_command", _benchmark_test_command),
    ("functional_test", _functional_test),
]


@pytest.mark.parametrize(("site", "run"), SPAWN_SITES, ids=[s for s, _ in SPAWN_SITES])
async def test_spawn_site_uses_scrubbed_environment(
    site: str,
    run: Callable[[Path], Awaitable[None]],
    spawn_spy: list[dict[str, Any]],
    tmp_path: Path,
) -> None:
    await run(tmp_path)
    assert spawn_spy, f"{site}: no subprocess was spawned"
    for kwargs in spawn_spy:
        env = kwargs.get("env")
        assert env is not None, f"{site}: subprocess inherits the full worker environment"
        leaked = sorted(set(env) & set(SECRET_ENV))
        assert not leaked, f"{site}: leaks {leaked}"
        assert env.get("PATH") == SAFE_ENV["PATH"]


async def test_cli_backend_keeps_explicit_extra_env(spawn_spy: list[dict[str, Any]], tmp_path: Path) -> None:
    await _cli_backend(tmp_path)
    assert spawn_spy[0]["env"]["AIDER_X"] == "1"


async def test_claude_code_cli_gets_its_own_credentials(
    spawn_spy: list[dict[str, Any]], monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-ant-123")
    await _claude_code_cli(tmp_path)
    env = spawn_spy[0]["env"]
    assert env["ANTHROPIC_API_KEY"] == "sk-ant-123"
    assert "CODEFORGE_INTERNAL_KEY" not in env
