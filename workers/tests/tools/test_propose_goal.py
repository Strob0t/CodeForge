"""Tests for the propose_goal tool."""

from __future__ import annotations

from unittest.mock import AsyncMock

import pytest

from codeforge.tools.propose_goal import PROPOSE_GOAL_DEFINITION, ProposeGoalExecutor


class TestProposeGoalDefinition:
    def test_name(self):
        assert PROPOSE_GOAL_DEFINITION.name == "propose_goal"

    def test_required_fields(self):
        assert "required" in PROPOSE_GOAL_DEFINITION.parameters
        assert set(PROPOSE_GOAL_DEFINITION.parameters["required"]) == {"action", "kind", "title", "content"}

    def test_action_enum(self):
        props = PROPOSE_GOAL_DEFINITION.parameters["properties"]
        assert props["action"]["enum"] == ["create", "update", "delete"]

    def test_kind_enum(self):
        props = PROPOSE_GOAL_DEFINITION.parameters["properties"]
        assert props["kind"]["enum"] == ["vision", "requirement", "constraint", "state", "context"]


class TestProposeGoalExecutor:
    @pytest.fixture
    def runtime(self):
        rt = AsyncMock()
        rt.publish_trajectory_event = AsyncMock()
        return rt

    @pytest.fixture
    def executor(self, runtime):
        return ProposeGoalExecutor(runtime=runtime)

    @pytest.mark.asyncio
    async def test_create_proposal_success(self, executor, runtime):
        args = {
            "action": "create",
            "kind": "requirement",
            "title": "User can search products",
            "content": "A search function that aggregates...",
            "priority": 90,
        }
        result = await executor.execute(args, "/tmp/workspace")

        assert result.success is True
        assert "User can search products" in result.output
        runtime.publish_trajectory_event.assert_called_once()
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["event_type"] == "agent.goal_proposed"
        data = event["data"]
        assert data["action"] == "create"
        assert data["kind"] == "requirement"
        assert data["title"] == "User can search products"

    @pytest.mark.asyncio
    async def test_create_missing_required_field(self, executor, runtime):
        args = {"action": "create", "kind": "requirement"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
        assert "title" in result.error or "content" in result.error
        runtime.publish_trajectory_event.assert_not_called()

    @pytest.mark.asyncio
    async def test_update_requires_goal_id(self, executor, runtime):
        args = {"action": "update", "kind": "requirement", "title": "Updated", "content": "New content"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
        assert "goal_id" in result.error

    @pytest.mark.asyncio
    async def test_delete_requires_goal_id(self, executor, runtime):
        args = {"action": "delete", "kind": "requirement", "title": "X", "content": "Y"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
        assert "goal_id" in result.error

    @pytest.mark.asyncio
    async def test_default_priority(self, executor, runtime):
        args = {"action": "create", "kind": "vision", "title": "T", "content": "C"}
        await executor.execute(args, "/tmp/workspace")
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["priority"] == 90

    @pytest.mark.asyncio
    async def test_proposal_id_is_uuid(self, executor, runtime):
        args = {"action": "create", "kind": "vision", "title": "T", "content": "C"}
        await executor.execute(args, "/tmp/workspace")
        event = runtime.publish_trajectory_event.call_args[0][0]
        import uuid

        uuid.UUID(event["data"]["proposal_id"])  # raises if not valid UUID

    @pytest.mark.asyncio
    async def test_update_with_goal_id_success(self, executor, runtime):
        args = {
            "action": "update",
            "kind": "requirement",
            "title": "Updated title",
            "content": "Updated content",
            "goal_id": "existing-goal-123",
        }
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is True
        assert "Updated title" in result.output
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["action"] == "update"
        assert event["data"]["goal_id"] == "existing-goal-123"

    @pytest.mark.asyncio
    async def test_delete_with_goal_id_success(self, executor, runtime):
        args = {
            "action": "delete",
            "kind": "requirement",
            "title": "To delete",
            "content": "N/A",
            "goal_id": "goal-to-delete-456",
        }
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is True
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["action"] == "delete"
        assert event["data"]["goal_id"] == "goal-to-delete-456"

    @pytest.mark.asyncio
    async def test_goal_id_none_when_not_provided(self, executor, runtime):
        args = {"action": "create", "kind": "vision", "title": "T", "content": "C"}
        await executor.execute(args, "/tmp/workspace")
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["goal_id"] is None

    @pytest.mark.asyncio
    async def test_custom_priority(self, executor, runtime):
        args = {"action": "create", "kind": "vision", "title": "T", "content": "C", "priority": 50}
        await executor.execute(args, "/tmp/workspace")
        event = runtime.publish_trajectory_event.call_args[0][0]
        assert event["data"]["priority"] == 50

    @pytest.mark.asyncio
    async def test_unknown_action(self, executor, runtime):
        args = {"action": "invalid", "kind": "vision", "title": "T", "content": "C"}
        result = await executor.execute(args, "/tmp/workspace")
        assert result.success is False
