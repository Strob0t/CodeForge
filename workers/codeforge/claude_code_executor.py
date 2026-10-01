"""ClaudeCodeExecutor — wraps Claude Code (Anthropic CLI agent) as a routing target.

Claude Code is an autonomous agent with its own tool loop. This executor:
- Runs the configured ``claude`` CLI (``claudecode_path``) as a subprocess
  with a scrubbed environment. The claude-code-sdk path is not used: the SDK
  (0.0.25) starts the CLI with ``{**os.environ, **options.env}``, which would
  hand the worker's credentials to the agent.
- Lets the Go policy layer decide every tool call (KI-72): a PreToolUse hook
  (``claude_code_policy_hook``) asks this run's policy socket, which calls
  ``RuntimeClient.request_tool_call`` (mode tool lists, path and command
  rules, HITL approval). The CLI loads no settings files and no MCP servers
  (the workspace is the user's repository) and runs in ``dontAsk`` mode, so
  only the hook's "allow" lets a tool run.
- Parses the CLI's output as it arrives, so a run that times out or is
  cancelled keeps its output and usage. ``claudecode_timeout`` limits the
  run time without the time spent waiting for policy decisions (HITL). On
  timeout or cancel the CLI's process group is stopped.
- Returns results in the standard ``AgentLoopResult`` format
- Guards concurrency with an asyncio semaphore
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import math
import os
import re
import secrets
import shlex
import shutil
import sys
import tempfile
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from codeforge import claude_code_policy_hook as policy_hook
from codeforge.config import get_settings
from codeforge.models import AgentLoopResult
from codeforge.policy_args import policy_request_args
from codeforge.pricing import resolve_cost
from codeforge.runtime import arguments_preview
from codeforge.subprocess_env import tool_env
from codeforge.subprocess_utils import terminate_process_group

if TYPE_CHECKING:
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)

_EXECUTOR_NAME = "claude-code-cli"

# Bash starts every call in the workspace: the CLI otherwise keeps the working
# directory of the previous call, and the policy, which resolves a call's
# relative redirection targets against the workspace, would place
# `echo x > aws.key` after an earlier `cd secrets` in the wrong directory.
_CLI_FIXED_ENV = {"CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR": "1"}

# The CLI's own credentials and settings; it gets nothing else from the worker
# except the fixed settings above and the policy socket and token of its run.
_CLAUDE_CLI_ENV = (
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_BASE_URL",
    "CLAUDE_CODE_OAUTH_TOKEN",
    "CLAUDE_CONFIG_DIR",
)


def _count(value: object) -> int:
    """A token or turn count from CLI output: a non-negative int, anything else is 0."""
    return value if type(value) is int and value > 0 else 0


def _usage(value: object) -> tuple[int, int]:
    if not isinstance(value, dict):
        return 0, 0
    return _count(value.get("input_tokens")), _count(value.get("output_tokens"))


@dataclass
class _RunAccumulator:
    """What a run produced so far: kept when the run fails, times out or is cancelled."""

    model: str
    content_parts: list[str] = field(default_factory=list)
    tool_uses: int = 0
    turns: int = 0
    # Usage per API message (largest seen: the CLI repeats a message per
    # content block), an estimate until a result event reports the totals.
    message_usage: dict[str, tuple[int, int]] = field(default_factory=dict)
    result_usage: tuple[int, int] | None = None

    def record_message_usage(self, message: dict[str, object]) -> None:
        usage = _usage(message.get("usage"))
        if usage == (0, 0):
            return
        message_id = message.get("id")
        key = message_id if isinstance(message_id, str) else f"#{len(self.message_usage)}"
        seen = self.message_usage.get(key, (0, 0))
        self.message_usage[key] = (max(seen[0], usage[0]), max(seen[1], usage[1]))

    def record_result(self, event: dict[str, object]) -> None:
        tokens_in, tokens_out = _usage(event.get("usage"))
        previous = self.result_usage or (0, 0)
        self.result_usage = (previous[0] + tokens_in, previous[1] + tokens_out)
        model = event.get("model")
        if isinstance(model, str) and model:
            self.model = model
        self.turns = _count(event.get("num_turns")) or self.turns

    def tokens(self) -> tuple[int, int]:
        if self.result_usage is not None:
            return self.result_usage
        return sum(u[0] for u in self.message_usage.values()), sum(u[1] for u in self.message_usage.values())

    @property
    def step_count(self) -> int:
        return self.turns or self.tool_uses


# The only tools a Claude Code run gets (--tools) and the only ones the policy
# socket lets Go decide: those the Go policy maps to a built-in tool
# (internal/domain/policy/toolnames.go), so presets, deny lists and mode tool
# lists apply to every call. An unmapped tool would be decided only by a
# preset's default (allow under acceptEdits) and no mode tool list would
# restrict it. WebFetch and WebSearch stay out on purpose: presets restrict
# network access through Bash command rules (curl, wget, ...), which a fetch
# tool would bypass; the agent loop has no web tools either. Names a CLI
# version does not have (MultiEdit, LS in 2.1) are ignored by --tools.
CLAUDE_CODE_TOOLS: tuple[str, ...] = (
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
)

# Largest decision request the policy socket reads: it carries the whole tool
# input (the content of a Write). A larger request is denied.
MAX_POLICY_REQUEST_BYTES = 16 * 1024 * 1024
# The hook writes its request right after connecting.
_REQUEST_READ_TIMEOUT_SECONDS = 10.0

_ALLOW = "allow"
_DENY = "deny"

# Command line options the policy enforcement relies on, checked against
# ``claude --help`` before a run.
_REQUIRED_CLI_OPTIONS: tuple[str, ...] = (
    "--print",
    "--output-format",
    "--verbose",
    "--settings",
    "--setting-sources",
    "--strict-mcp-config",
    "--mcp-config",
    "--permission-mode",
    "--tools",
)
# Options the CLI accepts but does not list in --help (2.1): checked by
# running the CLI with them (see _check_hidden_options).
_HIDDEN_CLI_OPTIONS: tuple[str, ...] = ("--max-turns", "--system-prompt-file")
# Denies every tool call the hook did not allow (nothing is auto-approved).
_PERMISSION_MODE = "dontAsk"
_CLI_CHECK_TIMEOUT_SECONDS = 30.0

# Calls of these tools cannot change the workspace; any other allowed call
# means a failed run may have left it partly modified.
_READ_ONLY_TOOLS = frozenset({"Read", "Grep", "Glob", "LS"})

# Largest stream-json line read from the CLI (a tool result can be big); a
# longer line is skipped.
_MAX_EVENT_LINE_BYTES = 32 * 1024 * 1024
_STDERR_TAIL_BYTES = 64 * 1024
# How often the run is checked for cancellation and its time limit.
_POLL_SECONDS = 0.5
# Time the CLI gets to finish writing and exit after its output ended.
_EXIT_GRACE_SECONDS = 5.0
# sun_path of AF_UNIX addresses holds 108 bytes including the terminating NUL.
_MAX_SOCKET_PATH_BYTES = 107

# Default model for cost estimation when Claude Code doesn't report one.
_DEFAULT_MODEL = "anthropic/claude-sonnet-4"


def get_default_max_turns() -> int:
    """Return the default max_turns from settings."""
    return get_settings().claudecode_max_turns


def get_timeout_seconds() -> int:
    """Return the CLI timeout from settings."""
    return get_settings().claudecode_timeout


def get_enabled_tiers() -> set[str]:
    """Return the set of complexity tiers that include Claude Code.

    Default: ``COMPLEX,REASONING``.
    """
    raw = get_settings().claudecode_tiers
    return {t.strip() for t in raw.split(",") if t.strip()}


# Module-level semaphore (lazy-init to avoid event-loop issues at import time).
_semaphore: asyncio.Semaphore | None = None


def _get_semaphore() -> asyncio.Semaphore:
    """Return the module-level concurrency semaphore, creating it on first use."""
    global _semaphore
    if _semaphore is None:
        _semaphore = asyncio.Semaphore(get_settings().claudecode_max_concurrent)
    return _semaphore


# ----------------------------------------------------------------------
# Policy decisions
# ----------------------------------------------------------------------


@dataclass(frozen=True)
class HookTimeouts:
    """How long each step of a tool call decision may take, in seconds.

    Each step outlasts the one before: the runtime waits for Go longer than
    Go's HITL approval timeout (``policy_wait_seconds``), the policy socket
    denies after ``decision``, the hook blocks the call after ``hook``, and
    the CLI kills the hook after ``cli``. A hook the CLI kills does NOT block
    the call, so the hook must always give up first.
    """

    decision: float
    hook: float
    cli: int


def hook_timeouts(policy_wait_seconds: float) -> HookTimeouts:
    """Return the decision timeouts of a run whose runtime waits ``policy_wait_seconds`` for Go."""
    return HookTimeouts(
        decision=policy_wait_seconds + 10,
        hook=policy_wait_seconds + 20,
        cli=math.ceil(policy_wait_seconds + 30),
    )


class ClaudeCodeCLIError(Exception):
    """The Claude Code CLI is missing or cannot enforce the policy on its tool calls."""


# Base of the runs' private directories: a long TMPDIR would make the socket
# path exceed the AF_UNIX limit. mkdtemp creates a fresh 0700 directory in it.
_SHORT_TMP = "/tmp"  # noqa: S108 - only the base of a mkdtemp() directory


def _socket_base_dir() -> str:
    """Return a short directory for a run's private directory (unix socket paths are short)."""
    return _SHORT_TMP if os.access(_SHORT_TMP, os.W_OK | os.X_OK) else tempfile.gettempdir()


