"""Tests for A4: Inference-Time Scaling for Conversations."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

# ---------------------------------------------------------------------------
# A4.1 — Single rollout (default) behaves identically
# ---------------------------------------------------------------------------


class TestSingleRollout:
    """rollout_count=1 must behave identically to current behavior."""

    @pytest.mark.asyncio
    async def test_single_rollout_no_snapshot(self) -> None:
        """rollout_count=1 -> no workspace snapshot, no selection, same output."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        mock_executor = AsyncMock()
        mock_executor.run = AsyncMock(
            return_value=MagicMock(
                final_content="Hello",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=20,
                step_count=1,
                model="test-model",
                error="",
                tool_messages=[],
            )
        )

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=1,
            workspace_path="/tmp/test-workspace",
        )
        result = await rollout_exec.execute(messages=[{"role": "user", "content": "hi"}], config=MagicMock())

        assert result.final_content == "Hello"
        assert result.total_cost == pytest.approx(0.01)
        mock_executor.run.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_single_rollout_no_git_stash(self) -> None:
        """rollout_count=1 -> no git stash/restore operations."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        mock_executor = AsyncMock()
        mock_executor.run = AsyncMock(
            return_value=MagicMock(
                final_content="result",
                total_cost=0.0,
                total_tokens_in=0,
                total_tokens_out=0,
                step_count=0,
                model="m",
                error="",
                tool_messages=[],
            )
        )

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=1,
            workspace_path="/tmp/test",
        )

        with (
            patch("codeforge.agent_loop._snapshot_workspace") as snap,
            patch("codeforge.agent_loop._restore_workspace") as restore,
        ):
            await rollout_exec.execute(messages=[], config=MagicMock())
            snap.assert_not_called()
            restore.assert_not_called()


# ---------------------------------------------------------------------------
# A4.2 — Multi-rollout executes N independent loops
# ---------------------------------------------------------------------------


class TestMultiRollout:
    """rollout_count > 1 runs N independent agent loops."""

    @pytest.mark.asyncio
    async def test_multi_rollout_calls_n_times(self) -> None:
        """rollout_count=3 -> agent loop called 3 times."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        call_count = 0

        async def _mock_run(messages, config=None):
            nonlocal call_count
            call_count += 1
            return MagicMock(
                final_content=f"result-{call_count}",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=20,
                step_count=1,
                model="test",
                error="",
                tool_messages=[],
            )

        mock_executor = MagicMock()
        mock_executor.run = _mock_run

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=3,
            workspace_path="/tmp/test",
        )
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._select_best_rollout", return_value=0),
        ):
            result = await rollout_exec.execute(messages=[{"role": "user", "content": "test"}], config=MagicMock())

        assert call_count == 3
        assert result.total_cost == pytest.approx(0.03)

    @pytest.mark.asyncio
    async def test_each_rollout_gets_unique_id(self) -> None:
        """Each rollout receives unique rollout_id in context."""
        from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig

        rollout_ids: list[int] = []

        async def _mock_run(messages, config=None):
            if config and hasattr(config, "rollout_id"):
                rollout_ids.append(config.rollout_id)
            return MagicMock(
                final_content="ok",
                total_cost=0.01,
                total_tokens_in=0,
                total_tokens_out=0,
                step_count=0,
                model="m",
                error="",
                tool_messages=[],
            )

        mock_executor = MagicMock()
        mock_executor.run = _mock_run

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=3,
            workspace_path="/tmp/test",
        )
        cfg = LoopConfig()
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._select_best_rollout", return_value=0),
        ):
            await rollout_exec.execute(messages=[], config=cfg)

        assert rollout_ids == [0, 1, 2]


# ---------------------------------------------------------------------------
# A4.3 — Best rollout selected via hybrid verification
# ---------------------------------------------------------------------------


