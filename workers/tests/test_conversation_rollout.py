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
            patch("codeforge.agent_loop.select_best_rollout", return_value=0),
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
            patch("codeforge.agent_loop.select_best_rollout", return_value=0),
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
        from codeforge.quality_tracking import select_best_rollout

        results = [
            MagicMock(final_content="a", error="", total_cost=0.01),
            MagicMock(final_content="b", error="", total_cost=0.01),
            MagicMock(final_content="c", error="", total_cost=0.01),
        ]
        scores = [0.3, 0.9, 0.6]
        idx = select_best_rollout(results, scores)
        assert idx == 1

    @pytest.mark.asyncio
    async def test_error_rollout_not_selected(self) -> None:
        """Rollout with error is deprioritized even with high score."""
        from codeforge.quality_tracking import select_best_rollout

        results = [
            MagicMock(final_content="a", error="failed", total_cost=0.01),
            MagicMock(final_content="b", error="", total_cost=0.01),
        ]
        scores = [1.0, 0.5]
        idx = select_best_rollout(results, scores)
        assert idx == 1  # error rollout excluded


# ---------------------------------------------------------------------------
# A4.4 — Confidence-based early stopping
# ---------------------------------------------------------------------------


class TestEarlyStopping:
    """Early stopping when rollouts converge."""

    def test_early_stop_on_agreement(self) -> None:
        """3 identical rollouts out of 8 -> should_stop returns True."""
        from codeforge.quality_tracking import should_early_stop

        outputs = ["same output", "same output", "same output"]
        exit_codes = [0, 0, 0]
        assert should_early_stop(outputs, exit_codes, total_rollouts=8) is True

    def test_no_early_stop_low_similarity(self) -> None:
        """3 rollouts with different outputs -> should_stop returns False."""
        from codeforge.quality_tracking import should_early_stop

        outputs = ["output A is very different", "output B is completely unique", "output C has nothing in common"]
        exit_codes = [0, 0, 0]
        assert should_early_stop(outputs, exit_codes, total_rollouts=8) is False

    def test_no_early_stop_with_error(self) -> None:
        """3 identical but one has exit_code != 0 -> no early stop."""
        from codeforge.quality_tracking import should_early_stop

        outputs = ["same", "same", "same"]
        exit_codes = [0, 0, 1]
        assert should_early_stop(outputs, exit_codes, total_rollouts=8) is False

    def test_no_early_stop_few_rollouts(self) -> None:
        """rollout_count <= 3 -> never early stop."""
        from codeforge.quality_tracking import should_early_stop

        outputs = ["same", "same", "same"]
        exit_codes = [0, 0, 0]
        assert should_early_stop(outputs, exit_codes, total_rollouts=3) is False


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
            patch("codeforge.agent_loop.select_best_rollout", return_value=0),
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


# ---------------------------------------------------------------------------
# A4.6 — Snapshot creates git stash, restore cleans workspace
# ---------------------------------------------------------------------------


class TestSnapshotRestore:
    """Verify git stash/restore operations are called with correct arguments."""

    @pytest.mark.asyncio
    async def test_snapshot_calls_git_stash(self) -> None:
        """_snapshot_workspace calls git stash push with rollout tag."""
        with patch("asyncio.create_subprocess_exec", new_callable=AsyncMock) as mock_exec:
            mock_proc = AsyncMock()
            mock_proc.communicate = AsyncMock(return_value=(b"", b""))
            mock_exec.return_value = mock_proc

            from codeforge.agent_loop import _snapshot_workspace

            await _snapshot_workspace("/tmp/ws", 2)

            mock_exec.assert_called_once()
            args = mock_exec.call_args
            # Positional args: git stash push -m rollout-2 --include-untracked
            assert args[0][0] == "git"
            assert args[0][1] == "stash"
            assert args[0][2] == "push"
            assert args[0][3] == "-m"
            assert args[0][4] == "rollout-2"
            assert args[0][5] == "--include-untracked"
            assert args[1]["cwd"] == "/tmp/ws"

    @pytest.mark.asyncio
    async def test_restore_calls_git_checkout_and_clean(self) -> None:
        """_restore_workspace calls git checkout . then git clean -fd."""
        calls: list[tuple] = []

        async def _fake_exec(*args, **kwargs):
            calls.append(args)
            mock_proc = AsyncMock()
            mock_proc.communicate = AsyncMock(return_value=(b"", b""))
            return mock_proc

        with patch("asyncio.create_subprocess_exec", side_effect=_fake_exec):
            from codeforge.agent_loop import _restore_workspace

            await _restore_workspace("/tmp/ws")

        assert len(calls) == 2
        # First: git checkout .
        assert calls[0][:3] == ("git", "checkout", ".")
        # Second: git clean -fd
        assert calls[1][:3] == ("git", "clean", "-fd")