class _BadPolicyRequestError(Exception):
    """A decision request that is denied without asking the policy."""


class PolicySocketServer:
    """Answers the policy hook's decision requests of one Claude Code run.

    Listens on a unix socket in a fresh private directory (0700). Each
    connection carries one JSON line ``{"token", "tool_name", "tool_input"}``
    and gets ``{"decision": "allow"|"deny", "reason"}`` back. The decision is
    the Go policy's (``request_tool_call``, which also waits for a HITL
    approval); a wrong token, a malformed request, an error or a decision that
    takes longer than ``decision_timeout`` is a deny.
    """

    def __init__(self, runtime: RuntimeClient, workspace: str, decision_timeout: float) -> None:
        self._runtime = runtime
        self._workspace = workspace
        self._decision_timeout = decision_timeout
        self._token = secrets.token_urlsafe(32)
        self._dir = ""
        self._server: asyncio.Server | None = None
        self._handlers: set[asyncio.Task[None]] = set()
        self._closed = False
        self._pending_decisions = 0
        self._waiting_since = 0.0
        self._waited = 0.0
        # Allowed calls of tools that can change the workspace.
        self.changes_allowed = 0

    @property
    def directory(self) -> str:
        """The run's private directory (0700), removed with the server."""
        return self._dir

    @property
    def socket_path(self) -> str:
        return os.path.join(self._dir, "policy.sock")

    @property
    def token(self) -> str:
        return self._token

    def waited_seconds(self) -> float:
        """Seconds during which at least one decision was pending (HITL approval waits included)."""
        if self._pending_decisions:
            return self._waited + time.monotonic() - self._waiting_since
        return self._waited

    async def __aenter__(self) -> PolicySocketServer:
        self._dir = tempfile.mkdtemp(prefix="cf-cc-", dir=_socket_base_dir())
        try:
            if len(os.fsencode(self.socket_path)) > _MAX_SOCKET_PATH_BYTES:
                raise ClaudeCodeCLIError(
                    f"policy socket path {self.socket_path!r} is too long for a unix socket "
                    f"(max {_MAX_SOCKET_PATH_BYTES} bytes)"
                )
            self._server = await asyncio.start_unix_server(
                self._handle, path=self.socket_path, limit=MAX_POLICY_REQUEST_BYTES
            )
            os.chmod(self.socket_path, 0o600)
        except BaseException:
            shutil.rmtree(self._dir, ignore_errors=True)
            raise
        return self

    async def __aexit__(self, *exc_info: object) -> None:
        """Stop listening, drop pending decisions and remove the socket directory."""
        self._closed = True
        if self._server is not None:
            self._server.close()
        handlers = list(self._handlers)
        for task in handlers:
            task.cancel()
        await asyncio.gather(*handlers, return_exceptions=True)
        shutil.rmtree(self._dir, ignore_errors=True)

    async def _handle(self, reader: asyncio.StreamReader, writer: asyncio.StreamWriter) -> None:
        task = asyncio.current_task()
        if task is not None:
            self._handlers.add(task)
        try:
            if self._closed:
                return
            decision, reason = await self._decide(reader)
            writer.write((json.dumps({"decision": decision, "reason": reason}) + "\n").encode())
            await writer.drain()
        except OSError as exc:
            # The hook is gone (killed by the CLI or the run ended); it blocks on its own.
            logger.warning("policy hook connection failed: %s", exc)
        finally:
            if task is not None:
                self._handlers.discard(task)
            writer.close()

    async def _decide(self, reader: asyncio.StreamReader) -> tuple[str, str]:
        try:
            line = await asyncio.wait_for(reader.readline(), timeout=_REQUEST_READ_TIMEOUT_SECONDS)
        except ValueError:
            # readline() raises ValueError for a line over MAX_POLICY_REQUEST_BYTES.
            return _DENY, "policy request too large"
        except TimeoutError:
            return _DENY, "policy request timed out"
        try:
            tool_name, tool_input = self._parse_request(line)
        except _BadPolicyRequestError as exc:
            logger.warning("claude code policy request rejected: %s", exc)
            return _DENY, str(exc)
        if tool_name not in CLAUDE_CODE_TOOLS:
            logger.warning("claude code tool %s is not available, denied", tool_name)
            return _DENY, f"tool {tool_name} is not available in CodeForge Claude Code runs"

        command, path = policy_request_args(tool_name, tool_input, self._workspace)
        preview = arguments_preview(tool_input)
        if not self._pending_decisions:
            self._waiting_since = time.monotonic()
        self._pending_decisions += 1
        try:
            decision = await asyncio.wait_for(
                self._runtime.request_tool_call(tool=tool_name, command=command, path=path, arguments_preview=preview),
                timeout=self._decision_timeout,
            )
        except TimeoutError:
            logger.warning("no policy decision for %s within %gs", tool_name, self._decision_timeout)
            return _DENY, f"no policy decision within {self._decision_timeout:g}s"
        except Exception as exc:
            logger.error("claude code policy request failed: tool=%s error=%s", tool_name, exc)
            return _DENY, f"policy check failed: {exc}"
        finally:
            self._pending_decisions -= 1
            if not self._pending_decisions:
                self._waited += time.monotonic() - self._waiting_since

        if decision.decision == _ALLOW:
            if tool_name not in _READ_ONLY_TOOLS:
                self.changes_allowed += 1
            return _ALLOW, decision.reason
        return _DENY, decision.reason or "denied by policy"

    def _parse_request(self, line: bytes) -> tuple[str, dict[str, object]]:
        try:
            request = json.loads(line)
        except ValueError as exc:
            raise _BadPolicyRequestError("malformed policy request") from exc
        if not isinstance(request, dict):
            raise _BadPolicyRequestError("malformed policy request")
        token = request.get("token")
        if not isinstance(token, str) or not secrets.compare_digest(token.encode(), self._token.encode()):
            raise _BadPolicyRequestError("invalid policy token")
        tool_name = request.get("tool_name")
        tool_input = request.get("tool_input")
        if not isinstance(tool_name, str) or not tool_name or not isinstance(tool_input, dict):
            raise _BadPolicyRequestError("malformed policy request")
        return tool_name, tool_input


