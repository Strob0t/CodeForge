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
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from codeforge import claude_code_policy_hook as policy_hook
from codeforge.config import get_settings
from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
)
from codeforge.pricing import resolve_cost
from codeforge.runtime import arguments_preview
from codeforge.subprocess_env import tool_env

if TYPE_CHECKING:
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)

_EXECUTOR_NAME = "claude-code-cli"

# The CLI's own credentials and settings; it gets nothing else from the worker
# except the policy socket and token of its run.
_CLAUDE_CLI_ENV = (
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_BASE_URL",
    "CLAUDE_CODE_OAUTH_TOKEN",
    "CLAUDE_CONFIG_DIR",
)


@dataclass
class _RunAccumulator:
    """Mutable accumulator for token/cost/step data during a run."""

    content_parts: list[str] = field(default_factory=list)
    tool_messages: list[ConversationMessagePayload] = field(default_factory=list)
    total_cost: float = 0.0
    total_tokens_in: int = 0
    total_tokens_out: int = 0
    step_count: int = 0
    model: str = ""


# Claude Code tool arguments that name the file or directory a tool works on,
# in order of preference. Tool names are sent unchanged: the Go policy layer
# maps them to canonical names (internal/domain/policy/toolnames.go).
_POLICY_PATH_KEYS: tuple[str, ...] = ("file_path", "notebook_path", "path")

# Claude Code tools that run a shell command (Go maps both to Bash): their
# command is evaluated by the command rules.
_COMMAND_TOOLS: frozenset[str] = frozenset({"Bash", "Monitor"})

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

# Command line flags the policy enforcement relies on, checked against
# ``claude --help`` before a run. --max-turns is not listed: the CLI hides it
# from --help, and an unknown option makes the CLI exit with an error before
# it runs any tool.
_REQUIRED_CLI_OPTIONS: tuple[str, ...] = (
    "--print",
    "--output-format",
    "--verbose",
    "--settings",
    "--setting-sources",
    "--strict-mcp-config",
    "--mcp-config",
    "--permission-mode",
    "--system-prompt",
    "--tools",
)
# Denies every tool call the hook did not allow (nothing is auto-approved).
_PERMISSION_MODE = "dontAsk"
_CLI_HELP_TIMEOUT_SECONDS = 30.0
_STOP_GRACE_SECONDS = 5.0

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


def policy_request_args(tool_name: str, tool_input: dict[str, object]) -> tuple[str, str, str]:
    """Return the (command, path, arguments preview) sent with a Claude Code tool call.

    Only tools that run a shell command send one; file tools send the file or
    directory they work on. The preview is shown to a human approver only.
    """
    command = ""
    if tool_name in _COMMAND_TOOLS:
        value = tool_input.get("command", "")
        command = value if isinstance(value, str) else ""
    path = ""
    for key in _POLICY_PATH_KEYS:
        value = tool_input.get(key)
        if isinstance(value, str) and value:
            path = value
            break
    return command, path, arguments_preview(tool_input)


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

    def __init__(self, runtime: RuntimeClient, decision_timeout: float) -> None:
        self._runtime = runtime
        self._decision_timeout = decision_timeout
        self._token = secrets.token_urlsafe(32)
        self._dir = ""
        self._server: asyncio.Server | None = None
        self._handlers: set[asyncio.Task[None]] = set()
        self._closed = False

    @property
    def socket_path(self) -> str:
        return os.path.join(self._dir, "policy.sock")

    @property
    def token(self) -> str:
        return self._token

    async def __aenter__(self) -> PolicySocketServer:
        self._dir = tempfile.mkdtemp(prefix="codeforge-claude-")
        try:
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

        command, path, preview = policy_request_args(tool_name, tool_input)
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

        if decision.decision == _ALLOW:
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


