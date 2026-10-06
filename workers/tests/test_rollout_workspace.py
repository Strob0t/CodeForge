"""Multi-rollout conversations keep the user's work and the best rollout (KI-195).

Before, rollout 0 ran ``git stash push --include-untracked`` and never
popped it, so the user's uncommitted edit and untracked files vanished into
``stash@{0}``; later rollouts ran ``checkout .`` and ``clean -fd``, and the
workspace ended with the last rollout's changes while the best one was
reported. These tests run real git in a temporary repository.
"""

from __future__ import annotations

import subprocess
from typing import TYPE_CHECKING
from unittest.mock import AsyncMock

import pytest

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
