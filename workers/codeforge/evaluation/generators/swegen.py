"""SWE-GEN: synthetic benchmark task generation from Git commits.

Inspired by R2E-Gym's environment generation pipeline:
1. Extract commit diffs from a project's Git history
2. LLM generates a test that FAILS before the commit and PASSES after
3. LLM back-translates the diff into a natural language issue description

This turns any CodeForge-managed project's history into a custom benchmark
suite for project-specific agent evaluation.
"""

from __future__ import annotations

import asyncio
from collections import Counter
from dataclasses import dataclass, field
from typing import Protocol

from codeforge.evaluation.providers.base import TaskSpec

# File extensions considered documentation/config (not source code).
_DOC_EXTENSIONS = frozenset(
    {
        ".md",
        ".txt",
        ".rst",
        ".adoc",
        ".csv",
        ".json",
        ".yaml",
        ".yml",
        ".toml",
        ".ini",
        ".cfg",
        ".conf",
        ".env",
        ".lock",
        ".sum",
        ".gitignore",
        ".dockerignore",
        ".editorconfig",
        ".prettierrc",
        ".eslintrc",
        ".png",
        ".jpg",
        ".jpeg",
        ".gif",
        ".svg",
        ".ico",
    }
)

_DOC_BASENAMES = frozenset(
    {
        "makefile",
        "dockerfile",
        "license",
        "changelog",
        "contributing",
        "readme",
        "authors",
        "codeowners",
    }
)

# Extension → language mapping.
_LANG_MAP: dict[str, str] = {
    ".py": "python",
    ".go": "go",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".js": "javascript",
    ".jsx": "javascript",
    ".rs": "rust",
    ".java": "java",
    ".kt": "kotlin",
    ".rb": "ruby",
    ".cpp": "cpp",
    ".c": "c",
    ".cs": "csharp",
    ".swift": "swift",
    ".php": "php",
}

# Language → default test command.
_TEST_COMMANDS: dict[str, str] = {
    "python": "pytest",
    "go": "go test ./...",
    "typescript": "npm test",
    "javascript": "npm test",
    "rust": "cargo test",
    "java": "mvn test",
    "ruby": "bundle exec rspec",
    "kotlin": "gradle test",
    "cpp": "make test",
    "c": "make test",
    "csharp": "dotnet test",
    "swift": "swift test",
    "php": "phpunit",
}


class LLMClient(Protocol):
    """Minimal LLM interface for SWE-GEN prompts."""

    async def complete(self, prompt: str) -> str: ...


@dataclass(frozen=True)
class CommitInfo:
    """Parsed Git commit metadata."""

    sha: str
    parent_sha: str
    message: str
    diff: str
    files_changed: list[str]
    additions: int = 0
    deletions: int = 0


@dataclass(frozen=True)
class SyntheticTask:
    """Generated benchmark task from a commit."""

    commit_sha: str
    parent_sha: str
    issue_description: str
    test_code: str
    test_command: str = ""
    difficulty: str = "medium"
    language: str = "unknown"
    repo_url: str = ""
    metadata: dict[str, str] = field(default_factory=dict)

    def to_task_spec(self) -> TaskSpec:
        """Convert to a standard TaskSpec for benchmark evaluation."""
        return TaskSpec(
            id=f"synthetic-{self.parent_sha[:8]}",
            name=f"Synthetic: {self.issue_description[:60]}",
            input=self.issue_description,
            test_command=self.test_command,
            repo_url=self.repo_url,
            repo_commit=self.parent_sha,
            difficulty=self.difficulty,
            initial_files={"test_generated.py": self.test_code} if self.test_code else {},
            metadata={
                "commit_sha": self.commit_sha,
                "parent_sha": self.parent_sha,
                "language": self.language,
                "eval_method": "synthetic",
                **self.metadata,
            },
        )


