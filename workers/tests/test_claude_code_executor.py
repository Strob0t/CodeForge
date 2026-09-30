"""Tests for ClaudeCodeExecutor."""

from __future__ import annotations

import asyncio
import json
import os
import shlex
import stat
import sys
import time
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, patch

import pytest

from codeforge import claude_code_executor as cce
from codeforge import claude_code_policy_hook as hook
from codeforge.claude_code_executor import (
    ClaudeCodeCLIError,
    ClaudeCodeExecutor,
    PolicySocketServer,
    build_cli_command,
    hook_timeouts,
    policy_request_args,
    resolve_cli,
)
from codeforge.config import get_settings
from codeforge.models import ToolCallDecision
from codeforge.runtime import arguments_preview
from codeforge.subprocess_env import tool_env

if TYPE_CHECKING:
    from collections.abc import Iterator
    from pathlib import Path


def _make_executor() -> ClaudeCodeExecutor:
    return ClaudeCodeExecutor(workspace_path="/tmp", runtime=AsyncMock())


class _FakeRuntime:
    """The part of RuntimeClient the executor uses; records every policy request."""

    def __init__(
        self,
        decision: str = "allow",
        reason: str = "",
        *,
        exc: Exception | None = None,
        delay: float = 0.0,
        policy_wait_seconds: float = 75.0,
    ) -> None:
        self.decision = decision
        self.reason = reason
        self.exc = exc
        self.delay = delay
        self.policy_wait_seconds = policy_wait_seconds
        self.calls: list[dict[str, str]] = []
        self.outputs: list[str] = []

    async def request_tool_call(
        self,
        tool: str,
        command: str = "",
        path: str = "",
        arguments_preview: str = "",
    ) -> ToolCallDecision:
        self.calls.append({"tool": tool, "command": command, "path": path, "arguments_preview": arguments_preview})
        if self.delay:
            await asyncio.sleep(self.delay)
        if self.exc is not None:
            raise self.exc
        return ToolCallDecision(call_id="c1", decision=self.decision, reason=self.reason)

    async def send_output(self, line: str) -> None:
        self.outputs.append(line)


class TestFormatMessagesAsPrompt:
    def test_single_user_message(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [{"role": "user", "content": "Fix the bug"}],
        )
        assert result == "Fix the bug"

    def test_system_messages_excluded(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [
                {"role": "system", "content": "You are helpful"},
                {"role": "user", "content": "Hello"},
            ]
        )
        assert result == "Hello"
        assert "system" not in result.lower()

    def test_multi_turn_preserves_history(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [
                {"role": "user", "content": "Write a function"},
                {"role": "assistant", "content": "def foo(): pass"},
                {"role": "user", "content": "Now add tests"},
            ]
        )
        assert "<conversation_history>" in result
        assert "[USER]: Write a function" in result
        assert "[ASSISTANT]: def foo(): pass" in result
        assert "Now add tests" in result
        assert "[USER]: Now add tests" not in result

    def test_empty_messages(self) -> None:
        assert _make_executor()._format_messages_as_prompt([]) == ""

    def test_only_system_messages(self) -> None:
        result = _make_executor()._format_messages_as_prompt(
            [{"role": "system", "content": "system only"}],
        )
        assert result == ""


class TestEstimateEquivalentCost:
    def test_returns_float(self) -> None:
        cost = _make_executor()._estimate_equivalent_cost(1000, 500)
        assert isinstance(cost, float)
        assert cost >= 0.0

    def test_zero_tokens_returns_zero(self) -> None:
        assert _make_executor()._estimate_equivalent_cost(0, 0) == 0.0

    def test_calls_resolve_cost_with_correct_args(self) -> None:
        with patch("codeforge.claude_code_executor.resolve_cost", return_value=0.05) as mock_rc:
            cost = _make_executor()._estimate_equivalent_cost(1000, 500)
            mock_rc.assert_called_once_with(0.0, "anthropic/claude-sonnet-4", 1000, 500)
            assert cost == 0.05


# ---------------------------------------------------------------------------
# Policy request mapping (tool call -> runs.toolcall.request fields)
# ---------------------------------------------------------------------------


