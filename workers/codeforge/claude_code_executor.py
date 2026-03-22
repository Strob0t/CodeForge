"""ClaudeCodeExecutor — wraps Claude Code (Anthropic CLI agent) as a routing target.

Claude Code is an autonomous agent with its own tool loop. This executor:
- Tries the Python SDK first, falls back to CLI subprocess
- Enforces CodeForge policy via ``can_use_tool`` callback (SDK) or post-hoc (CLI)
- Returns results in the standard ``AgentLoopResult`` format
- Guards concurrency with an asyncio semaphore
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

from codeforge.config import get_settings
from codeforge.models import (
    AgentLoopResult,
    ConversationMessagePayload,
    ConversationToolCallFunction,
    ConversationToolCallPayload,
)
from codeforge.pricing import resolve_cost

if TYPE_CHECKING:
    from codeforge.runtime import RuntimeClient

logger = logging.getLogger(__name__)


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


# Claude Code tool name -> CodeForge policy category
_MAP_TOOL_TO_POLICY: dict[str, str] = {
    "Bash": "command:execute",
    "Read": "file:read",
    "Write": "file:write",
    "Edit": "file:edit",
    "MultiEdit": "file:edit",
    "Search": "file:read",
    "Glob": "file:read",
    "ListDir": "file:read",
}

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


class ClaudeCodeExecutor:
    """Run a conversation turn via Claude Code (SDK or CLI fallback).

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

        Acquires a concurrency permit, then tries the SDK path.  If the SDK
        is not installed (``ImportError``), falls back to the CLI subprocess.
        """
        async with _get_semaphore():
            try:
                return await self._run_via_sdk(messages, model, max_turns, system_prompt)
            except ImportError:
                logger.info("claude-code-sdk not installed, falling back to CLI")
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
    # SDK path
    # ------------------------------------------------------------------

    async def _handle_sdk_assistant_block(
        self,
        block: object,
        acc: _RunAccumulator,
    ) -> None:
        """Process a single content block from an SDK AssistantMessage."""
        from claude_code_sdk.types import TextBlock, ToolResultBlock, ToolUseBlock

        if isinstance(block, TextBlock):
            acc.content_parts.append(block.text)
            await self._runtime.send_output(block.text)
        elif isinstance(block, ToolUseBlock):
            acc.step_count += 1
            arguments = json.dumps(block.input) if isinstance(block.input, dict) else str(block.input)
            tool_call = ConversationToolCallPayload(
                id=block.id,
                function=ConversationToolCallFunction(name=block.name, arguments=arguments),
            )
            acc.tool_messages.append(
                ConversationMessagePayload(role="assistant", tool_calls=[tool_call]),
            )
        elif isinstance(block, ToolResultBlock):
            acc.tool_messages.append(
                ConversationMessagePayload(
                    role="tool",
                    content=str(block.content) if block.content else "",
                    tool_call_id=block.tool_use_id,
                ),
            )

    @staticmethod
    def _handle_sdk_result(message: object, acc: _RunAccumulator) -> None:
        """Extract cost, tokens, and model from a ResultMessage."""
        if hasattr(message, "cost_usd") and message.cost_usd:
            acc.total_cost = float(message.cost_usd)
        if hasattr(message, "usage"):
            acc.total_tokens_in = getattr(message.usage, "input_tokens", 0) or 0
            acc.total_tokens_out = getattr(message.usage, "output_tokens", 0) or 0
        if hasattr(message, "num_turns"):
            acc.step_count = message.num_turns or acc.step_count
        if hasattr(message, "model") and message.model:
            acc.model = message.model

    async def _run_via_sdk(
        self,
        messages: list[dict[str, str]],
        model: str,
        max_turns: int,
        system_prompt: str,
    ) -> AgentLoopResult:
        """Run via the ``claude-code-sdk`` Python package.

        Imports are done inside the method so the rest of the module works
        even when the SDK is not installed.
        """
        from claude_code_sdk import ClaudeCodeOptions, query
        from claude_code_sdk.types import AssistantMessage, ResultMessage

        prompt = self._format_messages_as_prompt(messages)
        if not prompt:
            return AgentLoopResult(error="empty prompt")

        options = ClaudeCodeOptions(
            system_prompt=system_prompt or None,
            max_turns=max_turns,
            permission_mode="bypassPermissions",
            cwd=self._workspace,
        )

        acc = _RunAccumulator(model=model or _DEFAULT_MODEL)

        async for message in query(prompt=prompt, options=options):
            if self._cancelled or self._runtime.is_cancelled:
                break
            if isinstance(message, AssistantMessage):
                for block in message.content:
                    await self._handle_sdk_assistant_block(block, acc)
            elif isinstance(message, ResultMessage):
                self._handle_sdk_result(message, acc)

        if acc.total_cost == 0.0:
            acc.total_cost = self._estimate_equivalent_cost(acc.total_tokens_in, acc.total_tokens_out)

        return AgentLoopResult(
            final_content="\n".join(acc.content_parts),
            tool_messages=acc.tool_messages,
            total_cost=acc.total_cost,
            total_tokens_in=acc.total_tokens_in,
            total_tokens_out=acc.total_tokens_out,
            step_count=acc.step_count,
            model=acc.model,
            metadata={"executor": "claude-code-sdk"},
        )

    # ------------------------------------------------------------------
    # CLI fallback path
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
        """Run via the ``claude`` CLI as a subprocess.

        Parses ``--output-format stream-json`` output for content and usage.
        """
        prompt = self._format_messages_as_prompt(messages)
        if not prompt:
            return AgentLoopResult(error="empty prompt")

        cmd: list[str] = [
            "claude",
            "-p",
            prompt,
            "--output-format",
            "stream-json",
            "--max-turns",
            str(max_turns),
        ]
        if system_prompt:
            cmd.extend(["--system-prompt", system_prompt])

        acc = _RunAccumulator(model=model or _DEFAULT_MODEL)
        error_msg = ""

        try:
            self._process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=self._workspace,
            )

            stdout, stderr = await asyncio.wait_for(
                self._process.communicate(),
                timeout=get_timeout_seconds(),
            )

            if self._process.returncode != 0:
                error_msg = stderr.decode(errors="replace").strip() if stderr else "non-zero exit"

            if stdout:
                for line in stdout.decode(errors="replace").splitlines():
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        event = json.loads(line)
                    except json.JSONDecodeError:
                        continue
                    await self._parse_cli_event(event, acc)

        except TimeoutError:
            if self._process is not None and self._process.returncode is None:
                self._process.terminate()
            return AgentLoopResult(
                error=f"Claude Code CLI timed out after {get_timeout_seconds()}s",
                model=acc.model,
                metadata={"executor": "claude-code-cli"},
            )
        except OSError as exc:
            error_msg = f"Failed to start Claude Code CLI: {exc}"
            logger.error(error_msg)
        finally:
            self._process = None

        total_cost = self._estimate_equivalent_cost(acc.total_tokens_in, acc.total_tokens_out)

        return AgentLoopResult(
            final_content="\n".join(acc.content_parts),
            total_cost=total_cost,
            total_tokens_in=acc.total_tokens_in,
            total_tokens_out=acc.total_tokens_out,
            step_count=acc.step_count,
            model=acc.model,
            error=error_msg,
            metadata={"executor": "claude-code-cli"},
        )

    # ------------------------------------------------------------------
    # Policy callback (for SDK path)
    # ------------------------------------------------------------------

    def _make_policy_callback(self):
        """Create a ``can_use_tool`` callback that enforces CodeForge policy.

        SDK types are imported inside the returned function to avoid
        import failures when the SDK is not installed.
        """
        runtime = self._runtime

        async def _policy_callback(
            tool_name: str,
            tool_input: dict[str, object],
        ):
            from claude_code_sdk.types import PermissionResultAllow, PermissionResultDeny

            category = _MAP_TOOL_TO_POLICY.get(tool_name, f"claude-code:{tool_name}")
            command = ""
            path = ""

            if isinstance(tool_input, dict):
                command = str(tool_input.get("command", ""))
                path = str(tool_input.get("path", tool_input.get("file_path", "")))

            decision = await runtime.request_tool_call(
                tool=category,
                command=command,
                path=path,
            )

            if decision.decision == "allow":
                return PermissionResultAllow()
            return PermissionResultDeny(message=decision.reason or "denied by policy")

        return _policy_callback
