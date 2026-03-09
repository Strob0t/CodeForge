"""Tests for backend config passthrough (extra_env, working_dir_override)."""

from __future__ import annotations

import pytest

from codeforge.backends._base import BackendInfo, ConfigField
from codeforge.backends._streaming import run_streaming_subprocess


@pytest.mark.asyncio
async def test_extra_env_merged_into_subprocess() -> None:
    """Extra environment variables are visible to the subprocess."""
    lines: list[str] = []

    async def on_output(line: str) -> None:
        lines.append(line)

    exit_code, stdout, _stderr = await run_streaming_subprocess(
        cmd=["sh", "-c", "echo $CODEFORGE_TEST_VAR"],
        cwd="/tmp",
        on_output=on_output,
        env={"CODEFORGE_TEST_VAR": "hello_from_config"},
    )

    assert exit_code == 0
    assert "hello_from_config" in stdout


@pytest.mark.asyncio
async def test_extra_env_does_not_clobber_path() -> None:
    """Extra env merges with existing env, PATH is preserved."""
    exit_code, stdout, _stderr = await run_streaming_subprocess(
        cmd=["sh", "-c", "echo $PATH"],
        cwd="/tmp",
        env={"CUSTOM_KEY": "value"},
    )

    assert exit_code == 0
    # PATH should contain /usr or similar — not be empty
    assert "/usr" in stdout or "/bin" in stdout


@pytest.mark.asyncio
async def test_missing_extra_env_uses_default() -> None:
    """When no extra env is provided, default environment is used."""
    exit_code, stdout, _stderr = await run_streaming_subprocess(
        cmd=["sh", "-c", "echo $HOME"],
        cwd="/tmp",
    )

    assert exit_code == 0
    assert stdout.strip() != ""


@pytest.mark.asyncio
async def test_working_dir_override() -> None:
    """Working directory can be overridden."""
    exit_code, stdout, _stderr = await run_streaming_subprocess(
        cmd=["pwd"],
        cwd="/tmp",
    )

    assert exit_code == 0
    assert "/tmp" in stdout


def test_config_field_extra_env_schema() -> None:
    """ConfigField can describe extra_env as a dict type."""
    field = ConfigField(
        key="extra_env",
        type=dict,
        description="Extra environment variables",
        required=False,
    )
    assert field.key == "extra_env"
    assert field.type is dict


def test_backend_info_includes_extra_env_field() -> None:
    """BackendInfo can include extra_env in its config schema."""
    info = BackendInfo(
        name="test",
        display_name="Test",
        cli_command="test",
        config_schema=(
            ConfigField(key="extra_env", type=dict, description="Extra env vars"),
            ConfigField(key="working_dir_override", type=str, description="Override cwd"),
        ),
    )
    keys = [f.key for f in info.config_schema]
    assert "extra_env" in keys
    assert "working_dir_override" in keys
