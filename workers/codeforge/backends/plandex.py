"""Plandex backend executor — CLI subprocess wrapper."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, ConfigField, parse_extra_args
from codeforge.backends._cli_base import CLIBackendExecutor, ExecutorConfig
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


class PlandexExecutor(CLIBackendExecutor):
    """Execute tasks using the Plandex CLI."""

    def __init__(self, cli_path: str | None = None) -> None:
        super().__init__(cli_path, "CODEFORGE_PLANDEX_PATH", "plandex")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="plandex",
            display_name="Plandex",
            cli_command=self._cli_path,
            capabilities=["code-edit", "planning", "multi-file"],
            config_schema=(
                ConfigField(key="model", type=str, description="LLM model name"),
                ConfigField(key="timeout", type=int, default=_DEFAULT_TIMEOUT, description="Timeout in seconds"),
                ConfigField(key="extra_args", type=list, description="Extra CLI arguments"),
                ConfigField(key="extra_env", type=dict, description="Extra environment variables for subprocess"),
                ConfigField(key="working_dir_override", type=str, description="Override workspace path"),
            ),
        )

    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        """Build command list.

        Plandex uses a plan-then-apply workflow. The ``tell`` subcommand
        creates a plan and applies changes in one step when ``--yes`` is set.
        """
        cmd = [self._cli_path, "tell", "--yes", prompt]

        model = config.get("model")
        if model:
            cmd.extend(["--model", model])

        cmd.extend(parse_extra_args(config))
        return cmd
