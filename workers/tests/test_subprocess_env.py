"""Agent-controlled subprocesses must not inherit the worker's credentials.

The worker holds CODEFORGE_INTERNAL_KEY (admin on the core API) and the
database, NATS and LiteLLM credentials. Commands an LLM chooses (bash tool,
grep, quality gates, git in the workspace, benchmark test commands, external
agent CLIs) get an allowlisted environment instead of os.environ.
"""

from __future__ import annotations

import asyncio
import os
import subprocess
import sys
import types
from typing import TYPE_CHECKING

import pytest

from codeforge.subprocess_env import tool_env
from codeforge.tools.bash import BashTool

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable
    from pathlib import Path

# The worker's own credentials: never reach any subprocess.
WORKER_CREDENTIALS = {
    "CODEFORGE_INTERNAL_KEY": "internal-admin-key-123",
    "CODEFORGE_CORE_URL": "http://core:8080",
    "DATABASE_URL": "postgresql://codeforge:db-pass-123@postgres:5432/codeforge",
    "NATS_URL": "nats://user:nats-pass-123@nats:4222",
    "LITELLM_MASTER_KEY": "sk-master-123",
    "LITELLM_BASE_URL": "http://litellm:4000",
}
# Other variables that are not on the allowlist.
OTHER_UNLISTED = {
    "OPENAI_API_KEY": "sk-openai-123",
    "GITHUB_TOKEN": "ghp_token123",
    "AWS_SECRET_ACCESS_KEY": "aws-secret-123",
    "PGPASSWORD": "pg-pass-123",
    "SMTP_PASSWORD": "smtp-pass-123",
    "PYTHONPATH": "/app/workers",
    "UNRELATED_SETTING": "value",
    "AIDER_MODEL": "gpt-x",
    "GOOSE_PROVIDER": "openai",
}
SECRET_ENV = {**WORKER_CREDENTIALS, **OTHER_UNLISTED}
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
    "GOPROXY": "https://proxy.golang.org,direct",
    "GOFLAGS": "-mod=mod",
    "GOPRIVATE": "example.com/*",
    "GONOSUMDB": "example.com/*",
    "GONOSUMCHECK": "1",
    "GOINSECURE": "example.com/*",
    "GOTOOLCHAIN": "local",
    "XDG_CACHE_HOME": "/tmp/cache",
    "XDG_CONFIG_HOME": "/tmp/config",
    "npm_config_registry": "https://registry.npmjs.org/",
    "npm_config_cache": "/tmp/npm",
    "PIP_INDEX_URL": "https://pypi.org/simple",
    "PIP_EXTRA_INDEX_URL": "https://example.com/simple",
    "NODE_OPTIONS": "--max-old-space-size=4096",
    "CI": "true",
    "NO_COLOR": "1",
    "FORCE_COLOR": "0",
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

    @pytest.mark.parametrize(
        "name",
        ["npm_config__auth", "npm_config__authToken", "npm_config_//registry.example/:_authToken", "npm_config_token"],
    )
    def test_secret_looking_names_are_dropped_despite_an_allowed_prefix(
        self, name: str, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv(name, "s3cret")
        assert name not in tool_env()

    def test_empty_environment(self, monkeypatch: pytest.MonkeyPatch) -> None:
        for key in {**SAFE_ENV, **SECRET_ENV}:
            monkeypatch.delenv(key, raising=False)
        assert tool_env() == {}

    def test_passthrough_copies_named_variables_only_if_present(self) -> None:
        env = tool_env(passthrough=("OPENAI_API_KEY", "MISSING_VAR"))
        assert env["OPENAI_API_KEY"] == "sk-openai-123"
        assert "MISSING_VAR" not in env
        assert "CODEFORGE_INTERNAL_KEY" not in env

    def test_passthrough_prefixes(self) -> None:
        env = tool_env(passthrough_prefixes=("AIDER_",))
        assert env["AIDER_MODEL"] == "gpt-x"
        assert "GOOSE_PROVIDER" not in env

    @pytest.mark.parametrize("name", sorted(WORKER_CREDENTIALS))
    def test_passthrough_never_copies_worker_credentials(self, name: str) -> None:
        env = tool_env(passthrough=(name,), passthrough_prefixes=("CODEFORGE_", "LITELLM_", "DATABASE_", "NATS_"))
        assert name not in env

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

    async def communicate(self, input: bytes | None = None) -> tuple[bytes, bytes]:  # noqa: A002 - Process.communicate signature
        return b"", b""

    async def wait(self) -> int:
        return 0


# The env= argument of every recorded spawn (None: inherits os.environ).
SpawnCalls = list[dict[str, str] | None]


@pytest.fixture
def spawn_spy(monkeypatch: pytest.MonkeyPatch, worker_env: None) -> SpawnCalls:
    """Record the env argument of every subprocess spawn."""
    calls: SpawnCalls = []

    async def fake_spawn(*_args: object, env: dict[str, str] | None = None, **_kwargs: object) -> _FakeProc:
        calls.append(env)
        return _FakeProc()

    def fake_run(
        *args: object, env: dict[str, str] | None = None, **_kwargs: object
    ) -> subprocess.CompletedProcess[str]:
        calls.append(env)
        return subprocess.CompletedProcess(args=args, returncode=1, stdout="", stderr="")

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_spawn)
    monkeypatch.setattr(asyncio, "create_subprocess_shell", fake_spawn)
    monkeypatch.setattr(subprocess, "run", fake_run)
    return calls


