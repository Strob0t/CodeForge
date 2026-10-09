"""Multi-rollout conversations keep the user's work and the best rollout (KI-195).

Before, rollout 0 ran ``git stash push --include-untracked`` and never
popped it, so the user's uncommitted edit and untracked files vanished into
``stash@{0}``; later rollouts ran ``checkout .`` and ``clean -fd``, and the
workspace ended with the last rollout's changes while the best one was
reported. These tests run real git in a temporary repository.
"""

from __future__ import annotations

import asyncio
import subprocess
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock

import pytest

from codeforge import agent_loop
from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig
from codeforge.models import AgentLoopResult

if TYPE_CHECKING:
    from pathlib import Path


def _git(ws: Path, *args: str) -> str:
    return subprocess.run(["git", "-C", str(ws), *args], capture_output=True, text=True, check=True).stdout  # noqa: S603, S607


@pytest.fixture
def repo(tmp_path: Path) -> Path:
    ws = tmp_path / "ws"
    ws.mkdir()
    _git(ws, "init", "-q")
    _git(ws, "config", "user.email", "a@b.invalid")
    _git(ws, "config", "user.name", "a")
    (ws / ".gitignore").write_text("*.log\n")
    (ws / "a.txt").write_text("committed\n")
    (ws / "gone.txt").write_text("deleted by rollout 1\n")
    _git(ws, "add", ".")
    _git(ws, "commit", "-qm", "init")
    return ws


class _Rollouts:
    """Each rollout writes its own files; rollout 2 (index 1) is the best (no error)."""

    def __init__(self, ws: Path) -> None:
        self.ws = ws
        self.n = 0
        self.seen_at_start: list[list[str]] = []

    async def run(self, messages: list[dict[str, object]], config: LoopConfig) -> AgentLoopResult:
        self.seen_at_start.append(sorted(p.name for p in self.ws.iterdir() if p.name != ".git"))
        self.n += 1
        (self.ws / f"rollout{self.n}.txt").write_text(f"rollout {self.n}\n")
        (self.ws / "a.txt").write_text(f"edited by rollout {self.n}\n")
        if self.n == 2:
            (self.ws / "gone.txt").unlink()
        return AgentLoopResult(
            final_content=f"rollout {self.n}", error="" if self.n == 2 else "boom", step_count=1, total_cost=0.01
        )


async def test_the_best_rollout_is_left_in_a_clean_workspace(repo: Path) -> None:
    head = _git(repo, "rev-parse", "HEAD")
    rollouts = _Rollouts(repo)
    executor = ConversationRolloutExecutor(rollouts, rollout_count=3, workspace_path=str(repo))  # type: ignore[arg-type]

    result = await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    assert result.final_content == "rollout 2"
    assert result.metadata["selected_index"] == 1
    # Every rollout started from the start commit.
    assert rollouts.seen_at_start == [[".gitignore", "a.txt", "gone.txt"]] * 3
    # The workspace holds the reported (best) rollout, as uncommitted changes.
    files = sorted(p.name for p in repo.iterdir() if p.name != ".git")
    assert files == [".gitignore", "a.txt", "rollout2.txt"]
    assert (repo / "a.txt").read_text() == "edited by rollout 2\n"
    assert _git(repo, "rev-parse", "HEAD") == head
    assert _git(repo, "diff", "--cached", "--name-only") == "", "nothing is left staged"
    assert _git(repo, "stash", "list") == ""
    assert sorted(_git(repo, "status", "--porcelain").splitlines()) == [" D gone.txt", " M a.txt", "?? rollout2.txt"]