# ----------------------------------------------------------------------
# Command line and CLI capability check
# ----------------------------------------------------------------------


def _hook_command(timeout: float) -> str:
    # The CLI blocks a call only on exit code 2: "|| exit 2" also blocks when
    # the interpreter itself fails (exit 1, 127, a signal).
    return (
        f"{shlex.quote(sys.executable)} -I {shlex.quote(policy_hook.__file__)} "
        f"{policy_hook.TIMEOUT_ARG} {timeout:g} || exit 2"
    )


def build_cli_command(cli_path: str, *, max_turns: int, system_prompt_file: str, timeouts: HookTimeouts) -> list[str]:
    """Return the command line of a Claude Code run; the prompt goes to stdin.

    On the command line, a prompt that starts with "-" would be parsed as an
    option (``--dangerously-skip-permissions``). The system prompt is read
    from ``system_prompt_file`` (an argument is limited to 128 KiB and shown
    by ps).
    """
    pre_tool_use = {
        "matcher": "*",
        "hooks": [{"type": "command", "command": _hook_command(timeouts.hook), "timeout": timeouts.cli}],
    }
    cmd = [
        cli_path,
        "-p",
        "--output-format",
        "stream-json",
        # -p with stream-json requires --verbose.
        "--verbose",
        "--max-turns",
        str(max_turns),
        # No settings files: .claude/settings*.json in the workspace and the
        # user's settings can allow tools and add hooks that run commands.
        "--setting-sources",
        "",
        "--settings",
        json.dumps({"hooks": {"PreToolUse": [pre_tool_use]}}),
        # No MCP servers from .mcp.json or the user's config.
        "--strict-mcp-config",
        "--mcp-config",
        json.dumps({"mcpServers": {}}),
        "--tools",
        ",".join(CLAUDE_CODE_TOOLS),
        "--permission-mode",
        _PERMISSION_MODE,
    ]
    if system_prompt_file:
        cmd.extend(["--system-prompt-file", system_prompt_file])
    return cmd


