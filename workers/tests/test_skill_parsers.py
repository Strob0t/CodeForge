"""Tests for multi-format skill file parsers."""

from __future__ import annotations

import pytest

from codeforge.skills.parsers import parse_skill_file


class TestCodeForgeYAML:
    """Parse native CodeForge YAML skill files."""

    def test_parse_full_yaml(self) -> None:
        content = """
name: tdd-workflow
type: workflow
description: Test-driven development steps
tags: [tdd, testing]
content: |
  1. Write failing test
  2. Implement code
  3. Refactor
"""
        skill = parse_skill_file("tdd.yaml", content)
        assert skill.name == "tdd-workflow"
        assert skill.type == "workflow"
        assert skill.format_origin == "codeforge"
        assert skill.source == "import"
        assert "Write failing test" in skill.content
        assert skill.tags == ["tdd", "testing"]

    def test_parse_yml_extension(self) -> None:
        content = """
name: lint-fix
type: pattern
description: Auto-fix lint
content: run ruff check --fix
"""
        skill = parse_skill_file("lint.yml", content)
        assert skill.name == "lint-fix"
        assert skill.type == "pattern"
        assert skill.format_origin == "codeforge"

    def test_parse_yaml_minimal(self) -> None:
        content = """
name: minimal
"""
        skill = parse_skill_file("m.yaml", content)
        assert skill.name == "minimal"
        assert skill.type == "pattern"
        assert skill.content == ""
        assert skill.tags == []

    def test_parse_yaml_with_language(self) -> None:
        content = """
name: go-error-handling
type: pattern
language: go
content: |
  if err != nil { return fmt.Errorf("wrap: %w", err) }
"""
        skill = parse_skill_file("go-errors.yaml", content)
        assert skill.language == "go"


class TestClaudeSkills:
    """Parse Claude Code skill files (YAML frontmatter + Markdown body)."""

    def test_parse_claude_skill(self) -> None:
        content = """---
name: commit-skill
description: Safe commit workflow
---
## Steps
1. Run pre-commit
2. Stage files
3. Commit with message
"""
        skill = parse_skill_file("commit.md", content)
        assert skill.name == "commit-skill"
        assert skill.format_origin == "claude"
        assert skill.source == "import"
        assert "Run pre-commit" in skill.content

    def test_parse_claude_with_tags(self) -> None:
        content = """---
name: review-skill
type: workflow
tags: [review, code-quality]
---
Review all changes before committing.
"""
        skill = parse_skill_file("review.md", content)
        assert skill.name == "review-skill"
        assert skill.type == "workflow"
        assert skill.tags == ["review", "code-quality"]
        assert skill.format_origin == "claude"

    def test_parse_claude_empty_frontmatter(self) -> None:
        content = """---
---
Just a body.
"""
        skill = parse_skill_file("empty-front.md", content)
        assert skill.name == ""
        assert skill.format_origin == "claude"
        assert skill.content == "Just a body."


class TestCursorRules:
    """Parse Cursor .cursorrules and .mdc files."""

    def test_parse_cursorrules(self) -> None:
        content = """# Error Handling Rules

Always use structured error types.
Never swallow exceptions silently.
"""
        skill = parse_skill_file(".cursorrules", content)
        assert skill.name == "Error Handling Rules"
        assert skill.format_origin == "cursor"
        assert skill.type == "workflow"

    def test_parse_mdc_file(self) -> None:
        content = "Use pytest for all tests.\nAlways add docstrings."
        skill = parse_skill_file("rules.mdc", content)
        assert skill.format_origin == "cursor"
        assert skill.name == "rules.mdc"

    def test_parse_mdc_with_heading(self) -> None:
        content = """# Testing Policy

Use pytest fixtures. No mocking unless needed.
"""
        skill = parse_skill_file("testing.mdc", content)
        assert skill.name == "Testing Policy"
        assert skill.format_origin == "cursor"

    def test_parse_cursorrules_in_subdirectory(self) -> None:
        content = "# Subdir Rules\nKeep it simple."
        skill = parse_skill_file("project/.cursorrules", content)
        assert skill.format_origin == "cursor"
        assert skill.name == "Subdir Rules"


class TestPlainMarkdown:
    """Parse plain Markdown files (no frontmatter)."""

    def test_parse_plain_markdown(self) -> None:
        content = """# NATS Handler Pattern

```go
func handle(msg *nats.Msg) {
    defer msg.Ack()
}
```
"""
        skill = parse_skill_file("nats-pattern.md", content)
        assert skill.name == "NATS Handler Pattern"
        assert skill.format_origin == "markdown"
        assert skill.source == "import"
        assert "defer msg.Ack()" in skill.content

    def test_parse_markdown_no_heading(self) -> None:
        content = "Just raw text without a heading."
        skill = parse_skill_file("raw.md", content)
        assert skill.name == "Untitled Skill"
        assert skill.format_origin == "markdown"

    def test_parse_markdown_multiple_headings(self) -> None:
        content = """# First Heading

Some text.

# Second Heading

More text.
"""
        skill = parse_skill_file("multi.md", content)
        assert skill.name == "First Heading"


class TestEdgeCases:
    """Edge cases and error handling."""

    def test_parse_unknown_format_raises(self) -> None:
        with pytest.raises(ValueError, match="unsupported"):
            parse_skill_file("data.bin", "binary stuff")

    def test_parse_unknown_extension_raises(self) -> None:
        with pytest.raises(ValueError, match="unsupported"):
            parse_skill_file("script.sh", "#!/bin/bash")

    def test_empty_yaml_content(self) -> None:
        skill = parse_skill_file("empty.yaml", "")
        assert skill.name == ""
        assert skill.format_origin == "codeforge"

    def test_empty_markdown_content(self) -> None:
        skill = parse_skill_file("empty.md", "")
        assert skill.name == "Untitled Skill"
        assert skill.format_origin == "markdown"

    def test_whitespace_only_yaml(self) -> None:
        skill = parse_skill_file("ws.yaml", "   \n\n  ")
        assert skill.name == ""
        assert skill.format_origin == "codeforge"

    def test_filename_with_path(self) -> None:
        content = """
name: nested-skill
content: works
"""
        skill = parse_skill_file("/home/user/.codeforge/skills/nested-skill.yaml", content)
        assert skill.name == "nested-skill"

    def test_uppercase_extension(self) -> None:
        content = """
name: upper
content: test
"""
        skill = parse_skill_file("skill.YAML", content)
        assert skill.name == "upper"
        assert skill.format_origin == "codeforge"

    def test_yaml_with_no_name(self) -> None:
        content = """
type: workflow
content: do something
"""
        skill = parse_skill_file("noname.yaml", content)
        assert skill.name == ""
        assert skill.type == "workflow"
