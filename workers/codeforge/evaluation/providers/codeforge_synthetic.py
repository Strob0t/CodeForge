"""Synthetic benchmark provider — generates tasks from a project's Git history.

Uses the SWE-GEN pipeline (Phase 28F) to turn Git commits into benchmark tasks.
Self-registers as ``codeforge_synthetic`` in the provider registry.
"""

from __future__ import annotations

import subprocess
from typing import TYPE_CHECKING

import structlog

from codeforge.evaluation.generators.swegen import CommitInfo, SWEGenPipeline
from codeforge.evaluation.providers.base import (
    BenchmarkType,
    Capabilities,
    TaskSpec,
    register_provider,
)

if TYPE_CHECKING:
    from codeforge.evaluation.generators.swegen import LLMClient

logger = structlog.get_logger()


class CodeForgeSyntheticProvider:
    """Benchmark provider that generates tasks from a project's Git history.

    Args:
        workspace_path: Path to the project's checked-out repository.
        llm: LLM client for test generation and issue back-translation.
        max_tasks: Maximum number of tasks to generate.
        model: Model identifier for LLM calls.
    """

    def __init__(
        self,
        workspace_path: str,
        llm: LLMClient,
        max_tasks: int = 50,
        model: str = "openai/gpt-4o",
    ) -> None:
        self._workspace = workspace_path
        self._pipeline = SWEGenPipeline(llm=llm, model=model)
        self._max_tasks = max_tasks
        self._cached_tasks: list[TaskSpec] | None = None

    @property
    def name(self) -> str:
        return "codeforge_synthetic"

    @property
    def benchmark_type(self) -> BenchmarkType:
        return BenchmarkType.AGENT

    @property
    def capabilities(self) -> Capabilities:
        return Capabilities(functional_tests=True, llm_judge=True)

    async def load_tasks(self) -> list[TaskSpec]:
        """Load tasks by running SWE-GEN on the project's Git history."""
        if self._cached_tasks is not None:
            return self._cached_tasks

        commits = _load_commits(self._workspace, max_commits=self._max_tasks * 3)
        synthetic = await self._pipeline.generate_from_commits(commits)

        tasks = [t.to_task_spec() for t in synthetic[: self._max_tasks]]

        logger.info(
            "synthetic tasks generated",
            workspace=self._workspace,
            commits_scanned=len(commits),
            tasks_generated=len(tasks),
        )

        self._cached_tasks = tasks
        return tasks

    async def task_count(self) -> int:
        tasks = await self.load_tasks()
        return len(tasks)


def _load_commits(workspace: str, max_commits: int = 150) -> list[CommitInfo]:
    """Load recent commits from a Git repository."""
    try:
        log_output = subprocess.run(  # noqa: S603
            [  # noqa: S607
                "git",
                "log",
                f"-{max_commits}",
                "--format=%H%n%P%n%s%n---COMMIT_SEP---",
                "--no-merges",
            ],
            cwd=workspace,
            capture_output=True,
            text=True,
            timeout=30,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError):
        return []

    if log_output.returncode != 0:
        return []

    commits: list[CommitInfo] = []
    entries = log_output.stdout.strip().split("---COMMIT_SEP---")

    for entry in entries:
        lines = entry.strip().split("\n")
        if len(lines) < 3:
            continue

        sha = lines[0].strip()
        parents = lines[1].strip().split()
        message = lines[2].strip()

        if not sha or not parents:
            continue

        parent_sha = parents[0]

        # Get diff and stat for this commit.
        try:
            diff_result = subprocess.run(  # noqa: S603
                ["git", "diff", f"{parent_sha}..{sha}", "--stat", "--patch"],  # noqa: S607
                cwd=workspace,
                capture_output=True,
                text=True,
                timeout=15,
            )
        except (subprocess.TimeoutExpired, FileNotFoundError):
            continue

        if diff_result.returncode != 0:
            continue

        diff_text = diff_result.stdout
        files, additions, deletions = _parse_diff_stat(diff_text)

        commits.append(
            CommitInfo(
                sha=sha,
                parent_sha=parent_sha,
                message=message,
                diff=diff_text,
                files_changed=files,
                additions=additions,
                deletions=deletions,
            )
        )

    return commits


def _parse_diff_stat(diff_output: str) -> tuple[list[str], int, int]:
    """Extract changed files and line counts from git diff output."""
    files: list[str] = []
    additions = 0
    deletions = 0

    for line in diff_output.split("\n"):
        if line.startswith("diff --git a/"):
            parts = line.split(" b/", 1)
            if len(parts) == 2:
                files.append(parts[1])
        elif line.startswith("+") and not line.startswith("+++"):
            additions += 1
        elif line.startswith("-") and not line.startswith("---"):
            deletions += 1

    return files, additions, deletions


register_provider("codeforge_synthetic", CodeForgeSyntheticProvider)  # type: ignore[arg-type]
