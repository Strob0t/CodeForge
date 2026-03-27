"""Tests for the spawn_subagent tool."""

from __future__ import annotations

import uuid
from unittest.mock import AsyncMock

import pytest

from codeforge.tools.spawn_subagent import SPAWN_SUBAGENT_DEFINITION, SpawnSubagentExecutor


class TestSpawnSubagentDefinition:
    def test_name(self):
        assert SPAWN_SUBAGENT_DEFINITION.name == "spawn_subagent"

    def test_required_fields(self):
        assert "required" in SPAWN_SUBAGENT_DEFINITION.parameters
        assert set(SPAWN_SUBAGENT_DEFINITION.parameters["required"]) == {"role", "task"}

    def test_role_enum(self):
        props = SPAWN_SUBAGENT_DEFINITION.parameters["properties"]
        assert props["role"]["enum"] == ["researcher", "implementer", "reviewer", "debater"]

    def test_model_tier_enum(self):
        props = SPAWN_SUBAGENT_DEFINITION.parameters["properties"]
        assert props["model_tier"]["enum"] == ["weak", "mid", "strong"]


class TestSpawnSubagentExecutor:
    @pytest.fixture
    def runtime(self):
        rt = AsyncMock()
        rt.publish_trajectory_event = AsyncMock()
        return rt

    @pytest.fixture
    def executor(self, runtime):
        return SpawnSubagentExecutor(runtime=runtime)

    @pytest.mark.asyncio
    async def test_spawn_researcher(self, executor, runtime):
        result = await executor.execute(
            {"role": "researcher", "task": "Investigate auth patterns"},
            "/workspace",
        )
        assert result.success is True
        assert "researcher" in result.output
        runtime.publish_trajectory_event.assert_called_once()
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["event_type"] == "agent.subagent_requested"
        assert event["data"]["role"] == "researcher"
        assert event["data"]["model_tier"] == "mid"  # default

    @pytest.mark.asyncio
    async def test_spawn_with_explicit_tier(self, executor, runtime):
        await executor.execute(
            {"role": "implementer", "task": "Build user model", "model_tier": "weak"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["model_tier"] == "weak"

    @pytest.mark.asyncio
    async def test_spawn_with_strong_tier(self, executor, runtime):
        await executor.execute(
            {"role": "reviewer", "task": "Review PR #42", "model_tier": "strong"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["model_tier"] == "strong"

    @pytest.mark.asyncio
    async def test_missing_role(self, executor, runtime):
        result = await executor.execute({"task": "do something"}, "/workspace")
        assert result.success is False
        assert "role" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_missing_task(self, executor, runtime):
        result = await executor.execute({"role": "researcher"}, "/workspace")
        assert result.success is False
        assert "task" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_invalid_role(self, executor, runtime):
        result = await executor.execute(
            {"role": "hacker", "task": "do something"}, "/workspace"
        )
        assert result.success is False
        assert "role" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_invalid_model_tier(self, executor, runtime):
        result = await executor.execute(
            {"role": "researcher", "task": "research", "model_tier": "mega"},
            "/workspace",
        )
        assert result.success is False
        assert "model_tier" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_subagent_id_format(self, executor, runtime):
        await executor.execute(
            {"role": "debater", "task": "Debate caching strategy"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        subagent_id = event["data"]["subagent_id"]
        assert len(subagent_id) == 8
        # Must be a valid hex prefix of a UUID
        uuid.UUID(subagent_id + "0" * 24)

    @pytest.mark.asyncio
    async def test_context_passed_through(self, executor, runtime):
        await executor.execute(
            {
                "role": "researcher",
                "task": "Investigate auth",
                "context": "We use OAuth2 with PKCE",
            },
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["context"] == "We use OAuth2 with PKCE"

    @pytest.mark.asyncio
    async def test_context_default_empty(self, executor, runtime):
        await executor.execute(
            {"role": "researcher", "task": "Investigate auth"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["context"] == ""

    @pytest.mark.asyncio
    async def test_task_in_event_data(self, executor, runtime):
        await executor.execute(
            {"role": "implementer", "task": "Build the login page"},
            "/workspace",
        )
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["task"] == "Build the login page"

    @pytest.mark.asyncio
    async def test_output_contains_task_summary(self, executor, runtime):
        result = await executor.execute(
            {"role": "implementer", "task": "Build the login page"},
            "/workspace",
        )
        assert "implementer" in result.output
        assert "Build the login page" in result.output

    @pytest.mark.asyncio
    async def test_empty_task_rejected(self, executor, runtime):
        result = await executor.execute(
            {"role": "researcher", "task": ""}, "/workspace"
        )
        assert result.success is False
        assert "task" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_empty_role_rejected(self, executor, runtime):
        result = await executor.execute(
            {"role": "", "task": "do something"}, "/workspace"
        )
        assert result.success is False
        assert "role" in result.error
        runtime.publish_trajectory_event.assert_not_called()