class SWEGenPipeline:
    """Generates synthetic benchmark tasks from Git commit history.

    Args:
        llm: LLM client implementing ``complete(prompt) -> str``.
        model: Model identifier for LLM calls (informational).
        max_diff_lines: Skip commits with more total changed lines.
        min_diff_lines: Skip commits with fewer total changed lines.
    """

    def __init__(
        self,
        llm: LLMClient,
        model: str = "openai/gpt-4o",
        max_diff_lines: int = 500,
        min_diff_lines: int = 5,
    ) -> None:
        self._llm = llm
        self._model = model
        self._max_diff_lines = max_diff_lines
        self._min_diff_lines = min_diff_lines

    def should_include(self, commit: CommitInfo) -> bool:
        """Check if a commit should be used for task generation."""
        total_lines = commit.additions + commit.deletions

        if total_lines < self._min_diff_lines:
            return False
        if total_lines > self._max_diff_lines:
            return False
        if commit.message.startswith("Merge "):
            return False
        return not _is_docs_only(commit.files_changed)

    async def generate_from_commits(self, commits: list[CommitInfo]) -> list[SyntheticTask]:
        """Generate synthetic tasks from a list of commits.

        Filters commits, then for each valid commit runs test generation
        and issue back-translation concurrently.
        """
        tasks: list[SyntheticTask] = []

        for commit in commits:
            if not self.should_include(commit):
                continue

            test_code, issue_text = await asyncio.gather(
                self._generate_test(commit),
                self._generate_issue(commit),
            )

            if not test_code or not issue_text:
                continue

            language = _detect_language(commit.files_changed)
            test_cmd = _infer_test_command(language)
            difficulty = _estimate_difficulty(commit.additions + commit.deletions)

            tasks.append(
                SyntheticTask(
                    commit_sha=commit.sha,
                    parent_sha=commit.parent_sha,
                    issue_description=issue_text,
                    test_code=test_code,
                    test_command=test_cmd,
                    difficulty=difficulty,
                    language=language,
                )
            )

        return tasks

    async def _generate_test(self, commit: CommitInfo) -> str:
        """Ask LLM to generate a test for the given commit diff."""
        prompt = (
            "Given the following Git diff, write a test that FAILS on the "
            "parent commit (before the change) and PASSES after the change is applied.\n\n"
            f"Commit message: {commit.message}\n\n"
            f"Diff:\n{commit.diff}\n\n"
            "Respond with ONLY the test code, no explanation."
        )
        return await self._llm.complete(prompt)

    async def _generate_issue(self, commit: CommitInfo) -> str:
        """Ask LLM to back-translate the diff into a natural language issue."""
        prompt = (
            "Given the following Git diff, write a bug report or feature request "
            "that describes the problem being fixed or feature being added. "
            "Do NOT reveal the solution or mention specific code changes.\n\n"
            f"Commit message: {commit.message}\n\n"
            f"Diff:\n{commit.diff}\n\n"
            "Respond with ONLY the issue description, no extra text."
        )
        return await self._llm.complete(prompt)


def _is_docs_only(files: list[str]) -> bool:
    """Check if all changed files are documentation or config."""
    if not files:
        return True

    for f in files:
        name = f.rsplit("/", 1)[-1].lower()
        ext = ""
        if "." in name:
            ext = "." + name.rsplit(".", 1)[-1]
        basename_no_ext = name.rsplit(".", 1)[0] if "." in name else name

        if ext not in _DOC_EXTENSIONS and basename_no_ext not in _DOC_BASENAMES:
            return False

    return True


def _detect_language(files: list[str]) -> str:
    """Detect the primary language from file extensions (majority vote)."""
    if not files:
        return "unknown"

    langs: Counter[str] = Counter()
    for f in files:
        if "." in f:
            ext = "." + f.rsplit(".", 1)[-1].lower()
            lang = _LANG_MAP.get(ext)
            if lang:
                langs[lang] += 1

    if not langs:
        return "unknown"

    return langs.most_common(1)[0][0]


def _infer_test_command(language: str) -> str:
    """Infer the default test command for a language."""
    return _TEST_COMMANDS.get(language, "")


def _estimate_difficulty(total_lines: int) -> str:
    """Estimate task difficulty from diff size."""
    if total_lines <= 30:
        return "easy"
    if total_lines <= 150:
        return "medium"
    return "hard"