# ---------------------------------------------------------------------------
# A4.7 — rollout_count=0 or negative defaults to 1
# ---------------------------------------------------------------------------


class TestRolloutCountClamping:
    """Verify constructor clamps rollout_count to valid range."""

    @pytest.mark.asyncio
    async def test_rollout_count_zero_defaults_to_one(self) -> None:
        """rollout_count=0 -> clamped to 1, no multi-rollout behavior."""
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
            rollout_count=0,
            workspace_path="/tmp/test",
        )
        assert rollout_exec._rollout_count == 1

        result = await rollout_exec.execute(messages=[], config=MagicMock())
        assert result.final_content == "single"
        mock_executor.run.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_rollout_count_negative_defaults_to_one(self) -> None:
        """rollout_count=-5 -> clamped to 1."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        mock_executor = AsyncMock()
        mock_executor.run = AsyncMock(
            return_value=MagicMock(
                final_content="single",
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
            rollout_count=-5,
            workspace_path="/tmp/test",
        )
        assert rollout_exec._rollout_count == 1

    def test_rollout_count_capped_at_eight(self) -> None:
        """rollout_count=100 -> clamped to 8."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=MagicMock(),
            rollout_count=100,
            workspace_path="/tmp/test",
        )
        assert rollout_exec._rollout_count == 8

    def test_rollout_count_exactly_eight(self) -> None:
        """rollout_count=8 -> stays 8 (boundary)."""
        from codeforge.agent_loop import ConversationRolloutExecutor

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=MagicMock(),
            rollout_count=8,
            workspace_path="/tmp/test",
        )
        assert rollout_exec._rollout_count == 8


# ---------------------------------------------------------------------------
# A4.11 — Trajectory metadata published on multi-rollout completion
# ---------------------------------------------------------------------------


class TestTrajectoryMetadata:
    """Verify rollout trajectory event is published after multi-rollout."""

    @pytest.mark.asyncio
    async def test_trajectory_event_published(self) -> None:
        """Multi-rollout publishes trajectory.rollout_complete event."""
        from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig

        call_count = 0

        async def _mock_run(messages, config=None):
            nonlocal call_count
            call_count += 1
            return MagicMock(
                final_content=f"result-{call_count}",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=5,
                step_count=1,
                model="m",
                error="",
                tool_messages=[],
            )

        mock_executor = MagicMock()
        mock_executor.run = _mock_run

        mock_runtime = AsyncMock()
        mock_runtime.publish_trajectory_event = AsyncMock()

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=3,
            workspace_path="/tmp/test",
            runtime=mock_runtime,
        )
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
        ):
            await rollout_exec.execute(messages=[], config=LoopConfig())

        mock_runtime.publish_trajectory_event.assert_awaited_once()
        event = mock_runtime.publish_trajectory_event.call_args[0][0]
        assert event["event_type"] == "trajectory.rollout_complete"
        assert event["total_rollouts"] == 3
        assert event["selected_index"] >= 0
        assert len(event["scores"]) == 3
        assert event["early_stopped"] is False

    @pytest.mark.asyncio
    async def test_trajectory_not_published_without_runtime(self) -> None:
        """Without runtime, trajectory publish is skipped silently."""
        from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig

        async def _mock_run(messages, config=None):
            return MagicMock(
                final_content="ok",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=5,
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
            # No runtime provided
        )
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
        ):
            # Should not raise even without runtime.
            result = await rollout_exec.execute(messages=[], config=LoopConfig())

        assert result.total_cost == pytest.approx(0.03)

    @pytest.mark.asyncio
    async def test_result_metadata_populated(self) -> None:
        """Multi-rollout result includes metadata with scores and selection info."""
        from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig

        async def _mock_run(messages, config=None):
            return MagicMock(
                final_content="ok",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=5,
                step_count=1,
                model="m",
                error="",
                tool_messages=[],
            )

        mock_executor = MagicMock()
        mock_executor.run = _mock_run

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=2,
            workspace_path="/tmp/test",
        )
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
        ):
            result = await rollout_exec.execute(messages=[], config=LoopConfig())

        assert "rollout_count" in result.metadata
        assert result.metadata["rollout_count"] == 2
        assert "selected_index" in result.metadata
        assert "scores" in result.metadata
        assert "early_stopped" in result.metadata
        assert result.metadata["early_stopped"] is False

    @pytest.mark.asyncio
    async def test_early_stop_reflected_in_metadata(self) -> None:
        """When early stopping triggers, metadata reflects it."""
        from codeforge.agent_loop import ConversationRolloutExecutor, LoopConfig

        async def _mock_run(messages, config=None):
            return MagicMock(
                final_content="identical output",
                total_cost=0.01,
                total_tokens_in=10,
                total_tokens_out=5,
                step_count=1,
                model="m",
                error="",
                tool_messages=[],
            )

        mock_executor = MagicMock()
        mock_executor.run = _mock_run

        rollout_exec = ConversationRolloutExecutor(
            agent_loop_executor=mock_executor,
            rollout_count=8,
            workspace_path="/tmp/test",
        )
        with (
            patch("os.path.isdir", return_value=True),
            patch("codeforge.agent_loop._snapshot_workspace", new_callable=AsyncMock),
            patch("codeforge.agent_loop._restore_workspace", new_callable=AsyncMock),
        ):
            result = await rollout_exec.execute(messages=[], config=LoopConfig())

        # Early stop should trigger after 3 identical outputs (quorum=3).
        assert result.metadata["early_stopped"] is True
        assert result.metadata["rollout_count"] < 8  # Did not run all 8