def _only_env(calls: SpawnCalls) -> dict[str, str]:
    assert len(calls) == 1, f"expected one spawn, got {len(calls)}"
    env = calls[0]
    assert env is not None, "subprocess inherits the full worker environment"
    return env


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


async def _claude_code(ws: Path) -> None:
    from unittest.mock import AsyncMock, MagicMock, patch

    from codeforge.claude_code_executor import ClaudeCodeExecutor

    runtime = MagicMock()
    runtime.is_cancelled = False
    runtime.policy_wait_seconds = 75.0
    # The CLI capability check (its own spawn of `claude --help`) is covered by
    # test_claude_code_executor; here only the run's spawn is recorded.
    with patch("codeforge.claude_code_executor.resolve_cli", AsyncMock(return_value="claude")):
        await ClaudeCodeExecutor(str(ws), runtime).run([{"role": "user", "content": "hi"}], "", 1, "")


async def _claude_code_cli_check(ws: Path) -> None:
    import contextlib
    from unittest.mock import patch

    from codeforge.claude_code_executor import ClaudeCodeCLIError, resolve_cli

    # The fake process prints no --help text, so the check fails after its spawn.
    with (
        patch("codeforge.claude_code_executor._supported_clis", set()),
        patch("codeforge.claude_code_executor._cli_check_lock", None),
        contextlib.suppress(ClaudeCodeCLIError),
    ):
        await resolve_cli(sys.executable)


async def _benchmark_test_command(ws: Path) -> None:
    from codeforge.evaluation.runners.agent import _run_test_command

    await _run_test_command("pytest -q", ws)


async def _functional_test(ws: Path) -> None:
    from codeforge.evaluation.evaluators.functional_test import FunctionalTestEvaluator

    await FunctionalTestEvaluator(working_dir=str(ws))._run_command("pytest -q")


async def _synthetic_benchmark_git(ws: Path) -> None:
    from codeforge.evaluation.providers.codeforge_synthetic import _load_commits

    _load_commits(str(ws))


async def _run_subprocess_helper(ws: Path) -> None:
    from codeforge.subprocess_utils import run_subprocess

    await run_subprocess(["true"], cwd=str(ws))


SPAWN_SITES: list[tuple[str, Callable[[Path], Awaitable[None]]]] = [
    ("search_files", _search_files),
    ("quality_gate", _quality_gate),
    ("git_in_workspace", _git_in_workspace),
    ("cli_backend", _cli_backend),
    ("cli_backend_without_extra_env", _cli_backend_without_extra_env),
    ("claude_code", _claude_code),
    ("claude_code_cli_check", _claude_code_cli_check),
    ("benchmark_test_command", _benchmark_test_command),
    ("functional_test", _functional_test),
    ("synthetic_benchmark_git", _synthetic_benchmark_git),
    ("run_subprocess_helper", _run_subprocess_helper),
]