def _write_private_file(directory: str, name: str, content: str) -> str:
    """Write ``content`` to a new 0600 file in ``directory``; return its path ("" for no content)."""
    if not content:
        return ""
    path = os.path.join(directory, name)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        f.write(content)
    return path


# CLI binaries (real path, mtime) that passed the capability check. Only a
# success is cached: a failed check (a transient kill, a CLI being upgraded)
# is repeated on the next run. An upgraded binary has a new mtime.
_supported_clis: set[tuple[str, int]] = set()
# One check at a time: concurrent first runs wait for it instead of starting
# their own CLI processes. Created on first use (it binds to the event loop).
_cli_check_lock: asyncio.Lock | None = None


async def resolve_cli(cli_path: str) -> str:
    """Return the path of the configured CLI once it is known to support every option the executor uses.

    Raises ClaudeCodeCLIError when the CLI is missing or lacks an option: the
    run fails instead of starting the CLI without the policy hook.
    """
    global _cli_check_lock
    resolved = shutil.which(cli_path)
    if resolved is None:
        raise ClaudeCodeCLIError(f"Claude Code CLI {cli_path!r} not found")
    resolved = os.path.abspath(resolved)
    try:
        key = (os.path.realpath(resolved), os.stat(resolved).st_mtime_ns)
    except OSError as exc:
        raise ClaudeCodeCLIError(f"Claude Code CLI {resolved!r} not found: {exc}") from exc
    if key in _supported_clis:
        return resolved
    if _cli_check_lock is None:
        _cli_check_lock = asyncio.Lock()
    async with _cli_check_lock:
        if key not in _supported_clis:
            await _check_cli(resolved)
            _supported_clis.add(key)
    return resolved


