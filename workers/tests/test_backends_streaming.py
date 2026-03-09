"""Tests for the streaming subprocess runner."""

from __future__ import annotations

import pytest

from codeforge.backends._streaming import run_streaming_subprocess


@pytest.mark.asyncio
async def test_streaming_subprocess_echo() -> None:
    """Running a simple echo command streams output."""
    lines: list[str] = []

    async def on_output(line: str) -> None:
        lines.append(line)

    exit_code, stdout, _stderr = await run_streaming_subprocess(
        cmd=["echo", "hello world"],
        cwd="/tmp",
        on_output=on_output,
    )

    assert exit_code == 0
    assert "hello world" in stdout
    assert len(lines) >= 1


@pytest.mark.asyncio
async def test_streaming_subprocess_multiline() -> None:
    """Multi-line output is streamed line by line."""
    lines: list[str] = []

    async def on_output(line: str) -> None:
        lines.append(line)

    exit_code, _stdout, _stderr = await run_streaming_subprocess(
        cmd=["printf", "line1\\nline2\\nline3\\n"],
        cwd="/tmp",
        on_output=on_output,
    )

    assert exit_code == 0
    assert len(lines) == 3


@pytest.mark.asyncio
async def test_streaming_subprocess_failure() -> None:
    """Failed commands return non-zero exit code."""
    exit_code, _stdout, _stderr = await run_streaming_subprocess(
        cmd=["false"],
        cwd="/tmp",
    )
    assert exit_code != 0


@pytest.mark.asyncio
async def test_streaming_subprocess_timeout() -> None:
    """Timed-out commands return -1."""
    exit_code, _stdout, stderr = await run_streaming_subprocess(
        cmd=["sleep", "10"],
        cwd="/tmp",
        timeout=0.1,
    )
    assert exit_code == -1
    assert "timed out" in stderr.lower()


@pytest.mark.asyncio
async def test_streaming_no_callback() -> None:
    """Running without on_output callback works fine."""
    exit_code, stdout, _stderr = await run_streaming_subprocess(
        cmd=["echo", "test"],
        cwd="/tmp",
    )
    assert exit_code == 0
    assert "test" in stdout


@pytest.mark.asyncio
async def test_streaming_callback_error_ignored() -> None:
    """Callback errors don't break the subprocess."""
    lines: list[str] = []

    async def on_output(line: str) -> None:
        lines.append(line)
        raise RuntimeError("callback error")

    exit_code, _stdout, _stderr = await run_streaming_subprocess(
        cmd=["printf", "a\\nb\\n"],
        cwd="/tmp",
        on_output=on_output,
    )

    assert exit_code == 0
    assert len(lines) == 2  # Both lines captured despite errors