def build_cli_command(cli_path: str, *, max_turns: int, system_prompt: str, policy_wait_seconds: float) -> list[str]:
    """Return the command line of a Claude Code run; the prompt goes to stdin.

    On the command line, a prompt that starts with "-" would be parsed as an
    option (``--dangerously-skip-permissions``).
    """
    timeouts = hook_timeouts(policy_wait_seconds)
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
    if system_prompt:
        cmd.extend(["--system-prompt", system_prompt])
    return cmd


class ClaudeCodeCLIError(Exception):
    """The Claude Code CLI is missing or cannot enforce the policy on its tool calls."""


# Outcome of the --help check per CLI binary (real path, mtime): "" when the
# CLI supports every required flag, otherwise the error. An upgraded binary has
# a new mtime and is checked again.
_cli_support_cache: dict[tuple[str, int], str] = {}


async def resolve_cli(cli_path: str) -> str:
    """Return the path of the configured CLI once it is known to support every flag the executor uses.

    Raises ClaudeCodeCLIError when the CLI is missing or lacks a flag: the run
    fails instead of starting the CLI without the policy hook.
    """
    resolved = shutil.which(cli_path)
    if resolved is None:
        raise ClaudeCodeCLIError(f"Claude Code CLI {cli_path!r} not found")
    resolved = os.path.abspath(resolved)
    try:
        key = (os.path.realpath(resolved), os.stat(resolved).st_mtime_ns)
    except OSError as exc:
        raise ClaudeCodeCLIError(f"Claude Code CLI {resolved!r} not found: {exc}") from exc
    error = _cli_support_cache.get(key)
    if error is None:
        error = await _check_cli_help(resolved)
        _cli_support_cache[key] = error
    if error:
        raise ClaudeCodeCLIError(error)
    return resolved


async def _check_cli_help(cli: str) -> str:
    """Return "" when ``cli --help`` lists every required flag, else the error; raise on a failure to run it."""
    try:
        proc = await asyncio.create_subprocess_exec(
            cli,
            "--help",
            stdin=asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            env=tool_env(passthrough=_CLAUDE_CLI_ENV),
        )
    except OSError as exc:
        raise ClaudeCodeCLIError(f"cannot run Claude Code CLI {cli!r}: {exc}") from exc
    try:
        stdout, stderr = await asyncio.wait_for(proc.communicate(), timeout=_CLI_HELP_TIMEOUT_SECONDS)
    except TimeoutError:
        raise ClaudeCodeCLIError(f"Claude Code CLI {cli!r} --help timed out") from None
    finally:
        await _stop_process(proc)
    if proc.returncode != 0:
        detail = stderr.decode(errors="replace").strip()[:500]
        return f"Claude Code CLI {cli!r} --help failed (exit {proc.returncode}): {detail}"
    missing = _missing_cli_options(stdout.decode(errors="replace"))
    if missing:
        return (
            f"Claude Code CLI {cli!r} is not supported: it lacks {', '.join(missing)}, "
            "which CodeForge needs to decide its tool calls by policy. Install a current Claude Code version."
        )
    return ""


def _missing_cli_options(help_text: str) -> list[str]:
    missing = [
        flag for flag in _REQUIRED_CLI_OPTIONS if not re.search(rf"(?<![\w-]){re.escape(flag)}(?![\w-])", help_text)
    ]
    if not re.search(rf"\b{_PERMISSION_MODE}\b", help_text):
        missing.append(_PERMISSION_MODE)
    return missing


