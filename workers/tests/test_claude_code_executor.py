"""Tests for ClaudeCodeExecutor."""

from __future__ import annotations

import asyncio
import json
import os
import shlex
import stat
import sys
import tempfile
import time
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock, patch

import pytest

from codeforge import claude_code_executor as cce
from codeforge import claude_code_policy_hook as hook
from codeforge.claude_code_executor import (
    CLAUDE_CODE_TOOLS,
    ClaudeCodeCLIError,
    ClaudeCodeExecutor,
    PolicySocketServer,
    build_cli_command,
    hook_timeouts,
    resolve_cli,
)
from codeforge.config import get_settings
from codeforge.models import ToolCallDecision
from codeforge.subprocess_env import tool_env

if TYPE_CHECKING:
    from collections.abc import Iterator
    from pathlib import Path


# Workspace of the socket server tests (need not exist: paths are resolved as far as they do).
_WS = "/ws"


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
        self.reports_result: list[bool] = []
        self.results: list[dict[str, object]] = []
        self.report_exc: Exception | None = None
        self.outputs: list[str] = []
        self.is_cancelled = False

    async def request_tool_call(
        self,
        tool: str,
        command: str = "",
        path: str = "",
        arguments_preview: str = "",
        reports_result: bool = True,
    ) -> ToolCallDecision:
        self.calls.append({"tool": tool, "command": command, "path": path, "arguments_preview": arguments_preview})
        self.reports_result.append(reports_result)
        if self.delay:
            await asyncio.sleep(self.delay)
        if self.exc is not None:
            raise self.exc
        return ToolCallDecision(call_id="c1", decision=self.decision, reason=self.reason)

    async def report_tool_result(
        self, call_id: str, tool: str, success: bool, output: str = "", error: str = "", **_kwargs: object
    ) -> None:
        if self.report_exc is not None:
            raise self.report_exc
        self.results.append({"call_id": call_id, "tool": tool, "success": success, "output": output, "error": error})

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


class TestPolicyRequestMapping:
    """The socket sends Claude Code's own tool names (Go maps them) with the shared mapping (codeforge.policy_args)."""

    async def test_paths_are_relative_to_the_workspace(self) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, "Read", {"file_path": f"{_WS}/src/a.py"}))
            await _ask(server, _request(server, "Read", {"file_path": "/etc/passwd"}))
            await _ask(server, _request(server, "Glob", {"pattern": "/home/worker/.ssh/*"}))
            await _ask(server, _request(server, "Grep", {"pattern": "TODO"}))

        assert [c["path"] for c in runtime.calls] == [
            "src/a.py",
            os.path.realpath("/etc/passwd"),
            os.path.realpath("/home/worker/.ssh"),
            ".",
        ]

    @pytest.mark.parametrize("tool", ["Bash", "Monitor"])
    async def test_command_tools_send_their_command(self, tool: str) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, tool, {"command": "rm -rf /", "description": "cleanup"}))

        assert runtime.calls == [
            {
                "tool": tool,
                "command": "rm -rf /",
                "path": "",
                "arguments_preview": '{"command": "rm -rf /", "description": "cleanup"}',
            }
        ]