class TestBestRolloutSelection:
    """Verify rollout selection by score."""

    @pytest.mark.asyncio
    async def test_highest_score_selected(self) -> None:
        """3 rollouts with scores [0.3, 0.9, 0.6] -> rollout 1 selected."""
        from codeforge.agent_loop import _select_best_rollout

        results = [
            MagicMock(final_content="a", error="", total_cost=0.01),
            MagicMock(final_content="b", error="", total_cost=0.01),
            MagicMock(final_content="c", error="", total_cost=0.01),
        ]
        scores = [0.3, 0.9, 0.6]
        idx = _select_best_rollout(results, scores)
        assert idx == 1

    @pytest.mark.asyncio
    async def test_error_rollout_not_selected(self) -> None:
        """Rollout with error is deprioritized even with high score."""
        from codeforge.agent_loop import _select_best_rollout

        results = [
            MagicMock(final_content="a", error="failed", total_cost=0.01),
            MagicMock(final_content="b", error="", total_cost=0.01),
        ]
        scores = [1.0, 0.5]
        idx = _select_best_rollout(results, scores)
        assert idx == 1  # error rollout excluded


# ---------------------------------------------------------------------------
# A4.4 — Confidence-based early stopping
# ---------------------------------------------------------------------------


class TestEarlyStopping:
    """Early stopping when rollouts converge."""

    def test_early_stop_on_agreement(self) -> None:
        """3 identical rollouts out of 8 -> should_stop returns True."""
        from codeforge.agent_loop import _should_early_stop

        outputs = ["same output", "same output", "same output"]
        exit_codes = [0, 0, 0]
        assert _should_early_stop(outputs, exit_codes, total_rollouts=8) is True

    def test_no_early_stop_low_similarity(self) -> None:
        """3 rollouts with different outputs -> should_stop returns False."""
        from codeforge.agent_loop import _should_early_stop

        outputs = ["output A is very different", "output B is completely unique", "output C has nothing in common"]
        exit_codes = [0, 0, 0]
        assert _should_early_stop(outputs, exit_codes, total_rollouts=8) is False

    def test_no_early_stop_with_error(self) -> None:
        """3 identical but one has exit_code != 0 -> no early stop."""
        from codeforge.agent_loop import _should_early_stop

        outputs = ["same", "same", "same"]
        exit_codes = [0, 0, 1]
        assert _should_early_stop(outputs, exit_codes, total_rollouts=8) is False

    def test_no_early_stop_few_rollouts(self) -> None:
        """rollout_count <= 3 -> never early stop."""
        from codeforge.agent_loop import _should_early_stop

        outputs = ["same", "same", "same"]
        exit_codes = [0, 0, 0]
        assert _should_early_stop(outputs, exit_codes, total_rollouts=3) is False


# ---------------------------------------------------------------------------
# A4.5 — Cost aggregation
# ---------------------------------------------------------------------------


class TestCostAggregation:
    """All rollouts' costs must be summed."""

    @pytest.mark.asyncio
    async def test_costs_summed(self) -> None:
        """3 rollouts costing $0.01 each -> total = $0.03."""
        from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig

        async def _mock_run(messages, config=None):
            return MagicMock(
                final_content="ok",
                total_cost=0.01,
                total_tokens_in=100,
                total_tokens_out=50,
                step_count=1,
                model="m",
                error="",
                tool_messages=[],
            )

        mock_executor = MagicMock()
        mock_executor.run = _mock_run

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=3,
            workspace_path="/tmp/test",
        )
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._select_best_rollout", return_value=0),
        ):
            result = await rollout_exec.execute(messages=[], config=LoopConfig())

        assert result.total_cost == pytest.approx(0.03)
        assert result.total_tokens_in == 300
        assert result.total_tokens_out == 150


# ---------------------------------------------------------------------------
# A4.5b — Non-git workspace falls back to single rollout
# ---------------------------------------------------------------------------


class TestNonGitFallback:
    """Workspace without .git falls back to single rollout."""

    @pytest.mark.asyncio
    async def test_no_git_falls_back(self) -> None:
        """rollout_count=3 with no .git -> logs warning, single rollout."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        mock_executor = AsyncMock()
        mock_executor.run = AsyncMock(
            return_value=MagicMock(
                final_content="single",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=10,
                step_count=1,
                model="m",
                error="",
                tool_messages=[],
            )
        )

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=3,
            workspace_path="/tmp/no-git-here",
        )

        with patch("os.path.isdir", return_value=False):
            result = await rollout_exec.execute(messages=[], config=MagicMock())

        assert result.final_content == "single"
        mock_executor.run.assert_awaited_once()
        assert result.metadata.get("fallback_reason") == "no_git_repo"