@pytest.mark.parametrize("change", ["modified", "untracked", "staged"])
async def test_uncommitted_user_work_runs_once_and_is_kept(repo: Path, change: str) -> None:
    if change == "modified":
        (repo / "a.txt").write_text("user edit\n")
    elif change == "untracked":
        (repo / "user.txt").write_text("user work\n")
    else:
        (repo / "user.txt").write_text("user work\n")
        _git(repo, "add", "user.txt")
    status_before = _git(repo, "status", "--porcelain")
    loop = AsyncMock()
    loop.run = AsyncMock(return_value=AgentLoopResult(final_content="once"))
    runtime = AsyncMock()
    executor = ConversationRolloutExecutor(loop, rollout_count=3, workspace_path=str(repo), runtime=runtime)

    result = await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    loop.run.assert_awaited_once()
    assert result.final_content == "once"
    assert result.metadata == {"fallback_reason": "uncommitted_changes"}
    assert _git(repo, "status", "--porcelain") == status_before
    assert _git(repo, "stash", "list") == ""
    assert "uncommitted changes" in str(runtime.send_output.await_args.args[0])


async def test_ignored_user_files_do_not_block_and_survive(repo: Path) -> None:
    (repo / "debug.log").write_text("ignored\n")
    rollouts = _Rollouts(repo)
    executor = ConversationRolloutExecutor(rollouts, rollout_count=2, workspace_path=str(repo))  # type: ignore[arg-type]

    result = await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    assert rollouts.n == 2
    assert result.metadata["selected_index"] == 1
    assert (repo / "debug.log").read_text() == "ignored\n"


async def test_a_repository_without_commits_runs_once(tmp_path: Path) -> None:
    _git(tmp_path, "init", "-q")
    loop = AsyncMock()
    loop.run = AsyncMock(return_value=AgentLoopResult(final_content="once"))
    executor = ConversationRolloutExecutor(loop, rollout_count=2, workspace_path=str(tmp_path))

    result = await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    loop.run.assert_awaited_once()
    assert result.metadata == {"fallback_reason": "no_commit"}


async def test_a_rollout_that_commits_or_changes_nothing(repo: Path) -> None:
    """The best rollout committed its change: it is kept as an uncommitted change on the start commit."""
    head = _git(repo, "rev-parse", "HEAD")
    calls = 0

    async def run(messages: list[dict[str, object]], config: LoopConfig) -> AgentLoopResult:
        nonlocal calls
        calls += 1
        if calls == 1:
            (repo / "a.txt").write_text("committed by rollout 1\n")
            _git(repo, "commit", "-qam", "rollout 1")
            return AgentLoopResult(final_content="committed", step_count=1)
        return AgentLoopResult(final_content="nothing", error="no change", step_count=1)

    loop = AsyncMock()
    loop.run = run
    executor = ConversationRolloutExecutor(loop, rollout_count=2, workspace_path=str(repo))

    result = await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    assert result.metadata["selected_index"] == 0
    assert _git(repo, "rev-parse", "HEAD") == head
    assert (repo / "a.txt").read_text() == "committed by rollout 1\n"
    assert _git(repo, "status", "--porcelain") == " M a.txt\n"


class _HangingGit:
    """Starts ``sleep`` in place of git, as git stuck on a hook or a lock would hang."""

    def __init__(self, real: object) -> None:
        self._real = real
        self.procs: list[asyncio.subprocess.Process] = []
        self.options: list[dict[str, object]] = []

    async def __call__(self, program: str, *args: str, **kwargs: object) -> asyncio.subprocess.Process:
        self.options.append(kwargs)
        proc = await self._real("sleep", "30", **kwargs)  # type: ignore[operator]
        self.procs.append(proc)
        return proc