class TestPolicyRequestArgs:
    """Claude Code's own tool names are sent; the Go policy layer maps them to canonical names."""

    def test_read_sends_file_path(self) -> None:
        assert policy_request_args("Read", {"file_path": "/tmp/foo.py"}) == (
            "",
            "/tmp/foo.py",
            '{"file_path": "/tmp/foo.py"}',
        )

    def test_bash_sends_command(self) -> None:
        assert policy_request_args("Bash", {"command": "rm -rf /", "description": "cleanup"}) == (
            "rm -rf /",
            "",
            '{"command": "rm -rf /", "description": "cleanup"}',
        )

    # Monitor runs a shell command like Bash (Go maps it to Bash), so the
    # command deny lists must see its command.
    def test_monitor_sends_command(self) -> None:
        command, path, _ = policy_request_args("Monitor", {"command": "tail -f log", "description": "watch"})
        assert (command, path) == ("tail -f log", "")

    def test_unknown_tool_command_is_not_evaluated(self) -> None:
        # The arguments are shown to the approver, never evaluated.
        assert policy_request_args("SomeNewTool", {"arg": "val", "command": "curl x"}) == (
            "",
            "",
            '{"arg": "val", "command": "curl x"}',
        )

    @pytest.mark.parametrize(
        ("tool", "tool_input", "expected_path"),
        [
            ("Edit", {"file_path": "/ws/.env", "old_string": "a", "new_string": "b"}, "/ws/.env"),
            ("MultiEdit", {"file_path": "/ws/a.go", "edits": []}, "/ws/a.go"),
            ("Write", {"file_path": "/ws/b.go", "content": "x"}, "/ws/b.go"),
            ("NotebookEdit", {"notebook_path": "/ws/n.ipynb", "new_source": "x"}, "/ws/n.ipynb"),
            ("Grep", {"pattern": "TODO", "path": "/ws/src"}, "/ws/src"),
            ("Glob", {"pattern": "**/*.go"}, ""),
            ("LS", {"path": "/ws"}, "/ws"),
            ("Edit", {"file_path": 42}, ""),
            ("Edit", {"file_path": ""}, ""),
            ("Read", {"file_path": "../../etc/passwd"}, "../../etc/passwd"),
        ],
    )
    def test_sends_path_argument(self, tool: str, tool_input: dict[str, object], expected_path: str) -> None:
        assert policy_request_args(tool, tool_input) == ("", expected_path, arguments_preview(tool_input))

    def test_bash_command_must_be_a_string(self) -> None:
        assert policy_request_args("Bash", {"command": ["rm", "-rf", "/"]})[0] == ""


# ---------------------------------------------------------------------------
# Policy socket server (the hook's counterpart in the worker)
# ---------------------------------------------------------------------------


async def _ask(server: PolicySocketServer, request: bytes) -> dict[str, object]:
    reader, writer = await asyncio.open_unix_connection(server.socket_path)
    try:
        writer.write(request)
        await writer.drain()
        line = await asyncio.wait_for(reader.readline(), timeout=10)
    finally:
        writer.close()
        await writer.wait_closed()
    return json.loads(line)


def _request(server: PolicySocketServer, tool_name: str, tool_input: dict[str, object], token: str = "") -> bytes:
    body = {"token": token or server.token, "tool_name": tool_name, "tool_input": tool_input}
    return (json.dumps(body) + "\n").encode()