async def _stop_process(proc: asyncio.subprocess.Process) -> None:
    """Terminate a subprocess that is still running (kill it if it ignores SIGTERM) and reap it."""
    if proc.returncode is not None:
        return
    with contextlib.suppress(ProcessLookupError):
        proc.terminate()
    try:
        await asyncio.wait_for(proc.wait(), timeout=_STOP_GRACE_SECONDS)
    except TimeoutError:
        with contextlib.suppress(ProcessLookupError):
            proc.kill()
        await proc.wait()


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
        self._process: asyncio.subprocess.Process | None = None

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
        """Signal cancellation and terminate the subprocess if running."""
        self._cancelled = True
        if self._process is not None:
            with contextlib.suppress(ProcessLookupError):
                self._process.terminate()

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
        """Parse a single stream-json event from the CLI output."""
        event_type = event.get("type", "")

        if event_type == "assistant" and "message" in event:
            msg = event["message"]
            if isinstance(msg, dict):
                for block in msg.get("content", []):
                    if block.get("type") == "text":
                        text = block.get("text", "")
                        acc.content_parts.append(text)
                        await self._runtime.send_output(text)
                    elif block.get("type") == "tool_use":
                        acc.step_count += 1

        elif event_type == "result":
            usage = event.get("usage", {})
            if isinstance(usage, dict):
                acc.total_tokens_in += usage.get("input_tokens", 0)
                acc.total_tokens_out += usage.get("output_tokens", 0)
            if event.get("model"):
                acc.model = str(event["model"])
            if event.get("num_turns"):
                acc.step_count = int(event["num_turns"])

    async def _run_via_cli(
        self,
        messages: list[dict[str, str]],
        model: str,
        max_turns: int,
        system_prompt: str,
    ) -> AgentLoopResult:
        """Run via the ``claude`` CLI as a subprocess, every tool call decided by the policy.

        Parses ``--output-format stream-json`` output for content and usage.
        """
        prompt = self._format_messages_as_prompt(messages)
        if not prompt:
            return AgentLoopResult(error="empty prompt")

        acc = _RunAccumulator(model=model or _DEFAULT_MODEL)
        try:
            cli = await resolve_cli(get_settings().claudecode_path)
        except ClaudeCodeCLIError as exc:
            logger.error("Claude Code run not started: %s", exc)
            return AgentLoopResult(error=str(exc), model=acc.model, metadata={"executor": _EXECUTOR_NAME})

        policy_wait = self._runtime.policy_wait_seconds
        cmd = build_cli_command(cli, max_turns=max_turns, system_prompt=system_prompt, policy_wait_seconds=policy_wait)
        error_msg = ""
        stdout = b""
        try:
            async with PolicySocketServer(self._runtime, hook_timeouts(policy_wait).decision) as policy:
                env = tool_env(
                    passthrough=_CLAUDE_CLI_ENV,
                    extra={policy_hook.SOCKET_ENV: policy.socket_path, policy_hook.TOKEN_ENV: policy.token},
                )
                stdout, stderr, returncode = await self._communicate(cmd, env, prompt)
                if returncode != 0:
                    error_msg = stderr.decode(errors="replace").strip() or "non-zero exit"
        except TimeoutError:
            return AgentLoopResult(
                error=f"Claude Code CLI timed out after {get_timeout_seconds()}s",
                model=acc.model,
                metadata={"executor": _EXECUTOR_NAME},
            )
        except OSError as exc:
            error_msg = f"Failed to start Claude Code CLI: {exc}"
            logger.error(error_msg)

        for line in stdout.decode(errors="replace").splitlines():
            line = line.strip()
            if not line:
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if isinstance(event, dict):
                await self._parse_cli_event(event, acc)

        total_cost = self._estimate_equivalent_cost(acc.total_tokens_in, acc.total_tokens_out)

        return AgentLoopResult(
            final_content="\n".join(acc.content_parts),
            total_cost=total_cost,
            total_tokens_in=acc.total_tokens_in,
            total_tokens_out=acc.total_tokens_out,
            step_count=acc.step_count,
            model=acc.model,
            error=error_msg,
            metadata={"executor": _EXECUTOR_NAME},
        )

    async def _communicate(self, cmd: list[str], env: dict[str, str], prompt: str) -> tuple[bytes, bytes, int]:
        """Run the CLI with the prompt on stdin; stop and reap it on timeout or cancellation."""
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
            cwd=self._workspace,
            env=env,
        )
        self._process = process
        try:
            stdout, stderr = await asyncio.wait_for(process.communicate(prompt.encode()), timeout=get_timeout_seconds())
        finally:
            self._process = None
            await _stop_process(process)
        return stdout, stderr, process.returncode if process.returncode is not None else -1
