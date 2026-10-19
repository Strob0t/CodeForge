"""Tests for the Claude Code PreToolUse policy hook (KI-72).

The hook blocks a tool call with exit code 2 on every failure: Claude Code
lets a call run when a hook exits with any other code.
"""

from __future__ import annotations

import io
import json
import os
import socket
import subprocess
import sys
import tempfile
import threading
from pathlib import Path
from typing import TYPE_CHECKING

import pytest

from codeforge import claude_code_policy_hook as hook

if TYPE_CHECKING:
    from collections.abc import Callable, Iterator

HOOK_FILE = Path(hook.__file__)

BASH_CALL = {
    "session_id": "s1",
    "hook_event_name": "PreToolUse",
    "tool_name": "Bash",
    "tool_input": {"command": "echo hi", "description": "say hi"},
    "tool_use_id": "toolu_1",
}


class _PolicySocket:
    """A unix socket that answers each request line with ``respond(request)``."""

    def __init__(self, respond: Callable[[dict[str, object]], bytes]) -> None:
        # A short directory: unix socket paths are limited to ~100 bytes.
        self.dir = tempfile.mkdtemp(prefix="cf-hook-")
        self.path = os.path.join(self.dir, "p.sock")
        self.requests: list[dict[str, object]] = []
        self._respond = respond
        self._sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self._sock.bind(self.path)
        self._sock.listen()
        self._sock.settimeout(0.1)
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._serve, daemon=True)
        self._thread.start()

    def _serve(self) -> None:
        while not self._stop.is_set():
            try:
                conn, _ = self._sock.accept()
            except TimeoutError:
                continue
            except OSError:
                return
            conn.settimeout(None)
            with conn:
                data = b""
                while not data.endswith(b"\n"):
                    chunk = conn.recv(65536)
                    if not chunk:
                        break
                    data += chunk
                request = json.loads(data)
                self.requests.append(request)
                reply = self._respond(request)
                if reply:
                    conn.sendall(reply)

    def close(self) -> None:
        self._stop.set()
        self._thread.join(timeout=5)
        self._sock.close()
        for name in os.listdir(self.dir):
            os.unlink(os.path.join(self.dir, name))
        os.rmdir(self.dir)


def _reply(decision: str, reason: str = "") -> Callable[[dict[str, object]], bytes]:
    return lambda _request: (json.dumps({"decision": decision, "reason": reason}) + "\n").encode()


@pytest.fixture
def policy_socket() -> Iterator[Callable[[Callable[[dict[str, object]], bytes]], _PolicySocket]]:
    servers: list[_PolicySocket] = []

    def start(respond: Callable[[dict[str, object]], bytes]) -> _PolicySocket:
        server = _PolicySocket(respond)
        servers.append(server)
        return server

    yield start
    for server in servers:
        server.close()


def _run_main(
    stdin_text: str,
    environ: dict[str, str],
    argv: list[str] | None = None,
) -> tuple[int, str, str]:
    stdout, stderr = io.StringIO(), io.StringIO()
    code = hook.main(
        argv if argv is not None else [hook.TIMEOUT_ARG, "5"],
        io.StringIO(stdin_text),
        stdout,
        stderr,
        environ,
    )
    return code, stdout.getvalue(), stderr.getvalue()


def _env(server: _PolicySocket, token: str = "tok") -> dict[str, str]:  # noqa: S107 - test token
    return {hook.SOCKET_ENV: server.path, hook.TOKEN_ENV: token}