# The chat's live tool card of a Claude Code call completes with the call's
# result (KI-161): a denied call is reported at once with its reason, an
# allowed one when the CLI's output carries its tool_result (matched by the
# tool_use_id the hook forwards).
class TestPolicySocketServerResults:
    async def test_denied_call_is_reported_with_its_reason(self) -> None:
        runtime = _FakeRuntime("deny", "command matches command_deny")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, "Bash", {"command": "curl evil"}, tool_use_id="toolu_1"))

        assert runtime.reports_result == [True]
        assert runtime.results == [
            {
                "call_id": "c1",
                "tool": "Bash",
                "success": False,
                "output": "",
                "error": "Permission denied: command matches command_deny",
            }
        ]

    @pytest.mark.parametrize(
        ("block", "success", "output", "error"),
        [
            ({"content": "file contents"}, True, "file contents", ""),
            (
                {"content": [{"type": "text", "text": "a"}, {"type": "image"}, {"type": "text", "text": "b"}]},
                True,
                "a\nb",
                "",
            ),
            ({"content": "No such file", "is_error": True}, False, "", "No such file"),
            ({"is_error": True}, False, "", "tool call failed"),
            ({"content": "x" * 600}, True, "x" * 500, ""),
        ],
    )
    async def test_allowed_call_is_reported_when_the_cli_reports_its_result(
        self, block: dict[str, object], success: bool, output: str, error: str
    ) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, "Read", {"file_path": f"{_WS}/a.py"}, tool_use_id="toolu_1"))
            assert runtime.results == []
            await server.report_cli_result({"type": "tool_result", "tool_use_id": "toolu_1", **block})

        assert runtime.reports_result == [True]
        assert runtime.results == [
            {"call_id": "c1", "tool": "Read", "success": success, "output": output, "error": error}
        ]

    async def test_a_result_is_reported_once(self) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, "Read", {"file_path": "a"}, tool_use_id="toolu_1"))
            await server.report_cli_result({"type": "tool_result", "tool_use_id": "toolu_1", "content": "ok"})
            await server.report_cli_result({"type": "tool_result", "tool_use_id": "toolu_1", "content": "again"})

        assert len(runtime.results) == 1

    @pytest.mark.parametrize(
        "block",
        [
            {"type": "tool_result", "tool_use_id": "toolu_unknown", "content": "x"},
            {"type": "tool_result", "content": "no id"},
            {"type": "tool_result", "tool_use_id": 7},
            {"type": "text", "text": "not a result"},
        ],
    )
    async def test_results_of_no_awaited_call_are_ignored(self, block: dict[str, object]) -> None:
        # A denied call's synthesized error result among them: it was reported already.
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await server.report_cli_result(block)

        assert runtime.results == []

    async def test_denied_call_result_from_the_cli_is_not_reported_twice(self) -> None:
        runtime = _FakeRuntime("deny", "no")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, "Bash", {"command": "ls"}, tool_use_id="toolu_1"))
            await server.report_cli_result({"type": "tool_result", "tool_use_id": "toolu_1", "is_error": True})

        assert len(runtime.results) == 1

    async def test_call_without_a_tool_use_id_gets_no_card_and_no_report(self) -> None:
        # Its result could not be matched: Go shows no card (it would stay running).
        runtime = _FakeRuntime("deny", "no")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))
            await _ask(server, _request(server, "Read", {"file_path": "a"}, tool_use_id=""))

        assert reply == {"decision": "deny", "reason": "no"}
        assert runtime.reports_result == [False, False]
        assert runtime.results == []

    async def test_a_failed_report_does_not_change_the_decision(self) -> None:
        runtime = _FakeRuntime("deny", "no")
        runtime.report_exc = RuntimeError("nats down")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}, tool_use_id="toolu_1"))
            await _ask(server, _request(server, "Read", {"file_path": "a"}, tool_use_id="toolu_2"))
            await server.report_cli_result({"type": "tool_result", "tool_use_id": "toolu_2", "content": "ok"})

        assert reply == {"decision": "deny", "reason": "no"}

    async def test_calls_without_a_result_are_logged_at_exit(self, caplog: pytest.LogCaptureFixture) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            await _ask(server, _request(server, "Read", {"file_path": "a"}, tool_use_id="toolu_1"))

        assert runtime.results == []
        assert "1 allowed tool call(s) without a result" in caplog.text


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


def _request(
    server: PolicySocketServer,
    tool_name: str,
    tool_input: dict[str, object],
    token: str = "",
    tool_use_id: str | None = None,
) -> bytes:
    body: dict[str, object] = {"token": token or server.token, "tool_name": tool_name, "tool_input": tool_input}
    if tool_use_id is not None:
        body["tool_use_id"] = tool_use_id
    return (json.dumps(body) + "\n").encode()