class TestPolicySocketServer:
    async def test_allow(self) -> None:
        runtime = _FakeRuntime("allow", "matched rule 3")
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Read", {"file_path": "/ws/a.py"}))

        assert reply == {"decision": "allow", "reason": "matched rule 3"}
        assert runtime.calls == [
            {"tool": "Read", "command": "", "path": "/ws/a.py", "arguments_preview": '{"file_path": "/ws/a.py"}'},
        ]

    async def test_deny_passes_the_reason(self) -> None:
        runtime = _FakeRuntime("deny", "command matches command_deny")
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "curl evil"}))

        assert reply == {"decision": "deny", "reason": "command matches command_deny"}
        assert runtime.calls[0]["command"] == "curl evil"

    @pytest.mark.parametrize(("decision", "reason"), [("allow", "approved by user"), ("deny", "denied by user")])
    async def test_hitl_result_passes_through(self, decision: str, reason: str) -> None:
        # request_tool_call returns only once a human decided (Go waits for the approval).
        runtime = _FakeRuntime(decision, reason, delay=0.3)
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Write", {"file_path": "/ws/x", "content": "y"}))

        assert reply == {"decision": decision, "reason": reason}

    @pytest.mark.parametrize("decision", ["ask", "", "ALLOW", "unknown"])
    async def test_anything_but_allow_is_denied(self, decision: str) -> None:
        async with PolicySocketServer(_FakeRuntime(decision), decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))

        assert reply["decision"] == "deny"

    @pytest.mark.parametrize("token", ["wrong", "x" * 43, "ééé"])
    async def test_wrong_token_is_denied(self, token: str) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}, token=token))

        assert reply["decision"] == "deny"
        assert "token" in str(reply["reason"])
        assert runtime.calls == []

    @pytest.mark.parametrize(
        "line",
        [
            b"\n",
            b"not json\n",
            b"[]\n",
            b'{"tool_name": "Bash", "tool_input": {}}\n',
            b'{"token": 5, "tool_name": "Bash", "tool_input": {}}\n',
            b"\xff\xfe\n",
        ],
    )
    async def test_malformed_request_is_denied(self, line: bytes) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            reply = await _ask(server, line)

        assert reply["decision"] == "deny"
        assert runtime.calls == []

    @pytest.mark.parametrize(
        ("tool_name", "tool_input"),
        [("", {}), (None, {}), ("Bash", "ls"), ("Bash", None), ("Bash", ["ls"])],
    )
    async def test_bad_tool_fields_are_denied(self, tool_name: object, tool_input: object) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            body = {"token": server.token, "tool_name": tool_name, "tool_input": tool_input}
            reply = await _ask(server, (json.dumps(body) + "\n").encode())

        assert reply["decision"] == "deny"
        assert runtime.calls == []

    async def test_request_tool_call_raising_is_denied(self) -> None:
        runtime = _FakeRuntime(exc=RuntimeError("nats down"))
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))

        assert reply["decision"] == "deny"
        assert "nats down" in str(reply["reason"])

    async def test_slow_decision_is_denied_after_the_decision_timeout(self) -> None:
        runtime = _FakeRuntime("allow", delay=5)
        async with PolicySocketServer(runtime, decision_timeout=0.2) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))

        assert reply["decision"] == "deny"

    async def test_oversized_request_is_denied(self) -> None:
        runtime = _FakeRuntime("allow")
        big: dict[str, object] = {"file_path": "/ws/big", "content": "x" * (cce.MAX_POLICY_REQUEST_BYTES + 10)}
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            # The server may answer "deny" or drop the connection mid-request; the hook blocks either way.
            try:
                decision, _ = await asyncio.to_thread(
                    hook.ask_policy, server.socket_path, server.token, "Write", big, 10
                )
            except (hook.PolicyHookError, OSError):
                decision = "blocked"

        assert decision in {"deny", "blocked"}
        assert runtime.calls == []

    async def test_concurrent_requests(self) -> None:
        runtime = _FakeRuntime("allow", delay=0.2)
        async with PolicySocketServer(runtime, decision_timeout=5) as server:
            replies = await asyncio.gather(
                *(_ask(server, _request(server, "Read", {"file_path": f"/ws/{i}"})) for i in range(5))
            )

        assert [r["decision"] for r in replies] == ["allow"] * 5
        assert sorted(c["path"] for c in runtime.calls) == [f"/ws/{i}" for i in range(5)]

    async def test_socket_is_private_and_removed_on_exit(self) -> None:
        async with PolicySocketServer(_FakeRuntime(), decision_timeout=5) as server:
            path = server.socket_path
            directory = os.path.dirname(path)
            assert stat.S_IMODE(os.stat(directory).st_mode) == 0o700
            assert stat.S_ISSOCK(os.stat(path).st_mode)
            assert len(server.token) >= 32

        assert not os.path.exists(path)
        assert not os.path.exists(directory)

    async def test_each_run_gets_its_own_token_and_socket(self) -> None:
        async with (
            PolicySocketServer(_FakeRuntime(), decision_timeout=5) as first,
            PolicySocketServer(_FakeRuntime(), decision_timeout=5) as second,
        ):
            assert first.token != second.token
            assert first.socket_path != second.socket_path

    async def test_exit_does_not_wait_for_a_pending_decision(self) -> None:
        runtime = _FakeRuntime("allow", delay=30)
        started = time.monotonic()
        async with PolicySocketServer(runtime, decision_timeout=60) as server:
            _reader, writer = await asyncio.open_unix_connection(server.socket_path)
            writer.write(_request(server, "Bash", {"command": "ls"}))
            await writer.drain()
            while not runtime.calls:
                await asyncio.sleep(0.01)
        writer.close()

        assert time.monotonic() - started < 5