async def _check_cli(cli: str) -> None:
    """Raise ClaudeCodeCLIError unless the CLI supports every option the executor uses."""
    returncode, output = await _run_check(cli, ["--help"], tool_env(passthrough=_CLAUDE_CLI_ENV))
    if returncode != 0:
        raise ClaudeCodeCLIError(f"Claude Code CLI {cli!r} --help failed (exit {returncode}): {output[:500]}")
    missing = _missing_cli_options(output)
    if missing:
        raise ClaudeCodeCLIError(_unsupported(cli, ", ".join(missing)))
    await _check_hidden_options(cli)


async def _check_hidden_options(cli: str) -> None:
    """Raise ClaudeCodeCLIError when the CLI rejects an option --help does not list.

    Runs the CLI in print mode with those options and a system prompt file
    that does not exist, without credentials, settings or stdin: a CLI that
    knows the options fails on the missing file, one that does not fails
    with "unknown option" before anything else.
    """
    with tempfile.TemporaryDirectory(prefix="cf-cc-check-") as home:
        env = {"PATH": os.environ.get("PATH", ""), "HOME": home, "CLAUDE_CONFIG_DIR": home}
        missing_file = os.path.join(home, "no-system-prompt")
        args = ["-p", "--max-turns", "1", "--system-prompt-file", missing_file]
        _, output = await _run_check(cli, args, env)
    for line in output.splitlines():
        if "unknown option" in line.lower():
            raise ClaudeCodeCLIError(_unsupported(cli, line.strip()))