class TestPolicySocketServer:
    async def test_allow(self) -> None:
        runtime = _FakeRuntime("allow", "matched rule 3")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Read", {"file_path": "/ws/a.py"}))

        assert reply == {"decision": "allow", "reason": "matched rule 3"}
        assert runtime.calls == [
            {"tool": "Read", "command": "", "path": "a.py", "arguments_preview": '{"file_path": "/ws/a.py"}'},
        ]

    async def test_deny_passes_the_reason(self) -> None:
        runtime = _FakeRuntime("deny", "command matches command_deny")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "curl evil"}))

        assert reply == {"decision": "deny", "reason": "command matches command_deny"}
        assert runtime.calls[0]["command"] == "curl evil"

    @pytest.mark.parametrize(("decision", "reason"), [("allow", "approved by user"), ("deny", "denied by user")])
    async def test_hitl_result_passes_through(self, decision: str, reason: str) -> None:
        # request_tool_call returns only once a human decided (Go waits for the approval).
        runtime = _FakeRuntime(decision, reason, delay=0.3)
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Write", {"file_path": "/ws/x", "content": "y"}))

        assert reply == {"decision": decision, "reason": reason}

    @pytest.mark.parametrize("decision", ["ask", "", "ALLOW", "unknown"])
    async def test_anything_but_allow_is_denied(self, decision: str) -> None:
        async with PolicySocketServer(_FakeRuntime(decision), _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))

        assert reply["decision"] == "deny"

    @pytest.mark.parametrize("token", ["wrong", "x" * 43, "ééé"])
    async def test_wrong_token_is_denied(self, token: str) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
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
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, line)

        assert reply["decision"] == "deny"
        assert runtime.calls == []

    @pytest.mark.parametrize(
        ("tool_name", "tool_input"),
        [("", {}), (None, {}), ("Bash", "ls"), ("Bash", None), ("Bash", ["ls"])],
    )
    async def test_bad_tool_fields_are_denied(self, tool_name: object, tool_input: object) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            body = {"token": server.token, "tool_name": tool_name, "tool_input": tool_input}
            reply = await _ask(server, (json.dumps(body) + "\n").encode())

        assert reply["decision"] == "deny"
        assert runtime.calls == []

    # Only tools the Go policy maps to a built-in tool are decided by Go: an
    # unmapped tool would escape mode tool lists and be allowed by the default
    # of permissive presets (WebFetch while Bash denies curl).
    @pytest.mark.parametrize(
        "tool_name",
        ["WebFetch", "WebSearch", "Task", "Agent", "TodoWrite", "Workflow", "REPL", "PowerShell", "mcp__x__y", "bash"],
    )
    async def test_unmapped_tool_is_denied_without_asking_go(self, tool_name: str) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, tool_name, {"url": "https://evil.example/?q=secret"}))

        assert reply["decision"] == "deny"
        assert tool_name in str(reply["reason"])
        assert runtime.calls == []

    @pytest.mark.parametrize("tool_name", CLAUDE_CODE_TOOLS)
    async def test_mapped_tools_are_asked(self, tool_name: str) -> None:
        runtime = _FakeRuntime("allow")
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, tool_name, {}))

        assert reply["decision"] == "allow"
        assert [c["tool"] for c in runtime.calls] == [tool_name]

    async def test_request_tool_call_raising_is_denied(self) -> None:
        runtime = _FakeRuntime(exc=RuntimeError("nats down"))
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))

        assert reply["decision"] == "deny"
        assert "nats down" in str(reply["reason"])

    async def test_slow_decision_is_denied_after_the_decision_timeout(self) -> None:
        runtime = _FakeRuntime("allow", delay=5)
        async with PolicySocketServer(runtime, _WS, decision_timeout=0.2) as server:
            reply = await _ask(server, _request(server, "Bash", {"command": "ls"}))

        assert reply["decision"] == "deny"

    async def test_oversized_request_is_denied(self) -> None:
        runtime = _FakeRuntime("allow")
        big: dict[str, object] = {"file_path": "/ws/big", "content": "x" * (cce.MAX_POLICY_REQUEST_BYTES + 10)}
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
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
        async with PolicySocketServer(runtime, _WS, decision_timeout=5) as server:
            replies = await asyncio.gather(
                *(_ask(server, _request(server, "Read", {"file_path": f"/ws/{i}"})) for i in range(5))
            )

        assert [r["decision"] for r in replies] == ["allow"] * 5
        assert sorted(c["path"] for c in runtime.calls) == [str(i) for i in range(5)]

    async def test_socket_is_private_and_removed_on_exit(self) -> None:
        async with PolicySocketServer(_FakeRuntime(), _WS, decision_timeout=5) as server:
            path = server.socket_path
            directory = os.path.dirname(path)
            assert stat.S_IMODE(os.stat(directory).st_mode) == 0o700
            assert stat.S_ISSOCK(os.stat(path).st_mode)
            assert len(server.token) >= 32

        assert not os.path.exists(path)
        assert not os.path.exists(directory)

    async def test_each_run_gets_its_own_token_and_socket(self) -> None:
        async with (
            PolicySocketServer(_FakeRuntime(), _WS, decision_timeout=5) as first,
            PolicySocketServer(_FakeRuntime(), _WS, decision_timeout=5) as second,
        ):
            assert first.token != second.token
            assert first.socket_path != second.socket_path

    async def test_exit_does_not_wait_for_a_pending_decision(self) -> None:
        runtime = _FakeRuntime("allow", delay=30)
        started = time.monotonic()
        async with PolicySocketServer(runtime, _WS, decision_timeout=60) as server:
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
    def _cmd(self, system_prompt_file: str = "", policy_wait: float = 75.0) -> list[str]:
        return build_cli_command(
            "/opt/bin/claude",
            max_turns=7,
            system_prompt_file=system_prompt_file,
            timeouts=hook_timeouts(policy_wait),
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

    def test_tools_are_limited_to_the_tools_go_maps(self) -> None:
        cmd = self._cmd()
        i = cmd.index("--tools")
        assert cmd[i + 1].split(",") == list(CLAUDE_CODE_TOOLS)
        # --tools takes several values: the next argument must be an option.
        assert cmd[i + 2].startswith("--")
        assert set(CLAUDE_CODE_TOOLS) == {
            "Read",
            "Write",
            "Edit",
            "MultiEdit",
            "NotebookEdit",
            "Bash",
            "Grep",
            "Glob",
            "LS",
            "Monitor",
        }
        for network_tool in ("WebFetch", "WebSearch"):
            assert network_tool not in cmd[i + 1]

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

    def test_system_prompt_file(self) -> None:
        # The system prompt goes through a private file, not an argv entry
        # (MAX_ARG_STRLEN is 128 KiB, and argv is visible in ps).
        cmd = self._cmd(system_prompt_file="/tmp/cf-cc-x/system-prompt")
        assert cmd[cmd.index("--system-prompt-file") + 1] == "/tmp/cf-cc-x/system-prompt"
        assert "--system-prompt" not in cmd
        assert "--system-prompt-file" not in self._cmd()


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
  --tools <tools...>          Specify the list of available tools from the built-in set
"""

_FAKE_CLI = """#!{python}
import json, os, subprocess, sys, time
HERE = os.path.dirname(os.path.abspath(__file__))
cfg = json.load(open(os.path.join(HERE, "fake_cli.json")))
if sys.argv[1:] == ["--help"]:
    with open(os.path.join(HERE, "help_calls"), "a") as f:
        f.write("x\\n")
    time.sleep(cfg.get("help_sleep", 0))
    print(cfg["help"])
    sys.exit(cfg.get("help_exit", 0))
for opt in cfg.get("unknown_options", []):
    if opt in sys.argv:
        print("error: unknown option '" + opt + "'", file=sys.stderr)
        sys.exit(1)
record = {{"argv": sys.argv[1:], "cwd": os.getcwd(), "hooks": []}}
if "--system-prompt-file" in sys.argv:
    spf = sys.argv[sys.argv.index("--system-prompt-file") + 1]
    if not os.path.exists(spf):
        with open(os.path.join(HERE, "probe_calls"), "a") as f:
            f.write(json.dumps(sorted(os.environ)) + "\\n")
        print("Error: System prompt file not found: " + spf, file=sys.stderr)
        sys.exit(1)
    record["system_prompt"] = open(spf).read()
    record["system_prompt_mode"] = oct(os.stat(spf).st_mode & 0o777)
    record["system_prompt_file"] = spf
record["stdin"] = sys.stdin.read()
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
if cfg.get("spawn_child"):
    child = subprocess.Popen(["sleep", "60"])
    with open(os.path.join(HERE, "child.pid"), "w") as f:
        f.write(str(child.pid))
for line in cfg.get("raw", []):
    print(line, flush=True)
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

    def child_alive(self) -> bool:
        """Whether the background child the fake CLI started still runs (read from /proc, no signal)."""
        pid = (self.dir / "child.pid").read_text().strip()
        assert pid.isdigit()
        try:
            with open(f"/proc/{pid}/stat") as f:
                proc_stat = f.read()
        except FileNotFoundError:
            return False
        # A zombie waits only for its parent (or init) to reap it: it runs nothing.
        return proc_stat.rsplit(")", 1)[1].split()[0] != "Z"

    @property
    def probe_envs(self) -> list[list[str]]:
        calls = self.dir / "probe_calls"
        return [json.loads(line) for line in calls.read_text().splitlines()] if calls.exists() else []


@pytest.fixture
def _no_cli_cache(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(cce, "_supported_clis", set())
    monkeypatch.setattr(cce, "_cli_check_lock", None)


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
            == build_cli_command(str(fake_cli.path), max_turns=3, system_prompt_file="", timeouts=hook_timeouts(75.0))[
                1:
            ]
        )

    async def test_system_prompt_goes_through_a_private_file(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(events=_RESULT_EVENTS)
        system_prompt = "-You are the coder mode. " + "x" * 200_000

        result = await _run(tmp_path, _FakeRuntime(), system_prompt=system_prompt)

        assert result.error == ""
        record = fake_cli.record
        assert record["system_prompt"] == system_prompt
        assert record["system_prompt_mode"] == "0o600"
        assert all(system_prompt[:30] not in arg for arg in record["argv"])
        assert not os.path.exists(record["system_prompt_file"])

    async def test_env_adds_only_the_socket_token_and_fixed_cwd(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("DATABASE_URL", "postgres://secret")
        monkeypatch.setenv("CODEFORGE_INTERNAL_KEY", "admin")
        monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-test")
        fake_cli.configure(events=_RESULT_EVENTS)
        seen: list[dict[str, str]] = []
        real_exec = asyncio.create_subprocess_exec

        async def spy(*args: str, **kwargs: object) -> asyncio.subprocess.Process:
            if "--settings" in args:  # the run, not the capability check
                seen.append(dict(kwargs["env"]))  # type: ignore[arg-type]
            return await real_exec(*args, **kwargs)  # type: ignore[arg-type]

        with patch("codeforge.claude_code_executor.asyncio.create_subprocess_exec", spy):
            await _run(tmp_path, _FakeRuntime())

        (env,) = seen
        base = tool_env(passthrough=cce._CLAUDE_CLI_ENV)
        assert set(env) - set(base) == {
            hook.SOCKET_ENV,
            hook.TOKEN_ENV,
            "CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR",
            "DISABLE_AUTOUPDATER",
        }
        assert {k: v for k, v in env.items() if k in base} == base
        # Bash starts every call in the workspace (S6-G review, item 1).
        assert env["CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR"] == "1"
        # The image pins the CLI (KI-118): it never installs another version.
        assert env["DISABLE_AUTOUPDATER"] == "1"
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
        assert runtime.calls[1]["path"] == "a.py"
        hooks = fake_cli.record["hooks"]
        assert [h["exit"] for h in hooks] == [2, 2]
        assert "blocked by policy" in hooks[0]["stderr"]

    async def test_tool_results_of_the_cli_complete_the_cards(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(
            events=[
                {
                    "type": "user",
                    "message": {
                        "content": [
                            {"type": "tool_result", "tool_use_id": "toolu_1", "content": "a.py: print()"},
                            {"type": "tool_result", "tool_use_id": "toolu_2", "content": "ls: no", "is_error": True},
                        ]
                    },
                },
                *_RESULT_EVENTS,
            ],
            tool_calls=[
                {"tool_name": "Read", "tool_input": {"file_path": "a.py"}, "tool_use_id": "toolu_1"},
                {"tool_name": "Bash", "tool_input": {"command": "ls"}, "tool_use_id": "toolu_2"},
            ],
        )
        runtime = _FakeRuntime("allow")

        result = await _run(tmp_path, runtime)

        assert result.error == ""
        assert runtime.reports_result == [True, True]
        assert runtime.results == [
            {"call_id": "c1", "tool": "Read", "success": True, "output": "a.py: print()", "error": ""},
            {"call_id": "c1", "tool": "Bash", "success": False, "output": "", "error": "ls: no"},
        ]

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


def _set_timeout(monkeypatch: pytest.MonkeyPatch, seconds: int) -> None:
    monkeypatch.setenv("CODEFORGE_CLAUDECODE_TIMEOUT", str(seconds))
    get_settings.cache_clear()


async def _wait_for(predicate: object, timeout: float = 10.0) -> None:
    deadline = time.monotonic() + timeout
    while not predicate():  # type: ignore[operator]
        assert time.monotonic() < deadline, "condition not met in time"
        await asyncio.sleep(0.05)


_TEXT = {"type": "assistant", "message": {"id": "m1", "content": [{"type": "text", "text": "partial answer"}]}}
_USAGE = {
    "type": "assistant",
    "message": {
        "id": "m1",
        "content": [{"type": "tool_use", "id": "t1", "name": "Bash", "input": {}}],
        "usage": {"input_tokens": 1200, "output_tokens": 300},
    },
}


class TestRunSupervision:
    """Process group, run-time limit without approval waits, partial results, cancel, fallback safety."""

    async def test_timeout_stops_the_whole_process_group(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        _set_timeout(monkeypatch, 1)
        fake_cli.configure(spawn_child=True, sleep=30)

        result = await _run(tmp_path, _FakeRuntime())

        assert "timed out" in result.error
        await _wait_for(lambda: not fake_cli.child_alive())

    async def test_partial_output_and_usage_survive_a_timeout(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        _set_timeout(monkeypatch, 1)
        fake_cli.configure(events=[_TEXT, _USAGE], sleep=30)
        runtime = _FakeRuntime()

        result = await _run(tmp_path, runtime)

        assert "timed out" in result.error
        assert result.final_content == "partial answer"
        assert runtime.outputs == ["partial answer"]
        assert (result.total_tokens_in, result.total_tokens_out) == (1200, 300)
        assert result.total_cost == _make_executor()._estimate_equivalent_cost(1200, 300)
        assert result.step_count == 1

    async def test_result_usage_replaces_the_per_message_estimate(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        result_event = {"type": "result", "usage": {"input_tokens": 5000, "output_tokens": 900}, "num_turns": 4}
        fake_cli.configure(events=[_TEXT, _USAGE, _USAGE, result_event])

        result = await _run(tmp_path, _FakeRuntime())

        assert (result.total_tokens_in, result.total_tokens_out) == (5000, 900)
        assert result.step_count == 4

    async def test_approval_wait_does_not_count_against_the_timeout(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        _set_timeout(monkeypatch, 1)
        fake_cli.configure(events=_RESULT_EVENTS, tool_calls=[{"tool_name": "Write", "tool_input": {"file_path": "a"}}])
        # Go answers only after a human approved (2 s): longer than the run's time limit.
        runtime = _FakeRuntime("allow", delay=2.0)

        result = await _run(tmp_path, runtime)

        assert result.error == ""
        assert result.final_content == "done"

    async def test_runtime_cancel_stops_the_process_group(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(spawn_child=True, events=[_TEXT], sleep=30)
        runtime = _FakeRuntime()

        async def cancel_soon() -> None:
            await _wait_for(lambda: (fake_cli.dir / "child.pid").exists())
            runtime.is_cancelled = True

        t0 = time.monotonic()
        result, _ = await asyncio.gather(_run(tmp_path, runtime), cancel_soon())

        assert result.error == "cancelled"
        assert result.final_content == "partial answer"
        assert result.metadata["fallback_safe"] is False
        assert time.monotonic() - t0 < 15
        await _wait_for(lambda: not fake_cli.child_alive())

    async def test_executor_cancel_stops_the_process_group(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(spawn_child=True, sleep=30)
        executor = ClaudeCodeExecutor(workspace_path=str(tmp_path), runtime=_FakeRuntime())  # type: ignore[arg-type]

        async def cancel_soon() -> None:
            await _wait_for(lambda: (fake_cli.dir / "child.pid").exists())
            await executor.cancel()

        result, _ = await asyncio.gather(executor.run([{"role": "user", "content": "go"}], max_turns=3), cancel_soon())

        assert result.error == "cancelled"
        await _wait_for(lambda: not fake_cli.child_alive())

    async def test_failure_after_an_allowed_change_is_not_safe_to_retry(
        self, fake_cli: _FakeCli, tmp_path: Path
    ) -> None:
        fake_cli.configure(
            tool_calls=[{"tool_name": "Write", "tool_input": {"file_path": "a.py", "content": "x"}}], exit=1
        )

        result = await _run(tmp_path, _FakeRuntime("allow"))

        assert result.error
        assert "workspace may be partly modified" in result.error
        assert result.metadata["fallback_safe"] is False

    @pytest.mark.parametrize(
        ("decision", "tool_name"),
        [("deny", "Write"), ("allow", "Read"), ("allow", "Grep")],
    )
    async def test_failure_without_an_applied_change_is_safe_to_retry(
        self, fake_cli: _FakeCli, tmp_path: Path, decision: str, tool_name: str
    ) -> None:
        fake_cli.configure(tool_calls=[{"tool_name": tool_name, "tool_input": {"file_path": "a.py"}}], exit=1)

        result = await _run(tmp_path, _FakeRuntime(decision))

        assert result.error
        assert "partly modified" not in result.error
        assert result.metadata["fallback_safe"] is True

    async def test_odd_events_never_lose_the_output(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        odd: list[object] = [
            {"type": "assistant", "message": "not a dict"},
            {"type": "assistant", "message": {"content": "not a list"}},
            {"type": "assistant", "message": {"content": [None, 5, {"type": "text", "text": 7}]}},
            {"type": "assistant", "message": {"content": [], "usage": {"input_tokens": "9", "output_tokens": None}}},
            {"type": "assistant", "message": {"content": [], "usage": {"input_tokens": True, "output_tokens": -4}}},
            {"type": "result", "usage": "x", "num_turns": "3", "model": 5},
            {"type": 42},
            {"message": {"content": [{"type": "text", "text": "no type"}]}},
            _TEXT,
        ]
        fake_cli.configure(raw=["garbage", "[1, 2]", '"a string"', "", "{not json"], events=odd)
        runtime = _FakeRuntime()

        result = await _run(tmp_path, runtime)

        assert result.error == ""
        assert result.final_content == "partial answer"
        assert (result.total_tokens_in, result.total_tokens_out) == (0, 0)

    async def test_output_line_over_the_limit_is_skipped(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setattr(cce, "_MAX_EVENT_LINE_BYTES", 64 * 1024)
        huge = {"type": "user", "message": {"content": [{"type": "tool_result", "content": "x" * 200_000}]}}
        fake_cli.configure(events=[huge, _TEXT])

        result = await _run(tmp_path, _FakeRuntime())

        assert result.error == ""
        assert result.final_content == "partial answer"

    async def test_send_output_failure_keeps_the_content(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(events=[_TEXT])
        runtime = _FakeRuntime()
        runtime.send_output = AsyncMock(side_effect=RuntimeError("nats down"))  # type: ignore[method-assign]

        result = await _run(tmp_path, runtime)

        assert result.error == ""
        assert result.final_content == "partial answer"


class TestSocketDirectory:
    async def test_long_tmpdir_does_not_break_the_socket(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        long_dir = tmp_path / ("d" * 120)
        long_dir.mkdir()
        monkeypatch.setenv("TMPDIR", str(long_dir))
        monkeypatch.setattr(tempfile, "tempdir", None)

        async with PolicySocketServer(_FakeRuntime(), _WS, decision_timeout=5) as server:
            reply = await _ask(server, _request(server, "Read", {"file_path": "/ws/a"}))

        assert len(server.socket_path) < 108
        assert reply["decision"] == "allow"

    async def test_too_long_socket_path_is_a_clear_error(self, monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
        long_dir = tmp_path / ("d" * 120)
        long_dir.mkdir()
        monkeypatch.setattr(cce, "_socket_base_dir", lambda: str(long_dir))

        with pytest.raises(ClaudeCodeCLIError, match="too long"):
            async with PolicySocketServer(_FakeRuntime(), _WS, decision_timeout=5):
                pass

        assert os.listdir(long_dir) == []

    async def test_run_reports_a_too_long_socket_path(
        self, fake_cli: _FakeCli, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        long_dir = tmp_path / ("d" * 120)
        long_dir.mkdir()
        monkeypatch.setattr(cce, "_socket_base_dir", lambda: str(long_dir))
        fake_cli.configure(events=_RESULT_EVENTS)

        result = await _run(tmp_path, _FakeRuntime())

        assert "too long" in result.error
        assert not fake_cli.ran
        assert result.metadata["fallback_safe"] is True


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
        [
            "--setting-sources",
            "--settings",
            "--strict-mcp-config",
            "--mcp-config",
            "--permission-mode",
            "dontAsk",
            "--tools",
        ],
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
        assert result.metadata == {"executor": "claude-code-cli", "fallback_safe": True}

    async def test_help_failure_fails_closed(self, fake_cli: _FakeCli, tmp_path: Path) -> None:
        fake_cli.configure(help_exit=3)

        result = await _run(tmp_path, _FakeRuntime())

        assert result.error
        assert not fake_cli.ran

    async def test_only_success_is_cached(self, fake_cli: _FakeCli) -> None:
        # A failed check (a transient OOM kill, a CLI being upgraded) is retried on the next run.
        fake_cli.configure(help_exit=3)
        with pytest.raises(ClaudeCodeCLIError):
            await resolve_cli(str(fake_cli.path))
        fake_cli.configure()

        assert await resolve_cli(str(fake_cli.path)) == str(fake_cli.path)
        assert await resolve_cli(str(fake_cli.path)) == str(fake_cli.path)
        assert fake_cli.help_calls == 2

    async def test_concurrent_first_runs_check_once(self, fake_cli: _FakeCli) -> None:
        fake_cli.configure(help_sleep=0.3)

        paths = await asyncio.gather(*(resolve_cli(str(fake_cli.path)) for _ in range(5)))

        assert paths == [str(fake_cli.path)] * 5
        assert fake_cli.help_calls == 1

    @pytest.mark.parametrize("hidden", ["--system-prompt-file", "--max-turns"])
    async def test_hidden_option_unknown_fails_closed(self, fake_cli: _FakeCli, tmp_path: Path, hidden: str) -> None:
        """--help does not list these options; the check runs the CLI with them and fails on "unknown option"."""
        fake_cli.configure(unknown_options=[hidden], events=_RESULT_EVENTS)

        with pytest.raises(ClaudeCodeCLIError, match=hidden):
            await resolve_cli(str(fake_cli.path))
        result = await _run(tmp_path, _FakeRuntime())

        assert hidden in result.error
        assert not fake_cli.ran

    async def test_hidden_option_probe_has_no_credentials(
        self, fake_cli: _FakeCli, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("ANTHROPIC_API_KEY", "sk-live")
        monkeypatch.setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth")

        await resolve_cli(str(fake_cli.path))

        (env_names,) = fake_cli.probe_envs
        assert "ANTHROPIC_API_KEY" not in env_names
        assert "CLAUDE_CODE_OAUTH_TOKEN" not in env_names
        assert not fake_cli.ran

    @pytest.mark.usefixtures("_no_cli_cache")
    async def test_missing_cli_fails_closed(self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
        monkeypatch.setenv("CODEFORGE_CLAUDECODE_PATH", str(tmp_path / "no-such-claude"))
        get_settings.cache_clear()

        result = await _run(tmp_path, _FakeRuntime())

        # The public worker image ships without the CLI (owner decision
        # 2026-10-06): the error says how to get one that has it.
        assert result.error.startswith(
            "Claude Code CLI not installed in this worker image; build with --build-arg INSTALL_CLAUDE_CODE=true"
        )
        assert "no-such-claude" in result.error
        assert result.metadata["fallback_safe"] is True


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