# ---------------------------------------------------------------------------
# Command line
# ---------------------------------------------------------------------------


def _settings_of(cmd: list[str]) -> dict[str, object]:
    return json.loads(cmd[cmd.index("--settings") + 1])


def _hook_of(cmd: list[str]) -> dict[str, object]:
    settings = _settings_of(cmd)
    (entry,) = settings["hooks"]["PreToolUse"]
    (hook_cfg,) = entry["hooks"]
    return {"matcher": entry["matcher"], **hook_cfg}


class TestBuildCliCommand:
    def _cmd(self, system_prompt: str = "", policy_wait: float = 75.0) -> list[str]:
        return build_cli_command(
            "/opt/bin/claude", max_turns=7, system_prompt=system_prompt, policy_wait_seconds=policy_wait
        )

    def test_print_mode_with_stream_json(self) -> None:
        cmd = self._cmd()
        assert cmd[0] == "/opt/bin/claude"
        assert "-p" in cmd
        assert cmd[cmd.index("--output-format") + 1] == "stream-json"
        # -p with stream-json exits with an error without --verbose.
        assert "--verbose" in cmd
        assert cmd[cmd.index("--max-turns") + 1] == "7"

    def test_repo_and_user_settings_are_not_loaded(self) -> None:
        cmd = self._cmd()
        assert cmd[cmd.index("--setting-sources") + 1] == ""

    def test_only_an_empty_mcp_config(self) -> None:
        cmd = self._cmd()
        assert "--strict-mcp-config" in cmd
        i = cmd.index("--mcp-config")
        assert json.loads(cmd[i + 1]) == {"mcpServers": {}}
        # --mcp-config takes several values: the next argument must be an option.
        assert cmd[i + 2].startswith("--")

    def test_permission_mode_dont_ask_and_never_bypass(self) -> None:
        cmd = self._cmd()
        assert cmd[cmd.index("--permission-mode") + 1] == "dontAsk"
        joined = " ".join(cmd)
        assert "bypassPermissions" not in joined
        assert "dangerously" not in joined
        assert "--allowedTools" not in joined
        assert "--allowed-tools" not in joined

    def test_pre_tool_use_hook_for_every_tool(self) -> None:
        settings = _settings_of(self._cmd())
        assert set(settings) == {"hooks"}
        assert set(settings["hooks"]) == {"PreToolUse"}
        hook_cfg = _hook_of(self._cmd())
        assert hook_cfg["matcher"] == "*"
        assert hook_cfg["type"] == "command"

    def test_hook_command_runs_the_hook_file_with_sys_executable(self) -> None:
        command = str(_hook_of(self._cmd())["command"])
        args = shlex.split(command)
        assert args[:3] == [sys.executable, "-I", hook.__file__]
        assert args[3] == hook.TIMEOUT_ARG
        # A failure of the interpreter itself (exit 1, 127, signal) must block too.
        assert args[-3:] == ["||", "exit", "2"]

    def test_timeouts_outlast_the_policy_wait(self) -> None:
        cmd = self._cmd(policy_wait=75.0)
        hook_cfg = _hook_of(cmd)
        hook_arg = float(shlex.split(str(hook_cfg["command"]))[4])
        timeouts = hook_timeouts(75.0)

        assert timeouts.decision > 75.0
        assert hook_arg == timeouts.hook > timeouts.decision
        # A hook the CLI kills on its timeout does not block the tool: the hook gives up first.
        assert hook_cfg["timeout"] == timeouts.cli > timeouts.hook
        assert isinstance(hook_cfg["timeout"], int)

    def test_socket_and_token_are_not_on_the_command_line(self) -> None:
        # They reach the hook through the environment (not visible in ps).
        joined = " ".join(self._cmd())
        assert hook.TOKEN_ENV not in joined
        assert hook.SOCKET_ENV not in joined
        assert ".sock" not in joined

    def test_system_prompt(self) -> None:
        cmd = self._cmd(system_prompt="-be careful")
        assert cmd[cmd.index("--system-prompt") + 1] == "-be careful"
        assert "--system-prompt" not in self._cmd()


# ---------------------------------------------------------------------------
# Fake CLI (a real subprocess): records what the executor starts it with and
# runs the hook command like Claude Code does.
# ---------------------------------------------------------------------------