def _unsupported(cli: str, what: str) -> str:
    return (
        f"Claude Code CLI {cli!r} is not supported ({what}): CodeForge needs "
        f"{', '.join(_REQUIRED_CLI_OPTIONS + _HIDDEN_CLI_OPTIONS)} and permission mode "
        f"{_PERMISSION_MODE} to decide its tool calls by policy. Install a current Claude Code version."
    )


async def _run_check(cli: str, args: list[str], env: dict[str, str]) -> tuple[int, str]:
    """Run the CLI for a capability check; return its exit code and output (stdout, then stderr)."""
    try:
        proc = await asyncio.create_subprocess_exec(
            cli,
            *args,
            stdin=asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            env=env,
            start_new_session=True,
        )
    except OSError as exc:
        raise ClaudeCodeCLIError(f"cannot run Claude Code CLI {cli!r}: {exc}") from exc
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=_CLI_CHECK_TIMEOUT_SECONDS)
    except TimeoutError:
        raise ClaudeCodeCLIError(f"Claude Code CLI {cli!r} {args[0]} timed out") from None
    finally:
        if proc.returncode is None:
            await terminate_process_group(proc)
    output = (stdout + b"\n" + stderr).decode(errors="replace").strip()
    return proc.returncode if proc.returncode is not None else -1, output


def _missing_cli_options(help_text: str) -> list[str]:
    missing = [
        flag for flag in _REQUIRED_CLI_OPTIONS if not re.search(rf"(?<![\w-]){re.escape(flag)}(?![\w-])", help_text)
    ]
    if not re.search(rf"\b{_PERMISSION_MODE}\b", help_text):
        missing.append(_PERMISSION_MODE)
    return missing


