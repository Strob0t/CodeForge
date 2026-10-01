"""Template Method base class for CLI-based backend executors.

All five CLI backends (Aider, Goose, OpenCode, Plandex, SWE-agent) share
identical subprocess management: start process, stream stdout line-by-line,
handle timeout/cancel, collect output, return TaskResult. The only
differences are (a) the BackendInfo metadata and (b) how the command list
is built from the prompt and config.

Subclasses implement ``info`` and ``_build_command`` (~30 lines each).
Everything else lives here.
"""

from __future__ import annotations

import asyncio
import logging
from abc import ABC, abstractmethod
from typing import ClassVar, TypedDict

from codeforge.backends._base import BackendInfo, OutputCallback, TaskResult
from codeforge.config import resolve_backend_path
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS
from codeforge.subprocess_env import tool_env
from codeforge.subprocess_utils import check_cli_available, terminate_process_group
from codeforge.tool_process import start_tool_process

logger = logging.getLogger(__name__)

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS

# LLM provider credentials and endpoints the agent CLIs read directly. They
# are the backend's own credentials: the agent inside can read them, but not
# the worker's (tool_env never passes CODEFORGE_*, LITELLM_*, DATABASE_URL or
# NATS_URL). Anything else a backend needs goes into its extra_env config.
PROVIDER_ENV: tuple[str, ...] = (
    "OPENAI_API_KEY",
    "OPENAI_API_BASE",
    "OPENAI_BASE_URL",
    "OPENAI_ORGANIZATION",
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_BASE_URL",
    "GEMINI_API_KEY",
    "GOOGLE_API_KEY",
    "OPENROUTER_API_KEY",
    "GROQ_API_KEY",
    "MISTRAL_API_KEY",
    "DEEPSEEK_API_KEY",
    "XAI_API_KEY",
    "COHERE_API_KEY",
    "TOGETHERAI_API_KEY",
    "AZURE_API_KEY",
    "AZURE_API_BASE",
    "AZURE_API_VERSION",
    "OLLAMA_API_BASE",
    "OLLAMA_HOST",
)


class ExecutorConfig(TypedDict, total=False):
    """Typed configuration dict accepted by all CLI backend executors."""

    timeout: int
    model: str
    extra_args: list[str]
    extra_env: dict[str, str]
    working_dir_override: str


class CLIBackendExecutor(ABC):
    """Template Method base for CLI backends.

    Subclasses must implement:
    - ``info`` property returning ``BackendInfo``
    - ``_build_command(prompt, config)`` returning the CLI argument list
    """

    # The backend's own configuration variables (e.g. AIDER_*), passed to its
    # subprocess together with PROVIDER_ENV.
    env_prefixes: ClassVar[tuple[str, ...]] = ()
    env_names: ClassVar[tuple[str, ...]] = ()

    def __init__(self, cli_path: str | None, env_var: str, default_cmd: str) -> None:
        self._cli_path = resolve_backend_path(cli_path, env_var, default_cmd)
        self._processes: dict[str, asyncio.subprocess.Process] = {}

    @property
    @abstractmethod
    def info(self) -> BackendInfo: ...

    @abstractmethod
    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        """Build the full CLI argument list for execution.

        Implementations should use ``parse_extra_args(config)`` to append
        user-supplied extra arguments.
        """
        ...

    async def check_available(self) -> bool:
        """Check if the CLI tool is installed and reachable."""
        return await check_cli_available(self._cli_path)

    async def execute(
        self,
        task_id: str,
        prompt: str,
        workspace_path: str,
        config: ExecutorConfig | None = None,
        on_output: OutputCallback | None = None,
    ) -> TaskResult:
        """Run the CLI tool with the given prompt in the workspace directory."""
        effective_config: ExecutorConfig = config or {}  # type: ignore[assignment]
        timeout = effective_config.get("timeout", _DEFAULT_TIMEOUT)
        cwd = effective_config.get("working_dir_override") or workspace_path
        extra_env: dict[str, str] = effective_config.get("extra_env") or {}

        cmd = self._build_command(prompt, effective_config)
        name = self.info.name

        logger.info("%s exec task=%s cmd=%s cwd=%s", name, task_id, cmd[:4], cwd)

        try:
            proc = await start_tool_process(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                cwd=cwd or None,
                env=tool_env(
                    passthrough=PROVIDER_ENV + self.env_names,
                    passthrough_prefixes=self.env_prefixes,
                    extra=extra_env,
                ),
                # Its own process group: stopping the task stops everything
                # the agent started, not just the CLI.
                start_new_session=True,
            )
        except OSError as exc:
            return TaskResult(status="failed", error=f"Failed to start {name}: {exc}")

        self._processes[task_id] = proc
        output_lines: list[str] = []

        try:
            stdout = proc.stdout
            if stdout is None:
                return TaskResult(status="failed", error=f"Failed to capture {name} stdout")
            while True:
                try:
                    line_bytes = await asyncio.wait_for(stdout.readline(), timeout=timeout)
                except TimeoutError:
                    # The finally below stops the process group.
                    return TaskResult(
                        status="failed",
                        output="\n".join(output_lines),
                        error=f"{self.info.display_name} timed out after {timeout}s",
                    )

                if not line_bytes:
                    break

                line = line_bytes.decode("utf-8", errors="replace").rstrip("\n")
                output_lines.append(line)

                if on_output is not None:
                    await on_output(line)

            await proc.wait()

            output = "\n".join(output_lines)
            if proc.returncode == 0:
                return TaskResult(status="completed", output=output)
            return TaskResult(
                status="failed",
                output=output,
                error=f"{self.info.display_name} exited with code {proc.returncode}",
            )
        finally:
            self._processes.pop(task_id, None)
            if proc.returncode is None:
                # Timed out, cancelled (tasks.cancel, worker shutdown) or
                # failed while the agent runs.
                await terminate_process_group(proc)

    async def cancel(self, task_id: str) -> None:
        """Terminate the running CLI process and everything it started."""
        proc = self._processes.get(task_id)
        if proc is not None and proc.returncode is None:
            await terminate_process_group(proc)
            logger.info("%s process group terminated for task %s", self.info.name, task_id)