_FULL_HELP = """Usage: claude [options] [command] [prompt]
  -p, --print                 Print response and exit
  --output-format <format>    Output format
  --verbose                   Override verbose mode setting from config
  --mcp-config <configs...>   Load MCP servers
  --permission-mode <mode>    Permission mode (choices: "acceptEdits", "bypassPermissions", "default", "dontAsk", "plan")
  --setting-sources <sources> Comma-separated list of setting sources to load (user, project, local).
  --settings <file-or-json>   Path to a settings JSON file or a JSON string
  --strict-mcp-config         Only use MCP servers from --mcp-config
  --system-prompt <prompt>    System prompt to use for the session
"""

_FAKE_CLI = """#!{python}
import json, os, subprocess, sys, time
HERE = os.path.dirname(os.path.abspath(__file__))
cfg = json.load(open(os.path.join(HERE, "fake_cli.json")))
if sys.argv[1:] == ["--help"]:
    with open(os.path.join(HERE, "help_calls"), "a") as f:
        f.write("x\\n")
    print(cfg["help"])
    sys.exit(cfg.get("help_exit", 0))
record = {{"argv": sys.argv[1:], "stdin": sys.stdin.read(), "cwd": os.getcwd(), "hooks": []}}
settings = json.loads(sys.argv[sys.argv.index("--settings") + 1])
command = settings["hooks"]["PreToolUse"][0]["hooks"][0]["command"]
for call in cfg.get("tool_calls", []):
    proc = subprocess.run(
        command, shell=True, input=json.dumps({{"hook_event_name": "PreToolUse", **call}}),
        capture_output=True, text=True, timeout=60,
    )
    record["hooks"].append({{"exit": proc.returncode, "stdout": proc.stdout, "stderr": proc.stderr}})
with open(os.path.join(HERE, "record.json"), "w") as f:
    json.dump(record, f)
for event in cfg.get("events", []):
    print(json.dumps(event), flush=True)
time.sleep(cfg.get("sleep", 0))
sys.exit(cfg.get("exit", 0))
"""


class _FakeCli:
    def __init__(self, directory: Path, **cfg: object) -> None:
        self.dir = directory
        self.dir.mkdir(parents=True, exist_ok=True)
        self.path = directory / "claude"
        self.path.write_text(_FAKE_CLI.format(python=sys.executable))
        self.path.chmod(0o755)
        self.configure(**cfg)

    def configure(self, **cfg: object) -> None:
        cfg.setdefault("help", _FULL_HELP)
        (self.dir / "fake_cli.json").write_text(json.dumps(cfg))

    @property
    def record(self) -> dict[str, object]:
        return json.loads((self.dir / "record.json").read_text())

    @property
    def ran(self) -> bool:
        return (self.dir / "record.json").exists()

    @property
    def help_calls(self) -> int:
        calls = self.dir / "help_calls"
        return len(calls.read_text().splitlines()) if calls.exists() else 0


@pytest.fixture
def _no_cli_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(cce, "_cli_support_cache", {})


@pytest.fixture
def fake_cli(tmp_path: Path, monkeypatch: pytest.MonkeyPatch, _no_cli_cache: None) -> _FakeCli:
    cli = _FakeCli(tmp_path / "bin")
    monkeypatch.setenv("CODEFORGE_CLAUDECODE_PATH", str(cli.path))
    monkeypatch.setattr("codeforge.config.load_yaml_config", dict)
    get_settings.cache_clear()
    return cli


_RESULT_EVENTS = [
    {"type": "assistant", "message": {"content": [{"type": "text", "text": "done"}]}},
    {"type": "result", "usage": {"input_tokens": 10, "output_tokens": 5}, "num_turns": 1},
]


async def _run(
    workspace: Path, runtime: _FakeRuntime, prompt: str = "Fix the bug", system_prompt: str = ""
) -> cce.AgentLoopResult:
    executor = ClaudeCodeExecutor(workspace_path=str(workspace), runtime=runtime)  # type: ignore[arg-type]
    return await executor.run([{"role": "user", "content": prompt}], max_turns=3, system_prompt=system_prompt)


