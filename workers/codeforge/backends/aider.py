"""Aider backend executor — CLI subprocess wrapper."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, ConfigField, parse_extra_args
from codeforge.backends._cli_base import CLIBackendExecutor, ExecutorConfig
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


class AiderExecutor(CLIBackendExecutor):
    """Execute tasks using the Aider CLI."""

    def __init__(self, cli_path: str | None = None) -> None:
        super().__init__(cli_path, "CODEFORGE_AIDER_PATH", "aider")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="aider",
            display_name="Aider",
            cli_command=self._cli_path,
            capabilities=["code-edit", "git-commit", "multi-file"],
            config_schema=(
                ConfigField(key="model", type=str, description="LLM model name"),
                ConfigField(key="timeout", type=int, default=_DEFAULT_TIMEOUT, description="Timeout in seconds"),
                ConfigField(key="openai_api_base", type=str, description="OpenAI-compatible API base URL"),
                ConfigField(key="extra_args", type=list, description="Extra CLI arguments"),
                ConfigField(key="extra_env", type=dict, description="Extra environment variables for subprocess"),
                ConfigField(key="working_dir_override", type=str, description="Override workspace path"),
            ),
        )

    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        cmd = [self._cli_path, "--yes-always", "--no-auto-commits", "--message", prompt]

        model = config.get("model")
        if model:
            cmd.extend(["--model", model])

        api_base = config.get("openai_api_base")  # type: ignore[typeddict-item]
        if api_base:
            cmd.extend(["--openai-api-base", api_base])

        cmd.extend(parse_extra_args(config))
        return cmd
