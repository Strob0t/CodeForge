"""Tests for the Skill Pydantic model (skills subsystem)."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from codeforge.skills.models import Skill, SkillRecommendation


class TestSkillDefaults:
    """Verify default values for all new fields."""

    def test_new_fields_defaults(self) -> None:
        s = Skill(name="test", content="x")
        assert s.type == "pattern"
        assert s.source == "user"
        assert s.status == "active"
        assert s.format_origin == "codeforge"
        assert s.usage_count == 0
        assert s.source_url == ""

    def test_id_and_tenant_defaults(self) -> None:
        s = Skill(name="test")
        assert s.id == ""
        assert s.tenant_id == ""
        assert s.project_id == ""

    def test_tags_default_empty_list(self) -> None:
        s = Skill(name="test")
        assert s.tags == []

    def test_enabled_default_true(self) -> None:
        s = Skill(name="test")
        assert s.enabled is True

    def test_content_default_empty(self) -> None:
        s = Skill(name="test")
        assert s.content == ""

    def test_code_default_empty(self) -> None:
        s = Skill(name="test")
        assert s.code == ""

    def test_description_default_empty(self) -> None:
        s = Skill(name="test")
        assert s.description == ""

    def test_language_default_empty(self) -> None:
        s = Skill(name="test")
        assert s.language == ""


class TestSkillTypeField:
    """Verify the type field accepts expected values."""

    def test_workflow_type(self) -> None:
        s = Skill(name="test", content="steps", type="workflow")
        assert s.type == "workflow"

    def test_pattern_type_explicit(self) -> None:
        s = Skill(name="test", type="pattern")
        assert s.type == "pattern"

    def test_type_default_is_pattern(self) -> None:
        s = Skill(name="test")
        assert s.type == "pattern"


class TestSkillSourceField:
    """Verify the source field accepts expected values."""

    def test_source_builtin(self) -> None:
        s = Skill(name="test", source="builtin")
        assert s.source == "builtin"

    def test_source_import(self) -> None:
        s = Skill(name="test", source="import")
        assert s.source == "import"

    def test_source_agent(self) -> None:
        s = Skill(name="test", source="agent")
        assert s.source == "agent"

    def test_source_user_default(self) -> None:
        s = Skill(name="test")
        assert s.source == "user"


class TestSkillStatusField:
    """Verify the status field accepts expected values."""

    def test_status_draft(self) -> None:
        s = Skill(name="test", status="draft")
        assert s.status == "draft"

    def test_status_disabled(self) -> None:
        s = Skill(name="test", status="disabled")
        assert s.status == "disabled"

    def test_status_active_default(self) -> None:
        s = Skill(name="test")
        assert s.status == "active"


class TestSkillFormatOriginField:
    """Verify the format_origin field."""

    def test_format_origin_claude(self) -> None:
        s = Skill(name="test", format_origin="claude")
        assert s.format_origin == "claude"

    def test_format_origin_cursor(self) -> None:
        s = Skill(name="test", format_origin="cursor")
        assert s.format_origin == "cursor"

    def test_format_origin_markdown(self) -> None:
        s = Skill(name="test", format_origin="markdown")
        assert s.format_origin == "markdown"

    def test_format_origin_codeforge_default(self) -> None:
        s = Skill(name="test")
        assert s.format_origin == "codeforge"


class TestSkillBackwardsCompat:
    """Ensure backwards compatibility with existing code that uses the code field."""

    def test_code_field_still_works(self) -> None:
        s = Skill(name="test", code="print(1)")
        assert s.code == "print(1)"

    def test_code_field_with_all_legacy_fields(self) -> None:
        """Mimics the constructor call in registry.py."""
        s = Skill(
            id="abc",
            tenant_id="t1",
            project_id="p1",
            name="myskill",
            description="desc",
            language="python",
            code="print(1)",
            tags=["tag1", "tag2"],
            enabled=True,
        )
        assert s.id == "abc"
        assert s.tenant_id == "t1"
        assert s.project_id == "p1"
        assert s.name == "myskill"
        assert s.description == "desc"
        assert s.language == "python"
        assert s.code == "print(1)"
        assert s.tags == ["tag1", "tag2"]
        assert s.enabled is True
        # New fields should have defaults
        assert s.type == "pattern"
        assert s.source == "user"
        assert s.status == "active"
        assert s.content == ""

    def test_code_without_content(self) -> None:
        """Old rows that have code but no content should still work."""
        s = Skill(name="legacy", code="x = 1")
        assert s.code == "x = 1"
        assert s.content == ""

    def test_content_without_code(self) -> None:
        """New rows with content but no code."""
        s = Skill(name="new", content="## Steps\n1. Do X")
        assert s.content == "## Steps\n1. Do X"
        assert s.code == ""


class TestSkillUsageCount:
    """Verify the usage_count field."""

    def test_usage_count_default_zero(self) -> None:
        s = Skill(name="test")
        assert s.usage_count == 0

    def test_usage_count_custom(self) -> None:
        s = Skill(name="test", usage_count=42)
        assert s.usage_count == 42


class TestSkillSourceUrl:
    """Verify the source_url field."""

    def test_source_url_default_empty(self) -> None:
        s = Skill(name="test")
        assert s.source_url == ""

    def test_source_url_custom(self) -> None:
        s = Skill(name="test", source_url="https://example.com/skill.md")
        assert s.source_url == "https://example.com/skill.md"


class TestSkillNameRequired:
    """Verify that name is still required."""

    def test_name_is_required(self) -> None:
        with pytest.raises(ValidationError):
            Skill()  # type: ignore[call-arg]

    def test_name_cannot_be_none(self) -> None:
        with pytest.raises(ValidationError):
            Skill(name=None)  # type: ignore[arg-type]


class TestSkillSerialization:
    """Verify JSON serialization round-trip."""

    def test_model_dump_includes_new_fields(self) -> None:
        s = Skill(name="test", content="body", type="workflow", source="builtin")
        d = s.model_dump()
        assert d["type"] == "workflow"
        assert d["source"] == "builtin"
        assert d["content"] == "body"
        assert d["status"] == "active"
        assert d["format_origin"] == "codeforge"
        assert d["usage_count"] == 0
        assert d["source_url"] == ""

    def test_model_dump_includes_legacy_fields(self) -> None:
        s = Skill(name="test", code="x")
        d = s.model_dump()
        assert d["code"] == "x"
        assert d["enabled"] is True

    def test_round_trip_json(self) -> None:
        original = Skill(
            name="roundtrip",
            content="# Guide",
            type="workflow",
            source="import",
            source_url="https://example.com",
            format_origin="claude",
            status="draft",
            usage_count=5,
            code="old",
            tags=["a", "b"],
        )
        json_str = original.model_dump_json()
        restored = Skill.model_validate_json(json_str)
        assert restored == original


class TestSkillRecommendation:
    """SkillRecommendation should still work with the extended Skill model."""

    def test_recommendation_with_extended_skill(self) -> None:
        skill = Skill(name="test", content="body", type="workflow")
        rec = SkillRecommendation(skill=skill, score=0.85)
        assert rec.skill.type == "workflow"
        assert rec.skill.content == "body"
        assert rec.score == 0.85