# ---------------------------------------------------------------------------
# A4.10 — NATS contract: rollout_count in ConversationRunStartMessage
# ---------------------------------------------------------------------------


class TestRolloutCountModel:
    """Verify rollout_count field on the Pydantic model."""

    def test_default_rollout_count_is_one(self) -> None:
        """ConversationRunStartMessage.rollout_count defaults to 1."""
        from codeforge.models import ConversationRunStartMessage

        msg = ConversationRunStartMessage(
            run_id="r1",
            conversation_id="c1",
            project_id="p1",
            messages=[],
            system_prompt="test",
            model="m",
        )
        assert msg.rollout_count == 1

    def test_rollout_count_from_json(self) -> None:
        """rollout_count parses correctly from JSON payload."""
        import json

        from codeforge.models import ConversationRunStartMessage

        payload = {
            "run_id": "r1",
            "conversation_id": "c1",
            "project_id": "p1",
            "messages": [],
            "system_prompt": "test",
            "model": "m",
            "rollout_count": 5,
        }
        msg = ConversationRunStartMessage.model_validate_json(json.dumps(payload))
        assert msg.rollout_count == 5

    def test_rollout_count_omitted_defaults(self) -> None:
        """When rollout_count is omitted (Go omitempty), Python defaults to 1."""
        import json

        from codeforge.models import ConversationRunStartMessage

        payload = {
            "run_id": "r1",
            "conversation_id": "c1",
            "project_id": "p1",
            "messages": [],
            "system_prompt": "test",
            "model": "m",
        }
        msg = ConversationRunStartMessage.model_validate_json(json.dumps(payload))
        assert msg.rollout_count == 1


# ---------------------------------------------------------------------------
# A4.12 — AgentLoopResult.metadata field
# ---------------------------------------------------------------------------


class TestAgentLoopResultMetadata:
    """Verify metadata field on AgentLoopResult."""

    def test_metadata_default_empty(self) -> None:
        """AgentLoopResult.metadata defaults to empty dict."""
        from codeforge.models import AgentLoopResult

        result = AgentLoopResult()
        assert result.metadata == {}

    def test_metadata_set_and_read(self) -> None:
        """metadata dict can be set and read back."""
        from codeforge.models import AgentLoopResult

        result = AgentLoopResult(metadata={"key": "value", "count": 3})
        assert result.metadata["key"] == "value"
        assert result.metadata["count"] == 3
