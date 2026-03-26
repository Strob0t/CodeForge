"""SWE-agent backend executor -- CLI subprocess wrapper."""

from __future__ import annotations

from codeforge.backends._base import BackendInfo, ConfigField, parse_extra_args
from codeforge.backends._cli_base import CLIBackendExecutor, ExecutorConfig
from codeforge.constants import DEFAULT_BACKEND_TIMEOUT_SECONDS

_DEFAULT_TIMEOUT = DEFAULT_BACKEND_TIMEOUT_SECONDS


class SweagentExecutor(CLIBackendExecutor):
    """Execute tasks using the SWE-agent CLI."""

    def __init__(self, cli_path: str | None = None) -> None:
        super().__init__(cli_path, "CODEFORGE_SWEAGENT_PATH", "sweagent")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(
            name="sweagent",
            display_name="SWE-agent",
            cli_command=self._cli_path,
            requires_docker=True,
            capabilities=["code-edit", "sandbox", "multi-file"],
            config_schema=(
                ConfigField(key="model", type=str, description="LLM model name (passed to --agent.model.name)"),
                ConfigField(key="timeout", type=int, default=_DEFAULT_TIMEOUT, description="Timeout in seconds"),
                ConfigField(key="extra_args", type=list, description="Extra CLI arguments"),
                ConfigField(key="extra_env", type=dict, description="Extra environment variables for subprocess"),
                ConfigField(key="working_dir_override", type=str, description="Override workspace path"),
            ),
        )

    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        cmd = [self._cli_path, "run", "--problem_statement", prompt]

        model = config.get("model")
        if model:
            cmd.extend(["--agent.model.name", model])

        cmd.extend(parse_extra_args(config))
        return cmd