class TestMain:
    def test_allow_prints_decision_and_exits_0(self, policy_socket) -> None:
        server = policy_socket(_reply("allow", "matched rule 0"))

        code, out, err = _run_main(json.dumps(BASH_CALL), _env(server, "secret-token"))

        assert code == 0
        assert json.loads(out) == {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "allow",
                "permissionDecisionReason": "matched rule 0",
            }
        }
        assert err == ""
        # Only the token, tool name, tool input and tool_use_id go to the socket.
        assert server.requests == [
            {
                "token": "secret-token",
                "tool_name": "Bash",
                "tool_input": BASH_CALL["tool_input"],
                "tool_use_id": "toolu_1",
            },
        ]

    def test_deny_exits_2_with_reason_on_stderr(self, policy_socket) -> None:
        server = policy_socket(_reply("deny", "command matches command_deny"))

        code, out, err = _run_main(json.dumps(BASH_CALL), _env(server))

        assert code == hook.BLOCK_EXIT_CODE == 2
        assert out == ""
        assert "command matches command_deny" in err

    @pytest.mark.parametrize("decision", ["ask", "", "ALLOW", "allow "])
    def test_anything_but_allow_blocks(self, policy_socket, decision: str) -> None:
        server = policy_socket(_reply(decision))

        code, out, _ = _run_main(json.dumps(BASH_CALL), _env(server))

        assert code == 2
        assert out == ""

    @pytest.mark.parametrize(
        "reply",
        [
            b"not json\n",
            b"[1, 2]\n",
            b'{"decision": ["allow"]}\n',
            b'{"decision": "allow"',  # no newline, then the connection closes
            b"",  # closed without an answer
        ],
    )
    def test_bad_response_blocks(self, policy_socket, reply: bytes) -> None:
        server = policy_socket(lambda _request: reply)

        code, out, err = _run_main(json.dumps(BASH_CALL), _env(server))

        assert code == 2
        assert out == ""
        assert "blocked" in err

    def test_no_socket_blocks(self, tmp_path: Path) -> None:
        env = {hook.SOCKET_ENV: str(tmp_path / "missing.sock"), hook.TOKEN_ENV: "tok"}

        code, out, err = _run_main(json.dumps(BASH_CALL), env)

        assert code == 2
        assert out == ""
        assert "blocked" in err

    @pytest.mark.parametrize("missing", [hook.SOCKET_ENV, hook.TOKEN_ENV])
    def test_missing_env_blocks(self, policy_socket, missing: str) -> None:
        server = policy_socket(_reply("allow"))
        env = _env(server)
        env[missing] = ""

        code, _, err = _run_main(json.dumps(BASH_CALL), env)

        assert code == 2
        assert missing in err
        assert server.requests == []

    def test_timeout_blocks(self, policy_socket) -> None:
        release = threading.Event()

        def slow(_request: dict[str, object]) -> bytes:
            release.wait(10)
            return b""

        server = policy_socket(slow)
        try:
            code, out, err = _run_main(json.dumps(BASH_CALL), _env(server), argv=[hook.TIMEOUT_ARG, "0.3"])
        finally:
            release.set()

        assert code == 2
        assert out == ""
        assert "timed out" in err

    @pytest.mark.parametrize(
        "stdin_text",
        [
            "",
            "garbage",
            "[]",
            '"Bash"',
            json.dumps({"tool_input": {"command": "ls"}}),
            json.dumps({"tool_name": "", "tool_input": {}}),
            json.dumps({"tool_name": 7, "tool_input": {}}),
            json.dumps({"tool_name": "Bash", "tool_input": "ls"}),
        ],
    )
    def test_garbage_stdin_blocks(self, policy_socket, stdin_text: str) -> None:
        server = policy_socket(_reply("allow"))

        code, out, err = _run_main(stdin_text, _env(server))

        assert code == 2
        assert out == ""
        assert "blocked" in err
        assert server.requests == []

    def test_tool_use_id_is_forwarded(self, policy_socket) -> None:
        # The executor matches the call's result in the CLI's output by it (KI-161).
        server = policy_socket(_reply("allow"))

        code, _, _ = _run_main(json.dumps(BASH_CALL), _env(server))

        assert code == 0
        assert server.requests[0]["tool_use_id"] == "toolu_1"

    @pytest.mark.parametrize("tool_use_id", [None, 7, ""])
    def test_call_without_a_usable_tool_use_id_is_sent_without_one(self, policy_socket, tool_use_id: object) -> None:
        server = policy_socket(_reply("allow"))
        call = {**BASH_CALL, "tool_use_id": tool_use_id}
        if tool_use_id is None:
            del call["tool_use_id"]

        code, _, _ = _run_main(json.dumps(call), _env(server))

        assert code == 0
        assert "tool_use_id" not in server.requests[0]

    def test_tool_without_input_is_sent_with_empty_input(self, policy_socket) -> None:
        server = policy_socket(_reply("allow"))

        code, _, _ = _run_main(json.dumps({"tool_name": "TodoWrite"}), _env(server))

        assert code == 0
        assert server.requests == [{"token": "tok", "tool_name": "TodoWrite", "tool_input": {}}]

    @pytest.mark.parametrize(
        "argv",
        [[], [hook.TIMEOUT_ARG], [hook.TIMEOUT_ARG, "abc"], [hook.TIMEOUT_ARG, "0"], [hook.TIMEOUT_ARG, "-1"], ["5"]],
    )
    def test_bad_arguments_block(self, policy_socket, argv: list[str]) -> None:
        server = policy_socket(_reply("allow"))

        code, _, _ = _run_main(json.dumps(BASH_CALL), _env(server), argv=argv)

        assert code == 2
        assert server.requests == []