async def test_a_hanging_git_call_times_out_and_its_process_group_is_stopped(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    hanging = _HangingGit(agent_loop.start_tool_process)
    monkeypatch.setattr(agent_loop, "start_tool_process", hanging)
    monkeypatch.setattr(agent_loop, "_GIT_TIMEOUT_SECONDS", 0.2)

    with pytest.raises(RuntimeError, match="timed out"):
        await agent_loop._run_git(str(tmp_path), "status")

    assert hanging.options[0]["start_new_session"] is True
    assert hanging.procs[0].returncode is not None


async def test_a_cancelled_git_call_stops_its_process_group(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    hanging = _HangingGit(agent_loop.start_tool_process)
    monkeypatch.setattr(agent_loop, "start_tool_process", hanging)

    call = asyncio.create_task(agent_loop._run_git(str(tmp_path), "status"))
    while not hanging.procs:
        await asyncio.sleep(0.01)
    call.cancel()
    with pytest.raises(asyncio.CancelledError):
        await call

    assert hanging.procs[0].returncode is not None


@pytest.mark.parametrize("detached", [False, True])
async def test_a_rollout_that_checks_out_a_branch_leaves_its_commits_intact(repo: Path, detached: bool) -> None:
    """Resetting between rollouts moves the start branch only, never a branch a rollout checked out."""
    _git(repo, "checkout", "-q", "-b", "feature")
    (repo / "feature.txt").write_text("feature work\n")
    _git(repo, "add", "feature.txt")
    _git(repo, "commit", "-qm", "feature")
    feature_tip = _git(repo, "rev-parse", "feature")
    _git(repo, "checkout", "-q", "-")
    if detached:
        _git(repo, "checkout", "-q", "--detach")
    start_ref = _git(repo, "rev-parse", "--symbolic-full-name", "HEAD")
    head = _git(repo, "rev-parse", "HEAD")
    seen_refs: list[str] = []

    async def run(messages: list[dict[str, object]], config: LoopConfig) -> AgentLoopResult:
        seen_refs.append(_git(repo, "rev-parse", "--symbolic-full-name", "HEAD"))
        _git(repo, "checkout", "-q", "feature")
        (repo / "a.txt").write_text(f"rollout {len(seen_refs)}\n")
        return AgentLoopResult(final_content=f"rollout {len(seen_refs)}", step_count=1)

    loop = AsyncMock()
    loop.run = run
    executor = ConversationRolloutExecutor(loop, rollout_count=3, workspace_path=str(repo))

    await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    assert _git(repo, "rev-parse", "feature") == feature_tip
    assert seen_refs == [start_ref] * 3
    assert _git(repo, "rev-parse", "--symbolic-full-name", "HEAD") == start_ref
    assert _git(repo, "rev-parse", "HEAD") == head


@pytest.mark.parametrize(
    ("rollouts_before_change", "rollout_count"),
    [(1, 3), (2, 2)],  # before a reset between rollouts; before the best rollout is kept
)
async def test_a_change_no_rollout_wrote_stops_the_rollouts_and_is_kept(
    repo: Path, monkeypatch: pytest.MonkeyPatch, rollouts_before_change: int, rollout_count: int
) -> None:
    """Another turn or the user wrote to the workspace after a rollout: nothing is reset or deleted."""
    real_capture = agent_loop._RolloutWorkspace.capture
    captures = 0

    async def capture_then_someone_writes(self: agent_loop._RolloutWorkspace) -> str:
        nonlocal captures
        tree = await real_capture(self)
        captures += 1
        if captures == rollouts_before_change:
            (repo / "user.txt").write_text("written meanwhile\n")
        return tree

    monkeypatch.setattr(agent_loop._RolloutWorkspace, "capture", capture_then_someone_writes)
    rollouts = _Rollouts(repo)
    runtime = AsyncMock()
    executor = ConversationRolloutExecutor(
        rollouts, rollout_count=rollout_count, workspace_path=str(repo), runtime=runtime
    )  # type: ignore[arg-type]

    result = await executor.execute([{"role": "user", "content": "x"}], LoopConfig())

    assert rollouts.n == rollouts_before_change
    assert (repo / "user.txt").read_text() == "written meanwhile\n"
    # The workspace holds the last rollout, which is the one reported.
    assert (repo / f"rollout{rollouts_before_change}.txt").exists()
    assert result.final_content == f"rollout {rollouts_before_change}"
    assert result.metadata["selected_index"] == rollouts_before_change - 1
    assert result.metadata["stopped_reason"] == "workspace_changed"
    outputs = " ".join(str(c.args[0]) for c in runtime.send_output.await_args_list)
    assert "changed outside the rollouts" in outputs