@pytest.mark.parametrize(("site", "run"), SPAWN_SITES, ids=[s for s, _ in SPAWN_SITES])
async def test_spawn_site_uses_scrubbed_environment(
    site: str,
    run: Callable[[Path], Awaitable[None]],
    spawn_spy: SpawnCalls,
    tmp_path: Path,
) -> None:
    await run(tmp_path)
    env = _only_env(spawn_spy)
    leaked = sorted(set(env) & set(WORKER_CREDENTIALS))
    assert not leaked, f"{site}: leaks {leaked}"
    for name in ("GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "PGPASSWORD", "SMTP_PASSWORD", "PYTHONPATH"):
        assert name not in env, f"{site}: passes {name}"
    assert env.get("PATH") == SAFE_ENV["PATH"]


async def test_cli_backend_keeps_explicit_extra_env(spawn_spy: SpawnCalls, tmp_path: Path) -> None:
    await _cli_backend(tmp_path)
    assert _only_env(spawn_spy)["AIDER_X"] == "1"


# ---------------------------------------------------------------------------
# CLI backends keep their own provider credentials and configuration
# ---------------------------------------------------------------------------

PROVIDER_KEYS = {
    "OPENAI_API_KEY": "sk-openai-123",
    "ANTHROPIC_API_KEY": "sk-ant-123",
    "GEMINI_API_KEY": "gem-123",
    "GOOGLE_API_KEY": "goog-123",
    "OPENROUTER_API_KEY": "or-123",
    "OPENAI_API_BASE": "http://litellm:4000/v1",
}
BACKEND_CONFIG = {
    "aider": "AIDER_MODEL",
    "goose": "GOOSE_PROVIDER",
    "opencode": "OPENCODE_CONFIG",
    "plandex": "PLANDEX_ENV",
    "sweagent": "SWE_AGENT_CONFIG_ROOT",
}


def _backend(name: str) -> object:
    from codeforge.backends.aider import AiderExecutor
    from codeforge.backends.goose import GooseExecutor
    from codeforge.backends.opencode import OpenCodeExecutor
    from codeforge.backends.plandex import PlandexExecutor
    from codeforge.backends.sweagent import SweagentExecutor

    classes = {
        "aider": AiderExecutor,
        "goose": GooseExecutor,
        "opencode": OpenCodeExecutor,
        "plandex": PlandexExecutor,
        "sweagent": SweagentExecutor,
    }
    return classes[name](cli_path=name)


@pytest.mark.parametrize("backend", sorted(BACKEND_CONFIG))
async def test_cli_backend_passes_provider_keys_and_own_config(
    backend: str, spawn_spy: SpawnCalls, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    for name, value in PROVIDER_KEYS.items():
        monkeypatch.setenv(name, value)
    for name in BACKEND_CONFIG.values():
        monkeypatch.setenv(name, f"value-of-{name}")

    from codeforge.backends._cli_base import CLIBackendExecutor

    executor = _backend(backend)
    assert isinstance(executor, CLIBackendExecutor)
    await executor.execute("t1", "do it", str(tmp_path), {})
    env = _only_env(spawn_spy)

    for name, value in PROVIDER_KEYS.items():
        assert env.get(name) == value, f"{backend}: provider variable {name} missing"
    own = BACKEND_CONFIG[backend]
    assert env.get(own) == f"value-of-{own}"
    for other_backend, other in BACKEND_CONFIG.items():
        if other_backend != backend:
            assert other not in env, f"{backend}: gets {other_backend}'s {other}"
    leaked = sorted(set(env) & set(WORKER_CREDENTIALS))
    assert not leaked, f"{backend}: leaks {leaked}"
    for name in ("GITHUB_TOKEN", "AWS_SECRET_ACCESS_KEY", "PGPASSWORD"):
        assert name not in env


# ---------------------------------------------------------------------------
# Claude Code: CLI path only, with its own credentials
# ---------------------------------------------------------------------------


async def test_claude_code_gets_its_own_credentials(
    spawn_spy: SpawnCalls, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-ant-123")
    await _claude_code(tmp_path)
    env = _only_env(spawn_spy)
    assert env["ANTHROPIC_API_KEY"] == "sk-ant-123"
    assert "CODEFORGE_INTERNAL_KEY" not in env
    assert "OPENAI_API_KEY" not in env


async def test_claude_code_never_uses_the_sdk(
    spawn_spy: SpawnCalls, monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    """claude-code-sdk 0.0.25 starts the CLI with {**os.environ, **options.env}:
    the worker environment cannot be scrubbed there, so the executor must not
    use it even when it is installed."""
    sdk_queries: list[object] = []

    async def fake_query(**kwargs: object) -> object:
        sdk_queries.append(kwargs)
        yield None

    class _Message:
        pass

    sdk_types = types.ModuleType("claude_code_sdk.types")
    sdk_types.AssistantMessage = _Message  # type: ignore[attr-defined]
    sdk_types.ResultMessage = _Message  # type: ignore[attr-defined]
    sdk = types.ModuleType("claude_code_sdk")
    sdk.query = fake_query  # type: ignore[attr-defined]
    sdk.ClaudeCodeOptions = dict  # type: ignore[attr-defined]
    sdk.types = sdk_types  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "claude_code_sdk", sdk)
    monkeypatch.setitem(sys.modules, "claude_code_sdk.types", sdk_types)

    await _claude_code(tmp_path)
    assert sdk_queries == []
    _only_env(spawn_spy)
