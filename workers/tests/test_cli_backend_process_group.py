"""A stopped CLI backend task takes everything the agent started with it (KI-22).

Agent CLIs (Aider, Goose, ...) run shell commands and servers of their own.
Terminating only the CLI process left those running; the backend now runs in
its own process group and the whole group is stopped when the task is
cancelled (tasks.cancel, worker shutdown), cancelled via the router, or times
out. Real subprocesses: `sh` stands in for the agent CLI and starts a `sleep`.
"""

from __future__ import annotations

import asyncio
import os
from pathlib import Path

import pytest

from codeforge.backends._base import BackendInfo
from codeforge.backends._cli_base import CLIBackendExecutor, ExecutorConfig

pytestmark = pytest.mark.skipif(not Path("/proc").is_dir(), reason="needs /proc to inspect processes")


class _ShellAgent(CLIBackendExecutor):
    """An 'agent CLI' that starts a long-running child, reports its PID and waits for it."""

    def __init__(self) -> None:
        super().__init__("sh", "CODEFORGE_TEST_SHELL_AGENT_PATH", "sh")

    @property
    def info(self) -> BackendInfo:
        return BackendInfo(name="shell-agent", display_name="Shell agent", cli_command="sh")

    def _build_command(self, prompt: str, config: ExecutorConfig) -> list[str]:
        return ["sh", "-c", 'sleep 60 & echo "child $!"; echo "cli $$"; wait']


def _alive(pid: int) -> bool:
    """Whether *pid* runs (a zombie that nobody reaped yet counts as gone)."""
    try:
        stat = Path(f"/proc/{pid}/stat").read_text()
    except FileNotFoundError:
        return False
    return stat.rsplit(")", 1)[1].split()[0] != "Z"


async def _gone(pid: int, timeout: float = 5.0) -> bool:
    loop = asyncio.get_running_loop()
    deadline = loop.time() + timeout
    while loop.time() < deadline:
        if not _alive(pid):
            return True
        await asyncio.sleep(0.02)
    return False


class _Pids:
    def __init__(self) -> None:
        self.values: dict[str, int] = {}
        self.both = asyncio.Event()

    async def record(self, line: str) -> None:
        name, _, pid = line.partition(" ")
        if name in ("child", "cli") and pid.isdigit():
            self.values[name] = int(pid)
        if len(self.values) == 2:
            self.both.set()


async def _start(
    agent: _ShellAgent, workspace: Path, config: ExecutorConfig | None = None
) -> tuple[asyncio.Task, _Pids]:
    pids = _Pids()
    execution = asyncio.create_task(
        agent.execute("task-1", "do it", str(workspace), config=config, on_output=pids.record)
    )
    await asyncio.wait_for(pids.both.wait(), timeout=5)
    return execution, pids


async def test_cancelled_task_stops_the_cli_and_its_children(tmp_path: Path) -> None:
    execution, pids = await _start(_ShellAgent(), tmp_path)

    execution.cancel()
    with pytest.raises(asyncio.CancelledError):
        await execution

    assert await _gone(pids.values["cli"]), "the agent CLI survived the cancel"
    assert await _gone(pids.values["child"]), "a process started by the agent survived the cancel"


async def test_router_cancel_stops_the_whole_process_group(tmp_path: Path) -> None:
    agent = _ShellAgent()
    execution, pids = await _start(agent, tmp_path)

    await agent.cancel("task-1")
    result = await asyncio.wait_for(execution, timeout=10)

    assert result.status == "failed"
    assert await _gone(pids.values["child"]), "a process started by the agent survived the cancel"


async def test_timed_out_task_stops_the_whole_process_group(tmp_path: Path) -> None:
    execution, pids = await _start(_ShellAgent(), tmp_path, config={"timeout": 1})

    result = await asyncio.wait_for(execution, timeout=15)

    assert result.status == "failed"
    assert "timed out" in result.error
    assert await _gone(pids.values["child"]), "a process started by the agent survived the timeout"


async def test_the_cli_runs_in_its_own_process_group(tmp_path: Path) -> None:
    execution, pids = await _start(_ShellAgent(), tmp_path)
    try:
        assert os.getpgid(pids.values["cli"]) == pids.values["cli"]
        assert os.getpgid(pids.values["cli"]) != os.getpgid(0), "the worker must not share the agent's group"
    finally:
        execution.cancel()
        with pytest.raises(asyncio.CancelledError):
            await execution
