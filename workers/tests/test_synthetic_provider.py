"""Tests for SWE-GEN synthetic task generation pipeline.

Tests cover: commit filtering (too small, too large, merges, docs-only),
difficulty estimation, language detection, test command inference,
TaskSpec construction, mock LLM, empty history.
"""

from __future__ import annotations

import pytest

from codeforge.evaluation.generators.swegen import (
    CommitInfo,
    SWEGenPipeline,
    SyntheticTask,
    _detect_language,
    _estimate_difficulty,
    _infer_test_command,
    _is_docs_only,
)


def _commit(
    sha: str = "abc1234",
    parent_sha: str = "def5678",
    message: str = "fix: login bug",
    diff: str = "--- a/src/auth.py\n+++ b/src/auth.py\n@@ -1,3 +1,5 @@\n+def fix():\n+    pass",
    files_changed: list[str] | None = None,
    additions: int = 10,
    deletions: int = 5,
) -> CommitInfo:
    return CommitInfo(
        sha=sha,
        parent_sha=parent_sha,
        message=message,
        diff=diff,
        files_changed=files_changed or ["src/auth.py"],
        additions=additions,
        deletions=deletions,
    )


# ---------------------------------------------------------------------------
# Tests: Commit filtering
# ---------------------------------------------------------------------------


class TestCommitFiltering:
    def test_skip_too_small(self) -> None:
        """Commits with fewer than min_diff_lines are skipped."""
        pipeline = SWEGenPipeline(llm=_fake_llm(), min_diff_lines=10)
        commit = _commit(additions=2, deletions=1)

        assert not pipeline.should_include(commit)

    def test_skip_too_large(self) -> None:
        """Commits with more than max_diff_lines are skipped."""
        pipeline = SWEGenPipeline(llm=_fake_llm(), max_diff_lines=100)
        commit = _commit(additions=300, deletions=200)

        assert not pipeline.should_include(commit)

    def test_skip_merge_commits(self) -> None:
        """Merge commits (message starts with 'Merge') are skipped."""
        pipeline = SWEGenPipeline(llm=_fake_llm())
        commit = _commit(message="Merge branch 'feature' into main")

        assert not pipeline.should_include(commit)

    def test_skip_docs_only(self) -> None:
        """Commits that only touch docs/config files are skipped."""
        pipeline = SWEGenPipeline(llm=_fake_llm())
        commit = _commit(files_changed=["README.md", "docs/guide.md", "CHANGELOG.md"])

        assert not pipeline.should_include(commit)

    def test_accept_valid_commit(self) -> None:
        """Valid commits with code changes are accepted."""
        pipeline = SWEGenPipeline(llm=_fake_llm())
        commit = _commit(
            files_changed=["src/auth.py", "tests/test_auth.py"],
            additions=20,
            deletions=5,
        )

        assert pipeline.should_include(commit)


# ---------------------------------------------------------------------------
# Tests: Difficulty estimation
# ---------------------------------------------------------------------------


class TestDifficultyEstimation:
    def test_small_diff_easy(self) -> None:
        assert _estimate_difficulty(10) == "easy"

    def test_medium_diff(self) -> None:
        assert _estimate_difficulty(80) == "medium"

    def test_large_diff_hard(self) -> None:
        assert _estimate_difficulty(300) == "hard"


# ---------------------------------------------------------------------------
# Tests: Language detection
# ---------------------------------------------------------------------------


class TestLanguageDetection:
    def test_python_files(self) -> None:
        assert _detect_language(["src/main.py", "tests/test_main.py"]) == "python"

    def test_go_files(self) -> None:
        assert _detect_language(["cmd/main.go", "internal/service.go"]) == "go"

    def test_typescript_files(self) -> None:
        assert _detect_language(["src/App.tsx", "src/index.ts"]) == "typescript"

    def test_javascript_files(self) -> None:
        assert _detect_language(["src/index.js", "lib/utils.js"]) == "javascript"

    def test_mixed_files_majority(self) -> None:
        """When mixed, the majority language wins."""
        files = ["src/main.py", "src/utils.py", "README.md"]
        assert _detect_language(files) == "python"

    def test_unknown_files(self) -> None:
        assert _detect_language(["data.csv", "image.png"]) == "unknown"

    def test_empty_files(self) -> None:
        assert _detect_language([]) == "unknown"


# ---------------------------------------------------------------------------
# Tests: Test command inference
# ---------------------------------------------------------------------------


class TestTestCommandInference:
    def test_python(self) -> None:
        assert _infer_test_command("python") == "pytest"

    def test_go(self) -> None:
        assert _infer_test_command("go") == "go test ./..."

    def test_typescript(self) -> None:
        assert _infer_test_command("typescript") == "npm test"

    def test_javascript(self) -> None:
        assert _infer_test_command("javascript") == "npm test"

    def test_rust(self) -> None:
        assert _infer_test_command("rust") == "cargo test"

    def test_unknown(self) -> None:
        assert _infer_test_command("unknown") == ""