class ClaudeCodeExecutor:
    """Run a conversation turn via the Claude Code CLI.

    Parameters
    ----------
    workspace_path:
        Absolute path to the project workspace on disk.
    runtime:
        A ``RuntimeClient`` used for policy enforcement and streaming output.
    """

    def __init__(self, workspace_path: str, runtime: RuntimeClient) -> None:
        self._workspace = workspace_path
        self._runtime = runtime
        self._cancelled = False

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    async def run(
        self,
        messages: list[dict[str, str]],
        model: str = "",
        max_turns: int = 25,
        system_prompt: str = "",
    ) -> AgentLoopResult:
        """Run a conversation turn through Claude Code.

        Acquires a concurrency permit, then runs the CLI subprocess.
        """
        async with _get_semaphore():
            return await self._run_via_cli(messages, model, max_turns, system_prompt)

    async def cancel(self) -> None:
        """Stop the run: its supervision stops the CLI's process group within a poll interval."""
        self._cancelled = True

    # ------------------------------------------------------------------
    # Message formatting
    # ------------------------------------------------------------------

    def _format_messages_as_prompt(self, messages: list[dict[str, str]]) -> str:
        """Convert a conversation message list into a single prompt string.

        System messages are excluded.  If only one non-system message exists,
        its content is returned directly.  For multi-turn conversations, prior
        messages are wrapped in ``<conversation_history>`` tags with
        ``[ROLE]: content`` formatting, while the final message is appended
        without a role prefix.
        """
        non_system = [m for m in messages if m.get("role") != "system"]
        if not non_system:
            return ""
        if len(non_system) == 1:
            return non_system[0].get("content", "")

        history_lines: list[str] = []
        for msg in non_system[:-1]:
            role = msg.get("role", "unknown").upper()
            content = msg.get("content", "")
            history_lines.append(f"[{role}]: {content}")

        last_content = non_system[-1].get("content", "")

        return "<conversation_history>\n" + "\n".join(history_lines) + "\n</conversation_history>\n\n" + last_content

    # ------------------------------------------------------------------
    # Cost estimation
    # ------------------------------------------------------------------

    def _estimate_equivalent_cost(self, tokens_in: int, tokens_out: int) -> float:
        """Estimate cost using the default Claude model pricing.

        Returns 0.0 for zero tokens.  Otherwise delegates to
        ``resolve_cost`` with the default model name.
        """
        if tokens_in == 0 and tokens_out == 0:
            return 0.0
        return resolve_cost(0.0, _DEFAULT_MODEL, tokens_in, tokens_out)

    # ------------------------------------------------------------------
    # CLI path
    # ------------------------------------------------------------------

    async def _parse_cli_event(self, event: dict[str, object], acc: _RunAccumulator) -> None:
        """Take the text, tool uses and usage out of one stream-json event; ignore what does not fit."""
        event_type = event.get("type")
        if event_type == "assistant":
            message = event.get("message")
            if not isinstance(message, dict):
                return
            acc.record_message_usage(message)
            content = message.get("content")
            for block in content if isinstance(content, list) else []:
                if not isinstance(block, dict):
                    continue
                if block.get("type") == "text" and isinstance(block.get("text"), str):
                    await self._emit_text(block["text"], acc)
                elif block.get("type") == "tool_use":
                    acc.tool_uses += 1
        elif event_type == "result":
            acc.record_result(event)

    async def _emit_text(self, text: str, acc: _RunAccumulator) -> None:
        acc.content_parts.append(text)
        try:
            await self._runtime.send_output(text)
        except Exception as exc:
            logger.warning("claude code output not streamed: %s", exc)

    async def _run_via_cli(
        self,
        messages: list[dict[str, str]],
        model: str,
        max_turns: int,
        system_prompt: str,
    ) -> AgentLoopResult:
        """Run via the ``claude`` CLI, every tool call decided by the policy.

        ``fallback_safe`` in the result's metadata tells the caller whether the
        turn may be re-run on another model: not after a cancel, and not once a
        tool call that can change the workspace was allowed.
        """
        acc = _RunAccumulator(model=model or _DEFAULT_MODEL)
        prompt = self._format_messages_as_prompt(messages)
        if not prompt:
            return self._result(acc, "empty prompt", fallback_safe=True)
        try:
            cli = await resolve_cli(get_settings().claudecode_path)
        except ClaudeCodeCLIError as exc:
            logger.error("Claude Code run not started: %s", exc)
            return self._result(acc, str(exc), fallback_safe=True)

        timeouts = hook_timeouts(self._runtime.policy_wait_seconds)
        policy = PolicySocketServer(self._runtime, self._workspace, timeouts.decision)
        try:
            async with policy:
                system_prompt_file = _write_private_file(policy.directory, "system-prompt", system_prompt)
                cmd = build_cli_command(
                    cli, max_turns=max_turns, system_prompt_file=system_prompt_file, timeouts=timeouts
                )
                env = tool_env(
                    passthrough=_CLAUDE_CLI_ENV,
                    extra={
                        **_CLI_FIXED_ENV,
                        policy_hook.SOCKET_ENV: policy.socket_path,
                        policy_hook.TOKEN_ENV: policy.token,
                    },
                )
                end = await self._execute(cmd, env, prompt, acc, policy)
        except (ClaudeCodeCLIError, OSError) as exc:
            error = f"Failed to start Claude Code CLI: {exc}"
            logger.error(error)
            return self._result(acc, error, fallback_safe=policy.changes_allowed == 0)

        error = end.error()
        if error and end.status != _CANCELLED and policy.changes_allowed:
            error += (
                f" The workspace may be partly modified: {policy.changes_allowed} tool call(s) "
                "that can change files were allowed."
            )
        return self._result(acc, error, fallback_safe=end.status != _CANCELLED and policy.changes_allowed == 0)

    def _result(self, acc: _RunAccumulator, error: str, *, fallback_safe: bool) -> AgentLoopResult:
        tokens_in, tokens_out = acc.tokens()
        return AgentLoopResult(
            final_content="\n".join(acc.content_parts),
            total_cost=self._estimate_equivalent_cost(tokens_in, tokens_out),
            total_tokens_in=tokens_in,
            total_tokens_out=tokens_out,
            step_count=acc.step_count,
            model=acc.model,
            error=error,
            metadata={"executor": _EXECUTOR_NAME, "fallback_safe": fallback_safe},
        )

    async def _execute(
        self,
        cmd: list[str],
        env: dict[str, str],
        prompt: str,
        acc: _RunAccumulator,
        policy: PolicySocketServer,
    ) -> _RunEnd:
        """Run the CLI with the prompt on stdin, parsing its output as it arrives.

        The CLI runs in a process group of its own; whatever of the group
        still runs when the run ends (timeout, cancel, a caller's
        cancellation) is stopped, including the commands its tools started.
        """
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=self._workspace,
            env=env,
            start_new_session=True,
            limit=_MAX_EVENT_LINE_BYTES,
        )
        feed = asyncio.create_task(_feed_stdin(process, prompt))
        events = asyncio.create_task(self._read_events(process, acc))
        errors = asyncio.create_task(_read_tail(process.stderr))
        try:
            status = await self._watch(process, events, policy)
            if status == _EXITED:
                with contextlib.suppress(TimeoutError):
                    await asyncio.wait_for(process.wait(), timeout=_EXIT_GRACE_SECONDS)
        except BaseException:
            errors.cancel()
            raise
        finally:
            if process.returncode is None or not events.done():
                # Still running, or something of its group holds its output open.
                await terminate_process_group(process)
            feed.cancel()
            events.cancel()
            await asyncio.gather(feed, events, return_exceptions=True)
        try:
            stderr = await asyncio.wait_for(errors, timeout=_EXIT_GRACE_SECONDS)
        except TimeoutError:
            stderr = ""
        returncode = process.returncode if process.returncode is not None else -1
        return _RunEnd(status=status, returncode=returncode, stderr=stderr)

    async def _watch(
        self, process: asyncio.subprocess.Process, events: asyncio.Task[None], policy: PolicySocketServer
    ) -> str:
        """Wait until the CLI's output ends, the run is cancelled or its run time is used up.

        Run time excludes the time spent waiting for policy decisions, so HITL
        approvals do not count against ``claudecode_timeout``.
        """
        limit = get_timeout_seconds()
        started = time.monotonic()
        exited = asyncio.ensure_future(process.wait())
        try:
            while not events.done():
                if self._cancelled or self._runtime.is_cancelled:
                    return _CANCELLED
                if time.monotonic() - started - policy.waited_seconds() > limit:
                    return _TIMEOUT
                if exited.done():
                    # The CLI is gone: give the reader time to take what it wrote.
                    with contextlib.suppress(TimeoutError):
                        await asyncio.wait_for(asyncio.shield(events), timeout=_EXIT_GRACE_SECONDS)
                    return _EXITED
                await asyncio.wait({events, exited}, timeout=_POLL_SECONDS)
            return _EXITED
        finally:
            exited.cancel()

    async def _read_events(self, process: asyncio.subprocess.Process, acc: _RunAccumulator) -> None:
        stdout = process.stdout
        if stdout is None:
            return
        while True:
            try:
                raw = await stdout.readline()
            except ValueError:
                logger.warning("skipped a Claude Code output line over %d bytes", _MAX_EVENT_LINE_BYTES)
                continue
            if not raw:
                return
            await self._handle_line(raw, acc)

    async def _handle_line(self, raw: bytes, acc: _RunAccumulator) -> None:
        try:
            event = json.loads(raw)
        except ValueError:
            return
        if not isinstance(event, dict):
            return
        try:
            await self._parse_cli_event(event, acc)
        except Exception as exc:
            # One odd event must not lose the rest of the turn's output.
            logger.warning("skipped a Claude Code output event: %s", exc)


