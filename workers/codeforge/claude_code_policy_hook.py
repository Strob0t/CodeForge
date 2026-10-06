"""PreToolUse hook of Claude Code runs: asks the worker for the policy decision.

The Claude Code CLI starts this script before every tool call of a
``claudecode/*`` run (see ``claude_code_executor``) and passes the call as
JSON on stdin. The script forwards tool name and input to the run's policy
socket, which the executor serves by asking the Go policy layer
(``RuntimeClient.request_tool_call``), and prints the answer.

Claude Code's hook contract: exit 0 with a ``permissionDecision`` of "allow"
on stdout lets the call run; exit 2 blocks it and shows stderr to the model.
Any other exit code (and a hook the CLI kills on its timeout) does NOT block
the call, so every failure here, including a missing socket, a timeout or
malformed input, exits 2.

The script runs as ``python -I <this file>``: stdlib only, because the CLI's
environment has neither PYTHONPATH nor the worker's packages.
"""

from __future__ import annotations

import json
import os
import socket
import sys
import time
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from collections.abc import Mapping, Sequence
    from typing import TextIO

# Environment variables the executor sets for the CLI (and so for its hooks).
SOCKET_ENV = "CODEFORGE_CLAUDE_POLICY_SOCKET"
TOKEN_ENV = "CODEFORGE_CLAUDE_POLICY_TOKEN"  # noqa: S105 - a variable name, not a secret

TIMEOUT_ARG = "--timeout"
BLOCK_EXIT_CODE = 2

_MAX_RESPONSE_BYTES = 64 * 1024


class PolicyHookError(Exception):
    """The decision could not be obtained; the tool call is blocked."""


def ask_policy(
    socket_path: str,
    token: str,
    tool_name: str,
    tool_input: dict[str, object],
    timeout: float,
    tool_use_id: str = "",
) -> tuple[str, str]:
    """Send one decision request to the policy socket and return (decision, reason).

    ``tool_use_id`` (when the CLI gave one) lets the executor match the call's
    result in the CLI's output, which completes the chat's tool card.
    """
    body: dict[str, object] = {"token": token, "tool_name": tool_name, "tool_input": tool_input}
    if tool_use_id:
        body["tool_use_id"] = tool_use_id
    request = json.dumps(body) + "\n"
    deadline = time.monotonic() + timeout
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as sock:
        sock.settimeout(timeout)
        sock.connect(socket_path)
        sock.sendall(request.encode())
        response = _read_line(sock, deadline)
    try:
        payload = json.loads(response)
    except ValueError as exc:
        raise PolicyHookError(f"malformed policy response: {exc}") from exc
    if not isinstance(payload, dict):
        raise PolicyHookError("malformed policy response: not a JSON object")
    decision = payload.get("decision")
    if not isinstance(decision, str):
        raise PolicyHookError("malformed policy response: no decision")
    reason = payload.get("reason")
    return decision, reason if isinstance(reason, str) else ""


def _read_line(sock: socket.socket, deadline: float) -> bytes:
    data = b""
    while b"\n" not in data:
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise PolicyHookError("timed out waiting for the policy decision")
        sock.settimeout(remaining)
        chunk = sock.recv(4096)
        if not chunk:
            raise PolicyHookError("policy socket closed without a decision")
        data += chunk
        if len(data) > _MAX_RESPONSE_BYTES:
            raise PolicyHookError("policy response too large")
    return data.split(b"\n", 1)[0]


def _parse_timeout(argv: Sequence[str]) -> float:
    if len(argv) != 2 or argv[0] != TIMEOUT_ARG:
        raise PolicyHookError(f"usage: {TIMEOUT_ARG} <seconds>")
    try:
        timeout = float(argv[1])
    except ValueError as exc:
        raise PolicyHookError(f"invalid timeout {argv[1]!r}") from exc
    if not timeout > 0:
        raise PolicyHookError(f"invalid timeout {argv[1]!r}")
    return timeout


def _read_tool_call(stdin: TextIO) -> tuple[str, dict[str, object], str]:
    """Return the call's tool name, input and tool_use_id ("" when the CLI gave none)."""
    try:
        hook_input = json.loads(stdin.read())
    except ValueError as exc:
        raise PolicyHookError(f"malformed hook input: {exc}") from exc
    if not isinstance(hook_input, dict):
        raise PolicyHookError("malformed hook input: not a JSON object")
    tool_name = hook_input.get("tool_name")
    tool_input = hook_input.get("tool_input", {})
    if not isinstance(tool_name, str) or not tool_name:
        raise PolicyHookError("malformed hook input: no tool_name")
    if not isinstance(tool_input, dict):
        raise PolicyHookError("malformed hook input: tool_input is not an object")
    tool_use_id = hook_input.get("tool_use_id")
    return tool_name, tool_input, tool_use_id if isinstance(tool_use_id, str) else ""


def main(argv: Sequence[str], stdin: TextIO, stdout: TextIO, stderr: TextIO, environ: Mapping[str, str]) -> int:
    """Decide one tool call; return the hook's exit code (0 allow, 2 block)."""
    try:
        timeout = _parse_timeout(argv)
        tool_name, tool_input, tool_use_id = _read_tool_call(stdin)
        socket_path = environ.get(SOCKET_ENV, "")
        token = environ.get(TOKEN_ENV, "")
        if not socket_path or not token:
            raise PolicyHookError(f"{SOCKET_ENV} or {TOKEN_ENV} is not set")
        decision, reason = ask_policy(socket_path, token, tool_name, tool_input, timeout, tool_use_id)
    except Exception as exc:
        print(f"CodeForge policy check failed, tool call blocked: {exc}", file=stderr)
        return BLOCK_EXIT_CODE

    if decision != "allow":
        print(f"Denied by CodeForge policy: {reason or 'no reason given'}", file=stderr)
        return BLOCK_EXIT_CODE
    output = {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "allow",
            "permissionDecisionReason": reason or "allowed by CodeForge policy",
        }
    }
    print(json.dumps(output), file=stdout)
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:], sys.stdin, sys.stdout, sys.stderr, os.environ))
