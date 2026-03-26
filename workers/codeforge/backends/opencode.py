"""OpenCode backend executor — CLI subprocess wrapper."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, ConfigField, parse_extra_args
from codeforge.backends._cli_base import CLIBackendExecutor, ExecutorConfig
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


class OpenCodeExecutor(CLIBackendExecutor):
    """Execute tasks using the OpenCode CLI."""

    def __init__(self, cli_path: str | None = None) -> None:
        super().__init__(cli_path, "CODEFORGE_OPENCODE_PATH", "opencode")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="opencode",
            display_name="OpenCode",
            cli_command=self._cli_path,
            capabilities=["code-edit", "lsp"],
            config_schema=(
                ConfigField(key="model", type=str, description="LLM model name"),
                ConfigField(key="timeout", type=int, default=_DEFAULT_TIMEOUT, description="Timeout in seconds"),
                ConfigField(key="extra_args", type=list, description="Extra CLI arguments"),
                ConfigField(key="extra_env", type=dict, description="Extra environment variables for subprocess"),
                ConfigField(key="working_dir_override", type=str, description="Override workspace path"),
            ),
        )

    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        cmd = [self._cli_path, "run", "--prompt", prompt]

        model = config.get("model")
        if model:
            cmd.extend(["--model", model])

        cmd.extend(parse_extra_args(config))
        return cmd