_EXITED = "exited"
_TIMEOUT = "timeout"
_CANCELLED = "cancelled"


@dataclass(frozen=True)
class _RunEnd:
    """How the CLI run ended: exited (with returncode), timeout or cancelled."""

    status: str
    returncode: int
    stderr: str

    def error(self) -> str:
        if self.status == _CANCELLED:
            return "cancelled"
        if self.status == _TIMEOUT:
            return f"Claude Code CLI timed out after {get_timeout_seconds()}s of run time (approval waits not counted)."
        if self.returncode != 0:
            return self.stderr or f"Claude Code CLI exited with code {self.returncode}."
        return ""


async def _feed_stdin(process: asyncio.subprocess.Process, prompt: str) -> None:
    stdin = process.stdin
    if stdin is None:
        return
    try:
        stdin.write(prompt.encode())
        await stdin.drain()
    except (BrokenPipeError, ConnectionResetError):
        pass  # the CLI exited early; its exit code and stderr tell why
    finally:
        stdin.close()


async def _read_tail(stream: asyncio.StreamReader | None) -> str:
    """Read a stream to its end; return its last _STDERR_TAIL_BYTES as text."""
    if stream is None:
        return ""
    tail = b""
    while chunk := await stream.read(65536):
        tail = (tail + chunk)[-_STDERR_TAIL_BYTES:]
    return tail.decode(errors="replace").strip()