class TestScript:
    """The hook as the CLI runs it: ``python -I <file> --timeout N`` in a bare environment."""

    @staticmethod
    def _run(stdin_text: str, env: dict[str, str], timeout: str = "5") -> subprocess.CompletedProcess[str]:
        return subprocess.run(  # noqa: S603 - runs the hook under test
            [sys.executable, "-I", str(HOOK_FILE), hook.TIMEOUT_ARG, timeout],
            input=stdin_text,
            capture_output=True,
            text=True,
            env={"PATH": os.environ.get("PATH", ""), **env},
            timeout=30,
            check=False,
        )

    def test_allow(self, policy_socket) -> None:
        server = policy_socket(_reply("allow"))

        proc = self._run(json.dumps(BASH_CALL), _env(server))

        assert proc.returncode == 0, proc.stderr
        assert json.loads(proc.stdout)["hookSpecificOutput"]["permissionDecision"] == "allow"

    def test_deny(self, policy_socket) -> None:
        server = policy_socket(_reply("deny", "not in the tools of mode"))

        proc = self._run(json.dumps(BASH_CALL), _env(server))

        assert proc.returncode == 2
        assert "not in the tools of mode" in proc.stderr
        assert proc.stdout == ""

    def test_no_socket(self, tmp_path: Path) -> None:
        proc = self._run(json.dumps(BASH_CALL), {hook.SOCKET_ENV: str(tmp_path / "nope.sock"), hook.TOKEN_ENV: "t"})

        assert proc.returncode == 2
        assert proc.stdout == ""

    def test_timeout(self, policy_socket) -> None:
        release = threading.Event()

        def slow(_request: dict[str, object]) -> bytes:
            release.wait(10)
            return b""

        server = policy_socket(slow)
        try:
            proc = self._run(json.dumps(BASH_CALL), _env(server), timeout="0.5")
        finally:
            release.set()

        assert proc.returncode == 2
        assert proc.stdout == ""

    def test_garbage_stdin(self, policy_socket) -> None:
        server = policy_socket(_reply("allow"))

        proc = self._run("{not json", _env(server))

        assert proc.returncode == 2
        assert server.requests == []

    def test_runs_without_the_worker_packages(self) -> None:
        """Stdlib only: the CLI's environment carries no PYTHONPATH."""
        source = HOOK_FILE.read_text()

        assert "codeforge" not in "\n".join(
            line for line in source.splitlines() if line.startswith(("import ", "from "))
        )
