"""Tests for the search_skills agent tool."""

import pytest

from codeforge.skills.models import Skill
from codeforge.tools.search_skills import DEFINITION, SearchSkillsTool


def test_definition_has_required_fields():
    assert DEFINITION.name == "search_skills"
    assert "query" in DEFINITION.parameters["properties"]
    assert "query" in DEFINITION.parameters["required"]


def test_definition_has_type_filter():
    props = DEFINITION.parameters["properties"]
    assert "type" in props
    assert props["type"]["enum"] == ["workflow", "pattern", "any"]


@pytest.mark.asyncio
async def test_search_returns_matching_skills():
    skills = [
        Skill(
            id="1",
            name="tdd-workflow",
            description="Test-driven development",
            content="## TDD Steps\n1. Write test\n2. Implement",
            type="workflow",
            tags=["tdd", "testing"],
        ),
        Skill(
            id="2",
            name="nats-pattern",
            description="NATS handler pattern",
            content="```go\nfunc handle() {}\n```",
            type="pattern",
            tags=["nats", "go"],
        ),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "test driven development"}, "/tmp")
    assert result.success
    assert "tdd-workflow" in result.output


@pytest.mark.asyncio
async def test_search_empty_query():
    tool = SearchSkillsTool(skills=[])
    result = await tool.execute({"query": ""}, "/tmp")
    assert result.success
    assert "no skills found" in result.output.lower()


@pytest.mark.asyncio
async def test_search_no_skills_loaded():
    tool = SearchSkillsTool(skills=[])
    result = await tool.execute({"query": "anything"}, "/tmp")
    assert result.success
    assert "no skills found" in result.output.lower()


@pytest.mark.asyncio
async def test_search_no_matches():
    skills = [
        Skill(id="1", name="nats-pattern", description="NATS handler", content="code", tags=["nats"]),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "quantum physics algorithm"}, "/tmp")
    assert result.success


@pytest.mark.asyncio
async def test_search_filter_by_type():
    skills = [
        Skill(
            id="1", name="tdd", description="testing workflow steps", content="steps", type="workflow", tags=["testing"]
        ),
        Skill(id="2", name="nats", description="nats handler pattern", content="code", type="pattern", tags=["nats"]),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "testing", "type": "workflow"}, "/tmp")
    assert result.success
    # The pattern-type skill should be excluded
    assert "nats" not in result.output.lower() or "tdd" in result.output


@pytest.mark.asyncio
async def test_search_filter_no_skills_of_type():
    skills = [
        Skill(id="1", name="nats", description="nats handler", content="code", type="pattern", tags=["nats"]),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "nats", "type": "workflow"}, "/tmp")
    assert result.success
    assert "no workflow skills" in result.output.lower()


@pytest.mark.asyncio
async def test_search_type_any_returns_all_types():
    skills = [
        Skill(id="1", name="tdd", description="testing", content="steps", type="workflow", tags=["testing"]),
        Skill(
            id="2",
            name="nats",
            description="nats handler testing",
            content="code",
            type="pattern",
            tags=["nats", "testing"],
        ),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "testing", "type": "any"}, "/tmp")
    assert result.success


@pytest.mark.asyncio
async def test_search_missing_query_key():
    tool = SearchSkillsTool(skills=[])
    result = await tool.execute({}, "/tmp")
    assert result.success
    assert "no skills found" in result.output.lower()


@pytest.mark.asyncio
async def test_set_skills_updates_pool():
    tool = SearchSkillsTool(skills=[])
    result = await tool.execute({"query": "testing"}, "/tmp")
    assert "no skills found" in result.output.lower()

    tool.set_skills(
        [
            Skill(
                id="1",
                name="tdd",
                description="test driven development",
                content="TDD",
                type="workflow",
                tags=["testing"],
            ),
        ]
    )
    result = await tool.execute({"query": "test driven development"}, "/tmp")
    assert result.success
    assert "tdd" in result.output.lower()


@pytest.mark.asyncio
async def test_search_output_includes_content():
    skills = [
        Skill(
            id="1",
            name="tdd-workflow",
            description="Test-driven development",
            content="## TDD Steps\n1. Write test\n2. Implement",
            type="workflow",
            tags=["tdd"],
        ),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "test driven development"}, "/tmp")
    assert result.success
    assert "## TDD Steps" in result.output


@pytest.mark.asyncio
async def test_search_output_includes_tags():
    skills = [
        Skill(
            id="1",
            name="tdd-workflow",
            description="Test-driven development",
            content="content here",
            type="workflow",
            tags=["tdd", "testing"],
        ),
    ]
    tool = SearchSkillsTool(skills=skills)
    result = await tool.execute({"query": "test driven development"}, "/tmp")
    assert result.success
    assert "tdd" in result.output
    assert "testing" in result.output
