"""Tests for built-in skill loading from YAML files."""

from __future__ import annotations

from codeforge.skills.registry import load_builtin_skills


def test_builtin_skills_load() -> None:
    """At least one built-in skill is loaded from the builtins/ directory."""
    skills = load_builtin_skills()
    assert len(skills) >= 1


def test_builtin_skill_creator_exists() -> None:
    """The codeforge-skill-creator meta-skill is present."""
    skills = load_builtin_skills()
    names = [s.name for s in skills]
    assert "codeforge-skill-creator" in names


def test_builtin_skill_creator_valid() -> None:
    """The meta-skill has correct type, source, status, and content."""
    skills = load_builtin_skills()
    creator = next(s for s in skills if s.name == "codeforge-skill-creator")
    assert creator.type == "workflow"
    assert creator.source == "builtin"
    assert creator.status == "active"
    assert "create_skill" in creator.content
    assert len(creator.tags) >= 2


def test_builtin_skill_has_id_prefix() -> None:
    """All built-in skills have ids prefixed with 'builtin:'."""
    skills = load_builtin_skills()
    for s in skills:
        assert s.id.startswith("builtin:"), f"skill {s.name} has id={s.id}"