class TestRunWithFakeCli:
    async def test_uses_the_configured_cli_and_passes_the_prompt_on_stdin(
        self, fake_cli: _FakeCli, tmp_path: Path
    ) -> None:
        fake_cli.configure(events=_RESULT_EVENTS)
        prompt = "--dangerously-skip-permissions please"

        result = await _run(tmp_path, _FakeRuntime(), prompt=prompt)

        assert result.error == ""
        assert result.final_content == "done"
        record = fake_cli.record
        assert record["stdin"] == prompt
        assert prompt not in record["argv"]
        assert record["cwd"] == str(tmp_path)
        assert (
            record["argv"]
            == build_cli_command(str(fake_cli.path), max_turns=3, system_prompt="", policy_wait_seconds=75.0)[1:]
        )

    async def test_env_adds_only_the_socket_and_token(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("DATABASE_URL", "postgres://secret")
        monkeypatch.setenv("CODEFORGE_INTERNAL_KEY", "admin")
        monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-test")
        fake_cli.configure(events=_RESULT_EVENTS)
        seen: list[dict[str, str]] = []
        real_exec = asyncio.create_subprocess_exec

        async def spy(*args: str, **kwargs: object) -> asyncio.subprocess.Process:
            if "-p" in args:
                seen.append(dict(kwargs["env"]))  # type: ignore[arg-type]
            return await real_exec(*args, **kwargs)  # type: ignore[arg-type]

        with patch("codeforge.claude_code_executor.asyncio.create_subprocess_exec", spy):
            await _run(tmp_path, _FakeRuntime())

        (env,) = seen
        base = tool_env(passthrough=cce._CLAUDE_CLI_ENV)
        assert set(env) - set(base) == {hook.SOCKET_ENV, hook.TOKEN_ENV}
        assert {k: v for k, v in env.items() if k in base} == base
        assert env["ANTHROPIC_API_KEY"] == "sk-test"
        assert "DATABASE_URL" not in env
        assert "CODEFORGE_INTERNAL_KEY" not in env
        assert env[hook.SOCKET_ENV].endswith(".sock")
        # The socket is gone after the run.
        assert not os.path.exists(env[hook.SOCKET_ENV])

    async def test_hook_calls_are_decided_by_the_runtime(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(
            events=_RESULT_EVENTS,
            tool_calls=[
                {"tool_name": "Bash", "tool_input": {"command": "echo pwned > marker"}},
                {"tool_name": "Read", "tool_input": {"file_path": str(tmp_path / "a.py")}},
            ],
        )
        runtime = _FakeRuntime("deny", "blocked by policy")

        result = await _run(tmp_path, runtime)

        assert result.error == ""
        assert [c["tool"] for c in runtime.calls] == ["Bash", "Read"]
        assert runtime.calls[0]["command"] == "echo pwned > marker"
        assert runtime.calls[1]["path"] == str(tmp_path / "a.py")
        hooks = fake_cli.record["hooks"]
        assert [h["exit"] for h in hooks] == [2, 2]
        assert "blocked by policy" in hooks[0]["stderr"]

    async def test_allowed_hook_call(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(events=_RESULT_EVENTS, tool_calls=[{"tool_name": "Read", "tool_input": {"file_path": "a"}}])

        await _run(tmp_path, _FakeRuntime("allow"))

        (hook_run,) = fake_cli.record["hooks"]
        assert hook_run["exit"] == 0
        assert json.loads(hook_run["stdout"])["hookSpecificOutput"]["permissionDecision"] == "allow"

    async def test_timeout_stops_and_reaps_the_cli(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("CODEFORGE_CLAUDECODE_TIMEOUT", "1")
        get_settings.cache_clear()
        fake_cli.configure(sleep=30)
        started: list[asyncio.subprocess.Process] = []
        real_exec = asyncio.create_subprocess_exec

        async def spy(*args: str, **kwargs: object) -> asyncio.subprocess.Process:
            proc = await real_exec(*args, **kwargs)  # type: ignore[arg-type]
            started.append(proc)
            return proc

        with patch("codeforge.claude_code_executor.asyncio.create_subprocess_exec", spy):
            t0 = time.monotonic()
            result = await _run(tmp_path, _FakeRuntime())

        assert "timed out" in result.error
        assert time.monotonic() - t0 < 15
        assert all(p.returncode is not None for p in started)


class TestCliSupport:
    async def test_supported_cli_is_resolved_and_cached(self, fake_cli: _FakeCli) -> None:
        assert await resolve_cli(str(fake_cli.path)) == str(fake_cli.path)
        assert await resolve_cli(str(fake_cli.path)) == str(fake_cli.path)
        assert fake_cli.help_calls == 1

    async def test_bare_name_is_looked_up_on_path(self, fake_cli: _FakeCli, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("PATH", f"{fake_cli.dir}{os.pathsep}{os.environ.get('PATH', '')}")

        assert await resolve_cli("claude") == str(fake_cli.path)

    async def test_changed_binary_is_checked_again(self, fake_cli: _FakeCli) -> None:
        await resolve_cli(str(fake_cli.path))
        stat_result = os.stat(fake_cli.path)
        os.utime(fake_cli.path, ns=(stat_result.st_atime_ns, stat_result.st_mtime_ns + 1_000_000_000))

        await resolve_cli(str(fake_cli.path))

        assert fake_cli.help_calls == 2

    @pytest.mark.parametrize(
        "missing",
        ["--setting-sources", "--settings", "--strict-mcp-config", "--mcp-config", "--permission-mode", "dontAsk"],
    )
    async def test_missing_flag_fails_closed(self, fake_cli: _FakeCli, tmp_path: Path, missing: str) -> None:
        help_text = "\n".join(line for line in _FULL_HELP.splitlines() if missing not in line)
        fake_cli.configure(help=help_text, events=_RESULT_EVENTS)
        runtime = _FakeRuntime()

        with pytest.raises(ClaudeCodeCLIError, match=missing):
            await resolve_cli(str(fake_cli.path))
        result = await _run(tmp_path, runtime)

        assert missing in result.error
        assert "not supported" in result.error
        assert not fake_cli.ran
        assert result.metadata == {"executor": "claude-code-cli"}

    async def test_help_failure_fails_closed(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(help_exit=3)

        result = await _run(tmp_path, _FakeRuntime())

        assert result.error
        assert not fake_cli.ran

    @pytest.mark.usefixtures("_no_cli_cache")
    async def test_missing_cli_fails_closed(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CODEFORGE_CLAUDECODE_PATH", str(tmp_path / "no-such-claude"))
        get_settings.cache_clear()

        result = await _run(tmp_path, _FakeRuntime())

        assert "not found" in result.error


# ---------------------------------------------------------------------------
# Environment variable configuration tests
# ---------------------------------------------------------------------------


class TestEnvVarConfig:
    """Tests for CODEFORGE_CLAUDECODE_* env var support."""

    @pytest.fixture(autouse=True)
    def _fresh_settings(self, monkeypatch: pytest.MonkeyPatch) -> Iterator[None]:
        """get_settings() is lru_cached: rebuild it from each test's patched env, without a local codeforge.yaml."""
        monkeypatch.setattr("codeforge.config.load_yaml_config", dict)
        get_settings.cache_clear()
        yield
        get_settings.cache_clear()

    def test_default_max_turns(self) -> None:
        from codeforge.claude_code_executor import get_default_max_turns

        with patch.dict("os.environ", {}, clear=False):
            os.environ.pop("CODEFORGE_CLAUDECODE_MAX_TURNS", None)
            assert get_default_max_turns() == 50

    def test_custom_max_turns(self) -> None:
        from codeforge.claude_code_executor import get_default_max_turns

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_MAX_TURNS": "100"}):
            assert get_default_max_turns() == 100

    def test_default_timeout(self) -> None:
        from codeforge.claude_code_executor import get_timeout_seconds

        with patch.dict("os.environ", {}, clear=False):
            os.environ.pop("CODEFORGE_CLAUDECODE_TIMEOUT", None)
            assert get_timeout_seconds() == 300

    def test_custom_timeout(self) -> None:
        from codeforge.claude_code_executor import get_timeout_seconds

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_TIMEOUT": "600"}):
            assert get_timeout_seconds() == 600

    def test_default_tiers(self) -> None:
        from codeforge.claude_code_executor import get_enabled_tiers

        with patch.dict("os.environ", {}, clear=False):
            os.environ.pop("CODEFORGE_CLAUDECODE_TIERS", None)
            assert get_enabled_tiers() == {"COMPLEX", "REASONING"}

    def test_custom_tiers(self) -> None:
        from codeforge.claude_code_executor import get_enabled_tiers

        with patch.dict("os.environ", {"CODEFORGE_CLAUDECODE_TIERS": "SIMPLE,MEDIUM,COMPLEX"}):
            assert get_enabled_tiers() == {"SIMPLE", "MEDIUM", "COMPLEX"}