# ---------------------------------------------------------------------------
# Tests: Docs-only detection
# ---------------------------------------------------------------------------


class TestDocsOnly:
    def test_all_docs(self) -> None:
        assert _is_docs_only(["README.md", "docs/setup.md", "CHANGELOG.md"])

    def test_mixed_with_code(self) -> None:
        assert not _is_docs_only(["README.md", "src/main.py"])

    def test_config_only(self) -> None:
        assert _is_docs_only([".gitignore", "pyproject.toml", "Makefile"])

    def test_empty(self) -> None:
        assert _is_docs_only([])


# ---------------------------------------------------------------------------
# Tests: SWE-GEN pipeline
# ---------------------------------------------------------------------------


class _FakeLLM:
    """Fake LLM that returns canned responses."""

    def __init__(
        self, test_code: str = "def test_fix(): assert True", issue_text: str = "Login fails with error"
    ) -> None:
        self._test_code = test_code
        self._issue_text = issue_text
        self.call_count = 0

    async def complete(self, prompt: str) -> str:
        self.call_count += 1
        if "test" in prompt.lower():
            return self._test_code
        return self._issue_text


def _fake_llm(**kwargs) -> _FakeLLM:
    return _FakeLLM(**kwargs)


class TestSWEGenPipeline:
    @pytest.mark.asyncio
    async def test_generate_from_valid_commit(self) -> None:
        """Valid commit → generates a SyntheticTask."""
        llm = _fake_llm()
        pipeline = SWEGenPipeline(llm=llm)
        commit = _commit(additions=20, deletions=5)

        tasks = await pipeline.generate_from_commits([commit])

        assert len(tasks) == 1
        task = tasks[0]
        assert task.commit_sha == "abc1234"
        assert task.parent_sha == "def5678"
        assert task.issue_description == "Login fails with error"
        assert task.test_code == "def test_fix(): assert True"
        assert task.language == "python"

    @pytest.mark.asyncio
    async def test_skip_filtered_commits(self) -> None:
        """Filtered commits (too small) produce no tasks."""
        llm = _fake_llm()
        pipeline = SWEGenPipeline(llm=llm, min_diff_lines=100)
        commit = _commit(additions=5, deletions=2)

        tasks = await pipeline.generate_from_commits([commit])
        assert tasks == []
        assert llm.call_count == 0

    @pytest.mark.asyncio
    async def test_empty_commit_list(self) -> None:
        """Empty commit list → empty task list."""
        llm = _fake_llm()
        pipeline = SWEGenPipeline(llm=llm)

        tasks = await pipeline.generate_from_commits([])
        assert tasks == []

    @pytest.mark.asyncio
    async def test_multiple_commits(self) -> None:
        """Multiple valid commits → multiple tasks."""
        llm = _fake_llm()
        pipeline = SWEGenPipeline(llm=llm)
        commits = [
            _commit(sha="aaa", parent_sha="bbb", additions=20, deletions=5),
            _commit(sha="ccc", parent_sha="ddd", additions=30, deletions=10),
        ]

        tasks = await pipeline.generate_from_commits(commits)
        assert len(tasks) == 2

    @pytest.mark.asyncio
    async def test_llm_failure_skips_task(self) -> None:
        """If LLM returns empty output, skip that task."""

        class _FailLLM:
            async def complete(self, prompt: str) -> str:
                return ""

        pipeline = SWEGenPipeline(llm=_FailLLM())
        commit = _commit(additions=20, deletions=5)

        tasks = await pipeline.generate_from_commits([commit])
        assert tasks == []


class TestSyntheticTaskToTaskSpec:
    def test_to_task_spec(self) -> None:
        """SyntheticTask converts to TaskSpec correctly."""
        task = SyntheticTask(
            commit_sha="abc1234567890",
            parent_sha="def5678901234",
            issue_description="Login fails",
            test_code="def test_fix(): assert True",
            test_command="pytest",
            difficulty="medium",
            language="python",
            repo_url="https://github.com/test/repo",
        )

        spec = task.to_task_spec()

        assert spec.id == "synthetic-def56789"
        assert spec.input == "Login fails"
        assert spec.test_command == "pytest"
        assert spec.repo_commit == "def5678901234"
        assert spec.difficulty == "medium"
        assert "test_generated.py" in spec.initial_files
        assert spec.initial_files["test_generated.py"] == "def test_fix(): assert True"
        assert spec.metadata["commit_sha"] == "abc1234567890"
        assert spec.metadata["language"] == "python"
        assert spec.metadata["eval_method"] == "synthetic"
